package collectors

import (
	"fmt"
	"time"

	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

// EnclosuresCollector recolecta información de enclosures físicos.
type EnclosuresCollector struct {
	ttl time.Duration
}

// NewEnclosuresCollector crea un EnclosuresCollector con el TTL especificado.
func NewEnclosuresCollector(ttl time.Duration) *EnclosuresCollector {
	return &EnclosuresCollector{ttl: ttl}
}

// Name implementa Collector.
func (c *EnclosuresCollector) Name() string {
	return "enclosures"
}

// TTL implementa Collector.
func (c *EnclosuresCollector) TTL() time.Duration {
	return c.ttl
}

// Collect ejecuta "svcinfo lsenclosure -delim :" y parsea el resultado.
//
// Formato real de salida (tabular):
//
//	id:status:type:managed:IO_group_id:product_MTM:serial_number:total_canisters:...
//	1:online:control:yes:0:9846-AF8:78ABCD1:2:...
//
// Implementa Collector.
func (c *EnclosuresCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	output, err := client.Run("svcinfo lsenclosure -delim :")
	if err != nil {
		return nil, fmt.Errorf("enclosures collector: %w", err)
	}

	result := parser.Parse(output)
	return result.Records, nil
}
