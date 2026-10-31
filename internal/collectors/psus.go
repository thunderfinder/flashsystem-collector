package collectors

import (
	"fmt"
	"time"

	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

// PSUsCollector recolecta el estado de las fuentes de poder (PSUs) de los enclosures
// usando "svcinfo lsenclosurepsu -delim :".
type PSUsCollector struct {
	ttl time.Duration
}

// NewPSUsCollector crea una nueva instancia de PSUsCollector.
func NewPSUsCollector(ttl time.Duration) *PSUsCollector {
	return &PSUsCollector{ttl: ttl}
}

// Name retorna el identificador del collector.
func (c *PSUsCollector) Name() string { return "psus" }

// TTL retorna el tiempo de vida en cache para este recurso.
func (c *PSUsCollector) TTL() time.Duration { return c.ttl }

// Collect ejecuta "svcinfo lsenclosurepsu -delim :" vía SSH.
func (c *PSUsCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	output, err := client.Run("svcinfo lsenclosurepsu -delim :")
	if err != nil {
		return nil, fmt.Errorf("psus collector: %w", err)
	}
	result := parser.Parse(output)
	return result.Records, nil
}
