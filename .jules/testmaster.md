# TestMaster's Journal

## 2026-05-18 - IBM FlashSystem CLI Format Detection and Delimiter Rules

**Learning:** IBM FlashSystem CLI commands (`svcinfo -delim :`) produce two distinct response patterns that require careful format detection:
1. Vertical key-value pairs (e.g., `lssystem`), where lines contain single fields (`key:value`). Colons within values (such as timestamps, IP addresses with ports, or firmware build strings like `code_level:8.7.0.0 (build 123.456)`) must be preserved by splitting strictly on the first colon index (`strings.Index(line, ":")`).
2. Tabular matrices (e.g., `lsdrive`, `lsvdisk`), where the header line defines column names and subsequent lines define row values. When rows contain fewer fields than headers, missing columns must safely default to empty strings without crashing. When the final column itself contains colons, `strings.SplitN(line, ":", len(headers))` preserves the remainder in the last column.
3. Spectrum Virtualize error/empty strings (`CMMVC5845E`, `cmmvc...`, `no results`, `no objects found`) must normalize to `FormatEmpty` and empty `Records` slices (`[]Record{}`) rather than `nil` slices or runtime panics.

**Action:** Future tests covering parsers and collectors must explicitly verify colon preservation in vertical/tabular fields, IBM error code suppression into empty record slices, and dynamic header mapping across differing firmware versions.

## 2026-05-19 - Concurrent Collector Runner & Stale Fallback Isolation

**Learning:** `collectors.Runner` coordinates concurrent execution across collectors using a channel-based semaphore (`make(chan struct{}, MaxConcurrent)`) and manages cache lifecycle (valid hit, cache miss, corrupted entry fallback, and stale entry fallback upon SSH failure). Prior to this update, `collectors.Runner` and all 12 individual collector `Collect` functions had 0% unit and integration test coverage (overall package coverage was 22.0%), leaving concurrent semaphore handling, stale cache fallback semantics, and command-not-found fallback logic (e.g., replication v8.7+ `lsreplicationrelationship` falling back to `lsrcrelationship` on `CMMVC5753E`) untested against real SSH transport interfaces.

**Action:** Test collector concurrency and session management using an in-memory loopback SSH server (`golang.org/x/crypto/ssh` on `127.0.0.1:0`) and mock collectors. Always run tests with `-race` to ensure that semaphore acquisition, stale entry retrieval from disk cache, and concurrent results map writes remain thread-safe.

## 2026-05-20 - SSH Client Session Multiplexing, Cancellation Signals, and Failure Isolation

**Learning:** `internalssh.Client` wraps an active `*ssh.Client` and implements per-command timeouts with `context.WithTimeout`, asynchronous signal dispatching (`session.Signal(ssh.SIGTERM)`), and separate stdout/stderr capture on newly multiplexed channel sessions. Previously, `internal/ssh` lacked unit test coverage for its connection lifecycle (`New`, `Run`, `Ping`, `Close`), having only 39.8% coverage limited to config callback builders. In particular:
1. Command execution timeout cancellation must safely dispatch `ssh.SIGTERM` across the SSH session channel without blocking or deadlocking when the remote session hangs.
2. Stderr output must be reliably appended to the returned error upon non-zero exit codes to ensure diagnostic errors like `CMMVC5753E` are available to callers and parsers without leaking credentials.
3. Concurrent calls to `Run()` across multiple worker goroutines multiplexed over the same underlying TCP connection must execute without race conditions or cross-talk between session buffers.

**Action:** Maintain loopback SSH server test fixtures (`golang.org/x/crypto/ssh` on `127.0.0.1:0`) directly within `internal/ssh/client_test.go` to test SSH handshake failures, TCP dial failures, signal requests, timeout cancellations, and concurrent session multiplexing deterministically under race detection (`-race`).

## 2026-05-21 - LLD Discovery Collector Routing and Macro Contract Verification

