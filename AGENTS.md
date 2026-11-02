# AGENTS.md — Authoritative Repository Instructions for AI Agents

This is the authoritative, repository-level instruction file for all AI coding, testing, security, performance, and documentation agents working in `thunderfinder/flashsystem-collector`. Every agent must read and adhere to the guidelines, constraints, and contracts specified in this file and any applicable nested `AGENTS.md` files before modifying any file in this repository.

---

## A. Project Overview

### Purpose & Problem Statement
`flashsystem-collector` is an enterprise-grade Go utility and Zabbix external script integration designed for monitoring **IBM FlashSystem** and **IBM Spectrum Virtualize** storage systems (such as FlashSystem 5000, 5200, 7200, 7300, 9100, 9200, 9500, and legacy Storwize platforms).

Monitoring enterprise storage arrays via CLI poses several acute operational challenges:
1. **Controller Overhead**: Spawning multiple uncoordinated SSH sessions against storage management nodes can saturate SVC CLI daemons, throttle management processing, and cause CLI command timeouts.
2. **Alert Flapping & False Storms**: Transient management network blips or failovers can lead monitoring systems to record 0 capacity or report arrays as offline, triggering catastrophic alert storms.
3. **Zabbix Discovery Complexity**: Discovering hundreds or thousands of storage objects (volumes, drives, mdisk groups, enclosures, nodes, FC ports, batteries, PSUs) requires valid, RFC-compliant Zabbix Low-Level Discovery (LLD) JSON payloads mapped to specific Zabbix macros (`{#MACRO}`).

`flashsystem-collector` solves these problems by:
- Executing non-intrusive native CLI commands (`svcinfo`) over a **single multiplexed SSH connection** per collection cycle.
- Enforcing bounded concurrency (semaphore throttling at max 3 simultaneous remote commands).
- Implementing a **multi-tier persistent JSON cache** with atomic disk writes (`rename`) to prevent polling storms.
- Providing **stale cache resilience**: if storage interfaces are temporarily unreachable, expired cached telemetry is gracefully served tagged with diagnostic warnings, preserving historical monitoring graphs.
- Delivering dual execution modes: unified **Master Item** collection (`collect`) and granular **Low-Level Discovery** (`discover`) covering 8 discovery domains.

### Primary Technologies & Dependencies
- **Go**: Toolchain `1.25.0` (as defined in `go.mod`).
- **Dependencies**: `golang.org/x/crypto v0.52.0` (SSH client library).
- **Zabbix Integration**: Designed for Zabbix 6.0+, 6.4+, and 7.x via external scripts, JSONPath preprocessing, and the official template (`template/IBM FlashSystem Collector.yaml`).

---

## B. Mandatory Reading Before Making Changes

Before making any modifications in this repository, every agent **must**:
1. Read this root `/AGENTS.md` file in its entirety.
2. Read the nested `AGENTS.md` file located in the target directory (e.g., `internal/config/AGENTS.md`, `internal/collectors/AGENTS.md`, `internal/ssh/AGENTS.md`, `internal/cache/AGENTS.md`, `internal/parser/AGENTS.md`, `internal/zabbix/AGENTS.md`, `scripts/AGENTS.md`, `.github/workflows/AGENTS.md`, or `docs/AGENTS.md`).
3. Read `README.md` and `instructions.md` for user expectations and operational context.
4. Read relevant specialized journals under `.jules/` (`sentinel.md` for security, `bolt.md` for performance, `testmaster.md` for testing, `pipelinefixer.md` for CI/CD, `docmaster.md` for documentation) to understand historical architectural decisions and bug fixes.
5. Inspect `go.mod` to verify dependencies and Go version requirements.
6. Inspect existing unit tests and mock fixtures in the affected package.

**Agents must not start modifying code simply because a function or filename matches a prompt keyword.** Agents are required to explain their understanding of the affected component, its consumers, and its contracts before implementing substantial changes.

---

## C. Architecture and Execution Flow

### Repository Architecture Map

