package collectors

import (
	"fmt"
	"time"

	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

// NodesCollector recolecta información de nodos canisters.
type NodesCollector struct {
	ttl time.Duration
}

// NewNodesCollector crea un NodesCollector con el TTL especificado.
func NewNodesCollector(ttl time.Duration) *NodesCollector {
	return &NodesCollector{ttl: ttl}
}

// Name implementa Collector.
func (c *NodesCollector) Name() string {
	return "nodes"
}

// TTL implementa Collector.
func (c *NodesCollector) TTL() time.Duration {
	return c.ttl
}

// Collect ejecuta "svcinfo lsnode -delim :" y parsea el resultado.
//
// Formato real de salida (tabular):
//
//	id:name:status:IO_group_id:IO_group_name:config_node:UPS_serial_number:WWNN:...
//	1:node1:online:0:io_grp0:yes::500507680100D4AA:...
//	2:node2:online:0:io_grp0:no::500507680100D4AB:...
//
// Implementa Collector.
func (c *NodesCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	output, err := client.Run("svcinfo lsnode -delim :")
	if err != nil {
		return nil, fmt.Errorf("nodes collector: %w", err)
	}

	result := parser.Parse(output)

	// lsnode puede retornar vacío en sistemas con un solo nodo en modo degradado.
	// No es un error fatal.
	return result.Records, nil
}
