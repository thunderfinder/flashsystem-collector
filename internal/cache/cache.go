package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry representa una entrada individual en el cache.
type Entry struct {
	// Value contiene los datos serializados como JSON raw.
	// Se usa json.RawMessage para evitar doble serialización.
	Value     json.RawMessage `json:"value"`
	ExpiresAt time.Time       `json:"expires_at"`
	CachedAt  time.Time       `json:"cached_at"`
	Key       string          `json:"key"`
}

// IsExpired retorna true si la entrada superó su TTL.
func (e *Entry) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// store es la estructura interna serializada al disco.
type store struct {
	Entries   map[string]Entry `json:"entries"`
	UpdatedAt time.Time        `json:"updated_at"`
	Version   string           `json:"version"`
}

// Cache gestiona el almacenamiento persistente de resultados de collectors.
// Es seguro para uso concurrente.
type Cache struct {
	mu       sync.RWMutex
	filePath string
	data     store
}

const cacheVersion = "1"

// New carga (o inicializa) el cache desde el archivo en filePath.
// Si el archivo no existe o está corrupto, arranca con cache vacío.
// Nunca retorna error — un cache roto simplemente se descarta.
func New(filePath string) *Cache {
	c := &Cache{
		filePath: filePath,
		data: store{
			Entries: make(map[string]Entry),
			Version: cacheVersion,
		},
	}
	c.loadFromDisk()
	return c
}

// Get busca una entrada en el cache por clave.
// Retorna (entry, true) si existe y no está expirada.
// Retorna (Entry{}, false) si no existe o expiró.
func (c *Cache) Get(key string) (Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.data.Entries[key]
	if !ok {
		return Entry{}, false
	}
	if entry.IsExpired() {
		return Entry{}, false
	}
	return entry, true
}

// Set almacena un valor en el cache con el TTL especificado.
// El valor debe ser serializable a JSON.
// Persiste el cache completo al disco después de cada escritura.
// Retorna error solo si la serialización o escritura al disco falla.
func (c *Cache) Set(key string, value interface{}, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache: cannot serialize value for key %q: %w", key, err)
	}

	now := time.Now()
	entry := Entry{
		Value:     json.RawMessage(raw),
		ExpiresAt: now.Add(ttl),
		CachedAt:  now,
		Key:       key,
	}

	c.mu.Lock()
	c.data.Entries[key] = entry
	c.data.UpdatedAt = now
	c.mu.Unlock()

	// Persistir al disco fuera del lock de lectura/escritura de datos
	// para no bloquear lecturas durante la IO.
	return c.saveToDisk()
}

// Delete elimina una entrada del cache por clave.
// No retorna error si la clave no existe.
func (c *Cache) Delete(key string) error {
	c.mu.Lock()
	delete(c.data.Entries, key)
	c.data.UpdatedAt = time.Now()
	c.mu.Unlock()

	return c.saveToDisk()
}

// Purge elimina todas las entradas expiradas del cache y persiste.
// Debe llamarse periódicamente o al inicio para mantener el archivo limpio.
func (c *Cache) Purge() error {
	now := time.Now()

	c.mu.Lock()
	for key, entry := range c.data.Entries {
		if now.After(entry.ExpiresAt) {
			delete(c.data.Entries, key)
		}
	}
	c.data.UpdatedAt = now
	c.mu.Unlock()

	return c.saveToDisk()
}

// Len retorna el número de entradas activas (no expiradas) en el cache.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := 0
	now := time.Now()
	for _, entry := range c.data.Entries {
		if !now.After(entry.ExpiresAt) {
			count++
		}
	}
	return count
}

// Stats retorna información de diagnóstico sobre el estado del cache.
func (c *Cache) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	active := 0
	expired := 0

	for _, entry := range c.data.Entries {
		if now.After(entry.ExpiresAt) {
			expired++
		} else {
			active++
		}
	}

	return Stats{
		ActiveEntries:  active,
		ExpiredEntries: expired,
		TotalEntries:   active + expired,
		FilePath:       c.filePath,
		UpdatedAt:      c.data.UpdatedAt,
	}
}

