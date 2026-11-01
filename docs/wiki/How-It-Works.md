# How It Works ⚙️

This page provides a deep technical breakdown of the collector's internal execution sequence, from the initial Zabbix trigger down to the parsing algorithms and JSON serialization contracts.

---

## 1. Execution Sequence Overview

When Zabbix triggers an External Check item, execution proceeds through a deterministic, fail-safe lifecycle:

```mermaid
flowchart TD
    Start([Zabbix External Check Invocation]) --> WrapperInit[Bash Wrapper: Parameter Parsing & Sanitization]
    WrapperInit --> LoadEnv[Source /etc/zabbix/flashsystem/env]
    LoadEnv --> CheckBin[Validate Binary Path & Permissions]
    CheckBin --> AcquireLock[Acquire Host Lock via flock]
    AcquireLock --> ExecGo[Execute flashsystem-collector binary]

    ExecGo --> LoadConfig[internal/config: Resolve Flags, Env & Defaults]
    LoadConfig --> ValidateConfig{Validate Parameters & Permissions}
    ValidateConfig -- Invalid --> FatalExit[fatalJSON / fatalLLD: Structured Error to STDOUT + Exit 1]

    ValidateConfig -- Valid --> InitCache[internal/cache: Load & Purge Disk Store]
    InitCache --> CheckMode{Subcommand Mode}

    CheckMode -- collect --> RunCollect[collectors.Runner: Evaluate Cache Hits]
    RunCollect --> AllHits{All Collectors in Cache?}

    AllHits -- Yes --> BuildJSON[internal/zabbix: Assemble Master Output]
    AllHits -- No --> DialSSH[internal/ssh: Connect & Verify Host Key]

    DialSSH -- SSH Failed --> FallbackStale[buildFromStaleCache: Load Expired Entries]
    FallbackStale --> BuildJSON

    DialSSH -- SSH Connected --> ExecCommands[Run Collectors via Semaphore Slots]
    ExecCommands --> ParseOutput[internal/parser: Parse Colon-Delimited Data]
    ParseOutput --> UpdateCache[Write New Entries via Atomic Rename]
    UpdateCache --> BuildJSON

    BuildJSON --> CheckSize{Payload > MaxJSONBytes?}
    CheckSize -- Yes --> Truncate[Progressive Section Truncation]
    CheckSize -- No --> Serialize[Indent & Emit JSON to STDOUT]
    Truncate --> Serialize

    CheckMode -- discover --> CheckDiscCache{LLD Cached & Valid?}
    CheckDiscCache -- Yes --> EmitCachedLLD[Emit Cached LLD JSON Array]
    CheckDiscCache -- No --> DialSSHDisc[Connect SSH & Run Target Collector]
    DialSSHDisc -- Success --> CacheLLD[Save LLD Data to Cache]
    CacheLLD --> EmitLLD[Emit Formatted LLD JSON to STDOUT]
    DialSSHDisc -- Failure --> EmitEmptyLLD[Emit Empty LLD: data=[]]

    Serialize --> WrapperExit[Wrapper: Check Exit Code & Flush STDERR]
    EmitLLD --> WrapperExit
    EmitEmptyLLD --> WrapperExit
    EmitCachedLLD --> WrapperExit
    FatalExit --> WrapperExit
    WrapperExit --> End([Zabbix Ingestion & Preprocessing])
```

---

## 2. Configuration Resolution and Precedence

Configuration parameters are evaluated during startup using an explicit three-tier priority ladder:
1. **Command-Line Flags**: Explicit flags passed at the CLI (highest precedence).
2. **Environment Variables**: Operating system environment variables prefixed with `FS_*`.
3. **Internal Defaults**: Hardcoded safe system defaults (lowest precedence).

### Evaluation Logic
The loader initializes the configuration struct with default values (e.g., `Port: 22`, `CacheDir: "/var/tmp"`, `DialTimeout: 10s`). It then binds command-line flags. If a flag is explicitly provided, it overwrites the field. Finally, `applyEnv()` inspects the environment and fills in any fields that still hold their initial default values.

### Strict Validation Rules
Before any network sockets or caches are touched, `cfg.validate()` verifies:
- `Host` and `User` must not be empty.
- `Port` must be within `1`–`65535`.
- **Authentication**: Exactly one of `KeyFile` or `Password` must be configured. Setting both or neither triggers a fatal error.
- **Key Permissions**: If running on non-Windows platforms, private key permissions are inspected via `os.Stat()`. If permissions allow group or other read/write access (`perm & 0077 != 0`), execution halts immediately with a security violation error.
- **Cache Directory**: Verifies the cache directory exists and is writable by creating and removing a temporary test file (`.flashsystem_write_test_*`).

---

## 3. SSH Connection and Command Execution

The SSH client (`internal/ssh/client.go`) encapsulates `golang.org/x/crypto/ssh`:

### Connection Handshake
- Sets a connection dial timeout governed by `FS_DIAL_TIMEOUT` (default `10s`).
- Negotiates SSH host key verification:
  - If `FS_KNOWN_HOSTS` is defined, keys are cryptographically validated using `ssh.NewPublicKeyAuthMethod()`.
  - If neither `FS_KNOWN_HOSTS` nor `FS_INSECURE` is supplied, connection is refused.