```
.
├── main.go                                  # CLI entry point, subcommand dispatch, stale fallback coordination
├── go.mod / go.sum                          # Module definition (github.com/flashsystem-collector, Go 1.25.0)
├── internal/
│   ├── config/                              # Configuration loading (flags > env > defaults) & strict validation
│   │   ├── config.go
│   │   └── config_test.go
│   ├── ssh/                                 # Multiplexed SSH client, timeouts, strict known_hosts verification
│   │   ├── client.go
│   │   └── client_test.go
│   ├── cache/                               # Disk-backed thread-safe cache, atomic file replacement, stale fallback
│   │   ├── cache.go
│   │   └── cache_test.go
│   ├── parser/                              # Delimited (:), tabular, and vertical key-value parser for svcinfo output
│   │   ├── parser.go
│   │   └── parser_test.go
│   ├── collectors/                          # 12 resource collectors, interface Collector, throttled Runner
│   │   ├── collector.go                     # Collector interface, Runner struct, AllCollectors registry
│   │   ├── system.go                        # svcinfo lssystem -delim :
│   │   ├── nodes.go                         # svcinfo lsnode -delim :
│   │   ├── enclosures.go                    # svcinfo lsenclosure -delim :
│   │   ├── drives.go                        # svcinfo lsdrive -delim :
│   │   ├── pools.go                         # svcinfo lsmdiskgrp -bytes -delim :
│   │   ├── volumes.go                       # svcinfo lsvdisk -bytes -delim :
│   │   ├── ports.go                         # svcinfo lsportfc -delim :
│   │   ├── flashcopy.go                     # svcinfo lsfcmap -delim :
│   │   ├── replication.go                   # svcinfo lsreplicationrelationship (fallback: lsrcrelationship)
│   │   ├── performance.go                   # svcinfo lssystemstats -delim :
│   │   ├── batteries.go                     # svcinfo lsenclosurebattery -delim :
│   │   ├── psus.go                          # svcinfo lsenclosurepsu -delim :
│   │   └── collectors_test.go
│   └── zabbix/                              # Master Item JSON builder, LLD formatter, payload truncation
│       ├── output.go
│       └── output_test.go
├── scripts/
│   └── flashsystem_collector.sh             # Production Zabbix wrapper (flock locking, env loader, safe exit)
├── template/
│   └── IBM FlashSystem Collector.yaml       # Turnkey Zabbix 6.4+/7.x YAML template
├── docs/
│   └── wiki/                                # 24 GitHub Wiki topic pages + _Sidebar.md
└── .github/workflows/                       # CI/CD pipelines (Go, Lint, CodeQL, SLSA3, DevSkim, Semgrep)
```

### Execution Lifecycles

#### 1. Unified Collection Mode (`flashsystem-collector collect`)
1. **Entry & CLI Parsing**: `main.go` parses CLI flags and environment variables (`FS_*`) using `config.Load()`. Precedence is strictly: **CLI flags > Environment variables > Built-in defaults**.
2. **Validation**: Enforces strict mutual exclusivity of authentication (either `-keyfile` or `-password`, never both or neither), POSIX file permissions (`0600` for keyfile, `perm & 0022 == 0` for known_hosts), valid port (1–65535), and minimum TTL thresholds.
3. **Cache Initialization**: Initializes cache instance for the target storage host (`flashsystem_<sanitized_host>.cache.json`). Purges expired disk entries.
4. **SSH Connection**: Establishes a single TCP dial and SSH handshake using `internalssh.NewClient()`.
   - **On SSH Failure**: Intercepts error, invokes `buildFromStaleCache()`, populates metrics from stale entries if present on disk, sets `status.success: false`, and outputs valid diagnostic JSON.
5. **Collector Registration**: `collectors.AllCollectors()` instantiates all 12 collectors with their respective TTLs (`metricsTTL`, `discoveryTTL`, `performanceTTL`).
6. **Concurrent Runner Execution**: `collectors.NewRunner(3)` executes registered collectors concurrently:
   - For each collector: checks cache for fresh data (`cache.Get()`). If hit, returns cached records immediately without SSH.
   - On cache miss: acquires one of 3 semaphore slots, runs `col.Collect(client)` via SSH session channel, releases semaphore slot, and caches fresh records (`cache.Set()`).
   - If individual collector SSH command fails: attempts to retrieve stale cache (`cache.GetStale()`). If found, uses stale records and records diagnostic error in `status.errors`.
7. **JSON Assembly & Truncation**: `zabbix.Build()` constructs the master JSON structure. Checks payload size against `MaxJSONBytes` (default 1 MB). If exceeded, progressively truncates volumes (`MaxVolumes`), drives (`MaxDrives`), and secondary replication data to prevent Zabbix agent buffer overruns.
8. **Emission**: Emits formatted JSON to `stdout`.

