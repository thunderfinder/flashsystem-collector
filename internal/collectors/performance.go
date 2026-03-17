package collectors

import (
	"fmt"
	"time"

	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

// PerformanceCollector recolecta estadísticas de rendimiento del sistema.
type PerformanceCollector struct {
	ttl time.Duration
}

// NewPerformanceCollector crea un PerformanceCollector con el TTL especificado.
func NewPerformanceCollector(ttl time.Duration) *PerformanceCollector {
	return &PerformanceCollector{ttl: ttl}
}

// Name implementa Collector.
func (c *PerformanceCollector) Name() string {
	return "performance"
}

// TTL implementa Collector.
func (c *PerformanceCollector) TTL() time.Duration {
	return c.ttl
}

// Collect ejecuta "svcinfo lssystemstats -delim :" y parsea el resultado.
//
// Formato real de salida (tabular — una fila por stat):
//
//	stat_name:stat_current:stat_peak:stat_peak_time
//	compression_cpu_pc:0:0:231017191500
//	cpu_pc:3:8:231017161500
//	fc_mb:42:156:231017155500
//	fc_io:1823:4821:231017155500
//	sas_mb:0:0:231017191500
//	sas_io:0:0:231017191500
//	iscsi_mb:0:0:231017191500
//	iscsi_io:0:0:231017191500
//	read_mb:21:98:231017155500
//	write_mb:21:58:231017155500
//	read_io:912:2634:231017155500
//	write_io:911:2187:231017155500
//	read_ms:0:2:231017155500
//	write_ms:0:1:231017155500
//	drive_r_mb:21:98:231017155500
//	drive_w_mb:21:58:231017155500
//	drive_r_io:912:2634:231017155500
//	drive_w_io:911:2187:231017155500
//	drive_ms:0:2:231017155500
//	vdisk_mb:42:156:231017155500
//	vdisk_io:1823:4821:231017155500
//	vdisk_ms:0:1:231017155500
//	mdisk_mb:42:156:231017155500
//	mdisk_io:1823:4821:231017155500
//	mdisk_ms:0:2:231017155500
//
// Implementa Collector.
func (c *PerformanceCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	output, err := client.Run("svcinfo lssystemstats -delim :")
	if err != nil {
		return nil, fmt.Errorf("performance collector: %w", err)
	}

	result := parser.Parse(output)

	if result.Format == parser.FormatEmpty {
		return nil, fmt.Errorf("performance collector: empty response from lssystemstats (statistics may be disabled — run 'svctask startstats')")
	}

	return result.Records, nil
}
