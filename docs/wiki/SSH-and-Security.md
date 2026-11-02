# SSH and Security 🔐

Security is a primary design pillar of `flashsystem-collector`. Because monitoring agents require remote command execution access to core enterprise storage controllers, this document outlines the authentication mechanics, host-key verification models, file permission enforcement, and least-privilege standards required for secure operation.

---

## 1. SSH Authentication Model

The collector supports two authentication methods for connecting to the IBM FlashSystem CLI:

1. **Public-Key Authentication (Strongly Recommended)**:
   - Configured via `-keyfile` or `FS_KEYFILE`.
   - Uses an OpenSSH-compatible private key file (ED25519 or RSA $\ge 2048$ bits).
   - Supported keys must be in standard unencrypted OpenSSH PEM format (keys with passphrases cannot be unlocked non-interactively).
2. **Password Authentication (Lab / Testing Only)**:
   - Configured via `-password` or `FS_PASSWORD`.
   - **Warning**: Using passwords exposes credentials to process table listings (`ps -ef`), command history, and log dumps. Passwords should never be used in production environments.

> ⚠️ **Mutual Exclusivity Rule**: The collector strictly enforces that exactly one authentication method is specified. Configuring both `-keyfile` and `-password` (or setting both environment variables) causes the collector to abort immediately.

---

## 2. Hardened Private Key File Permissions

To prevent privilege escalation or accidental exposure of storage credentials to other local users on the Zabbix monitoring server, the collector enforces POSIX file permission checks directly in `internal/config/config.go`:

```go
// Security check enforced before opening SSH connection
if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
    return fmt.Errorf("SSH key file %q has insecure permissions (%04o): "+
        "must not be accessible by group or others (chmod 600 required)",
        cfg.SSH.KeyFile, info.Mode().Perm())
}
```

If the key file has permissions such as `0644` (world-readable) or `0660` (group-readable), execution halts immediately with an error.

### Setting Correct Key Permissions
```bash
sudo chown zabbix:zabbix /etc/zabbix/ssh/flashsystem_key
sudo chmod 0600 /etc/zabbix/ssh/flashsystem_key
```

---

## 3. Cryptographic Host-Key Verification

By default, the collector protects against Machine-In-The-Middle (MITM) attacks and DNS/IP spoofing by strictly verifying the storage controller's SSH host key.

### Secure Default Behavior & Resolution Order
`internal/ssh/client.go` defaults to `InsecureHostKey: false`. The collector builds an `ssh.HostKeyCallback` using `golang.org/x/crypto/ssh/knownhosts` with the following resolution order:

1. **Insecure Mode (`FS_INSECURE=true`)**: If explicitly enabled, returns `ssh.InsecureIgnoreHostKey()`.
2. **Explicit Known Hosts (`FS_KNOWN_HOSTS` / `-known-hosts`)**: Validates that the file is a regular file and enforces strict POSIX write permissions before parsing with `knownhosts.New()`.
3. **Automatic Fallback (`~/.ssh/known_hosts`)**: If `FS_KNOWN_HOSTS` is empty, attempts to locate and validate `~/.ssh/known_hosts` for the executing system user (`zabbix`).
4. **Strict Error**: If neither `FS_INSECURE=true` nor an accessible, valid `known_hosts` file exists, the collector aborts:
```text
SSH host key verification not configured: specify -known-hosts or FS_KNOWN_HOSTS
```

### Host Key File Integrity and Permission Enforcement
To prevent unauthorized users or compromised processes on a shared monitoring server from modifying trusted host keys to intercept FlashSystem monitoring traffic, `internal/config/config.go` and `internal/ssh/client.go` enforce strict integrity checks on `known_hosts`:

```go
// POSIX write permission check for known_hosts
if runtime.GOOS != "windows" && info.Mode().Perm()&0022 != 0 {
    return fmt.Errorf("known_hosts file %q has insecure permissions (%04o): "+
        "must not be writable by group or others (chmod 644 or 600 required)",
        path, info.Mode().Perm())
}
```

