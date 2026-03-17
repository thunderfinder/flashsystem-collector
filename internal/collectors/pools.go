package collectors

import (
	"fmt"
	"time"

	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

// PoolsCollector recolecta información de storage pools (mdiskgrp).
type PoolsCollector struct {
	ttl time.Duration
}

// NewPoolsCollector crea un PoolsCollector con el TTL especificado.
func NewPoolsCollector(ttl time.Duration) *PoolsCollector {
	return &PoolsCollector{ttl: ttl}
}

// Name implementa Collector.
func (c *PoolsCollector) Name() string {
	return "pools"
}

// TTL implementa Collector.
func (c *PoolsCollector) TTL() time.Duration {
	return c.ttl
}

// Collect ejecuta "svcinfo lsmdiskgrp -delim :" y parsea el resultado.
//
// Formato real de salida (tabular):
//
//	id:name:status:mdisk_count:vdisk_count:capacity:extent_size:free_capacity:...
//	0:Pool0:online:4:23:44.0TB:1024:31.7TB:...
//
// Implementa Collector.
func (c *PoolsCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	output, err := client.Run("svcinfo lsmdiskgrp -bytes -delim :")
	if err != nil {
		return nil, fmt.Errorf("pools collector: %w", err)
	}

	result := parser.Parse(output)
	return result.Records, nil
}
