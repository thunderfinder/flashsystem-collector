# AGENTS.md — Repository Instructions for AI Coding Agents

This is the authoritative instruction guide for AI agents working in `thunderfinder/flashsystem-collector`. Every agent must read and follow the instructions in this file and any relevant nested `AGENTS.md` files before making modifications to the repository.

---

## A. Project Overview

`flashsystem-collector` is a Go application and Zabbix external script wrapper designed to collect hardware, status, capacity, and performance metrics from **IBM FlashSystem** and **Spectrum Virtualize** storage systems via SSH.

### Key Capabilities
- **CLI Command Execution**: Executes native IBM storage CLI commands (`svcinfo`) over SSH using a single management connection.
- **Dual Execution Modes**:
  - `collect`: Runs all collectors in parallel and produces a single structured JSON payload intended for a Zabbix **Master Item**.
  - `discover`: Runs a targeted collector for a specific category (`drives`, `pools`, `volumes`, `enclosures`, `nodes`, `ports`) and emits Zabbix **Low-Level Discovery (LLD)** JSON with `{#MACRO}` key mappings.
- **Persistent JSON Caching**: Thread-safe, disk-backed atomic cache per host to prevent SSH connection flooding. Supports stale-cache fallback if SSH connection fails.
- **Output Safety & Truncation**: Guarantees valid JSON output on stdout (never empty output) even on fatal configuration errors, with size and record count limits (`MaxJSONBytes`, `MaxVolumes`, `MaxDrives`).

### Key Technologies
- **Go**: Version `1.25.0` (specified in `go.mod`).
- **Dependencies**: `golang.org/x/crypto` for SSH client capabilities.
- **Integration Target**: Zabbix Server / Proxy (6.0+ and 7.x) via External Scripts and JSONPath preprocessing.

---

## B. Mandatory Reading Before Making Changes

Before making any changes in this repository, coding agents **must**:
1. Read this `/AGENTS.md` file in its entirety.
2. Read any applicable nested `AGENTS.md` files in the directories where files will be edited (e.g., `internal/config/AGENTS.md`, `internal/ssh/AGENTS.md`).
3. Read `README.md` and `instructions.md` for background operational context.
4. Read `go.mod` to confirm module name and dependency constraints.
5. Inspect source code and existing tests in the target package before editing.
6. Verify understanding of affected consumers and data contracts.

---

## C. Architecture and Execution Flow

### Directory and Package Structure
```
.
├── main.go                             # Application entry point, subcommand routing, stale cache fallback
├── go.mod                              # Module definition (github.com/flashsystem-collector)
├── internal/
│   ├── config/config.go                # Config loading (flags > env > defaults) & validation
│   ├── ssh/client.go                   # SSH connection management, dial/command timeouts, host-key handling
│   ├── cache/cache.go                  # Disk-backed atomic JSON cache, thread-safety, TTL, stale fallback
│   ├── parser/parser.go                # svcinfo -delim : parser (dual vertical key-value and tabular matrix)
│   ├── collectors/                     # Collector interface, runner (max 3 concurrent), individual resource collectors
│   │   ├── collector.go                # Interface Collector, Runner struct, AllCollectors factory
│   │   ├── system.go                   # lssystem
│   │   ├── nodes.go                    # lsnode
│   │   ├── enclosures.go               # lsenclosure
│   │   ├── drives.go                   # lsdrive
│   │   ├── pools.go                    # lsmdiskgrp
│   │   ├── volumes.go                  # lsvdisk
│   │   ├── ports.go                    # lsportfc
│   │   ├── flashcopy.go                # lsfcmap
│   │   ├── replication.go              # lsreplicationrelationship (fallback: lsrcrelationship)
│   │   └── performance.go              # lssystemstats
│   └── zabbix/output.go                # Master Item JSON output builder, LLD output builder, record truncation
├── scripts/
│   └── flashsystem_collector.sh        # Zabbix wrapper script (flock, env file loading, logging)
└── template/
    └── IBM FlashSystem Collector.yaml # Zabbix template definition
```

