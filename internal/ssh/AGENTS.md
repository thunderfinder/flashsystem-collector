# AGENTS.md — internal/ssh Component Instructions

This directory provides the SSH client connection wrapper for executing remote IBM FlashSystem `svcinfo` commands over encrypted transport.

---

## Technical Responsibilities

- **Connection Management**: Establishes TCP dial and SSH handshakes with IBM storage controllers via `golang.org/x/crypto/ssh`.
- **Multiplexed Sessions**: Provides thread-safe command execution where multiple channels/sessions are executed concurrently over a single underlying TCP connection (`c.Run(cmd)`).
- **Timeouts**: Applies explicit timeouts for network dial (`DialTimeout`) and individual command executions (`CommandTimeout`).
- **Security & Host Verification**: Enforces host-key verification using standard `known_hosts` files or explicit insecure flag toggles.

---

## Safety & Security Rules (Sentinel Guidelines)

1. **Strict Host Key Verification**:
   - Never default to `InsecureIgnoreHostKey()`.
   - If `InsecureHostKey` is `false` and no `KnownHostsFile` is provided, `buildHostKeyCallback` checks `~/.ssh/known_hosts`. If that fallback is absent or insecure, it must return an explicit error.
2. **Known Hosts File Integrity**:
   - Host key database files must be regular files (`info.Mode().IsRegular()`).
   - Must forbid group and other write permissions (`info.Mode().Perm() & 0022 == 0`, e.g., `0644` or `0600`) to prevent local unprivileged users from poisoning host keys.
3. **Private Key Permissions**:
   - When keyfile authentication is used, key files must be regular files and forbid all group/other permissions (`perm & 0077 == 0`, e.g., `0600`).
4. **Credential Safety**:
   - Do NOT log passwords, raw private keys, or decrypted byte streams.
   - Include `stderr` output in command failure errors for troubleshooting without leaking credentials.
5. **Session Resource Leaks**:
   - Always call `session.Close()` in a `defer` block inside `Run(cmd)`.
   - Always close TCP connections when `Client.Close()` is invoked.

---

## Engineering & Execution Rules

1. **Timeout Handling**:
   - `Run(cmd)` wraps command execution with `context.WithTimeout(..., c.commandTimeout)`.
   - If a command exceeds `CommandTimeout`, send `SIGTERM` signal to the remote SSH session channel and return a timeout error.
2. **Concurrent Safety**:
   - `Client` must remain thread-safe. Multiple collector goroutines can invoke `Run()` simultaneously on independent SSH sessions over the shared client connection.
3. **Command Output Capture**:
   - Capture `stdout` and `stderr` separately into `bytes.Buffer`. If command execution fails, append `stderr` output to the returned error string for diagnostic clarity.

---

## Testing Guidelines

- Unit tests must use the in-memory loopback SSH server (`golang.org/x/crypto/ssh` on `127.0.0.1:0`) to test authentication, host keys, command execution, and timeouts deterministically without live hardware.
- Verify timeout logic when remote commands hang or sleep.
- Test authentication handling (key file vs password vs missing credentials).
- Test host key verification failures and file permission rejection (`0666`, `0777`).