### Command Execution Lifecycle
When a collector invokes `client.Run(cmd)`:
1. Opens an individual SSH `Session` from the underlying authenticated SSH connection.
2. Directs `session.Stdout` and `session.Stderr` into memory buffers.
3. Spawns an execution timer governed by `FS_CMD_TIMEOUT` (default `30s`).
4. If the timer expires before the command completes, `session.Signal(ssh.SIGKILL)` is dispatched and the session is closed, returning an explicit context deadline exceeded error.
5. Captures and trims stdout output.

---

## 4. The Colon-Delimited Parser Pipeline

IBM FlashSystem CLI tools support output formatting with the `-delim <char>` argument. The collector standardizes on colon delimiters (`-delim :`).

`internal/parser/parser.go` implements an intelligent dual-mode parser capable of handling both key-value configurations and tabular matrices:

### Mode 1: Single-Record Key-Value Output
Commands such as `svcinfo lssystem -delim :` produce key-value pairs formatted vertically:
```text
id:0000020420201234
name:FS9200-Cluster01
location:local
cluster_status:online
code_level:8.5.2.1
total_mdisk_capacity:145.5TB
space_in_mdisk_grps:120.2TB
space_allocated_to_vdisks:98.4TB
total_free_space:25.3TB
```
The parser detects that each line contains a single `:`. It splits the line into key and value, sanitizes whitespace, and packages the result into a single `parser.Record` map (`map[string]string`).

### Mode 2: Multi-Record Tabular Output
Commands such as `svcinfo lsmdiskgrp -delim :` or `svcinfo lsdrive -delim :` output a header line followed by row records:
```text
id:name:status:mdisk_count:vdisk_count:capacity:free_capacity
0:Pool_NVMe:online:12:45:10995116277760:4398046511104
1:Pool_SAS:online:24:80:43980465111040:10995116277760
```
The parser reads the first line to extract column headers. Subsequent lines are split by `:` and mapped index-by-index to corresponding headers, constructing a slice of `parser.Record` objects (`[]parser.Record`).

---

## 5. Collector Concurrency and Runner Architecture

The `Runner` orchestrates collection across all 12 domains:

1. **Cache Pre-check**: For each collector, the runner checks the in-memory cache index. If an unexpired entry exists, it is deserialized immediately (`CacheHit: true`).
2. **Semaphore Throttling**: For cache misses, goroutines queue up on a buffered channel:
   ```go
   sem <- struct{}{}
   defer func() { <-sem }()
   ```
   This guarantees that no more than 3 commands run simultaneously against the storage controller.
3. **Cache Storage**: Successful query results are serialized and stored in cache tagged with the collector's respective TTL.
4. **Stale Fallback on Error**: If `col.Collect(client)` returns an error (e.g., connection lost), the runner attempts `c.GetStale(cacheKey)`. If expired data exists, it returns those records, setting `CacheHit: true` and annotating `Error` with `"SSH failed (using stale cache): <err>"`.

---

## 6. Output Assembly and Size Safeguards

Once all collectors finish, `internal/zabbix/output.go` synthesizes the final JSON envelope:

### Payload Truncation Rules
Zabbix External Checks have upper bounds on payload sizes (historically 1MB to 16MB). The collector enforces a strict ceiling governed by `MaxJSONBytes` (default `1MB`):
- High-cardinality arrays (`volumes` and `drives`) are initially capped by `MaxVolumes` (default 500) and `MaxDrives` (default 500).
- If the resulting JSON still exceeds `MaxJSONBytes`, `reduceOutput()` applies progressive downsampling:
  1. Reduces volume and drive limits by 50%.
  2. Reduces volume and drive limits to 25%.
  3. Caches FlashCopy and Replication sections to `[]` and sets volume/drive limits to 100.
  4. Reduces volume/drive limits to 50.
  5. Clears volume and drive sections completely if the threshold is still violated.
- Whenever truncation occurs, the original record count is preserved in `status.truncated_at` (e.g., `{"volumes": 1420}`).

### Structured JSON Schema
```json
{
  "timestamp": "2026-03-17T12:00:00Z",
  "host": "10.10.10.50",
  "version": "2.0.0",
  "status": {
    "success": true,
    "duration_ms": 1420,
    "collector_runs": [
      {
        "name": "system",
        "record_count": 1,
        "cache_hit": false,
        "duration_ms": 120
      }
    ],
    "cache": {
      "total_entries": 12,
      "valid_entries": 12,
      "expired_entries": 0,
      "cache_hits": 8,
      "cache_misses": 4,
      "file_path": "/var/tmp/flashsystem_10.10.10.50.cache.json"
    }
  },
  "system": [...],
  "nodes": [...],
  "enclosures": [...],
  "drives": [...],
  "pools": [...],
  "volumes": [...],
  "ports": [...],
  "flashcopy": [...],
  "replication": [...],
  "performance": [...],
  "batteries": [...],
  "psus": [...]
}
```
