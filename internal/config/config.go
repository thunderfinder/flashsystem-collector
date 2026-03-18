package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contiene toda la configuración del collector.
type Config struct {
	SSH     SSHConfig
	Cache   CacheConfig
	Limits  LimitsConfig
	Logging LogConfig
}

// SSHConfig contiene parámetros de conexión SSH.
type SSHConfig struct {
	Host            string
	Port            int
	User            string
	Password        string // Vacío si se usa KeyFile
	KeyFile         string // Vacío si se usa Password
	DialTimeout     time.Duration
	CommandTimeout  time.Duration
	InsecureHostKey bool   // Solo para entornos controlados/testing
	KnownHostsFile  string // Ruta al known_hosts; ignorado si InsecureHostKey=true
}

// CacheConfig contiene parámetros del sistema de cache.
type CacheConfig struct {
	Dir            string        // Directorio donde se guarda el archivo de cache
	MetricsTTL     time.Duration // TTL para métricas operativas
	DiscoveryTTL   time.Duration // TTL para datos de discovery (LLD)
	PerformanceTTL time.Duration // TTL para stats de rendimiento
}

// LimitsConfig define límites operativos.
type LimitsConfig struct {
	MaxJSONBytes int // Tamaño máximo del JSON de salida en bytes
	MaxVolumes   int // Máximo de volúmenes a incluir en output
	MaxDrives    int // Máximo de drives a incluir en output
}

// LogConfig define configuración de logging.
type LogConfig struct {
	Verbose bool // Si true, escribe logs de debug a stderr
}

// defaultConfig retorna una configuración con valores seguros por defecto.
func defaultConfig() Config {
	return Config{
		SSH: SSHConfig{
			Port:            22,
			DialTimeout:     10 * time.Second,
			CommandTimeout:  30 * time.Second,
			InsecureHostKey: false,
			KnownHostsFile:  "",
		},
		Cache: CacheConfig{
			Dir:            "/var/tmp",
			MetricsTTL:     300 * time.Second,   // 5 minutos
			DiscoveryTTL:   14400 * time.Second, // 4 horas
			PerformanceTTL: 60 * time.Second,    // 1 minuto
		},
		Limits: LimitsConfig{
			MaxJSONBytes: 1 * 1024 * 1024, // 1 MB
			MaxVolumes:   500,
			MaxDrives:    500,
		},
		Logging: LogConfig{
			Verbose: false,
		},
	}
}

