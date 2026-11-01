# Testing and Quality 🧪

This document describes the testing practices, race detection requirements, and automated CI/CD validation pipelines enforced in `flashsystem-collector`.

---

## 1. Running the Automated Test Suite

The repository includes comprehensive unit and integration tests covering configuration loading, cache atomicity, parser normalization, and Zabbix output formatting.

### Executing Full Tests with Race Detection
Execute tests across all packages with race detection enabled:
```bash
go test -race ./...
```

Expected output:
```text
ok  	github.com/flashsystem-collector/internal/cache	1.141s
ok  	github.com/flashsystem-collector/internal/collectors	1.022s
ok  	github.com/flashsystem-collector/internal/config	1.037s
ok  	github.com/flashsystem-collector/internal/parser	1.020s
ok  	github.com/flashsystem-collector/internal/zabbix	1.099s
```

### Running Targeted Package Tests
```bash
# Test parser behavior with edge cases
go test -v ./internal/parser

# Test atomic cache writes and TTL expiration
go test -v ./internal/cache

# Test configuration validation and permission checking
go test -v ./internal/config
```

---

## 2. Testing Methodologies

### 1. Parser Testing (`internal/parser/parser_test.go`)
- Asserts that vertical key-value lists (e.g. `lssystem`) are correctly transformed into single record maps.
- Asserts that tabular multi-row data (e.g. `lsmdiskgrp`, `lsdrive`) are split into column headers and row maps.
- Asserts that corrupted data, blank lines, and malformed strings do not trigger panics.

### 2. Cache Testing (`internal/cache/cache_test.go`)
- Verifies that `Set()` writes valid JSON to disk.
- Verifies that `Get()` returns records within TTL and reports misses after TTL expiration.
- Validates atomic rename guarantees under concurrent access.
- Tests `GetStale()` to verify expired records can be retrieved during simulated SSH outages.

### 3. Config Validation Testing (`internal/config/config_test.go`)
- Verifies flag overriding of environment variables.
- Asserts that missing authentication yields an error.
- Asserts that insecure file permissions (`0644`) on private key files trigger an immediate security error on non-Windows systems.

---

## 3. Continuous Integration Workflows

The repository uses GitHub Actions workflows under `.github/workflows/`:

1. **`go.yml`**:
   - Compiles using Go 1.25.
   - Runs `go vet ./...`.
   - Executes `go test -race ./...`.
   - Validates static compilation (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build`).
2. **`lint.yml`**:
   - Lints workflow definitions with `actionlint` and `zizmor`.
   - Evaluates deadcode and Go static analysis.
