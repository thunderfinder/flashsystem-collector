package collectors

import (
	"fmt"
	"time"

	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

// DrivesCollector recolecta información de drives físicos.
type DrivesCollector struct {
	ttl time.Duration
}

// NewDrivesCollector crea un DrivesCollector con el TTL especificado.
func NewDrivesCollector(ttl time.Duration) *DrivesCollector {
	return &DrivesCollector{ttl: ttl}
}

// Name implementa Collector.
func (c *DrivesCollector) Name() string {
	return "drives"
}

// TTL implementa Collector.
func (c *DrivesCollector) TTL() time.Duration {
	return c.ttl
}

// Collect ejecuta "svcinfo lsdrive -delim :" y parsea el resultado.
//
// Formato real de salida (tabular):
//
//	id:status:error_sequence_number:use:tech_type:capacity:enclosure_id:slot_id:...
//	0:online:0:member:flash:894.3GB:1:1:...
//	1:online:0:member:flash:894.3GB:1:2:...
//
// Implementa Collector.
func (c *DrivesCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	output, err := client.Run("svcinfo lsdrive -delim :")
	if err != nil {
		return nil, fmt.Errorf("drives collector: %w", err)
	}

	result := parser.Parse(output)
	return result.Records, nil
}
