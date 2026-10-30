# AGENTS.md — internal/ssh Component Instructions

This directory provides the SSH client connection wrapper for executing remote IBM FlashSystem `svcinfo` commands.

---

## Technical Responsibilities

- **Connection Management**: Establishes TCP dial and SSH handshakes with IBM storage controllers via `golang.org/x/crypto/ssh`.
- **Multiplexed Sessions**: Provides thread-safe command execution where multiple channels/sessions are executed concurrently over a single underlying TCP connection (`c.Run(cmd)`).
- **Timeouts**: Applies explicit timeouts for network dial (`DialTimeout`) and individual command executions (`CommandTimeout`).
- **Security & Host Verification**: Enforces host-key verification using standard `known_hosts` files or explicit insecure flag toggles.

---

## Safety & Security Rules

1. **Host Key Verification**:
   - Never default to `InsecureIgnoreHostKey()`.
   - If `InsecureHostKey` is `false` and no `KnownHostsFile` is provided, `buildHostKeyCallback` must return an explicit error.
2. **Credential Safety**:
   - Do NOT log passwords, raw private keys, or decrypted byte streams.
   - Include `stderr` output in command failure errors for troubleshooting without leaking authentication data.
3. **Session Resource Leaks**:
   - Always call `session.Close()` in a `defer` block inside `Run(cmd)`.
   - Always close TCP connections when `Client.Close()` is invoked.

---

## Engineering & Execution Rules

1. **Timeout Handling**:
   - `Run(cmd)` wraps command execution in a goroutine with `context.WithTimeout(..., c.commandTimeout)`.
   - If a command exceeds `CommandTimeout`, send `SIGTERM` signal to the SSH session and return a timeout error.
2. **Concurrent Safety**:
   - `Client` must remain thread-safe. Multiple collector goroutines can invoke `Run()` simultaneously.
3. **Command Output Capture**:
   - Capture `stdout` and `stderr` separately into `bytes.Buffer`. If command execution fails, append `stderr` output to the returned error string for diagnostic clarity.

---

## Testing Guidelines

- Use a mock or test SSH server for SSH integration tests.
- Verify timeout logic when remote commands hang.
- Test authentication handling (key file vs password vs missing credentials).
- Test host key verification failures.