#### 2. Discovery Mode (`flashsystem-collector discover -category <cat>`)
1. **Validation**: Verifies that `-category` matches one of the 8 valid categories: `drives`, `pools`, `volumes`, `enclosures`, `nodes`, `ports`, `batteries`, `psus`.
2. **Cache Check**: Checks discovery cache key (`<host>:discovery:<category>`). If valid, immediately outputs `{"data": [...]}` JSON and terminates.
3. **SSH Dial**: On cache miss, establishes SSH connection. If SSH fails, emits `{"data":[]}` (empty LLD) to `stdout` and logs error to `stderr` (preventing Zabbix from dropping discovered entities during transient network outages).
4. **Execution & LLD Formatting**: Runs the targeted collector, maps parsed records to Zabbix macros (`{#DRIVEID}`, `{#POOLNAME}`, etc.), caches the result with `DiscoveryTTL` (default 4h), and emits RFC-compliant LLD JSON.

---

## D. Technology and Development Environment

### Compiler & Toolchain
- **Required Go Version**: Go `1.25.0` (mandated by `go.mod`).
- **Target OS/Arch**: Linux AMD64 (`GOOS=linux GOARCH=amd64`).

### Essential Developer Commands

```bash
# Manage and verify dependencies
go mod tidy
go mod verify

# Local compilation
go build -o flashsystem-collector .

# Production static binary build (Linux AMD64)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o flashsystem-collector .

# Run all unit tests
go test ./...

# Run all tests with race detector enabled
go test -race ./...

# Run tests with coverage
go test -cover ./...

# Static analysis and vetting
go vet ./...

# Git whitespace and format verification
git diff --check
```

---

## E. Engineering Principles

1. **Idiomatic Go**: Adhere to standard Go conventions (`gofmt`, camelCase, receiver naming).
2. **Explicit Error Propagation**: Always handle and wrap errors with semantic context using `fmt.Errorf("context: %w", err)`. Never discard errors silently.
3. **No Hidden Global State**: Pass configuration, SSH client, cache, and logger instances explicitly through constructors and method arguments.
4. **Strict Stability Contracts**:
   - Never change CLI flag names or environment variable names (`FS_*`).
   - Never rename top-level JSON fields in `zabbix.Output`.
   - Never alter Zabbix LLD macro names without synchronized template and documentation updates.
5. **Small, Atomic Changes**: Keep modifications modular, focused, and well-covered by tests. Avoid gratuitous refactoring.
6. **Robust Graceful Degradation**: Always emit valid JSON on `stdout`. Diagnostic messages, debug traces, and panic dumps belong exclusively on `stderr` or log files.

---

## F. Testing Requirements

1. **Mandatory Test Execution**:
   - Any agent modifying Go code must execute `go test ./...` and `go test -race ./...`.
   - An agent must never declare work complete without actually running the test suite. If an execution environment prevents running tests, the agent must document the reason and the remaining risk.
2. **Unit Tests Coverage by Component**:
   - `internal/config`: Test configuration precedence (flags > env > defaults), validation logic, mutual exclusivity of credentials, file permission verification for keys and known_hosts.
   - `internal/parser`: Test vertical key-value parsing, tabular parsing, colon preservation in values, empty results, and IBM error codes (`CMMVC*`). Test `FormatCapacity` binary unit formatting.
   - `internal/cache`: Test atomic write and rename behavior, TTL expiration, stale cache retrieval, and concurrent read/write safety (`-race`).
   - `internal/ssh`: Test authentication builder, known_hosts permissions, dial/command timeouts, and multiplexed execution using in-memory loopback SSH server fixtures.
   - `internal/collectors`: Test collector registry, 12 collectors, fallback logic (replication), and semaphore concurrency throttling.
   - `internal/zabbix`: Test JSON schema serialization, LLD macro transformations across all 8 discovery categories, and payload truncation rules.

---

## G. Security Rules

1. **SSH Host-Key Verification**:
   - Default is `InsecureHostKey = false`. Strict verification is mandatory for production.
   - Known hosts database files (`FS_KNOWN_HOSTS` or fallback `~/.ssh/known_hosts`) must be regular files and must forbid group and other write permissions (`perm & 0022 == 0`).
   - `FS_INSECURE=true` (`-insecure`) is strictly reserved for isolated lab or testing environments and must never be set as default.
2. **Private Key Protection**:
   - Private key files (`FS_KEYFILE` / `-keyfile`) must be regular files with strict POSIX permissions forbidding any access by group or other users (`perm & 0077 == 0`, e.g., `0600`).
   - Private keys and passwords must never be logged, printed to `stdout`, or stored in disk caches.
3. **Mutual Exclusivity of Credentials**:
   - Exactly one authentication method must be provided: either `-keyfile` or `-password`. Providing both or neither is a fatal validation error.
