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