### Main Operational Workflows

#### 1. `collect` Workflow
1. `main.go` routes to `runCollect()`.
2. `config.Load()` parses CLI flags and environment variables (`FS_*`) and validates parameters.
3. `cache.New()` initializes disk cache file (`<cache-dir>/flashsystem_<sanitized_host>.cache.json`). `c.Purge()` clears expired entries.
4. `dialSSH()` establishes TCP and SSH handshake with the FlashSystem.
   - **If SSH fails**: Calls `buildFromStaleCache()`, populating results from stale cache if available, and emits output JSON gracefully.
5. `collectors.AllCollectors()` creates instances for all 10 collectors.
6. `collectors.NewRunner()` sets up a concurrent runner with max concurrency = 3.
7. `runner.RunAll(client)` executes collectors in parallel:
   - For each collector: checks cache hit. If cache hit, returns cached records.
   - On cache miss: acquires semaphore slot, executes `col.Collect(client)` over SSH, releases slot, updates cache.
8. `zabbix.Build()` constructs final `Output` struct, enforces `MaxVolumes`, `MaxDrives`, and `MaxJSONBytes` constraints.
9. `emitJSON()` outputs formatted JSON to `stdout`.

#### 2. `discover` Workflow
1. `main.go` routes to `runDiscover()`.
2. `config.Load()` parses configuration and `-category` flag (`drives|pools|volumes|enclosures|nodes|ports`).
3. Checks discovery cache key (`<host>:discovery:<category>`).
   - If cache hit: `emitLLDFromCache()` outputs `{"data": [...]}` JSON immediately.
4. If cache miss: establishes SSH connection via `dialSSH()`.
   - On SSH failure: calls `emitEmptyLLD()` (outputs `{"data":[]}`) and logs error to stderr.
5. `runDiscoveryCollector()` runs the target collector, maps record fields to Zabbix macros (`{#DRIVEID}`, etc.), caches the result with `DiscoveryTTL` (default 4h), and prints LLD JSON to `stdout`.

---

## D. Technology and Development Environment

### Software Requirements
- **Go Compiler**: Go `1.25.0` (as required by `go.mod`). Note: documentation references Go 1.21+, but `go.mod` specifies `1.25.0`.
- **Target OS**: Linux x86_64 (`GOOS=linux GOARCH=amd64`).
- **Dependencies**: Listed in `go.mod` (`golang.org/x/crypto v0.49.0`).

### Build & Test Commands
```bash
# Download dependencies
go mod tidy

# Standard build
go build -o flashsystem-collector .

# Static cross-compilation for Linux AMD64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o flashsystem-collector .

# Run tests
go test ./...

# Run tests with race detection
go test -race ./...

# Lint check (standard vet)
go vet ./...
```

---

## E. Engineering Principles

1. **Idiomatic Go**: Follow standard Go formatting (`gofmt`) and naming conventions.
2. **Explicit Error Handling**: Never ignore returned errors. Handle or wrap them with context (`fmt.Errorf("...: %w", err)`).
3. **No Hidden Global State**: Pass configuration, logger, cache, and SSH client instances explicitly down call trees.
4. **Preserve Compatibility Contracts**:
   - Do not alter CLI flag names or environment variable names (`FS_*`).
   - Do not change output JSON top-level keys or types in `zabbix.Output`.
   - Do not rename Zabbix LLD macro keys (`{#DRIVEID}`, `{#POOLNAME}`, etc.).
5. **Small, Focused Changes**: Keep changes minimal, clear, and well-tested. Avoid unrelated refactoring.
6. **Graceful Degradation**: Always emit valid JSON on `stdout`. Diagnostics and errors belong in `status.errors` or on `stderr`.

---

## F. Testing Requirements

