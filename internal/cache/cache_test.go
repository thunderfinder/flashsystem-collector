package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

		// 4. File ownership helper logic tests (authorized vs unauthorized UIDs)
		curUID := uint32(os.Getuid())
		if !isAuthorizedOwner(curUID, curUID) {
			t.Errorf("expected current UID %d to be authorized", curUID)
		}
		if !isAuthorizedOwner(0, curUID) {
			t.Error("expected root UID 0 to be authorized")
		}

		// Unauthorized foreign UID test (must be rejected when not root)
		if curUID != 0 {
			foreignUID := curUID + 1000
			if isAuthorizedOwner(foreignUID, curUID) {
				t.Errorf("expected foreign UID %d to be rejected for current UID %d", foreignUID, curUID)
			}
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

func TestCacheDiskWriteFailuresAndCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping POSIX permission test on Windows")
	}

	tempDir := t.TempDir()
	readOnlyDir := filepath.Join(tempDir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0500); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	defer func() {
		// Restore write permission so TempDir cleanup succeeds
		_ = os.Chmod(readOnlyDir, 0700)
	}()

	cachePath := filepath.Join(readOnlyDir, "test.cache.json")
	c := New(cachePath)

	// Set should fail because directory is read-only (os.CreateTemp fails)
	err := c.Set("key1", "val1", time.Hour)
	if err == nil {
		t.Fatal("expected Set to fail in read-only directory, got nil")
	}
	if !strings.Contains(err.Error(), "cannot create temp file") {
		t.Errorf("expected 'cannot create temp file' error, got: %v", err)
	}

	// Verify no dangling .flashsystem_cache_*.tmp files leaked
	entries, readErr := os.ReadDir(readOnlyDir)
	if readErr != nil {
		t.Fatalf("failed to read dir: %v", readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".flashsystem_cache_") {
			t.Errorf("found dangling temp file: %s", entry.Name())
		}
	}

	// Delete and Purge should also fail cleanly when saveToDisk fails
	if err := c.Delete("key1"); err == nil {
		t.Fatal("expected Delete to fail in read-only directory, got nil")
	}
	if err := c.Purge(); err == nil {
		t.Fatal("expected Purge to fail in read-only directory, got nil")
	}
}

func TestCacheIncompatibleVersion(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "version_mismatch.cache.json")

	// Write cache file with an incompatible version identifier ("99" instead of cacheVersion "1")
	incompatibleJSON := `{
		"version": "99",
		"updated_at": "2026-05-24T00:00:00Z",
		"entries": {
			"host:col": {
				"key": "host:col",
				"value": [1, 2, 3],
				"cached_at": "2026-05-24T00:00:00Z",
				"expires_at": "2026-05-25T00:00:00Z"
			}
		}
	}`

	if err := os.WriteFile(cachePath, []byte(incompatibleJSON), 0600); err != nil {
		t.Fatalf("failed to write version mismatch file: %v", err)
	}

	// loadFromDisk should discard entries due to version mismatch and start empty
	c := New(cachePath)
	if c.Len() != 0 {
		t.Fatalf("expected empty cache from version mismatch, got len %d", c.Len())
	}
	if _, ok := c.Get("host:col"); ok {
		t.Fatal("expected Get to fail for discarded incompatible version cache")
	}

	// Saving fresh entry should write version "1"
	if err := c.Set("host:col2", "fresh_val", time.Hour); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	reloaded := New(cachePath)
	if _, ok := reloaded.Get("host:col2"); !ok {
		t.Fatal("expected fresh entry to be retrievable after version upgrade")
	}
}

func TestCacheNullEntriesSafety(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "null_entries.cache.json")

	// Store JSON where entries is explicitly null
	nullEntriesJSON := `{
		"version": "1",
		"updated_at": "2026-05-24T00:00:00Z",
		"entries": null
	}`

	if err := os.WriteFile(cachePath, []byte(nullEntriesJSON), 0600); err != nil {
		t.Fatalf("failed to write null entries file: %v", err)
	}

	c := New(cachePath)
	if c.Len() != 0 {
		t.Fatalf("expected 0 active entries, got %d", c.Len())
	}

	// Should not panic on Set or Delete with re-initialized non-nil map
	if err := c.Set("new_key", "value", time.Hour); err != nil {
		t.Fatalf("Set failed on recovered null-entries cache: %v", err)
	}
	if _, ok := c.Get("new_key"); !ok {
		t.Fatal("expected new_key to be found after Set")
	}
	if err := c.Delete("new_key"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

func TestCacheEntryUnmarshalError(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "unmarshal_err.cache.json")

	c := New(cachePath)
	key := "host:stats"
	if err := c.Set(key, map[string]string{"foo": "bar"}, time.Hour); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	entry, ok := c.Get(key)
	if !ok {
		t.Fatal("expected cache hit")
	}

	// Try to unmarshal map into incompatible type ([]int)
	var intSlice []int
	err := entry.Unmarshal(&intSlice)
	if err == nil {
		t.Fatal("expected unmarshal error for incompatible destination type, got nil")
	}
	if !strings.Contains(err.Error(), "cannot deserialize entry") || !strings.Contains(err.Error(), key) {
		t.Errorf("expected descriptive unmarshal error with key %q, got: %v", key, err)
	}
}