// Stats contiene métricas del estado del cache.
type Stats struct {
	ActiveEntries  int       `json:"active_entries"`
	ExpiredEntries int       `json:"expired_entries"`
	TotalEntries   int       `json:"total_entries"`
	FilePath       string    `json:"file_path"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// loadFromDisk lee el archivo de cache desde disco.
// Si el archivo no existe o es inválido, inicializa el cache vacío sin error.
func (c *Cache) loadFromDisk() {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.filePath)
	if err != nil {
		// Archivo no existe todavía — es el caso normal en la primera ejecución.
		return
	}

	var s store
	if err := json.Unmarshal(data, &s); err != nil {
		// Archivo corrupto — lo descartamos y arrancamos vacío.
		// No borramos el archivo aquí; saveToDisk lo sobreescribirá.
		return
	}

	// Validar versión del formato.
	if s.Version != cacheVersion {
		// Formato incompatible — cache vacío.
		return
	}

	// Asegurar que el map no sea nil si el JSON tenía entries: null.
	if s.Entries == nil {
		s.Entries = make(map[string]Entry)
	}

	c.data = s
}

// saveToDisk persiste el estado actual del cache al archivo.
// Usa escritura atómica: escribe a un archivo temporal y luego hace rename.
// Esto garantiza que el archivo nunca quede en estado inconsistente
// aunque el proceso muera a mitad de la escritura.
func (c *Cache) saveToDisk() error {
	c.mu.RLock()
	data, err := json.Marshal(c.data)
	c.mu.RUnlock()

	if err != nil {
		return fmt.Errorf("cache: cannot serialize store: %w", err)
	}

	// Crear archivo temporal en el mismo directorio que el destino.
	// Es crítico que estén en el mismo filesystem para que rename sea atómico.
	dir := filepath.Dir(c.filePath)
	tmp, err := os.CreateTemp(dir, ".flashsystem_cache_*.tmp")
	if err != nil {
		return fmt.Errorf("cache: cannot create temp file in %q: %w", dir, err)
	}
	tmpName := tmp.Name()

	// Asegurar limpieza del archivo temporal si algo falla.
	success := false
	defer func() {
		if !success {
			os.Remove(tmpName)
		}
	}()

	// Escribir datos al archivo temporal.
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("cache: cannot write to temp file %q: %w", tmpName, err)
	}

	// Sync para garantizar que los datos llegaron al disco.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("cache: cannot sync temp file %q: %w", tmpName, err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cache: cannot close temp file %q: %w", tmpName, err)
	}

	// Rename atómico: reemplaza el archivo de cache en una operación.
	if err := os.Rename(tmpName, c.filePath); err != nil {
		return fmt.Errorf("cache: cannot rename %q to %q: %w", tmpName, c.filePath, err)
	}

	success = true
	return nil
}

// MakeKey construye una clave de cache estandarizada.
// Formato: "host:collector" — ej: "10.10.10.50:pools"
func MakeKey(host, collector string) string {
	return fmt.Sprintf("%s:%s", host, collector)
}

// MakeDiscoveryKey construye una clave de cache para datos de discovery LLD.
// Formato: "host:discovery:collector" — ej: "10.10.10.50:discovery:drives"
func MakeDiscoveryKey(host, collector string) string {
	return fmt.Sprintf("%s:discovery:%s", host, collector)
}

// Unmarshal deserializa el valor de una Entry al tipo destino.
// Uso típico:
//
//	var records []parser.Record
//	if err := entry.Unmarshal(&records); err != nil { ... }
func (e *Entry) Unmarshal(dest interface{}) error {
	if err := json.Unmarshal(e.Value, dest); err != nil {
		return fmt.Errorf("cache: cannot deserialize entry %q: %w", e.Key, err)
	}
	return nil
}
