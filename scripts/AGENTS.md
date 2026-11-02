# AGENTS.md — scripts Component Instructions

This directory contains the shell wrapper scripts for Zabbix integration.

---

## Technical Responsibilities

- **Zabbix ExternalScript Wrapper**: `scripts/flashsystem_collector.sh` provides the execution entry point invoked by Zabbix Server/Proxy external script checks.
- **Environment Loading**: Reads `/etc/zabbix/flashsystem/env` for credentials and environment settings (`FS_USER`, `FS_KEYFILE`, `FS_PASSWORD`, `FS_INSECURE`, `FS_CACHE_DIR`, `FS_METRICS_TTL`, `FS_DISCOVERY_TTL`, `FS_PERFORMANCE_TTL`, `FS_VERBOSE`).
- **Host Concurrency Locking**: Uses `flock` on `/var/lock/flashsystem/flashsystem_<sanitized_host>.lock` (55s timeout) to prevent overlapping poll runs on the same storage system.
- **Log Rotation**: Manages wrapper execution logging to `/var/log/zabbix/flashsystem_collector.log` with a 10MB rotation cap (`.log.1`).
- **Fail-Safe Output**: Guarantees JSON output on stdout even when external scripts fail or environment configuration is missing (`emit_error_json` or `emit_empty_lld`).

---

## Runtime Assumptions & System Dependencies

- **Shell**: Bash 4+ (`set -euo pipefail`, `mapfile`).
- **Positional Arguments**:
  - `$1`: Target storage host / IP (e.g., `10.10.10.50`).
  - `$2`: Operation mode (`collect` or `discover`).
  - `$3`: Discovery category (required only when mode is `discover`: `drives|pools|volumes|enclosures|nodes|ports|batteries|psus`).
- **Commands**: `flock`, `stat`, `date`, `mkdir`, `touch`, `mv`, `printf`.
- **System Paths**:
  - Collector binary: `/usr/local/bin/flashsystem-collector`
  - Env file: `/etc/zabbix/flashsystem/env`
  - Log file: `/var/log/zabbix/flashsystem_collector.log`
  - Lock directory: `/var/lock/flashsystem`

---

## Engineering Rules

1. **Stdout Safety**:
   - Never output non-JSON text or diagnostic logs to `stdout`. All logs must go to `stderr` or `${LOG_FILE}`.
   - On `collect` errors, emit a valid `Output` error JSON schema with `status.success: false`.
   - On `discover` errors, emit `{"data":[]}` so Zabbix discovery rules do not purge existing items.
2. **Bash Compatibility**:
   - Ensure scripts pass `shellcheck` linting.
   - Use double quotes around all array expansions and variables.
3. **Permissions**:
   - Installed wrapper script must be executable (`chmod 755`) and owned by `zabbix:zabbix`.
   - Credentials file `/etc/zabbix/flashsystem/env` must have restricted permissions (`chmod 640`, `zabbix:zabbix`).

---

## Testing Guidelines

- Test wrapper execution in both `collect` and `discover` modes across all 8 discovery categories.
- Verify environment loading when `/etc/zabbix/flashsystem/env` is present or missing.
- Test missing binary behavior and missing configuration behavior.
- Test lock file creation and timeout handling under concurrent invocations.