4. **Command Execution Safety**:
   - Never construct shell commands dynamically with unsanitized user strings. Commands executed over SSH are static SVC CLI commands (`svcinfo ...`).
5. **Cache & File Security**:
   - Atomic disk writes must occur via temporary files with restrictive permissions in the cache directory (`/var/tmp`) before renaming.
6. **Zero Secrets in Code**:
   - Never commit credentials, private keys, passwords, live storage tokens, or production hostnames to git.

---

## H. IBM FlashSystem Integration Rules

1. **12 Supported CLI Commands**:
   - System: `svcinfo lssystem -delim :`
   - Nodes: `svcinfo lsnode -delim :`
   - Enclosures: `svcinfo lsenclosure -delim :`
   - Drives: `svcinfo lsdrive -delim :`
   - Storage Pools: `svcinfo lsmdiskgrp -bytes -delim :`
   - Volumes: `svcinfo lsvdisk -bytes -delim :`
   - Fibre Channel Ports: `svcinfo lsportfc -delim :`
   - FlashCopy: `svcinfo lsfcmap -delim :`
   - Replication: `svcinfo lsreplicationrelationship -delim :` (Fallback: `svcinfo lsrcrelationship -delim :` when encountering `CMMVC5753E`, `not found`, or `unknown command`)
   - Performance: `svcinfo lssystemstats -delim :`
   - Batteries: `svcinfo lsenclosurebattery -delim :`
   - Power Supply Units: `svcinfo lsenclosurepsu -delim :`
2. **Parsing Rules**:
   - Always pass `-delim :` to guarantee colon-delimited output.
   - Dynamic header mapping: Always read column names from the header row rather than assuming hardcoded column index offsets. Firmware updates frequently reorder or add columns.
   - Colon preservation: Values containing colons (timestamps, WWPNs, IP addresses) must be preserved using `strings.Index(line, ":")` or `strings.SplitN(line, ":", len(headers))`.
3. **Capacity Formatting**:
   - Capacities in drives/disks must use binary units (base-2, 1024): values $< 1\text{ TB}$ are formatted in `GB`, values $\ge 1\text{ TB}$ in `TB` via `parser.FormatCapacity`.
   - Volumes and pools use `-bytes` for raw 64-bit integer precision.

---

## I. Zabbix Compatibility Rules

1. **Master Item Payload Contract (`zabbix.Output`)**:
   Top-level JSON keys must remain exact:
   `timestamp`, `host`, `version`, `system`, `nodes`, `enclosures`, `drives`, `pools`, `volumes`, `ports`, `flashcopy`, `replication`, `performance`, `batteries`, `psus`, `units_info`, `status`, `cache_stats`.
2. **8 Low-Level Discovery (LLD) Categories & Macro Contracts**:
   Output must follow `{"data": [{"{#MACRO}": "value"}]}`. Macro keys are strictly bound to the Zabbix YAML template:
   - **`drives`**: `{#DRIVEID}`, `{#ENCLOSUREID}`, `{#SLOTID}`, `{#DRIVESTATUS}`, `{#DRIVETYPE}`, `{#DRIVECAPACITY}`
   - **`pools`**: `{#POOLID}`, `{#POOLNAME}`, `{#POOLSTATUS}`
   - **`volumes`**: `{#VOLUMEID}`, `{#VOLUMENAME}`, `{#VOLUMESTATUS}`, `{#VOLUMEPOOL}`
   - **`enclosures`**: `{#ENCLOSUREID}`, `{#ENCLOSURESTATUS}`, `{#ENCLOSUREMODEL}`
   - **`nodes`**: `{#NODEID}`, `{#NODENAME}`, `{#NODESTATUS}`, `{#NODEIOGROUP}`
   - **`ports`**: `{#PORTID}`, `{#WWPN}`, `{#PORTSTATUS}`, `{#PORTNODENAME}`, `{#PORTSPEED}`
   - **`batteries`**: `{#BATTERYID}`, `{#ENCLOSUREID}`, `{#BATTERYSTATUS}`, `{#BATTERYCHARGE}`, `{#BATTERYEOL}`, `{#BATTERYRECO}`
   - **`psus`**: `{#PSUID}`, `{#ENCLOSUREID}`, `{#PSUSTATUS}`, `{#PSUINPUT}`, `{#PSUOUTPUT}`, `{#PSUFAN}`
3. **Empty LLD & Error Resilience**:
   - On discovery error, output must be `{"data":[]}` so Zabbix does not trigger false entity deletion alerts.
   - On collection failure, `status.success` is `false`, and errors are listed in `status.errors[]`.