// Load carga la configuración desde flags y variables de entorno.
// Flags tienen prioridad sobre env vars. Env vars tienen prioridad sobre defaults.
// Debe llamarse después de flag.Parse().
func Load() (*Config, error) {

	cfg := defaultConfig()

	// --- Flags de línea de comandos ---
	host := flag.String("host", "", "FlashSystem management IP or hostname (env: FS_HOST)")
	port := flag.Int("port", 0, "SSH port, default 22 (env: FS_PORT)")
	user := flag.String("user", "", "SSH username (env: FS_USER)")
	password := flag.String("password", "", "SSH password, insecure: visible in ps (env: FS_PASSWORD)")
	keyFile := flag.String("keyfile", "", "Path to SSH private key file (env: FS_KEYFILE)")
	insecure := flag.Bool("insecure", false, "Disable SSH host key verification (env: FS_INSECURE)")
	knownHosts := flag.String("known-hosts", "", "Path to known_hosts file (env: FS_KNOWN_HOSTS)")
	cacheDir := flag.String("cache-dir", "", "Directory for cache files, default /var/tmp (env: FS_CACHE_DIR)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging to stderr (env: FS_VERBOSE)")
	dialTimeout := flag.Duration("dial-timeout", 0, "SSH dial timeout, default 10s (env: FS_DIAL_TIMEOUT)")
	cmdTimeout := flag.Duration("cmd-timeout", 0, "SSH command timeout, default 30s (env: FS_CMD_TIMEOUT)")
	metricsTTL := flag.Duration("metrics-ttl", 0, "Cache TTL for metrics, default 5m (env: FS_METRICS_TTL)")
	discoveryTTL := flag.Duration("discovery-ttl", 0, "Cache TTL for discovery, default 4h (env: FS_DISCOVERY_TTL)")
	perfTTL := flag.Duration("perf-ttl", 0, "Cache TTL for performance stats, default 60s (env: FS_PERF_TTL)")
	maxJSON := flag.Int("max-json-bytes", 0, "Max JSON output size in bytes, default 1MB (env: FS_MAX_JSON_BYTES)")
	maxVols := flag.Int("max-volumes", 0, "Max volumes in output, default 500 (env: FS_MAX_VOLUMES)")
	maxDrives := flag.Int("max-drives", 0, "Max drives in output, default 500 (env: FS_MAX_DRIVES)")

	flag.Parse()

	// --- Aplicar flags (si fueron seteados) ---
	if *host != "" {
		cfg.SSH.Host = *host
	}
	if *port != 0 {
		cfg.SSH.Port = *port
	}
	if *user != "" {
		cfg.SSH.User = *user
	}
	if *password != "" {
		cfg.SSH.Password = *password
	}
	if *keyFile != "" {
		cfg.SSH.KeyFile = *keyFile
	}
	if *insecure {
		cfg.SSH.InsecureHostKey = true
	}
	if *knownHosts != "" {
		cfg.SSH.KnownHostsFile = *knownHosts
	}
	if *cacheDir != "" {
		cfg.Cache.Dir = *cacheDir
	}
	if *verbose {
		cfg.Logging.Verbose = true
	}
	if *dialTimeout != 0 {
		cfg.SSH.DialTimeout = *dialTimeout
	}
	if *cmdTimeout != 0 {
		cfg.SSH.CommandTimeout = *cmdTimeout
	}
	if *metricsTTL != 0 {
		cfg.Cache.MetricsTTL = *metricsTTL
	}
	if *discoveryTTL != 0 {
		cfg.Cache.DiscoveryTTL = *discoveryTTL
	}
	if *perfTTL != 0 {
		cfg.Cache.PerformanceTTL = *perfTTL
	}
	if *maxJSON != 0 {
		cfg.Limits.MaxJSONBytes = *maxJSON
	}
	if *maxVols != 0 {
		cfg.Limits.MaxVolumes = *maxVols
	}
	if *maxDrives != 0 {
		cfg.Limits.MaxDrives = *maxDrives
	}

	// --- Aplicar variables de entorno (solo si el flag no fue seteado) ---
	applyEnv(&cfg)

	// --- Validación ---
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// applyEnv lee variables de entorno y las aplica solo si el campo aún tiene el valor default.
func applyEnv(cfg *Config) {

	if cfg.SSH.Host == "" {
		cfg.SSH.Host = strings.TrimSpace(os.Getenv("FS_HOST"))
	}
	if cfg.SSH.Port == 22 {
		if v := os.Getenv("FS_PORT"); v != "" {
			if p, err := strconv.Atoi(v); err == nil && p > 0 {
				cfg.SSH.Port = p
			}
		}
	}
	if cfg.SSH.User == "" {
		cfg.SSH.User = strings.TrimSpace(os.Getenv("FS_USER"))
	}
	if cfg.SSH.Password == "" {
		cfg.SSH.Password = os.Getenv("FS_PASSWORD")
	}
	if cfg.SSH.KeyFile == "" {
		cfg.SSH.KeyFile = strings.TrimSpace(os.Getenv("FS_KEYFILE"))
	}
	if !cfg.SSH.InsecureHostKey {
		if v := os.Getenv("FS_INSECURE"); v == "true" || v == "1" {
			cfg.SSH.InsecureHostKey = true
		}
	}
	if cfg.SSH.KnownHostsFile == "" {
		cfg.SSH.KnownHostsFile = strings.TrimSpace(os.Getenv("FS_KNOWN_HOSTS"))
	}
	if cfg.Cache.Dir == "/var/tmp" {
		if v := os.Getenv("FS_CACHE_DIR"); v != "" {
			cfg.Cache.Dir = strings.TrimSpace(v)
		}
	}
	if !cfg.Logging.Verbose {
		if v := os.Getenv("FS_VERBOSE"); v == "true" || v == "1" {
			cfg.Logging.Verbose = true
		}
	}
	if d := parseDurationEnv("FS_DIAL_TIMEOUT"); d != 0 {
		cfg.SSH.DialTimeout = d
	}
	if d := parseDurationEnv("FS_CMD_TIMEOUT"); d != 0 {
		cfg.SSH.CommandTimeout = d
	}
	if d := parseDurationEnv("FS_METRICS_TTL"); d != 0 {
		cfg.Cache.MetricsTTL = d
	}
	if d := parseDurationEnv("FS_DISCOVERY_TTL"); d != 0 {
		cfg.Cache.DiscoveryTTL = d
	}
	if d := parseDurationEnv("FS_PERF_TTL"); d != 0 {
		cfg.Cache.PerformanceTTL = d
	}
	if v := os.Getenv("FS_MAX_JSON_BYTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Limits.MaxJSONBytes = n
		}
	}
	if v := os.Getenv("FS_MAX_VOLUMES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Limits.MaxVolumes = n
		}
	}
	if v := os.Getenv("FS_MAX_DRIVES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Limits.MaxDrives = n
		}
	}
}

