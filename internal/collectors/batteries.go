package collectors

import (
	"fmt"
	"time"

	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

// BatteriesCollector recolecta el estado de las baterías de los enclosures
// usando "svcinfo lsenclosurebattery -delim :".
type BatteriesCollector struct {
	ttl time.Duration
}

// NewBatteriesCollector crea una nueva instancia de BatteriesCollector.
func NewBatteriesCollector(ttl time.Duration) *BatteriesCollector {
	return &BatteriesCollector{ttl: ttl}
}

// Name retorna el identificador del collector.
func (c *BatteriesCollector) Name() string { return "batteries" }

// TTL retorna el tiempo de vida en cache para este recurso.
func (c *BatteriesCollector) TTL() time.Duration { return c.ttl }

// Collect ejecuta "svcinfo lsenclosurebattery -delim :" vía SSH.
func (c *BatteriesCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	output, err := client.Run("svcinfo lsenclosurebattery -delim :")
	if err != nil {
		return nil, fmt.Errorf("batteries collector: %w", err)
	}
	result := parser.Parse(output)
	return result.Records, nil
}