func TestCacheExpirationBoundariesAndMixedPurge(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "expire_boundaries.cache.json")

	c := New(cachePath)

	// Entry 1: Fresh (1 hour TTL)
	keyFresh := MakeKey("host1", "system")
	if err := c.Set(keyFresh, "data1", time.Hour); err != nil {
		t.Fatalf("failed to set fresh entry: %v", err)
	}

	// Entry 2: Expired (-10s TTL)
	keyExpired := MakeKey("host1", "pools")
	if err := c.Set(keyExpired, "data2", -10*time.Second); err != nil {
		t.Fatalf("failed to set expired entry: %v", err)
	}

	// Entry 3: Another fresh entry (2 hours TTL)
	keyFresh2 := MakeDiscoveryKey("host1", "drives")
	if err := c.Set(keyFresh2, "data3", 2*time.Hour); err != nil {
		t.Fatalf("failed to set second fresh entry: %v", err)
	}

	// Verify stats accounting before Purge
	stats := c.Stats()
	if stats.ActiveEntries != 2 || stats.ExpiredEntries != 1 || stats.TotalEntries != 3 {
		t.Errorf("unexpected pre-purge stats: %+v (want active: 2, expired: 1, total: 3)", stats)
	}
	if c.Len() != 2 {
		t.Errorf("expected Len() 2, got %d", c.Len())
	}

	// Check Get vs GetStale
	if _, ok := c.Get(keyExpired); ok {
		t.Error("expected expired entry to return miss on Get")
	}
	if _, ok := c.GetStale(keyExpired); !ok {
		t.Error("expected expired entry to return hit on GetStale")
	}

	// Purge expired entries
	if err := c.Purge(); err != nil {
		t.Fatalf("Purge failed: %v", err)
	}

	// After Purge: expired entry must be completely removed from memory and disk
	statsPost := c.Stats()
	if statsPost.ActiveEntries != 2 || statsPost.ExpiredEntries != 0 || statsPost.TotalEntries != 2 {
		t.Errorf("unexpected post-purge stats: %+v (want active: 2, expired: 0, total: 2)", statsPost)
	}
	if _, ok := c.GetStale(keyExpired); ok {
		t.Error("expected expired entry to be gone from GetStale after Purge")
	}

	// Fresh entries must remain intact
	if _, ok := c.Get(keyFresh); !ok {
		t.Error("expected keyFresh to remain intact after Purge")
	}
	if _, ok := c.Get(keyFresh2); !ok {
		t.Error("expected keyFresh2 to remain intact after Purge")
	}

	// Reload from disk to verify atomic disk persistence of Purge
	reloaded := New(cachePath)
	if reloaded.Len() != 2 {
		t.Errorf("expected reloaded Len() 2, got %d", reloaded.Len())
	}
	if _, ok := reloaded.GetStale(keyExpired); ok {
		t.Error("expected expired entry to be gone from disk reload after Purge")
	}
}

func TestCacheConcurrencyWithDeleteAndPurge(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "concurrency_full.cache.json")

	c := New(cachePath)

	var wg sync.WaitGroup
	workers := 8
	iterations := 25

	for i := 0; i < workers; i++ {
		workerID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := MakeKey("host", fmt.Sprintf("worker_%d_col_%d", workerID, j%5))

				// Mixed writes: some with positive TTL, some immediately expired
				ttl := time.Minute
				if j%2 == 0 {
					ttl = -1 * time.Second
				}
				_ = c.Set(key, map[string]int{"val": j}, ttl)

				_, _ = c.Get(key)
				_, _ = c.GetStale(key)
				_ = c.Len()
				_ = c.Stats()

				if j%4 == 0 {
					_ = c.Delete(key)
				}
				if j%10 == 0 {
					_ = c.Purge()
				}
			}
		}()
	}

	wg.Wait()
}
