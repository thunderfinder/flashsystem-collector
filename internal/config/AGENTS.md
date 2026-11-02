# AGENTS.md — internal/config Component Instructions

This directory handles configuration parsing, environment loading, and validation for `flashsystem-collector`.

---

## Technical Responsibilities

- **Config Struct Definition**: Defines `Config`, `SSHConfig`, `CacheConfig`, `LimitsConfig`, and `LogConfig`.
- **Hierarchical Loading**: Loads defaults first, applies CLI flags, and falls back to environment variables (`FS_*`) if flags are unset.
- **Strict Validation**: Validates all configuration parameters before execution starts (`cfg.validate()`).
- **Cache Path Resolver**: Generates sanitized host-specific cache file paths (`CacheFile(host)`).

---

## Configuration Hierarchy & Precedence

1. **CLI Flags** (highest priority)
2. **Environment Variables** (`FS_*`) (medium priority)
3. **Default Values** (lowest priority)

### Environment Variables & Flag Mappings
| Flag | Variable | Struct Mapping | Default | Constraints |
|------|----------|----------------|---------|-------------|
| `-host` | `FS_HOST` | `SSH.Host` | `""` | Required |
| `-port` | `FS_PORT` | `SSH.Port` | `22` | $1 \le \text{Port} \le 65535$ |
| `-user` | `FS_USER` | `SSH.User` | `""` | Required |
| `-password` | `FS_PASSWORD` | `SSH.Password` | `""` | Mutually exclusive with `KeyFile` |
| `-keyfile` | `FS_KEYFILE` | `SSH.KeyFile` | `""` | Mutually exclusive with `Password`; POSIX `0600` (`perm & 0077 == 0`) |
| `-insecure` | `FS_INSECURE` | `SSH.InsecureHostKey` | `false` | Default `false`. If true, bypasses host key checks |
| `-known-hosts` | `FS_KNOWN_HOSTS` | `SSH.KnownHostsFile` | `""` | Must be regular file; no group/other write (`perm & 0022 == 0`) |
| `-cache-dir` | `FS_CACHE_DIR` | `Cache.Dir` | `/var/tmp` | Directory must exist, be writable |
| `-metrics-ttl` | `FS_METRICS_TTL` | `Cache.MetricsTTL` | `5m` | Duration $\ge 1\text{s}$ |
| `-discovery-ttl` | `FS_DISCOVERY_TTL` | `Cache.DiscoveryTTL` | `4h` | Duration $\ge 1\text{m}$ |
| `-performance-ttl` (alias `-perf-ttl`) | `FS_PERFORMANCE_TTL` (alias `FS_PERF_TTL`) | `Cache.PerformanceTTL` | `1m` | Duration $\ge 1\text{s}$ |
| `-dial-timeout` | `FS_DIAL_TIMEOUT` | `SSH.DialTimeout` | `10s` | Duration $\ge 1\text{s}$ |
| `-cmd-timeout` | `FS_CMD_TIMEOUT` | `SSH.CommandTimeout` | `30s` | Duration $\ge 1\text{s}$ |
| `-max-json-bytes` | `FS_MAX_JSON_BYTES` | `Limits.MaxJSONBytes` | `1048576` (1MB) | Min `65536` (64KB) |
| `-max-volumes` | `FS_MAX_VOLUMES` | `Limits.MaxVolumes` | `500` | Min `1` |
| `-max-drives` | `FS_MAX_DRIVES` | `Limits.MaxDrives` | `500` | Min `1` |
| `-verbose` | `FS_VERBOSE` | `Logging.Verbose` | `false` | Boolean |

---

## Validation & Safety Rules

1. **Authentication Mutual Exclusivity**:
   - Exactly one of `KeyFile` or `Password` must be provided. Providing both or neither returns a validation error.
2. **Private Key File Permissions**:
   - `KeyFile` must be a regular file (`os.Stat(KeyFile).Mode().IsRegular()`).
   - Private key must strictly forbid group and other permissions: `info.Mode().Perm() & 0077 == 0` (e.g., `0600`).
3. **Known Hosts File Permissions & Integrity**:
   - When `-known-hosts` (`FS_KNOWN_HOSTS`) is supplied, it must be a regular file.
   - It must forbid group and other write permissions: `info.Mode().Perm() & 0022 == 0` (e.g., `0644` or `0600`) to prevent MITM tampering.
4. **Cache Directory Validation**:
   - `validateCacheDir` verifies that the cache directory exists, is a directory, and creates/deletes a temporary file (`.flashsystem_write_test_*`) to confirm write permissions.
5. **Sanitized File Paths**:
   - `CacheFile(host)` replaces `:`, `/`, `\`, and space characters with `_` to generate safe filenames: `<cache-dir>/flashsystem_<sanitized_host>.cache.json`.

---

## Engineering Rules

- When adding new configuration parameters:
  1. Add the field to the appropriate config struct (`SSHConfig`, `CacheConfig`, `LimitsConfig`, or `LogConfig`).
  2. Set safe defaults in `defaultConfig()`.
  3. Declare the CLI flag in `Load()`.
  4. Implement environment variable handling in `applyEnv()`.
  5. Add validation rules in `validate()`.
  6. Update usage docs in `main.go`, `README.md`, `instructions.md`, and relevant wiki pages.

---

## Testing Guidelines

- Test `Load()` with various combinations of CLI flags, environment variables, and missing mandatory values.
- Test validation failures (invalid SSH port, missing host/user, both keyfile and password set, non-existent keyfile path, insecure file permissions).
- Test duration env variable parsing (`parseDurationEnv`) including aliases (`-performance-ttl` and `-perf-ttl`).
- Test `CacheFile()` hostname sanitization.
