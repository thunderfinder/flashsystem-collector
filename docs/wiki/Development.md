# Development 💻

This guide provides guidelines for Go engineers extending or modifying `flashsystem-collector`.

---

## 1. Development Environment & Toolchain

- **Go Version**: **Go 1.25.0+**
- **Architecture**: Linux AMD64 / ARM64
- **Required Build Tags**: Static linkage (`CGO_ENABLED=0`)

### Building the Project Locally
```bash
# Clone the repository
git clone https://github.com/thunderfinder/flashsystem-collector.git
cd flashsystem-collector

# Compile static binary
CGO_ENABLED=0 go build -trimpath -o flashsystem-collector .
```

---

## 2. Repository Layout

```text
flashsystem-collector/
├── main.go                     # Entrypoint & CLI dispatching (collect/discover)
├── internal/
│   ├── cache/                  # Atomic file caching with TTLs and stale recovery
│   ├── collectors/             # 12 Domain collectors & concurrent Runner
│   ├── config/                 # Flag & environment variable evaluation
│   ├── parser/                 # Delimited svcinfo parser engine
│   ├── ssh/                    # Crypto SSH client with timeouts & known_hosts
│   └── zabbix/                 # JSON assembling & LLD field maps
├── scripts/
│   └── flashsystem_collector.sh# Bash wrapper with flock
├── template/
│   └── IBM FlashSystem Collector.yaml # Zabbix 6.4+ YAML template
├── docs/wiki/                  # Complete technical Wiki documentation
└── go.mod                      # Module definition (Go 1.25.0)
```

---

## 3. Adding a New Collector

To add monitoring for an additional storage CLI command (e.g., `svcinfo lshost`):

### Step 1: Implement the `Collector` Interface
Create `internal/collectors/hosts.go`:

```go
package collectors

import (
	"time"
	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

type HostsCollector struct {
	ttl time.Duration
}

func NewHostsCollector(ttl time.Duration) *HostsCollector {
	return &HostsCollector{ttl: ttl}
}

func (c *HostsCollector) Name() string {
	return "hosts"
}

func (c *HostsCollector) TTL() time.Duration {
	return c.ttl
}

func (c *HostsCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	output, err := client.Run("svcinfo lshost -delim :")
	if err != nil {
		return nil, err
	}
	return parser.Parse(output)
}
```

### Step 2: Register in `AllCollectors`
In `internal/collectors/collector.go`:
```go
func AllCollectors(metricsTTL, discoveryTTL, performanceTTL time.Duration) []Collector {
	return []Collector{
		// ... existing collectors ...
		NewHostsCollector(metricsTTL),
	}
}
```

### Step 3: Add to `internal/zabbix/output.go`
Expose the new section on the `Output` struct:
```go
type Output struct {
    // ...
    Hosts json.RawMessage `json:"hosts"`
}
```
And add `out.Hosts = marshalSection(results, "hosts", &sectionErrors)` in `Build()`.

---

## 4. Coding Conventions

- **Concurrency**: Goroutines must be coordinated via `sync.WaitGroup` and throttled via semaphores. Never spawn unbounded goroutines.
- **Fail-Safe JSON**: Collectors must never panic or write unformatted strings to stdout. All errors must be wrapped in `fatalJSON` or returned in the `status.errors` array.
- **Parser Robustness**: The parser must handle empty strings, single key-value vertical records, and multi-line matrices without throwing index out-of-range errors.