// parseDurationEnv parsea una variable de entorno como time.Duration.
// Retorna 0 si la variable no existe o no es válida.
func parseDurationEnv(key string) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0
	}
	return d
}

// validate verifica que la configuración sea completa y coherente.
func (cfg *Config) validate() error {

	if cfg.SSH.Host == "" {
		return errors.New("SSH host is required: use -host flag or FS_HOST env var")
	}

	if cfg.SSH.User == "" {
		return errors.New("SSH user is required: use -user flag or FS_USER env var")
	}

	if cfg.SSH.Port < 1 || cfg.SSH.Port > 65535 {
		return fmt.Errorf("SSH port must be between 1 and 65535, got %d", cfg.SSH.Port)
	}

	// Exactamente uno de keyfile o password debe estar configurado.
	hasKey := cfg.SSH.KeyFile != ""
	hasPass := cfg.SSH.Password != ""

	if !hasKey && !hasPass {
		return errors.New("SSH authentication required: use -keyfile or -password flag (or FS_KEYFILE / FS_PASSWORD env vars)")
	}

	if hasKey && hasPass {
		return errors.New("specify either -keyfile or -password, not both")
	}

	if hasKey {
		if _, err := os.Stat(cfg.SSH.KeyFile); err != nil {
			return fmt.Errorf("SSH key file not accessible: %w", err)
		}
	}

	if !cfg.SSH.InsecureHostKey && cfg.SSH.KnownHostsFile != "" {
		if _, err := os.Stat(cfg.SSH.KnownHostsFile); err != nil {
			return fmt.Errorf("known_hosts file not accessible: %w", err)
		}
	}

	if cfg.SSH.DialTimeout < time.Second {
		return fmt.Errorf("dial-timeout must be >= 1s, got %s", cfg.SSH.DialTimeout)
	}

	if cfg.SSH.CommandTimeout < time.Second {
		return fmt.Errorf("cmd-timeout must be >= 1s, got %s", cfg.SSH.CommandTimeout)
	}

	if cfg.Cache.MetricsTTL < time.Second {
		return fmt.Errorf("metrics-ttl must be >= 1s, got %s", cfg.Cache.MetricsTTL)
	}

	if cfg.Cache.DiscoveryTTL < time.Minute {
		return fmt.Errorf("discovery-ttl must be >= 1m, got %s", cfg.Cache.DiscoveryTTL)
	}

	if cfg.Cache.PerformanceTTL < time.Second {
		return fmt.Errorf("perf-ttl must be >= 1s, got %s", cfg.Cache.PerformanceTTL)
	}

	if cfg.Limits.MaxJSONBytes < 1024 {
		return fmt.Errorf("max-json-bytes must be >= 1024, got %d", cfg.Limits.MaxJSONBytes)
	}

	if cfg.Limits.MaxVolumes < 1 {
		return fmt.Errorf("max-volumes must be >= 1, got %d", cfg.Limits.MaxVolumes)
	}

	if cfg.Limits.MaxDrives < 1 {
		return fmt.Errorf("max-drives must be >= 1, got %d", cfg.Limits.MaxDrives)
	}

	// Validar que el directorio de cache existe y es escribible.
	if err := validateCacheDir(cfg.Cache.Dir); err != nil {
		return fmt.Errorf("cache directory error: %w", err)
	}

	return nil
}

// validateCacheDir verifica que el directorio existe y tiene permisos de escritura.
func validateCacheDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("cannot access directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", dir)
	}

	// Verificar escritura creando un archivo temporal.
	tmp, err := os.CreateTemp(dir, ".flashsystem_write_test_*")
	if err != nil {
		return fmt.Errorf("directory %q is not writable: %w", dir, err)
	}
	tmp.Close()
	os.Remove(tmp.Name())

	return nil
}

// CacheFile retorna la ruta completa del archivo de cache para un host dado.
// El nombre sanitiza caracteres no válidos en nombres de archivo.
func (cfg *Config) CacheFile(host string) string {
	safe := strings.NewReplacer(":", "_", "/", "_", "\\", "_", " ", "_").Replace(host)
	return fmt.Sprintf("%s/flashsystem_%s.cache.json", cfg.Cache.Dir, safe)
}
