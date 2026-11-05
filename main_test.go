package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flashsystem-collector/internal/cache"
	"github.com/flashsystem-collector/internal/config"
	"github.com/flashsystem-collector/internal/parser"
)

func TestEscapeJSONString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`hello world`, `hello world`},
		{`hello "world"`, `hello \"world\"`},
		{`line1` + "\n" + `line2`, `line1\nline2`},
		{``, ``},
	}

	for _, tt := range tests {
		got := escapeJSONString(tt.input)
		if got != tt.expected {
			t.Errorf("escapeJSONString(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestRecordsToLLDItems(t *testing.T) {
	records := []parser.Record{
		{
			"id":     "0",
			"name":   "Pool0",
			"status": "online",
		},
		{
			"id":   "1",
			"name": "Pool1",
		},
	}

	fieldMap := map[string]string{
		"id":     "{#POOLID}",
		"name":   "{#POOLNAME}",
		"status": "{#POOLSTATUS}",
	}

	items := recordsToLLDItems(records, fieldMap)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0]["{#POOLID}"] != "0" || items[0]["{#POOLNAME}"] != "Pool0" || items[0]["{#POOLSTATUS}"] != "online" {
		t.Errorf("unexpected item 0: %+v", items[0])
	}

	if items[1]["{#POOLID}"] != "1" || items[1]["{#POOLNAME}"] != "Pool1" || items[1]["{#POOLSTATUS}"] != "" {
		t.Errorf("unexpected item 1: %+v", items[1])
	}
}

func TestEmitEmptyLLD(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	emitEmptyLLD()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := strings.TrimSpace(buf.String())

	if output != `{"data":[]}` {
		t.Errorf("expected {\"data\":[]}, got %q", output)
	}
}

func TestEmitLLDFromCache(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	items := []map[string]string{
		{"{#DRIVEID}": "0", "{#DRIVESTATUS}": "online"},
	}
	emitLLDFromCache(items)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, `"{#DRIVEID}": "0"`) || !strings.Contains(output, `"data"`) {
		t.Errorf("expected JSON containing {#DRIVEID}, got %q", output)
	}
}

func TestBuildFromStaleCache(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test.cache.json")
	c := cache.New(cachePath)

	host := "192.168.1.100"
	cfg := &config.Config{
		SSH: config.SSHConfig{
			Host: host,
		},
		Limits: config.LimitsConfig{
			MaxJSONBytes: 1048576,
			MaxVolumes:   500,
			MaxDrives:    500,
		},
	}

	// Populate some stale entries
	key := cache.MakeKey(host, "pools")
	records := []parser.Record{
		{"id": "0", "name": "Pool0", "status": "online"},
	}
	// Expire immediately
	if err := c.Set(key, records, -1*time.Second); err != nil {
		t.Fatalf("failed setting cache: %v", err)
	}

	startTime := time.Now()
	sshErr := errors.New("connection timeout")

	out := buildFromStaleCache(c, cfg, startTime, sshErr)
	if out == nil {
		t.Fatal("expected non-nil output")
	}

	if out.Host != host {
		t.Errorf("expected host %q, got %q", host, out.Host)
	}

	if string(out.Pools) == "[]" {
		t.Errorf("expected pools to be populated from stale cache, got %s", string(out.Pools))
	}

}

func TestBuildFromStaleCacheRejectsTooOld(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "old.cache.json")
	c := cache.New(cachePath)

	host := "192.168.1.100"
	cfg := &config.Config{
		SSH: config.SSHConfig{
			Host: host,
		},
		Limits: config.LimitsConfig{
			MaxJSONBytes: 1048576,
			MaxVolumes:   500,
			MaxDrives:    500,
		},
	}

	key := cache.MakeKey(host, "pools")
	// Save entry that was cached 25 hours ago (beyond maxStaleAge = 24h)
	oldTime := time.Now().Add(-25 * time.Hour)
	entry := cache.Entry{
		Key:       key,
		Value:     []byte(`[{"id":"0","name":"Pool0","status":"online"}]`),
		CachedAt:  oldTime,
		ExpiresAt: oldTime.Add(5 * time.Minute),
	}

	// Manually save this entry via internal save file test
	cacheData := struct {
		Version   string                 `json:"version"`
		UpdatedAt time.Time              `json:"updated_at"`
		Entries   map[string]cache.Entry `json:"entries"`
	}{
		Version:   "1.0",
		UpdatedAt: oldTime,
		Entries:   map[string]cache.Entry{key: entry},
	}
	data, _ := json.Marshal(cacheData)
	if err := os.WriteFile(cachePath, data, 0600); err != nil {
		t.Fatalf("failed to write old cache file: %v", err)
	}

	// Re-load cache from disk
	c = cache.New(cachePath)
	startTime := time.Now()
	sshErr := errors.New("storage unreachable")

	out := buildFromStaleCache(c, cfg, startTime, sshErr)
	if out == nil {
		t.Fatal("expected non-nil output")
	}

	// Pools should NOT be populated because the entry was cached > 24 hours ago
	if string(out.Pools) != "[]" {
		t.Errorf("expected pools to be empty [] due to stale expiration, got: %s", string(out.Pools))
	}
}

func TestRunDiscoveryCollectorInvalidCategory(t *testing.T) {
	cfg := &config.Config{}
	_, _, err := runDiscoveryCollector(nil, "nonexistent_cat", cfg)
	if err == nil {
		t.Fatal("expected error for invalid category, got nil")
	}
	if !strings.Contains(err.Error(), "unknown discovery category") {
		t.Errorf("unexpected error message: %v", err)
	}
}
