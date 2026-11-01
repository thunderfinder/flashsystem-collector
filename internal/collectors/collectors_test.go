package collectors

import (
	"testing"
	"time"

	"github.com/flashsystem-collector/internal/parser"
)

func TestAllCollectorsRegistration(t *testing.T) {
	metricsTTL := 5 * time.Minute
	discoveryTTL := 4 * time.Hour
	perfTTL := 60 * time.Second

	all := AllCollectors(metricsTTL, discoveryTTL, perfTTL)
	if len(all) != 12 {
		t.Fatalf("expected 12 registered collectors, got %d", len(all))
	}

	names := make(map[string]time.Duration)
	for _, c := range all {
		names[c.Name()] = c.TTL()
	}

	expected := map[string]time.Duration{
		"system":      metricsTTL,
		"nodes":       metricsTTL,
		"enclosures":  discoveryTTL,
		"drives":      discoveryTTL,
		"pools":       metricsTTL,
		"volumes":     metricsTTL,
		"ports":       discoveryTTL,
		"flashcopy":   metricsTTL,
		"replication": metricsTTL,
		"performance": perfTTL,
		"batteries":   discoveryTTL,
		"psus":        discoveryTTL,
	}

	for expName, expTTL := range expected {
		ttl, ok := names[expName]
		if !ok {
			t.Errorf("collector %q was not registered in AllCollectors", expName)
		} else if ttl != expTTL {
			t.Errorf("collector %q expected TTL %v, got %v", expName, expTTL, ttl)
		}
	}
}

func TestDrivesCapacityFormatting(t *testing.T) {
	col := NewDrivesCollector(4 * time.Hour)
	if col.Name() != "drives" {
		t.Errorf("expected name 'drives', got %s", col.Name())
	}
	if col.TTL() != 4*time.Hour {
		t.Errorf("expected TTL 4h, got %v", col.TTL())
	}

	// Verify that Drive capacity formatting correctly processes raw drive lines
	rawOutput := "id:status:error_sequence_number:use:tech_type:capacity:enclosure_id:slot_id\n" +
		"0:online:0:member:flash:960247313203:1:1\n" +
		"1:online:0:member:flash:2089072092774:1:2\n" +
		"2:online:0:member:flash:894.3GB:1:3\n" +
		"3:online:0:member:flash:1.9TB:1:4\n"

	res := parser.Parse(rawOutput)
	for _, rec := range res.Records {
		if capVal, ok := rec["capacity"]; ok && capVal != "" {
			rec["capacity"] = parser.FormatCapacity(capVal)
		}
	}

	expected := []string{"894.3GB", "1.9TB", "894.3GB", "1.9TB"}
	for i, exp := range expected {
		if res.Records[i]["capacity"] != exp {
			t.Errorf("drive record %d: expected capacity %s, got %s", i, exp, res.Records[i]["capacity"])
		}
	}
}