**Learning:** `runDiscoveryCollector` routes 8 distinct discovery categories (`drives`, `pools`, `volumes`, `enclosures`, `nodes`, `ports`, `batteries`, `psus`) to their corresponding collector constructors and Zabbix LLD macro mappings (`zabbix.DriveLLDFields`, `zabbix.PoolLLDFields`, etc.). Previously, only the negative invalid category case was tested, leaving actual SSH command execution, parser-to-macro translation (`recordsToLLDItems`), command failure wrapping, and corrupted stale cache unmarshaling unverified in the main binary entry point.
Testing with an in-memory loopback SSH server proved that:
1. Each discovery category invokes its expected CLI command and correctly converts field names into Zabbix LLD macros (e.g. `tech_type` -> `{#DRIVETYPE}`, `percent_charged` -> `{#BATTERYCHARGE}`).
2. Command execution errors properly surface as wrapped errors (`collector "..." failed: ...`).
3. Corrupted cache entries (where JSON is valid on disk but unmarshal into `[]parser.Record` fails) are safely isolated into `out.Status.CollectorRuns` errors without crashing `buildFromStaleCache`.

**Action:** Future tests touching discovery categories or Zabbix template macro contracts must ensure full coverage across all 8 supported resource types using loopback SSH test fixtures and verify that missing fields default to empty strings rather than causing nil map crashes or malformed LLD JSON arrays.

## 2026-05-22 - Zabbix Master Item Schema and Multi-Tier Reduction Resilience

**Learning:** `internal/zabbix` serializes the master item telemetry payload consumed by Zabbix Dependent Items and LLD rules in `template/IBM FlashSystem Collector.yaml`. Three critical behavior contracts required verification:
1. Master item section arrays (`system`, `nodes`, `enclosures`, `drives`, `pools`, `volumes`, `ports`, `flashcopy`, `replication`, `performance`, `batteries`, `psus`) must never marshal as `null` or `{}` even during partial failures or completely empty collection results; they must always emit valid JSON empty arrays `[]` (`json.RawMessage("[]")`) to prevent Zabbix JSONPath evaluation aborts.
2. When storage size exceeds `MaxJSONBytes`, `reduceOutput` applies 4 progressive reduction tiers (halving volumes/drives, quartering volumes/drives, capping at 100 and clearing flashcopy, capping at 50 and clearing replication, and finally emptying volumes/drives completely). Previously only a single basic reduction scenario was tested; the progressive tiers and fallthrough empty sectioning were unverified.
3. In `reTruncateSection`, malformed or non-array section JSON must gracefully recover by resetting the section to `[]` without error, and non-positive `maxRecords` must safely leave sections untouched.
4. All 8 LLD discovery mapping dictionaries (`DriveLLDFields`, `PoolLLDFields`, `VolumeLLDFields`, `EnclosureLLDFields`, `NodeLLDFields`, `PortLLDFields`, `BatteryLLDFields`, `PSULLDFields`) must generate valid `{"data": [...]}` JSON structures in `LLDOutput`, correctly mapping missing fields to empty strings `""` rather than omitting keys or generating `null`.

**Action:** Maintain table-driven LLD tests for all 8 resource categories, verify template Dependent Item root key contracts, and validate multi-tier progressive reduction down to total clearing in `internal/zabbix/output_test.go`.

## 2026-05-23 - Top-Level Orchestration, SSH Dialing, and Full Stale Fallback Verification

**Learning:** `main.go` coordinates binary entry points, `dialSSH` configuration propagation, and emergency stale cache output generation when storage SSH is unreachable. While individual internal packages had high unit test coverage, three core integration pathways in the application entry point were previously uncovered:
1. `dialSSH`: Mapping loaded `*config.Config` settings into `internalssh.Client` creation must correctly propagate timeouts, authentication parameters, and host key policies, and cleanly propagate TCP dial or handshake failures without panicking or masking errors.
2. Full 12-Collector Stale Fallback: `buildFromStaleCache` must reconstruct all 12 storage resource sections (`system`, `nodes`, `enclosures`, `drives`, `pools`, `volumes`, `ports`, `flashcopy`, `replication`, `performance`, `batteries`, `psus`) from disk cache. In mixed scenarios where some collectors have stale entries and others do not, unpopulated sections must safely default to valid JSON empty arrays `[]` while logging specific `"SSH failed and no cache available"` errors in `status.collector_runs`.
3. Error payload string escaping: `escapeJSONString` must safely handle raw control characters (quotes, backslashes, tabs, carriage returns) without corrupting fallback JSON strings emitted to stdout.