1. **Unit Tests Required**:
   - Any new or modified package logic must be accompanied by unit tests (`*_test.go`).
   - `internal/parser`: Verify parsing of both vertical key-value output (`lssystem`) and tabular output (`lsvdisk`, `lsdrive`) using realistic `svcinfo` fixture strings.
   - `internal/config`: Test configuration precedence (flags override env vars, env vars override defaults) and validation error cases.
   - `internal/cache`: Test atomic write behavior, concurrent reads/writes (`-race`), expiration TTLs, and stale cache retrieval.
   - `internal/zabbix`: Test JSON serialization, LLD macro formatting, and truncation behavior (`MaxVolumes`, `MaxDrives`, `MaxJSONBytes`).
2. **Test Execution**:
   - Always run `go test ./...` before submitting changes.
   - If tests cannot be executed in the environment, report the exact reason and remaining risk.

---

## G. Security Rules

1. **Host-Key Verification**:
   - Default is `InsecureHostKey = false`.
   - Never default to disabling host key checks.
   - Host key verification requires either `-insecure` (`FS_INSECURE=true`) explicitly OR a valid `-known-hosts` file (`FS_KNOWN_HOSTS`).
2. **Credential Protection**:
   - Private key authentication (`-keyfile` / `FS_KEYFILE`) is strongly preferred over passwords.
   - Passwords must never be printed to stdout, logged to stderr, or cached to disk.
   - Never commit private keys, passwords, environment files with real secrets, or test credentials into git.
3. **File Permissions & Cache Security**:
   - Cache files and directory creations must respect strict POSIX permissions.
   - Default cache location is `/var/tmp`. Ensure atomic writes do not expose insecure file permissions.

---

## H. IBM FlashSystem Integration Rules

1. **Supported CLI Commands**:
   - `lssystem -delim :`
   - `lsnode -delim :`
   - `lsenclosure -delim :`
   - `lsdrive -delim :`
   - `lsmdiskgrp -delim :`
   - `lsvdisk -delim :`
   - `lsportfc -delim :`
   - `lsfcmap -delim :`
   - `lsreplicationrelationship -delim :` (Fallback: `lsrcrelationship -delim :` for legacy firmware versions returning `cmmvc5753e`, `not found`, or `unknown command`)
   - `lssystemstats -delim :`
   - `lsenclosurebattery -delim :`
   - `lsenclosurepsu -delim :`
2. **Parsing Rules**:
   - Always request colon-separated output (`-delim :`).
   - Treat all parsed record values as `string`. Do not attempt unsafe type coercions in parsers.
3. **Firmware Compatibility**:
   - IBM FlashSystem / Spectrum Virtualize firmware versions vary in returned columns and supported commands.
   - Parsers must dynamically map columns based on header rows rather than relying on hardcoded index offsets.
   - If command fallback logic is required, inspect error messages safely before attempting alternative commands.

---

## I. Zabbix Compatibility Rules

1. **Master Item Payload Contract**:
   - Output must strictly match `zabbix.Output` JSON structure:
     `timestamp`, `host`, `version`, `system`, `nodes`, `enclosures`, `drives`, `pools`, `volumes`, `ports`, `flashcopy`, `replication`, `performance`, `status`, `cache_stats`.
2. **LLD Macro Contract**:
   - Output must follow `{"data": [{"{#MACRO}": "value"}]}`.
   - Macro names must remain stable:
     - Drives: `{#DRIVEID}`, `{#ENCLOSUREID}`, `{#SLOTID}`, `{#DRIVESTATUS}`, `{#DRIVETYPE}`, `{#DRIVECAPACITY}`
     - Pools: `{#POOLID}`, `{#POOLNAME}`, `{#POOLSTATUS}`
     - Volumes: `{#VOLUMEID}`, `{#VOLUMENAME}`, `{#VOLUMESTATUS}`, `{#VOLUMEPOOL}`
     - Enclosures: `{#ENCLOSUREID}`, `{#ENCLOSURESTATUS}`, `{#ENCLOSUREMODEL}`
     - Nodes: `{#NODEID}`, `{#NODENAME}`, `{#NODESTATUS}`, `{#NODEIOGROUP}`
     - Ports: `{#PORTID}`, `{#WWPN}`, `{#PORTSTATUS}`, `{#PORTNODENAME}`, `{#PORTSPEED}`
     - Batteries: `{#BATTERYID}`, `{#ENCLOSUREID}`, `{#BATTERYSTATUS}`, `{#BATTERYCHARGE}`, `{#BATTERYEOL}`, `{#BATTERYRECO}`
     - PSUs: `{#PSUID}`, `{#ENCLOSUREID}`, `{#PSUSTATUS}`, `{#PSUINPUT}`, `{#PSUOUTPUT}`, `{#PSUFAN}`
