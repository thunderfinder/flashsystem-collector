package collectors

import (
	"fmt"
	"time"

	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

// FlashCopyCollector recolecta información de FlashCopy mappings.
type FlashCopyCollector struct {
	ttl time.Duration
}

// NewFlashCopyCollector crea un FlashCopyCollector con el TTL especificado.
func NewFlashCopyCollector(ttl time.Duration) *FlashCopyCollector {
	return &FlashCopyCollector{ttl: ttl}
}

// Name implementa Collector.
func (c *FlashCopyCollector) Name() string {
	return "flashcopy"
}

// TTL implementa Collector.
func (c *FlashCopyCollector) TTL() time.Duration {
	return c.ttl
}

// Collect ejecuta "svcinfo lsfcmap -delim :" y parsea el resultado.
//
// Formato real de salida (tabular):
//
//	id:name:source_vdisk_id:source_vdisk_name:target_vdisk_id:target_vdisk_name:...
//	0:fcmap0:0:vol_prod_001:1:vol_snap_001:idle_or_copied:100:0:...
//
// Retorna Records vacío (no error) si no hay FlashCopy mappings configurados.
//
// Implementa Collector.
func (c *FlashCopyCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	output, err := client.Run("svcinfo lsfcmap -delim :")
	if err != nil {
		return nil, fmt.Errorf("flashcopy collector: %w", err)
	}

	result := parser.Parse(output)
	return result.Records, nil
}