**Action:** Maintain end-to-end integration tests in `main_test.go` covering `dialSSH` against loopback SSH fixtures, full/partial stale cache fallback across all 12 storage collectors, and string escaping edge cases.

## 2026-05-24 - End-to-End Discovery and Collection Execution Resilience

**Learning:** `runDiscover` and `runCollect` in `main.go` implement critical operational safety behaviors for Zabbix Low-Level Discovery and Master Item polling. Previous tests examined lower-level helper functions in isolation, but the orchestration pathways for discovery cache hits, transport outages, and live discovery caching were unverified:
1. `runDiscover` Cache Hit Fast-Path: When a fresh discovery cache entry exists (`MakeDiscoveryKey`), `runDiscover` must immediately serialize and emit LLD JSON (`emitLLDFromCache`) without attempting any TCP dial or SSH handshake.
2. Discovery Stale Cache Boundary (`maxStaleAge = 24 * time.Hour`): During storage interface failures or remote command failures, `runDiscover` must serve stale discovery items if cached within 24 hours, while logging a diagnostic warning to stderr. Once past 24 hours, stale discovery items must be strictly rejected and normalize to empty LLD `{"data":[]}` to prevent Zabbix from retaining nonexistent or decommissioned infrastructure.
3. Collector Execution Failure Resilience: When SSH connects successfully but the specific discovery collector command fails (e.g. CMMVC errors), `runDiscover` must fall back to stale discovery cache if available, rather than terminating prematurely.
4. End-to-End Master Item Verification: `runCollect` orchestrates config loading, SSH dialing, atomic cache purging upon success, parallel collector execution across 12 resources, and progressive output reduction, generating an 18-root-key JSON telemetry payload matching the Zabbix template contract.

**Action:** Future tests of discovery and collection orchestration must exercise the complete flow against in-memory loopback SSH server fixtures and temporary on-disk cache files, verifying both stdout JSON formatting and stderr diagnostic outputs under race detection (`-race`).

## 2026-05-25 - Atomic Cache Disk Write Failures, Incompatible Versions, and Concurrent Lifecycle Safety

**Learning:** `internal/cache` manages on-disk JSON caching via atomic temporary file generation (`.flashsystem_cache_*.tmp`), `saveToDisk` file syncing, and format versioning. Prior to this run, several failure boundaries and concurrency scenarios in `cache.go` lacked direct unit test coverage:
1. Disk write failure and temp file cleanup: When the cache target directory is read-only or unwritable, `os.CreateTemp` or `os.Rename` fails. The deferred cleanup closure (`defer func() { if !success { os.Remove(tmpName) } }()`) must ensure that no orphaned temporary `.tmp` files leak into the directory to prevent filesystem inode exhaustion in high-frequency monitoring environments.
2. Incompatible cache versions: Cache stores with mismatched version identifiers (e.g. future or corrupt formats) must be safely discarded on load, reverting to an empty cache without returning an unhandled error or corrupting subsequent fresh writes.
3. Explicit null entries: JSON caches serialized with `"entries": null` must gracefully instantiate `s.Entries = make(map[string]Entry)` during `loadFromDisk()`, preventing nil map assignment panics on subsequent `Set()` calls.
4. Concurrency with `Delete()` and `Purge()`: While basic concurrent read/write was tested, high-concurrency workloads executing simultaneous `Set`, `Get`, `GetStale`, `Len`, `Stats`, `Delete`, and `Purge` operations require validation under `-race` to ensure locking discipline between `c.mu.Lock()` and `c.mu.RLock()` avoids deadlocks or data races during disk sync.

**Action:** Maintain tests verifying write error cleanup without orphaned `.tmp` files, schema version mismatch rejection, null-entry safety, and concurrent lifecycle operations with `Delete()` and `Purge()` in `internal/cache/cache_test.go`.
