# Configuration ⚙️

Configuration for `flashsystem-collector` follows a strict precedence model:
$$\mathbf{CLI\ Flags} > \mathbf{Environment\ Variables\ (FS\_*)} > \mathbf{Default\ Values}$$

Any parameter passed directly as a command-line flag will override the corresponding environment variable. If neither is specified, the internal default value is used.

---

## 1. Complete Configuration Reference

| CLI Flag | Environment Variable | Default Value | Accepted Values / Format | Description & Validation |
| :--- | :--- | :--- | :--- | :--- |
| `-host` | `FS_HOST` | *(empty)* | String (IP or DNS) | **Required**. FlashSystem cluster management IP address or FQDN. |
| `-port` | `FS_PORT` | `22` | Integer (`1`–`65535`) | SSH service port on FlashSystem management controller. |
| `-user` | `FS_USER` | *(empty)* | String | **Required**. FlashSystem user account (recommended: `Monitor` role). |
| `-keyfile` | `FS_KEYFILE` | *(empty)* | File path | Path to OpenSSH private key file. Must have `chmod 0600` permissions. Mutually exclusive with `-password`. |
| `-password` | `FS_PASSWORD` | *(empty)* | String | Storage user password. **Insecure** (visible in process lists). Mutually exclusive with `-keyfile`. |
| `-known-hosts`| `FS_KNOWN_HOSTS` | *(empty)* | File path | Path to `known_hosts` file for cryptographic SSH host-key validation. |
| `-insecure` | `FS_INSECURE` | `false` | Boolean (`true`, `false`, `1`, `0`) | Disables SSH host-key checking. **Permitted only in isolated lab environments**. |
| `-cache-dir` | `FS_CACHE_DIR` | `/var/tmp` | Directory path | Writable directory storing JSON cache files. Validated on startup with a write probe. |
| `-metrics-ttl`| `FS_METRICS_TTL` | `5m` | Duration ($\ge 1\text{s}$, e.g. `5m`, `300s`) | Cache validity duration for general inventory, nodes, pools, and volumes. |
| `-discovery-ttl`| `FS_DISCOVERY_TTL` | `4h` | Duration ($\ge 1\text{m}$, e.g. `4h`, `240m`) | Cache validity duration for LLD discovery scans (drives, enclosures, ports). |
| `-performance-ttl` / `-perf-ttl` | `FS_PERFORMANCE_TTL` / `FS_PERF_TTL` | `1m` | Duration ($\ge 1\text{s}$, e.g. `1m`, `60s`) | Cache validity duration for real-time I/O performance telemetry (`lssystemstats`). |
| `-dial-timeout`| `FS_DIAL_TIMEOUT` | `10s` | Duration ($\ge 1\text{s}$, e.g. `10s`) | Network dial timeout for TCP connection and SSH handshake. |
| `-cmd-timeout` | `FS_CMD_TIMEOUT` | `30s` | Duration ($\ge 1\text{s}$, e.g. `30s`) | Execution timeout per individual storage CLI command. |
| `-max-json-bytes`| `FS_MAX_JSON_BYTES` | `1048576` (1MB)| Integer ($\ge 1024$) | Upper limit on stdout JSON payload size before truncation logic triggers. |
| `-max-volumes` | `FS_MAX_VOLUMES` | `500` | Integer ($\ge 1$) | Maximum volume records included in stdout JSON array. |
| `-max-drives` | `FS_MAX_DRIVES` | `500` | Integer ($\ge 1$) | Maximum physical drive records included in stdout JSON array. |
| `-verbose` | `FS_VERBOSE` | `false` | Boolean (`true`, `false`, `1`, `0`) | Enables detailed execution and diagnostic logging to `stderr`. |
| `-category` | *(none)* | *(empty)* | String (Category name) | **Required for `discover`**. One of: `drives`, `pools`, `volumes`, `enclosures`, `nodes`, `ports`, `batteries`, `psus`. |

---

## 2. Environment Variables Specification

The wrapper script loads `/etc/zabbix/flashsystem/env` before invoking the Go binary.

### Example Environment Configuration (`/etc/zabbix/flashsystem/env`)

```bash
# Storage Credentials
FS_USER=zabbix_monitor
FS_KEYFILE=/etc/zabbix/ssh/flashsystem_key

# Host Verification (Mandatory for secure production setups)
FS_KNOWN_HOSTS=/etc/zabbix/ssh/known_hosts
FS_INSECURE=false

# Cache Configuration
FS_CACHE_DIR=/var/tmp
FS_METRICS_TTL=5m
FS_DISCOVERY_TTL=4h
FS_PERFORMANCE_TTL=1m

# Safeguards & Timeouts
FS_MAX_JSON_BYTES=1048576
FS_MAX_VOLUMES=500
FS_MAX_DRIVES=500
FS_DIAL_TIMEOUT=10s
FS_CMD_TIMEOUT=30s
FS_VERBOSE=false
```

---

## 3. Strict Validation & Error Handling

During initialization, the collector executes rigorous validation logic:

1. **Authentication Mutual Exclusivity**:
   - If neither `FS_KEYFILE` nor `FS_PASSWORD` is configured: returns `"SSH authentication required: use -keyfile or -password flag"`.
   - If both `FS_KEYFILE` and `FS_PASSWORD` are configured: returns `"specify either -keyfile or -password, not both"`.
2. **Private Key File Permissions**:
   - `internal/config/config.go` verifies that `KeyFile` is a regular file and that its POSIX permission mode forbids group and other read/write access:
     ```go
     if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
         return fmt.Errorf("SSH key file %q has insecure permissions (%04o): must not be accessible by group or others (chmod 600 required)", cfg.SSH.KeyFile, info.Mode().Perm())
     }
     ```
3. **Host Key Security**:
   - If `InsecureHostKey` is `false` and `FS_KNOWN_HOSTS` is empty: returns `"SSH host key verification not configured"`.
4. **Cache Directory Validation**:
   - Checks that `CacheDir` exists, is a directory, and creates a transient file `.flashsystem_write_test_*` to verify write permissions before accepting the path.

---

## 4. Cache Path Generation

The collector stores JSON cache files using the sanitized host identifier:
```text
<FS_CACHE_DIR>/flashsystem_<sanitized_host>.cache.json
```
Characters `:`, `/`, `\`, and spaces are replaced with underscores `_`. For example:
- Host `10.10.10.50` produces `/var/tmp/flashsystem_10.10.10.50.cache.json`
- Host `fs9200-cluster.corp.lan` produces `/var/tmp/flashsystem_fs9200-cluster.corp.lan.cache.json`
