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

func TestBuildSectionsNeverNull(t *testing.T) {
	cfg := BuilderConfig{
		Host:         "10.10.10.50",
		Version:      "2.0.0",
		MaxJSONBytes: 1024 * 1024,
		MaxVolumes:   500,
		MaxDrives:    500,
	}

	// Completely empty results map (simulating initial collection error)
	results := map[string]collectors.Result{}
	startTime := time.Now()

	out, err := Build(cfg, results, cache.Stats{}, startTime)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	sections := map[string]json.RawMessage{
		"system":      out.System,
		"nodes":       out.Nodes,
		"enclosures":  out.Enclosures,
		"drives":      out.Drives,
		"pools":       out.Pools,
		"volumes":     out.Volumes,
		"ports":       out.Ports,
		"flashcopy":   out.FlashCopy,
		"replication": out.Replication,
		"performance": out.Performance,
		"batteries":   out.Batteries,
		"psus":        out.PSUs,
	}

	for name, raw := range sections {
		if raw == nil {
			t.Errorf("section %s is nil, expected json []", name)
		} else if string(raw) != "[]" {
			t.Errorf("section %s = %s, expected []", name, string(raw))
		}
	}

	serialized, err := Serialize(out)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	var jsonMap map[string]interface{}
	if err := json.Unmarshal(serialized, &jsonMap); err != nil {
		t.Fatalf("Unmarshal serialized failed: %v", err)
	}

	// Verify contract required for Zabbix Dependent Items JSONPath expressions
	expectedRootKeys := []string{
		"timestamp", "host", "version", "system", "nodes", "enclosures",
		"drives", "pools", "volumes", "ports", "flashcopy", "replication",
		"performance", "status", "cache_stats", "batteries", "psus", "units_info",
	}

	for _, key := range expectedRootKeys {
		val, ok := jsonMap[key]
		if !ok {
			t.Errorf("expected root key %q missing in serialized JSON", key)
		}
		if val == nil {
			t.Errorf("root key %q in serialized JSON is null", key)
		}
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

func TestReduceOutputProgressiveTiers(t *testing.T) {
	// Generate massive record payload for volumes, drives, flashcopy, and replication
	createRecords := func(prefix string, count int) []parser.Record {
		recs := make([]parser.Record, count)
		for i := 0; i < count; i++ {
			recs[i] = parser.Record{
				"id":     fmt.Sprintf("%d", i),
				"name":   fmt.Sprintf("%s_item_record_number_%05d_long_identifier_string", prefix, i),
				"status": "online",
				"info":   "extra_long_payload_field_to_consume_bytes_quickly_in_zabbix_builder_test",
			}
		}
		return recs
	}

	results := map[string]collectors.Result{
		"volumes": {
			Name:    "volumes",
			Records: createRecords("vol", 600),
		},
		"drives": {
			Name:    "drives",
			Records: createRecords("drive", 600),
		},
		"flashcopy": {
			Name:    "flashcopy",
			Records: createRecords("fc", 100),
		},
		"replication": {
			Name:    "replication",
			Records: createRecords("repl", 100),
		},
	}

	t.Run("ExtremeConstraintClearsAllTiers", func(t *testing.T) {
		cfg := BuilderConfig{
			Host:         "10.10.10.50",
			Version:      "2.0.0",
			MaxJSONBytes: 100, // Impossibly small to trigger ALL reduction phases down to total clearing
			MaxVolumes:   500,
			MaxDrives:    500,
		}

		out, err := Build(cfg, results, cache.Stats{}, time.Now())
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		// When even the most aggressive tier (50 volumes, 50 drives, no FC, no Repl) exceeds 100 bytes,
		// Volumes and Drives must be completely emptied to "[]".
		if string(out.Volumes) != "[]" {
			t.Errorf("expected volumes to be cleared to [], got: %s", string(out.Volumes))
		}
		if string(out.Drives) != "[]" {
			t.Errorf("expected drives to be cleared to [], got: %s", string(out.Drives))
		}
		if string(out.FlashCopy) != "[]" {
			t.Errorf("expected flashcopy to be cleared to [], got: %s", string(out.FlashCopy))
		}
		if string(out.Replication) != "[]" {
			t.Errorf("expected replication to be cleared to [], got: %s", string(out.Replication))
		}
	})

	t.Run("ModerateConstraintPreservesLimitedRecords", func(t *testing.T) {
		cfg := BuilderConfig{
			Host:         "10.10.10.50",
			Version:      "2.0.0",
			MaxJSONBytes: 15 * 1024, // 15KB budget triggers intermediate reduction
			MaxVolumes:   500,
			MaxDrives:    500,
		}

		out, err := Build(cfg, results, cache.Stats{}, time.Now())
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		serialized, err := Serialize(out)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		if len(serialized) > cfg.MaxJSONBytes {
			t.Errorf("serialized length %d exceeded max budget %d", len(serialized), cfg.MaxJSONBytes)
		}
	})
}

func TestReTruncateSectionEdgeCases(t *testing.T) {
	// 1. Nil section
	if err := reTruncateSection(nil, 10); err != nil {
		t.Errorf("expected nil error for nil section, got %v", err)
	}

	// 2. Empty section string "[]"
	emptySec := json.RawMessage("[]")
	if err := reTruncateSection(&emptySec, 10); err != nil {
		t.Errorf("expected nil error for empty section, got %v", err)
	}
	if string(emptySec) != "[]" {
		t.Errorf("expected '[]', got %s", string(emptySec))
	}

	// 3. maxRecords <= 0
	populatedSec := json.RawMessage(`[{"id":"1"}]`)
	if err := reTruncateSection(&populatedSec, 0); err != nil {
		t.Errorf("expected nil error for maxRecords <= 0, got %v", err)
	}
	if string(populatedSec) != `[{"id":"1"}]` {
		t.Errorf("expected untouched section for maxRecords 0, got %s", string(populatedSec))
	}

	// 4. Invalid JSON resets to "[]" without failing
	invalidSec := json.RawMessage(`{not_valid_json`)
	if err := reTruncateSection(&invalidSec, 5); err != nil {
		t.Errorf("expected nil error for unparseable section, got %v", err)
	}
	if string(invalidSec) != "[]" {
		t.Errorf("expected invalid section to reset to [], got %s", string(invalidSec))
	}

	// 5. Valid truncation
	validSec := json.RawMessage(`[{"id":"1"},{"id":"2"},{"id":"3"}]`)
	if err := reTruncateSection(&validSec, 2); err != nil {
		t.Fatalf("reTruncateSection failed: %v", err)
	}
	var res []parser.Record
	if err := json.Unmarshal(validSec, &res); err != nil {
		t.Fatalf("Unmarshal truncated section failed: %v", err)
	}
	if len(res) != 2 || res[0]["id"] != "1" || res[1]["id"] != "2" {
		t.Errorf("unexpected truncated records: %+v", res)
	}
}

func TestLLDOutputAllCategories(t *testing.T) {
	testCases := []struct {
		name          string
		fieldMap      map[string]string
		records       []parser.Record
		expectedMacro string
		expectedVal   string
	}{
		{
			name:     "drives",
			fieldMap: DriveLLDFields,
			records: []parser.Record{
				{"id": "0", "enclosure_id": "1", "slot_id": "1", "status": "online", "tech_type": "flash", "capacity": "1.9TB"},
			},
			expectedMacro: "{#DRIVEID}",
			expectedVal:   "0",
		},
		{
			name:     "pools",
			fieldMap: PoolLLDFields,
			records: []parser.Record{
				{"id": "0", "name": "pool_tier0", "status": "online"},
			},
			expectedMacro: "{#POOLNAME}",
			expectedVal:   "pool_tier0",
		},
		{
			name:     "volumes",
			fieldMap: VolumeLLDFields,
			records: []parser.Record{
				{"id": "10", "name": "vol_web_prod", "status": "online", "mdisk_grp_name": "pool_tier0"},
			},
			expectedMacro: "{#VOLUMEPOOL}",
			expectedVal:   "pool_tier0",
		},
		{
			name:     "enclosures",
			fieldMap: EnclosureLLDFields,
			records: []parser.Record{
				{"id": "1", "status": "online", "product_MTM": "2076-24F"},
			},
			expectedMacro: "{#ENCLOSUREMODEL}",
			expectedVal:   "2076-24F",
		},
		{
			name:     "nodes",
			fieldMap: NodeLLDFields,
			records: []parser.Record{
				{"id": "1", "name": "node_canister_1", "status": "online", "IO_group_name": "io_grp0"},
			},
			expectedMacro: "{#NODEIOGROUP}",
			expectedVal:   "io_grp0",
		},
		{
			name:     "ports",
			fieldMap: PortLLDFields,
			records: []parser.Record{
				{"id": "1", "WWPN": "500507680100ABCD", "status": "active", "node_name": "node_canister_1", "port_speed": "16Gb"},
			},
			expectedMacro: "{#WWPN}",
			expectedVal:   "500507680100ABCD",
		},
		{
			name:     "batteries",
			fieldMap: BatteryLLDFields,
			records: []parser.Record{
				{
					"enclosure_id":        "1",
					"battery_id":          "1",
					"status":              "online",
					"percent_charged":     "99",
					"end_of_life_warning": "no",
					"recondition_needed":  "no",
				},
			},
			expectedMacro: "{#BATTERYCHARGE}",
			expectedVal:   "99",
		},
		{
			name:     "psus",
			fieldMap: PSULLDFields,
			records: []parser.Record{
				{
					"enclosure_id":  "1",
					"psu_id":        "2",
					"status":        "online",
					"input_failed":  "no",
					"output_failed": "no",
					"fan_failed":    "no",
				},
			},
			expectedMacro: "{#PSUID}",
			expectedVal:   "2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := LLDOutput(tc.records, tc.fieldMap)
			if err != nil {
				t.Fatalf("LLDOutput failed for %s: %v", tc.name, err)
			}

			type lldData struct {
				Data []map[string]string `json:"data"`
			}
			var parsed lldData
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("Unmarshal LLD failed for %s: %v", tc.name, err)
			}

			if len(parsed.Data) != len(tc.records) {
				t.Fatalf("expected %d LLD items for %s, got %d", len(tc.records), tc.name, len(parsed.Data))
			}

			item := parsed.Data[0]
			val, ok := item[tc.expectedMacro]
			if !ok {
				t.Fatalf("expected macro %s in LLD item: %+v", tc.expectedMacro, item)
			}
			if val != tc.expectedVal {
				t.Errorf("macro %s = %q, expected %q", tc.expectedMacro, val, tc.expectedVal)
			}

			// Verify all fields in fieldMap exist in LLD item
			for _, macro := range tc.fieldMap {
				if _, ok := item[macro]; !ok {
					t.Errorf("macro %s missing from LLD item: %+v", macro, item)
				}
			}
		})
	}
}

func TestLLDOutputMissingFieldsDefaultToEmpty(t *testing.T) {
	// Drive record missing capacity and slot_id
	records := []parser.Record{
		{
			"id":     "0",
			"status": "online",
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

	if len(parsed.Data) != 1 {
		t.Fatalf("expected 1 item, got %d", len(parsed.Data))
	}

	item := parsed.Data[0]
	if item["{#DRIVEID}"] != "0" {
		t.Errorf("expected {#DRIVEID} '0', got %q", item["{#DRIVEID}"])
	}
	if item["{#SLOTID}"] != "" {
		t.Errorf("expected missing {#SLOTID} to default to empty string '', got %q", item["{#SLOTID}"])
	}
	if item["{#DRIVECAPACITY}"] != "" {
		t.Errorf("expected missing {#DRIVECAPACITY} to default to empty string '', got %q", item["{#DRIVECAPACITY}"])
	}
}
