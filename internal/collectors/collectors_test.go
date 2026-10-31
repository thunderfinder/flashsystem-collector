package collectors

import (
	"testing"
	"time"
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