3. **Empty / Error Output Contract**:
   - On discovery error, output must be `{"data":[]}` so Zabbix does not purge existing discovered entities or cause false alerts.
   - On collect fatal error, output JSON must contain `status.success: false` and populate `status.errors[]`.

---

## J. Cache and Reliability Rules

1. **File Naming & Path**:
   - Path generated via `cfg.CacheFile(host)`: `<cache-dir>/flashsystem_<sanitized_host>.cache.json`.
   - Characters `:`, `/`, `\`, and spaces in the hostname are sanitized to `_`.
2. **Atomic Writes**:
   - Writes must occur to a temporary file (`.flashsystem_cache_*.tmp`) in the same directory, followed by `os.Rename()` to guarantee atomicity.
3. **Stale Cache Fallback**:
   - If SSH connection or execution fails during `collect`, the runner attempts to serve expired cached entries (`c.GetStale()`).
   - The result error string must record `"SSH failed (stale cache from <timestamp>): <error>"`.

---

## K. Performance Rules

1. **Concurrency Control**:
   - Parallel collector execution must be limited using `MaxConcurrent: 3` semaphore in `Runner`.
   - Do not open separate SSH sessions for each collector; reuse the single `internalssh.Client` connection sequentially or across allowed concurrent channels.
2. **Memory & Size Safety**:
   - Enforce `MaxVolumes` and `MaxDrives` truncation on array outputs.
   - Enforce `MaxJSONBytes` payload safety.

---

## L. Documentation Requirements

Any change that modifies configuration options, CLI flags, output formats, installation procedures, or supported environments must update:
1. `README.md`
2. `instructions.md`
3. Usage help string in `main.go` (`printUsage()`)
4. Applicable `AGENTS.md` instruction files

---

## M. Standard Workflow for Every Coding Agent

When implementing changes in this repository, follow this step-by-step procedure:

1. **Read Guidance**: Read root `AGENTS.md` and applicable nested `AGENTS.md` files.
2. **Inspect Code**: Study the existing implementation and test files for the target component.
3. **Verify Baseline**: Run existing tests (`go test ./...`) to ensure a clean starting state.
4. **Implement Smallest Suitable Change**: Make concise, modular modifications.
5. **Add Tests**: Write unit tests covering new/changed logic and edge cases using mock fixtures.
6. **Validate**:
   - Execute `go test ./...` and `go build ./...`.
   - Run `git diff --check` to verify no whitespace/formatting issues.
7. **Review Diff**: Examine `git diff` to verify only intended files were changed.
8. **Document Results**: Provide a thorough summary of changes, test outcomes, and risk assessments.

---

## N. Prohibited Behaviors

- **Do NOT** modify application functionality or Go code when requested only to update repository instructions/documentation.
- **Do NOT** disable or weaken SSH host-key verification defaults.
- **Do NOT** edit generated build artifacts directly.
- **Do NOT** delete or disable existing tests to bypass test failures.
- **Do NOT** introduce external dependencies without explicit justification.
- **Do NOT** rename top-level JSON fields in `zabbix.Output` or Zabbix LLD macro keys.
- **Do NOT** log private keys, passwords, or sensitive connection parameters.
- **Do NOT** commit credentials, environment files with real secrets, or cache files to git.

---

## O. Completion Criteria

A task is complete only when:
1. All changes comply with instructions in `AGENTS.md`.
2. Code compiles cleanly with no compiler warnings or errors (`go build ./...`).
3. All relevant unit tests pass (`go test ./...`).
4. Updated documentation accurately reflects code implementation.
5. A clear summary report is provided outlining changes, validation steps, and risk evaluation.
