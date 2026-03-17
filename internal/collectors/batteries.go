package collectors

import (
	"fmt"
	"time"

	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

type BatteriesCollector struct {
	ttl time.Duration
}

func NewBatteriesCollector(ttl time.Duration) *BatteriesCollector {
	return &BatteriesCollector{ttl: ttl}
}

func (c *BatteriesCollector) Name() string       { return "batteries" }
func (c *BatteriesCollector) TTL() time.Duration { return c.ttl }

// Collect ejecuta lsenclosurebattery -delim :
// Campos clave: enclosure_id, battery_id, status, percent_charged,
// end_of_life_warning, recondition_needed
// status valores: online, degraded, offline
func (c *BatteriesCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	output, err := client.Run("svcinfo lsenclosurebattery -delim :")
	if err != nil {
		return nil, fmt.Errorf("batteries collector: %w", err)
	}
	result := parser.Parse(output)
	return result.Records, nil
}
