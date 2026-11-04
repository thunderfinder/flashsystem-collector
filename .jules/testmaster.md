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

## 2026-05-21 - Root Package Orchestration, Discovery Dispatch, and Stale Fallback

**Learning:** The root package `main` contained no test coverage (`[no test files]`), leaving crucial application orchestration unverified:
1. `buildFromStaleCache` handles cluster-wide SSH outages by reconstituting master item payloads strictly from disk cache across all 12 collectors. When disk cache is completely absent or an entry is corrupted, it must safely format error annotations into `collector_runs` and `status.errors` while preserving valid JSON output for Zabbix.
2. `recordsToLLDItems` maps parser records to Zabbix LLD macro keys (`{#MACRO}`). Missing fields in records must reliably populate as empty strings (`""`) rather than omitting keys, ensuring consistent JSON objects for Zabbix discovery rules.
3. `runDiscoveryCollector` must accurately map each of the 8 supported categories (`drives`, `pools`, `volumes`, `enclosures`, `nodes`, `ports`, `batteries`, `psus`) to its corresponding collector and LLD field map, while properly rejecting invalid categories and propagating SSH transport errors.

**Action:** Future root-level CLI modifications must maintain deterministic integration tests in `main_test.go` with loopback SSH fixtures to verify collector dispatch, LLD schema conversions, and stale cache fallback behavior.