---

## J. Cache and Reliability Rules

1. **Cache File Naming**:
   - Sanitized path: `<cache-dir>/flashsystem_<sanitized_host>.cache.json`.
   - Special characters `:`, `/`, `\`, and spaces in hostname are sanitized to `_`.
2. **Atomic Persistence**:
   - Cache files must always be written to a temporary file (`.flashsystem_cache_*.tmp`) in the cache directory before atomic replacement via `os.Rename()`.
3. **Multi-Tier TTLs**:
   - Metrics TTL (`FS_METRICS_TTL`): Default `5m` (general telemetry).
   - Discovery TTL (`FS_DISCOVERY_TTL`): Default `4h` (hardware inventory discovery).
   - Performance TTL (`FS_PERFORMANCE_TTL` / `FS_PERF_TTL`): Default `1m`, minimum `1s`.
4. **Stale Cache Fallback**:
   - When SSH connectivity fails, `main.go` and `Runner` attempt to retrieve expired cached entries (`c.GetStale()`).
   - Diagnostic error message is recorded: `"SSH failed (stale cache from <timestamp>): <error>"`.

---

## K. Performance Rules

1. **Parser Optimizations**:
   - In `internal/parser`, pre-size `Record` maps with known column lengths (`make(Record, len(headers))`) to eliminate runtime map rehashing.
   - Use zero-allocation case-insensitive comparisons (`strings.EqualFold`) and sliced prefix checks instead of allocating new strings via `strings.ToLower`.
2. **Concurrency & Resource Throttling**:
   - All collectors execute over a single shared `*internalssh.Client`.
   - Concurrency is throttled to 3 simultaneous goroutines using a channel semaphore (`RunnerConfig.MaxConcurrent = 3`).
3. **Memory & Payload Limits**:
   - Enforce `MaxVolumes` (default 500) and `MaxDrives` (default 500) array limits.
   - Enforce `MaxJSONBytes` (default 1 MB) payload limit via progressive reduction before output emission.

---

## L. Documentation Requirements

Any change impacting CLI flags, configuration parameters, output formats, template macros, or operational steps must update:
1. `README.md`
2. `instructions.md`
3. Relevant topic pages in `docs/wiki/` (and `_Sidebar.md` if pages are added/removed)
4. CLI help message in `main.go` (`printUsage()`)
5. Applicable `AGENTS.md` instruction files

---

## M. Standard Workflow for Every Coding Agent

Every agent working on this repository must execute the following workflow:

1. **Read Guidance**: Read root `AGENTS.md` and applicable nested `AGENTS.md` files.
2. **Inspect Existing Implementation**: Examine source files, tests, and relevant journals in `.jules/`.
3. **Verify Baseline**: Run `go test ./...` to confirm the working tree is clean.
4. **Implement Focused Change**: Make small, cohesive modifications adhering to existing design patterns.
5. **Add/Update Tests**: Add unit tests verifying new functionality, edge cases, and failure modes.
6. **Run Full Test Suite**: Execute `go test ./...` and `go test -race ./...`.
7. **Verify Formatting & Linting**: Run `go vet ./...` and `git diff --check`.
8. **Review Diff**: Ensure only intended files are changed and no extraneous files or secrets are present.
9. **Report Findings**: Provide a comprehensive final summary of changes, test executions, and risk assessments.

---

## N. Prohibited Behaviors

- **Do NOT** modify application code when the assigned task is strictly documentation or instruction maintenance.
- **Do NOT** default to insecure SSH host-key verification (`InsecureHostKey = true`).
- **Do NOT** weaken file permission checks (`0600` for private keys, `perm & 0022 == 0` for known hosts).
- **Do NOT** alter top-level keys in `zabbix.Output` or rename Zabbix LLD macros (`{#DRIVEID}`, etc.).
- **Do NOT** delete or skip existing unit tests to make a build pass.
- **Do NOT** introduce external dependencies without explicit requirement and justification.
- **Do NOT** log credentials, private keys, or decrypted connection strings.
- **Do NOT** commit credentials, environment files with secrets, or temporary cache files.

---

## O. Completion Criteria

A task is considered complete only when:
1. All changes adhere strictly to the guidelines in `AGENTS.md`.
2. All Go code compiles cleanly without errors or warnings (`go build ./...`).
3. All unit tests pass with race detection enabled (`go test -race ./...`).
4. Documentation, templates, and instruction files accurately reflect the implementation.
5. A detailed report is provided documenting changes, validation commands, compatibility impact, and residual risks.
