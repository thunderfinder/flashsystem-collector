# TestMaster's Journal

## 2026-05-18 - IBM FlashSystem CLI Format Detection and Delimiter Rules

**Learning:** IBM FlashSystem CLI commands (`svcinfo -delim :`) produce two distinct response patterns that require careful format detection:
1. Vertical key-value pairs (e.g., `lssystem`), where lines contain single fields (`key:value`). Colons within values (such as timestamps, IP addresses with ports, or firmware build strings like `code_level:8.7.0.0 (build 123.456)`) must be preserved by splitting strictly on the first colon index (`strings.Index(line, ":")`).
2. Tabular matrices (e.g., `lsdrive`, `lsvdisk`), where the header line defines column names and subsequent lines define row values. When rows contain fewer fields than headers, missing columns must safely default to empty strings without crashing. When the final column itself contains colons, `strings.SplitN(line, ":", len(headers))` preserves the remainder in the last column.
3. Spectrum Virtualize error/empty strings (`CMMVC5845E`, `cmmvc...`, `no results`, `no objects found`) must normalize to `FormatEmpty` and empty `Records` slices (`[]Record{}`) rather than `nil` slices or runtime panics.

**Action:** Future tests covering parsers and collectors must explicitly verify colon preservation in vertical/tabular fields, IBM error code suppression into empty record slices, and dynamic header mapping across differing firmware versions.

## 2026-05-19 - Isolated In-Process SSH Server Testing for Multiplexed Sessions

**Learning:** Testing SSH client implementations without external network daemons or live FlashSystem hardware requires an in-process mock SSH server (`golang.org/x/crypto/ssh`). Key testing constraints discovered:
1. Extended data streams (such as `stderr` on remote command failures) in `golang.org/x/crypto/ssh` channels are written via `channel.Stderr().Write(...)` rather than indexing into `channel.Extended(...)`.
2. Command timeouts must test both execution cancellation and channel signal propagation (`SIGTERM`) without hanging the test suite, using short `CommandTimeout` values and deterministic sleep delays on handler goroutines.
3. Multiplexed concurrent session execution (`Client.Run` across multiple goroutines on a single TCP connection) must be verified under Go's race detector (`-race`) to prevent channel or buffer race conditions when handling simultaneous requests.
4. Host key validation tests using `knownhosts.New` must provide well-formed `known_hosts` entries formatted via `knownhosts.Line([]string{addr}, pubKey)` to validate both matching and key mismatch detection accurately.

**Action:** When writing tests that interact with `internal/ssh`, utilize in-process `net.Listen("tcp", "127.0.0.1:0")` SSH test servers with configured host keys and authorized keys instead of requiring external credentials or real storage appliances.
