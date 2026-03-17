package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/flashsystem-collector/internal/cache"
	"github.com/flashsystem-collector/internal/collectors"
	"github.com/flashsystem-collector/internal/config"
	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
	"github.com/flashsystem-collector/internal/zabbix"
)

const version = "2.0.0"

func main() {
	// Definir subcomando antes de flag.Parse().
	// Uso:
	//   flashsystem-collector collect   -host ... -user ... -keyfile ...
	//   flashsystem-collector discover  -host ... -user ... -keyfile ... -category drives
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subcommand := os.Args[1]

	// Remover subcomando de os.Args para que flag.Parse() funcione normalmente.
	os.Args = append(os.Args[:1], os.Args[2:]...)

	switch subcommand {
	case "collect":
		runCollect()
	case "discover":
		runDiscover()
	case "version":
		fmt.Printf("flashsystem-collector %s\n", version)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

// runCollect ejecuta todos los collectors y emite el JSON completo.
// Este es el modo principal usado como Master Item en Zabbix.
func runCollect() {
	startTime := time.Now()

	cfg, err := config.Load()
	if err != nil {
		fatalJSON(fmt.Sprintf("config error: %v", err), startTime)
	}

	// Inicializar cache.
	cacheFile := cfg.CacheFile(cfg.SSH.Host)
	c := cache.New(cacheFile)

	// Purgar entradas expiradas al inicio para mantener el archivo limpio.
	// El error de purge no es fatal.
	_ = c.Purge()

	// Conectar SSH.
	client, err := dialSSH(cfg)
	if err != nil {
		// Si SSH falla, intentar servir todo desde cache stale.
		// Esto previene alertas falsas durante mantenimiento del storage.
		output := buildFromStaleCache(c, cfg, startTime, err)
		emitJSON(output)
		return
	}
	defer client.Close()

	// Construir y ejecutar collectors.
	cols := collectors.AllCollectors(
		cfg.Cache.MetricsTTL,
		cfg.Cache.DiscoveryTTL,
		cfg.Cache.PerformanceTTL,
	)

	runner := collectors.NewRunner(
		collectors.RunnerConfig{
			Host:          cfg.SSH.Host,
			MaxConcurrent: 3,
		},
		c,
		cols,
	)

	results := runner.RunAll(client)

	// Construir output Zabbix.
	output, err := zabbix.Build(
		zabbix.BuilderConfig{
			Host:         cfg.SSH.Host,
			Version:      version,
			MaxJSONBytes: cfg.Limits.MaxJSONBytes,
			MaxVolumes:   cfg.Limits.MaxVolumes,
			MaxDrives:    cfg.Limits.MaxDrives,
		},
		results,
		c.Stats(),
		startTime,
	)
	if err != nil {
		fatalJSON(fmt.Sprintf("build output error: %v", err), startTime)
	}

	emitJSON(output)
}

// runDiscover ejecuta discovery LLD para una categoría específica.
// Uso desde Zabbix: flashsystem-collector discover -host ... -category drives
func runDiscover() {
	// Flag adicional para discover.
	category := flag.String("category", "", "Discovery category: drives|pools|volumes|enclosures|nodes|ports|batteries|psus")

	cfg, err := config.Load()
	if err != nil {
		fatalLLD(fmt.Sprintf("config error: %v", err))
	}

	if *category == "" {
		fatalLLD("discover requires -category flag: drives|pools|volumes|enclosures|nodes|ports|batteries|psus")
	}

	// Inicializar cache con TTL de discovery (largo).
	cacheFile := cfg.CacheFile(cfg.SSH.Host)
	c := cache.New(cacheFile)

	// Clave de cache para discovery.
	cacheKey := cache.MakeDiscoveryKey(cfg.SSH.Host, *category)

	// Verificar cache de discovery primero.
	if entry, ok := c.Get(cacheKey); ok {
		var records []map[string]string
		if err := entry.Unmarshal(&records); err == nil {
			// Cache hit: emitir LLD desde cache.
			emitLLDFromCache(records)
			return
		}
	}

	// Cache miss: conectar SSH y ejecutar collector correspondiente.
	client, err := dialSSH(cfg)
	if err != nil {
		// Discovery fallida: emitir LLD vacío (Zabbix no crea items = no alertas falsas).
		emitEmptyLLD()
		logStderr("discover SSH failed: %v", err)
		return
	}
	defer client.Close()

	records, fieldMap, err := runDiscoveryCollector(client, *category, cfg)
	if err != nil {
		emitEmptyLLD()
		logStderr("discover collector failed for %q: %v", *category, err)
		return
	}

	// Guardar en cache de discovery con TTL largo.
	// Convertir records a formato LLD para cachear el resultado final.
	lldItems := recordsToLLDItems(records, fieldMap)
	_ = c.Set(cacheKey, lldItems, cfg.Cache.DiscoveryTTL)

	data, err := zabbix.LLDOutput(records, fieldMap)
	if err != nil {
		emitEmptyLLD()
		logStderr("discover LLD serialization failed: %v", err)
		return
	}

	fmt.Println(string(data))
}

// dialSSH establece la conexión SSH usando la configuración cargada.
func dialSSH(cfg *config.Config) (*internalssh.Client, error) {
	return internalssh.New(internalssh.ClientConfig{
		Host:            cfg.SSH.Host,
		Port:            cfg.SSH.Port,
		User:            cfg.SSH.User,
		Password:        cfg.SSH.Password,
		KeyFile:         cfg.SSH.KeyFile,
		DialTimeout:     cfg.SSH.DialTimeout,
		CommandTimeout:  cfg.SSH.CommandTimeout,
		InsecureHostKey: cfg.SSH.InsecureHostKey,
		KnownHostsFile:  cfg.SSH.KnownHostsFile,
	})
}

// runDiscoveryCollector ejecuta el collector adecuado para la categoría de discovery.
// Retorna los records y el fieldMap de macros Zabbix LLD.
func runDiscoveryCollector(
	client *internalssh.Client,
	category string,
	cfg *config.Config,
) ([]parser.Record, map[string]string, error) {

	type collectorEntry struct {
		col      collectors.Collector
		fieldMap map[string]string
	}

	available := map[string]collectorEntry{
		"drives": {
			col:      collectors.NewDrivesCollector(cfg.Cache.DiscoveryTTL),
			fieldMap: zabbix.DriveLLDFields,
		},
		"pools": {
			col:      collectors.NewPoolsCollector(cfg.Cache.MetricsTTL),
			fieldMap: zabbix.PoolLLDFields,
		},
		"volumes": {
			col:      collectors.NewVolumesCollector(cfg.Cache.MetricsTTL),
			fieldMap: zabbix.VolumeLLDFields,
		},
		"enclosures": {
			col:      collectors.NewEnclosuresCollector(cfg.Cache.DiscoveryTTL),
			fieldMap: zabbix.EnclosureLLDFields,
		},
		"nodes": {
			col:      collectors.NewNodesCollector(cfg.Cache.MetricsTTL),
			fieldMap: zabbix.NodeLLDFields,
		},
		"ports": {
			col:      collectors.NewPortsCollector(cfg.Cache.DiscoveryTTL),
			fieldMap: zabbix.PortLLDFields,
		},
		"batteries": {
			col:      collectors.NewBatteriesCollector(cfg.Cache.DiscoveryTTL),
			fieldMap: zabbix.BatteryLLDFields,
		},
		"psus": {
			col:      collectors.NewPSUsCollector(cfg.Cache.DiscoveryTTL),
			fieldMap: zabbix.PSULLDFields,
		},
	}

	entry, ok := available[category]
	if !ok {
		return nil, nil, fmt.Errorf("unknown discovery category %q: valid values are drives|pools|volumes|enclosures|nodes|ports|batteries|psus", category)
	}

	parserRecords, err := entry.col.Collect(client)
	if err != nil {
		return nil, nil, fmt.Errorf("collector %q failed: %w", category, err)
	}

	return parserRecords, entry.fieldMap, nil
}

// buildFromStaleCache construye un output usando únicamente datos expirados del cache.
// Usado como fallback cuando SSH falla, para evitar alertas falsas durante mantenimiento.
func buildFromStaleCache(
	c *cache.Cache,
	cfg *config.Config,
	startTime time.Time,
	sshErr error,
) *zabbix.Output {

	collectorNames := []string{
		"system", "nodes", "enclosures", "drives",
		"pools", "volumes", "ports", "flashcopy",
		"replication", "performance", "batteries", "psus",
	}

	results := make(map[string]collectors.Result, len(collectorNames))

	for _, name := range collectorNames {
		key := cache.MakeKey(cfg.SSH.Host, name)
		entry, ok := c.GetStale(key)
		if !ok {
			results[name] = collectors.Result{
				Name:    name,
				Records: []parser.Record{},
				Error:   fmt.Sprintf("SSH failed and no cache available: %v", sshErr),
			}
			continue
		}

		var records []parser.Record
		if err := entry.Unmarshal(&records); err != nil {
			results[name] = collectors.Result{
				Name:    name,
				Records: []parser.Record{},
				Error:   fmt.Sprintf("SSH failed and cache corrupted: %v", sshErr),
			}
			continue
		}

		cachedAt := entry.CachedAt
		results[name] = collectors.Result{
			Name:     name,
			Records:  records,
			CacheHit: true,
			Error:    fmt.Sprintf("SSH failed (stale cache from %s): %v", cachedAt.Format(time.RFC3339), sshErr),
			CachedAt: &cachedAt,
		}
	}

	output, err := zabbix.Build(
		zabbix.BuilderConfig{
			Host:         cfg.SSH.Host,
			Version:      version,
			MaxJSONBytes: cfg.Limits.MaxJSONBytes,
			MaxVolumes:   cfg.Limits.MaxVolumes,
			MaxDrives:    cfg.Limits.MaxDrives,
		},
		results,
		c.Stats(),
		startTime,
	)
	if err != nil {
		// En el peor caso, emitir JSON mínimo de error.
		return &zabbix.Output{
			Timestamp: startTime.UTC().Format(time.RFC3339),
			Host:      cfg.SSH.Host,
			Version:   version,
			Status: zabbix.CollectionStatus{
				Success: false,
				Errors:  []string{fmt.Sprintf("SSH failed: %v", sshErr)},
			},
		}
	}

	return output
}

// emitJSON serializa el output y lo escribe a stdout.
// Si la serialización falla, escribe un JSON de error mínimo.
func emitJSON(out *zabbix.Output) {
	data, err := zabbix.Serialize(out)
	if err != nil {
		// Fallback de emergencia: JSON mínimo válido.
		fmt.Printf(`{"error":"serialization failed: %s","timestamp":"%s"}`,
			escapeJSONString(err.Error()),
			time.Now().UTC().Format(time.RFC3339),
		)
		return
	}
	fmt.Println(string(data))
}

// fatalJSON emite un JSON de error estructurado y termina con exit code 1.
// Garantiza que Zabbix siempre reciba JSON válido, nunca output vacío.
func fatalJSON(errMsg string, startTime time.Time) {
	out := &zabbix.Output{
		Timestamp: startTime.UTC().Format(time.RFC3339),
		Version:   version,
		Status: zabbix.CollectionStatus{
			Success:    false,
			DurationMs: time.Since(startTime).Milliseconds(),
			Errors:     []string{errMsg},
		},
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Printf(`{"error":"fatal: %s"}`, escapeJSONString(errMsg))
	} else {
		fmt.Println(string(data))
	}

	logStderr("FATAL: %s", errMsg)
	os.Exit(1)
}

// fatalLLD emite un LLD vacío estructurado y termina con exit code 1.
func fatalLLD(errMsg string) {
	emitEmptyLLD()
	logStderr("FATAL discover: %s", errMsg)
	os.Exit(1)
}

// emitEmptyLLD escribe un LLD vacío válido para Zabbix.
func emitEmptyLLD() {
	fmt.Println(`{"data":[]}`)
}

// emitLLDFromCache escribe datos LLD cacheados a stdout.
func emitLLDFromCache(items []map[string]string) {
	type lldData struct {
		Data []map[string]string `json:"data"`
	}
	result := lldData{Data: items}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		emitEmptyLLD()
		return
	}
	fmt.Println(string(data))
}

// recordsToLLDItems convierte []parser.Record a []map[string]string
// usando el fieldMap para traducir nombres de campo a macros Zabbix.
func recordsToLLDItems(
	records []parser.Record,
	fieldMap map[string]string,
) []map[string]string {

	items := make([]map[string]string, 0, len(records))
	for _, record := range records {
		item := make(map[string]string, len(fieldMap))
		for field, macro := range fieldMap {
			if val, ok := record[field]; ok {
				item[macro] = val
			} else {
				item[macro] = ""
			}
		}
		items = append(items, item)
	}
	return items
}

// logStderr escribe un mensaje de log a stderr.
// Zabbix captura stderr para el log de external scripts.
func logStderr(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[flashsystem-collector] "+format+"\n", args...)
}

// escapeJSONString escapa caracteres especiales para uso seguro en JSON raw.
func escapeJSONString(s string) string {
	data, err := json.Marshal(s)
	if err != nil {
		return "unknown error"
	}
	// json.Marshal incluye las comillas — removerlas.
	if len(data) >= 2 {
		return string(data[1 : len(data)-1])
	}
	return string(data)
}

// printUsage imprime el mensaje de uso del binario.
func printUsage() {
	fmt.Fprintf(os.Stderr, `flashsystem-collector %s

Usage:
  flashsystem-collector collect  [flags]   Collect all metrics (Master Item mode)
  flashsystem-collector discover [flags]   Run LLD discovery for a category
  flashsystem-collector version            Show version

Collect flags:
  -host string          FlashSystem IP or hostname (env: FS_HOST)
  -port int             SSH port, default 22 (env: FS_PORT)
  -user string          SSH username (env: FS_USER)
  -keyfile string       Path to SSH private key (env: FS_KEYFILE)
  -password string      SSH password, insecure (env: FS_PASSWORD)
  -insecure             Disable SSH host key verification (env: FS_INSECURE)
  -known-hosts string   Path to known_hosts file (env: FS_KNOWN_HOSTS)
  -cache-dir string     Cache directory, default /var/tmp (env: FS_CACHE_DIR)
  -metrics-ttl duration Cache TTL for metrics, default 5m (env: FS_METRICS_TTL)
  -discovery-ttl dur    Cache TTL for discovery, default 4h (env: FS_DISCOVERY_TTL)
  -max-json-bytes int   Max JSON output size, default 1048576 (env: FS_MAX_JSON_BYTES)
  -verbose              Enable verbose logging to stderr (env: FS_VERBOSE)

Discover flags:
  (all collect flags) +
  -category string      drives|pools|volumes|enclosures|nodes|ports

Examples:
  # Collect with SSH key (recommended):
  flashsystem-collector collect -host 10.10.10.50 -user monitor -keyfile /etc/zabbix/ssh/fs_key

  # Collect with password (isolated networks only):
  flashsystem-collector collect -host 10.10.10.50 -user monitor -password secret -insecure

  # Discover drives:
  flashsystem-collector discover -host 10.10.10.50 -user monitor -keyfile /etc/zabbix/ssh/fs_key -category drives

  # Using environment variables:
  FS_HOST=10.10.10.50 FS_USER=monitor FS_KEYFILE=/etc/zabbix/ssh/fs_key flashsystem-collector collect

`, version)
}
