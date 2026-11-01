package zabbix

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/flashsystem-collector/internal/cache"
	"github.com/flashsystem-collector/internal/collectors"
	"github.com/flashsystem-collector/internal/parser"
)

func TestBuildOutput(t *testing.T) {
	cfg := BuilderConfig{
		Host:         "10.10.10.50",
		Version:      "2.0.0",
		MaxJSONBytes: 1024 * 1024,
		MaxVolumes:   500,
		MaxDrives:    500,
	}

	results := map[string]collectors.Result{
		"system": {
			Name: "system",
			Records: []parser.Record{
				{"id": "0", "name": "FS_Cluster", "status": "online"},
			},
			CacheHit: false,
		},
		"pools": {
			Name: "pools",
			Records: []parser.Record{
				{"id": "0", "name": "Pool0", "status": "online"},
			},
			CacheHit: true,
		},
		"volumes": {
			Name:  "volumes",
			Error: "SSH connection timed out",
		},
	}

	cacheStats := cache.Stats{
		ActiveEntries: 2,
		FilePath:      "/var/tmp/test.cache.json",
	}

	startTime := time.Now().Add(-100 * time.Millisecond)

	out, err := Build(cfg, results, cacheStats, startTime)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if out.Host != "10.10.10.50" {
		t.Errorf("expected host 10.10.10.50, got %s", out.Host)
	}
	// Success should remain true if at least one collector succeeded
	if !out.Status.Success {
		t.Errorf("expected status.success true for partial success, got false")
	}
	if len(out.Status.Errors) == 0 {
		t.Errorf("expected status.errors to contain volume failure")
	}

	// Verify serialization
	bytes, err := Serialize(out)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	if !strings.Contains(string(bytes), `"FS_Cluster"`) {
		t.Errorf("expected serialized output to contain cluster name")
	}
}

func TestBuildAllFailed(t *testing.T) {
	cfg := BuilderConfig{
		Host:         "10.10.10.50",
		Version:      "2.0.0",
		MaxJSONBytes: 1024 * 1024,
		MaxVolumes:   500,
		MaxDrives:    500,
	}

	results := map[string]collectors.Result{
		"system": {
			Name:  "system",
			Error: "connection refused",
		},
		"pools": {
			Name:  "pools",
			Error: "connection refused",
		},
	}

	out, err := Build(cfg, results, cache.Stats{}, time.Now())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if out.Status.Success {
		t.Errorf("expected status.success false when all collectors fail, got true")
	}
}

func TestReduceOutput(t *testing.T) {
	// Create thousands of volume records to exceed small byte budget
	largeRecords := make([]parser.Record, 500)
	for i := 0; i < 500; i++ {
		largeRecords[i] = parser.Record{
			"id":             fmt.Sprintf("%d", i),
			"name":           fmt.Sprintf("volume_%04d_production_database_system_storage", i),
			"mdisk_grp_name": "production_high_performance_tier_storage_pool",
			"status":         "online",
			"capacity":       "1048576000000",
		}
	}

	cfg := BuilderConfig{
		Host:         "10.10.10.50",
		Version:      "2.0.0",
		MaxJSONBytes: 10 * 1024, // Very strict 10KB limit to trigger reduction
		MaxVolumes:   500,
		MaxDrives:    500,
	}

	results := map[string]collectors.Result{
		"volumes": {
			Name:    "volumes",
			Records: largeRecords,
		},
	}

	out, err := Build(cfg, results, cache.Stats{}, time.Now())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	serialized, err := Serialize(out)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	if len(serialized) > 15*1024 {
		t.Errorf("expected reduced output to respect size budget, got %d bytes", len(serialized))
	}
}

func TestLLDOutput(t *testing.T) {
	records := []parser.Record{
		{
			"id":           "0",
			"enclosure_id": "1",
			"slot_id":      "1",
			"status":       "online",
			"tech_type":    "flash",
			"capacity":     "1.9TB",
		},
		{
			"id":           "1",
			"enclosure_id": "1",
			"slot_id":      "2",
			"status":       "offline",
			"tech_type":    "flash",
			"capacity":     "1.9TB",
		},
	}

	data, err := LLDOutput(records, DriveLLDFields)
	if err != nil {
		t.Fatalf("LLDOutput failed: %v", err)
	}

	type lldData struct {
		Data []map[string]string `json:"data"`
	}
	var parsed lldData
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal LLD failed: %v", err)
	}

	if len(parsed.Data) != 2 {
		t.Fatalf("expected 2 LLD items, got %d", len(parsed.Data))
	}

	item0 := parsed.Data[0]
	if item0["{#DRIVEID}"] != "0" {
		t.Errorf("expected {#DRIVEID} 0, got %s", item0["{#DRIVEID}"])
	}
	if item0["{#DRIVESTATUS}"] != "online" {
		t.Errorf("expected {#DRIVESTATUS} online, got %s", item0["{#DRIVESTATUS}"])
	}
	if item0["{#DRIVECAPACITY}"] != "1.9TB" {
		t.Errorf("expected {#DRIVECAPACITY} 1.9TB, got %s", item0["{#DRIVECAPACITY}"])
	}
}
