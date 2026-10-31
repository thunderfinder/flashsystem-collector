package cache

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/flashsystem-collector/internal/parser"
)

func TestCacheSetGetPurge(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test.cache.json")
	c := New(cachePath)

	records := []parser.Record{
		{"id": "0", "status": "online"},
	}

	// Set entry with 50ms TTL
	if err := c.Set("host:test", records, 50*time.Millisecond); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Immediate Get should hit
	entry, ok := c.Get("host:test")
	if !ok {
		t.Fatalf("expected cache hit with valid record, got ok=%v", ok)
	}
	var retrieved []parser.Record
	if err := json.Unmarshal(entry.Value, &retrieved); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(retrieved) != 1 || retrieved[0]["status"] != "online" {
		t.Fatalf("unexpected cached value: %v", retrieved)
	}

	// Sleep past expiration
	time.Sleep(70 * time.Millisecond)

	// Normal Get should fail (expired)
	_, ok = c.Get("host:test")
	if ok {
		t.Fatal("expected expired entry to miss Get()")
	}

	// Stale Get should still succeed
	staleEntry, ok := c.GetStale("host:test")
	if !ok {
		t.Fatalf("expected GetStale to return expired entry, got ok=%v", ok)
	}
	if err := json.Unmarshal(staleEntry.Value, &retrieved); err != nil {
		t.Fatalf("unmarshal stale error: %v", err)
	}
	if len(retrieved) != 1 {
		t.Fatalf("expected 1 record from stale cache, got %d", len(retrieved))
	}

	// Purge should remove expired entries
	if err := c.Purge(); err != nil {
		t.Fatalf("Purge failed: %v", err)
	}

	// Stale Get should now miss
	_, ok = c.GetStale("host:test")
	if ok {
		t.Fatal("expected GetStale to fail after Purge()")
	}
}
