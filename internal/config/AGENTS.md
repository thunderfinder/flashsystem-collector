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

### Environment Variables
| Variable | Struct Mapping | Default |
|----------|----------------|---------|
| `FS_HOST` | `SSH.Host` | `""` |
| `FS_PORT` | `SSH.Port` | `22` |
| `FS_USER` | `SSH.User` | `""` |
| `FS_PASSWORD` | `SSH.Password` | `""` |
| `FS_KEYFILE` | `SSH.KeyFile` | `""` |
| `FS_INSECURE` | `SSH.InsecureHostKey` | `false` |
| `FS_KNOWN_HOSTS` | `SSH.KnownHostsFile` | `""` |
| `FS_CACHE_DIR` | `Cache.Dir` | `/var/tmp` |
| `FS_METRICS_TTL` | `Cache.MetricsTTL` | `5m` |
| `FS_DISCOVERY_TTL` | `Cache.DiscoveryTTL` | `4h` |
| `FS_PERF_TTL` | `Cache.PerformanceTTL` | `60s` |
| `FS_DIAL_TIMEOUT` | `SSH.DialTimeout` | `10s` |
| `FS_CMD_TIMEOUT` | `SSH.CommandTimeout` | `30s` |
| `FS_MAX_JSON_BYTES` | `Limits.MaxJSONBytes` | `1048576` (1MB) |
| `FS_MAX_VOLUMES` | `Limits.MaxVolumes` | `500` |
| `FS_MAX_DRIVES` | `Limits.MaxDrives` | `500` |
| `FS_VERBOSE` | `Logging.Verbose` | `false` |

---

## Validation & Safety Rules

1. **Authentication Mutual Exclusivity**:
   - Exactly one of `KeyFile` or `Password` must be provided. Setting both or neither returns a validation error.
   - If `KeyFile` is provided, `os.Stat(KeyFile)` must succeed.
2. **Host Key Safety**:
   - `InsecureHostKey` default is `false`.
   - If `!InsecureHostKey` and `KnownHostsFile` is provided, `os.Stat(KnownHostsFile)` must succeed.
3. **Cache Directory Validation**:
   - `validateCacheDir` verifies that the cache directory exists, is a directory, and creates/deletes a temporary file (`.flashsystem_write_test_*`) to confirm write permissions.
4. **Sanitized File Paths**:
   - `CacheFile(host)` replaces `:`, `/`, `\`, and space characters with `_` to generate safe filenames: `<cache-dir>/flashsystem_<sanitized_host>.cache.json`.

---

## Engineering Rules

- When adding new configuration parameters:
  1. Add the field to the appropriate config struct (`SSHConfig`, `CacheConfig`, `LimitsConfig`, or `LogConfig`).
  2. Set safe defaults in `defaultConfig()`.
  3. Declare the CLI flag in `Load()`.
  4. Implement environment variable handling in `applyEnv()`.
  5. Add validation rules in `validate()`.
  6. Update usage docs in `main.go`, `README.md`, and `instructions.md`.

---

## Testing Guidelines

- Test `Load()` with various combinations of CLI flags, environment variables, and missing mandatory values.
- Test validation failures (invalid SSH port, missing host/user, both keyfile and password set, non-existent keyfile path).
- Test duration env variable parsing (`parseDurationEnv`).
- Test `CacheFile()` hostname sanitization.
