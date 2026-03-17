package collectors

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/flashsystem-collector/internal/cache"
	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

// Collector es la interfaz que debe implementar cada collector.
// Name() retorna el identificador único del collector (ej: "pools").
// Collect() ejecuta el comando SSH y retorna los registros parseados.
// TTL() retorna el tiempo de vida del resultado en cache.
type Collector interface {
	Name() string
	Collect(client *internalssh.Client) ([]parser.Record, error)
	TTL() time.Duration
}

// Result contiene el resultado de ejecutar un Collector.
type Result struct {
	Name       string          `json:"name"`
	Records    []parser.Record `json:"records"`
	CacheHit   bool            `json:"cache_hit"`
	Error      string          `json:"error,omitempty"`
	DurationMs int64           `json:"duration_ms"`
	CachedAt   *time.Time      `json:"cached_at,omitempty"`
}

// RunnerConfig contiene la configuración del Runner.
type RunnerConfig struct {
	// Host es el identificador del FlashSystem (usado como prefijo de clave de cache).
	Host string
	// MaxConcurrent limita el número de sesiones SSH simultáneas.
	// Recomendado: 3-5 para no saturar el FlashSystem.
	MaxConcurrent int
}

// Runner orquesta la ejecución de todos los collectors.
// Consulta cache antes de SSH. Escribe cache después de SSH.
// Ejecuta collectors en paralelo con concurrencia controlada.
type Runner struct {
	cfg        RunnerConfig
	cache      *cache.Cache
	collectors []Collector
}

// NewRunner crea un Runner con los collectors y cache provistos.
func NewRunner(cfg RunnerConfig, c *cache.Cache, cols []Collector) *Runner {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 3
	}
	return &Runner{
		cfg:        cfg,
		cache:      c,
		collectors: cols,
	}
}

// RunAll ejecuta todos los collectors registrados en paralelo.
// Para cada collector:
//  1. Consulta el cache → si hay hit válido, retorna sin SSH.
//  2. Si miss → adquiere slot del semáforo → ejecuta SSH → libera slot.
//  3. Guarda resultado en cache con el TTL del collector.
//
// Retorna un map[nombre]Result con los resultados de todos los collectors.
// Nunca retorna error — los errores individuales están en Result.Error.
func (r *Runner) RunAll(client *internalssh.Client) map[string]Result {
	results := make(map[string]Result, len(r.collectors))
	var mu sync.Mutex

	// Semáforo para limitar sesiones SSH concurrentes.
	sem := make(chan struct{}, r.cfg.MaxConcurrent)

	var wg sync.WaitGroup
	wg.Add(len(r.collectors))

	for _, col := range r.collectors {
		col := col // captura de variable para goroutine
		go func() {
			defer wg.Done()

			res := r.runOne(client, col, sem)

			mu.Lock()
			results[col.Name()] = res
			mu.Unlock()
		}()
	}

	wg.Wait()
	return results
}

// runOne ejecuta un único collector respetando cache y semáforo.
func (r *Runner) runOne(
	client *internalssh.Client,
	col Collector,
	sem chan struct{},
) Result {
	start := time.Now()
	cacheKey := cache.MakeKey(r.cfg.Host, col.Name())

	// --- Consultar cache ---
	if entry, ok := r.cache.Get(cacheKey); ok {
		var records []parser.Record
		if err := entry.Unmarshal(&records); err == nil {
			cachedAt := entry.CachedAt
			return Result{
				Name:       col.Name(),
				Records:    records,
				CacheHit:   true,
				DurationMs: time.Since(start).Milliseconds(),
				CachedAt:   &cachedAt,
			}
		}
		// Si Unmarshal falla, el entry está corrupto — continuar con SSH.
	}

	// --- Cache miss: ejecutar SSH con semáforo ---
	sem <- struct{}{}        // adquirir slot
	defer func() { <-sem }() // liberar slot al terminar

	records, err := col.Collect(client)
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		// En caso de error SSH, intentar retornar el último valor cacheado
		// aunque esté expirado (stale cache como fallback).
		if stale, ok := r.getStale(cacheKey); ok {
			cachedAt := stale.CachedAt
			return Result{
				Name:       col.Name(),
				Records:    stale.Records,
				CacheHit:   true,
				Error:      fmt.Sprintf("SSH failed (using stale cache): %v", err),
				DurationMs: durationMs,
				CachedAt:   &cachedAt,
			}
		}

		return Result{
			Name:       col.Name(),
			Records:    []parser.Record{},
			CacheHit:   false,
			Error:      err.Error(),
			DurationMs: durationMs,
		}
	}

	// Nunca almacenar nil en cache.
	if records == nil {
		records = []parser.Record{}
	}

	// --- Guardar en cache ---
	// El error de cache no es fatal — el resultado igual se retorna.
	_ = r.cache.Set(cacheKey, records, col.TTL())

	return Result{
		Name:       col.Name(),
		Records:    records,
		CacheHit:   false,
		DurationMs: durationMs,
	}
}

// getStale busca el último valor cacheado aunque esté expirado.
// Accede directamente al store interno usando una entrada stale.
// Retorna (staleResult, true) si encuentra algo, (zero, false) si no.
func (r *Runner) getStale(cacheKey string) (staleResult, bool) {
	// Usamos un TTL artificialmente largo para forzar que Get encuentre
	// la entrada aunque esté expirada. Sin embargo, cache.Get solo retorna
	// entradas no expiradas. Necesitamos acceso raw al store.
	//
	// Solución: cache expone GetStale para este caso.
	entry, ok := r.cache.GetStale(cacheKey)
	if !ok {
		return staleResult{}, false
	}

	var records []parser.Record
	if err := entry.Unmarshal(&records); err != nil {
		return staleResult{}, false
	}

	return staleResult{
		Records:  records,
		CachedAt: entry.CachedAt,
	}, true
}

// staleResult es un tipo auxiliar interno para el fallback de cache expirado.
type staleResult struct {
	Records  []parser.Record
	CachedAt time.Time
}

// AllCollectors retorna la lista completa de collectors para un FlashSystem.
// Esta es la función de registro central — agregar aquí nuevos collectors.
func AllCollectors(metricsTTL, discoveryTTL, performanceTTL time.Duration) []Collector {
	return []Collector{
		NewSystemCollector(metricsTTL),
		NewNodesCollector(metricsTTL),
		NewEnclosuresCollector(discoveryTTL),
		NewDrivesCollector(discoveryTTL),
		NewPoolsCollector(metricsTTL),
		NewVolumesCollector(metricsTTL),
		NewPortsCollector(discoveryTTL),
		NewFlashCopyCollector(metricsTTL),
		NewReplicationCollector(metricsTTL),
		NewPerformanceCollector(performanceTTL),
	}
}

// MarshalRecords serializa un slice de Records a JSON raw.
// Retorna [] si records es nil o vacío.
func MarshalRecords(records []parser.Record) json.RawMessage {
	if len(records) == 0 {
		return json.RawMessage("[]")
	}
	data, err := json.Marshal(records)
	if err != nil {
		return json.RawMessage("[]")
	}
	return json.RawMessage(data)
}
