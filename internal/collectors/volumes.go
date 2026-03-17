package collectors

import (
	"fmt"
	"time"

	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

// VolumesCollector recolecta información de volúmenes virtuales (vdisk).
type VolumesCollector struct {
	ttl time.Duration
}

// NewVolumesCollector crea un VolumesCollector con el TTL especificado.
func NewVolumesCollector(ttl time.Duration) *VolumesCollector {
	return &VolumesCollector{ttl: ttl}
}

// Name implementa Collector.
func (c *VolumesCollector) Name() string {
	return "volumes"
}

// TTL implementa Collector.
func (c *VolumesCollector) TTL() time.Duration {
	return c.ttl
}

// Collect ejecuta "svcinfo lsvdisk -delim :" y parsea el resultado.
//
// Formato real de salida (tabular):
//
//	id:name:IO_group_id:IO_group_name:status:mdisk_grp_id:mdisk_grp_name:...
//	0:vol_prod_001:0:io_grp0:online:0:Pool0:...
//	1:vol_prod_002:0:io_grp0:online:0:Pool0:...
//
// Nota: lsvdisk sin flags retorna resumen (no detalles completos).
// Para detalles de un volumen específico: lsvdisk -delim : <id>
//
// Implementa Collector.
func (c *VolumesCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	output, err := client.Run("svcinfo lsvdisk -delim :")
	if err != nil {
		return nil, fmt.Errorf("volumes collector: %w", err)
	}

	result := parser.Parse(output)
	return result.Records, nil
}
