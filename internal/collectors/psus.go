package collectors

import (
	"fmt"
	"time"

	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

type PSUsCollector struct {
	ttl time.Duration
}

func NewPSUsCollector(ttl time.Duration) *PSUsCollector {
	return &PSUsCollector{ttl: ttl}
}

func (c *PSUsCollector) Name() string       { return "psus" }
func (c *PSUsCollector) TTL() time.Duration { return c.ttl }

func (c *PSUsCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	output, err := client.Run("svcinfo lsenclosurepsu -delim :")
	if err != nil {
		return nil, fmt.Errorf("psus collector: %w", err)
	}
	result := parser.Parse(output)
	return result.Records, nil
}
