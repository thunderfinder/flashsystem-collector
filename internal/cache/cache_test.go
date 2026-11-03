package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestCacheSetGetDelete(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "test.cache.json")

	c := New(cachePath)

	key := MakeKey("10.0.0.1", "pools")
	val := map[string]string{"name": "pool0", "status": "online"}

	// Miss inicial
	if _, ok := c.Get(key); ok {
		t.Fatal("expected cache miss for unpopulated key")
	}

	// Set con TTL
	if err := c.Set(key, val, 1*time.Hour); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Hit
	entry, ok := c.Get(key)
	if !ok {
		t.Fatal("expected cache hit after Set")
	}

	var retrieved map[string]string
	if err := entry.Unmarshal(&retrieved); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if retrieved["name"] != "pool0" {
		t.Errorf("expected pool0, got %s", retrieved["name"])
	}

	// Persistence reload from disk
	c2 := New(cachePath)
	entry2, ok := c2.Get(key)
	if !ok {
		t.Fatal("expected cache hit from reloaded disk cache")
	}
	if err := entry2.Unmarshal(&retrieved); err != nil || retrieved["name"] != "pool0" {
		t.Fatalf("failed reloading entry from disk: %v", err)
	}

	// Delete
	if err := c.Delete(key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, ok := c.Get(key); ok {
		t.Fatal("expected cache miss after Delete")
	}
}

func TestCacheExpirationAndStale(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "test_expire.cache.json")

	c := New(cachePath)
	key := MakeDiscoveryKey("10.0.0.1", "drives")
	val := []string{"drive0", "drive1"}

	// Set con TTL negativo (inmediatamente expirado)
	if err := c.Set(key, val, -1*time.Minute); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get debe retornar miss
	if _, ok := c.Get(key); ok {
		t.Fatal("expected Get to fail for expired entry")
	}

	// GetStale debe retornar hit
	staleEntry, ok := c.GetStale(key)
	if !ok {
		t.Fatal("expected GetStale to succeed for expired entry")
	}
	var drives []string
	if err := staleEntry.Unmarshal(&drives); err != nil || len(drives) != 2 {
		t.Fatalf("GetStale Unmarshal failed: %v", err)
	}

	// Stats
	stats := c.Stats()
	if stats.ActiveEntries != 0 || stats.ExpiredEntries != 1 {
		t.Errorf("unexpected stats: %+v", stats)
	}
	if c.Len() != 0 {
		t.Errorf("expected Len() 0, got %d", c.Len())
	}

	// Purge debe eliminar la entrada expirada
	if err := c.Purge(); err != nil {
		t.Fatalf("Purge failed: %v", err)
	}
	if _, ok := c.GetStale(key); ok {
		t.Fatal("expected entry to be removed after Purge")
	}
}

func TestCacheCorruptFileHandling(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "corrupted.cache.json")

	// Escribir JSON corrupto con permisos 0600
	if err := os.WriteFile(cachePath, []byte("NOT_VALID_JSON{{{"), 0600); err != nil {
		t.Fatalf("failed writing corrupt file: %v", err)
	}
	_ = os.Chmod(cachePath, 0600)

	// New debe cargar un cache vacío sin panickear
	c := New(cachePath)
	if c.Len() != 0 {
		t.Fatalf("expected empty cache from corrupted file, got len %d", c.Len())
	}

	// Debe poder escribir y sobreescribir el archivo corrupto
	if err := c.Set("test_key", "value", time.Hour); err != nil {
		t.Fatalf("Set failed on recovered cache: %v", err)
	}
	if _, ok := c.Get("test_key"); !ok {
		t.Fatal("expected hit after writing to recovered cache")
	}
}

func TestCacheFileSecurity(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Create a valid cache file with 0600 permissions
	validPath := filepath.Join(tempDir, "valid.cache.json")
	cValid := New(validPath)
	if err := cValid.Set("valid_key", "valid_val", time.Hour); err != nil {
		t.Fatalf("failed setting cache entry: %v", err)
	}

	// Verify file permissions on disk after saveToDisk
	info, err := os.Stat(validPath)
	if err != nil {
		t.Fatalf("failed stat valid cache file: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("expected regular file, got %v", info.Mode())
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("expected cache file permissions 0600, got %04o", info.Mode().Perm())
	}

	// Loading valid 0600 cache file should succeed
	reloaded := New(validPath)
	if _, ok := reloaded.Get("valid_key"); !ok {
		t.Fatal("expected valid 0600 cache file to load successfully")
	}

	// 2. Symlink rejection test
	symlinkPath := filepath.Join(tempDir, "symlink.cache.json")
	if err := os.Symlink(validPath, symlinkPath); err != nil {
		t.Fatalf("failed creating symlink: %v", err)
	}
	cSymlink := New(symlinkPath)
	if _, ok := cSymlink.Get("valid_key"); ok {
		t.Fatal("expected symlink cache file to be rejected on load")
	}

	// 3. Permissive file rejection on POSIX (0644, 0666)
	if runtime.GOOS != "windows" {
		insecurePath := filepath.Join(tempDir, "insecure.cache.json")
		data, err := os.ReadFile(validPath)
		if err != nil {
			t.Fatalf("failed reading valid cache file: %v", err)
		}
		if err := os.WriteFile(insecurePath, data, 0644); err != nil {
			t.Fatalf("failed writing insecure cache file: %v", err)
		}
		if err := os.Chmod(insecurePath, 0644); err != nil {
			t.Fatalf("failed chmod 0644: %v", err)
		}

		cInsecure := New(insecurePath)
		if _, ok := cInsecure.Get("valid_key"); ok {
			t.Fatal("expected 0644 cache file to be rejected on load")
		}

		if err := os.Chmod(insecurePath, 0666); err != nil {
			t.Fatalf("failed chmod 0666: %v", err)
		}
		cInsecure2 := New(insecurePath)
		if _, ok := cInsecure2.Get("valid_key"); ok {
			t.Fatal("expected 0666 cache file to be rejected on load")
		}
	}
}

func TestCacheConcurrency(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "concurrency.cache.json")

	c := New(cachePath)

	var wg sync.WaitGroup
	workers := 10
	iterations := 20

	for i := 0; i < workers; i++ {
		workerID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := MakeKey("host", fmt.Sprintf("col_%d", workerID))
				_ = c.Set(key, j, time.Minute)
				_, _ = c.Get(key)
				_, _ = c.GetStale(key)
				_ = c.Stats()
			}
		}()
	}

	wg.Wait()
}
