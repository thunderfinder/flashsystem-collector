package collectors

import (
	"fmt"
	"time"

	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

// PortsCollector recolecta información de puertos Fibre Channel.
type PortsCollector struct {
	ttl time.Duration
}

// NewPortsCollector crea un PortsCollector con el TTL especificado.
func NewPortsCollector(ttl time.Duration) *PortsCollector {
	return &PortsCollector{ttl: ttl}
}

// Name implementa Collector.
func (c *PortsCollector) Name() string {
	return "ports"
}

// TTL implementa Collector.
func (c *PortsCollector) TTL() time.Duration {
	return c.ttl
}

// Collect ejecuta "svcinfo lsportfc -delim :" y parsea el resultado.
//
// Formato real de salida (tabular):
//
//	id:fc_io_port_id:port_id:type:slot_id:port_speed:node_id:node_name:WWPN:...
//	1:1:1:fc:1:16Gb:1:node1:500507680140D4AA:...
//	2:2:2:fc:1:16Gb:1:node1:500507680140D4AB:...
//
// Implementa Collector.
func (c *PortsCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	output, err := client.Run("svcinfo lsportfc -delim :")
	if err != nil {
		return nil, fmt.Errorf("ports collector: %w", err)
	}

	result := parser.Parse(output)
	return result.Records, nil
}