- **Regular File Check**: Symlinks, directories, FIFO pipes, or device files are rejected (`!info.Mode().IsRegular()`).
- **Permission Check**: Group and other write permissions are strictly forbidden (`perm & 0022 != 0`). Only `0644` (world-readable, owner-writable) or `0600` (strictly owner-only) are allowed. Permissions like `0664` or `0666` cause immediate startup failure.

### Setting Up `known_hosts` in Production
Use `ssh-keyscan` to capture the storage controller's public host key:
```bash
sudo -u zabbix ssh-keyscan -p 22 <FLASHYSTEM_IP> > /etc/zabbix/ssh/known_hosts
sudo chmod 0644 /etc/zabbix/ssh/known_hosts
sudo chown zabbix:zabbix /etc/zabbix/ssh/known_hosts
```

In your `/etc/zabbix/flashsystem/env` file, configure:
```bash
FS_KNOWN_HOSTS=/etc/zabbix/ssh/known_hosts
FS_INSECURE=false
```

### Insecure Testing Mode (`FS_INSECURE=true`)
In isolated sandbox or staging lab networks where DNS is absent and controllers are rebuilt frequently, host key checking can be explicitly bypassed:
```bash
# Permitted ONLY for lab verification:
FS_INSECURE=true
```
> 🛑 **Security Warning**: Setting `FS_INSECURE=true` in production exposes your monitoring infrastructure to active MITM attacks, DNS redirection, and credential interception. Never deploy `FS_INSECURE=true` in enterprise environments.

---

## 4. Least-Privilege Role Assignment on IBM FlashSystem

The collector connects to the FlashSystem CLI to run read-only diagnostic commands. It should never be granted administrative or security administrator privileges.

### IBM Storage User Configuration
Create a dedicated user account on the IBM FlashSystem restricted to the **`Monitor`** role:

```bash
# 1. SSH into the FlashSystem as superuser or SecurityAdmin:
ssh superuser@<FLASHSYSTEM_IP>

# 2. Create the monitoring user group (if not already present):
# Note: The built-in Monitor role grants read-only access to all svcinfo commands.

# 3. Create the user with the Monitor role:
mkuser -name zabbix_monitor -usergrp Monitor -keyfile /path/to/uploaded_key.pub
```

### Verifying Authorization
The `Monitor` role allows all `svcinfo` commands used by the collector:
- `svcinfo lssystem`
- `svcinfo lsnode`
- `svcinfo lsenclosure`
- `svcinfo lsdrive`
- `svcinfo lsmdiskgrp`
- `svcinfo lsvdisk`
- `svcinfo lsportfc`
- `svcinfo lsfcmap`
- `svcinfo lsreplicationrelationship`
- `svcinfo lssystemstats`
- `svcinfo lsenclosurebattery`
- `svcinfo lsenclosurepsu`

Any attempt to run destructive or modifying commands (e.g., `svctask rmvdisk` or `svctask chsystem`) is blocked by the storage system's Role-Based Access Control (RBAC).

---

## 5. Defense-in-Depth Checklist

| Layer | Hardening Action | Status / Command |
| :--- | :--- | :--- |
| **Storage RBAC** | Restrict user to `Monitor` role | `mkuser -name zabbix_monitor -usergrp Monitor` |
| **Authentication** | Enforce SSH key-based auth (forbid passwords) | `FS_KEYFILE=/etc/zabbix/ssh/flashsystem_key` |
| **File Permissions** | Private key restricted to `0600` | `chmod 0600 /etc/zabbix/ssh/flashsystem_key` |
| **Config Security** | Environment file restricted to `0640` | `chmod 0640 /etc/zabbix/flashsystem/env` |
| **Host Verification**| Populate and verify `known_hosts` (mode `0644`/`0600`) | `FS_KNOWN_HOSTS=/etc/zabbix/ssh/known_hosts` |
| **Process Locking** | Dedicated non-overlapping flock locks | `/var/lock/flashsystem/` (mode `0755`) |
| **Cache Protection** | Cache directory protected | `/var/tmp/` (mode `1777` with sticky bit) |
