package zabbix

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/flashsystem-collector/internal/cache"
	"github.com/flashsystem-collector/internal/collectors"
	"github.com/flashsystem-collector/internal/parser"
)

func TestLLDOutput(t *testing.T) {
	records := []parser.Record{
		{"id": "0", "status": "online", "capacity": "1TB"},
		{"id": "1", "status": "degraded", "capacity": "2TB"},
	}
	mapping := map[string]string{
		"id":     "{#ID}",
		"status": "{#STATUS}",
	}

	data, err := LLDOutput(records, mapping)
	if err != nil {
		t.Fatalf("LLDOutput failed: %v", err)
	}

	var parsed struct {
		Data []map[string]string `json:"data"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal LLD output failed: %v", err)
	}

	if len(parsed.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(parsed.Data))
	}
	if parsed.Data[0]["{#ID}"] != "0" || parsed.Data[0]["{#STATUS}"] != "online" {
		t.Errorf("unexpected item 0: %v", parsed.Data[0])
	}
}

func TestBuildOutput(t *testing.T) {
	cfg := BuilderConfig{
		Host:         "10.0.0.1",
		Version:      "2.0.0",
		MaxJSONBytes: 1048576,
		MaxVolumes:   10,
		MaxDrives:     10,
	}

	results := map[string]collectors.Result{
		"system": {
			Name:       "system",
			Records:    []parser.Record{{"id": "0", "name": "FS01"}},
			DurationMs: 10,
		},
		"batteries": {
			Name:       "batteries",
			Records:    []parser.Record{{"battery_id": "1", "status": "online"}},
			DurationMs: 5,
		},
	}

	stats := cache.Stats{TotalEntries: 2}

	out, err := Build(cfg, results, stats, time.Now())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if !out.Status.Success {
		t.Errorf("expected Status.Success to be true")
	}
	if out.Host != "10.0.0.1" {
		t.Errorf("expected host 10.0.0.1, got %s", out.Host)
	}
	if out.UnitsInfo["temp_c"] != "celsius" {
		t.Errorf("expected UnitsInfo[temp_c] == celsius, got %s", out.UnitsInfo["temp_c"])
	}

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("failed to marshal Output: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty JSON output")
	}
}
