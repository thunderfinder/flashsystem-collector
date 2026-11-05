# AGENTS.md — internal/cache Component Instructions

This directory implements the disk-backed, thread-safe, persistent JSON caching layer for `flashsystem-collector`.

---

## Technical Responsibilities

- **Thread-Safe In-Memory Cache**: Manages cache entries in memory guarded by `sync.RWMutex`.
- **Atomic Disk Persistence**: Writes cache state atomically using temporary files (`.flashsystem_cache_*.tmp`) followed by `os.Rename()` to prevent file corruption during unexpected crashes or signals.
- **Multi-Tier TTL & Expiration**: Manages per-entry expiration timestamps (`ExpiresAt`) across three TTL tiers:
  - Metrics TTL (default 5m)
  - Discovery TTL (default 4h)
  - Performance TTL (default 1m)
- **Stale Data Fallback**: Provides `GetStale(key)` to retrieve expired entries when SSH connectivity to the storage system fails.
- **Cache File Security**: Enforces strict POSIX permissions (`0600`) upon cache write/rename and validates that existing cache files are regular files without group/world permissions (`perm & 0077 == 0`). Prevents path traversal via token sanitization.

---

## Architecture & Data Contracts

### Key Structures
- `Cache`: Primary interface struct holding filename, lock (`sync.RWMutex`), and `CacheFile` payload.
- `CacheFile`: Serialized JSON payload containing `version: "1"`, `updated_at`, and `entries: map[string]CacheEntry`.
- `CacheEntry`: Stores `Key`, `Value` (`raw json.RawMessage`), `CachedAt`, and `ExpiresAt`.

### Key Functions
- `MakeKey(host, collector)` -> returns `"<host>:<collector>"`
- `MakeDiscoveryKey(host, category)` -> returns `"<host>:discovery:<category>"`
- `Get(key)` -> Returns entry if found and `time.Now().Before(ExpiresAt)`.
- `GetStale(key)` -> Returns entry if found regardless of expiration status. Caller verifies entry age against `maxStaleAge` (24h).
- `Set(key, value, ttl)` -> Serializes value, sets expiration, updates map, and flushes atomically to disk.
- `Purge()` -> Removes all expired entries from disk state. Invoked by caller (`main.go`) only after a successful SSH connection to safeguard stale fallback.
- `Delete(key)` -> Removes a specific key from the cache.
- `Len()` -> Returns the count of entries in memory.

---

## Engineering & Maintenance Rules

1. **Atomic File Writes**:
   - Always write new cache contents to a temporary file created in the same target directory (`os.CreateTemp(dir, ".flashsystem_cache_*.tmp")`) before performing `os.Rename()`.
   - Never write directly to the destination cache file.
2. **Serialization Compatibility**:
   - Maintain `Version = "1"` compatibility. If cache structure changes incompatibly, handle version checks gracefully by resetting outdated cache without panicking.
3. **Thread Safety**:
   - Hold `c.mu.RLock()` for read operations (`Get`, `GetStale`, `Stats`).
   - Hold `c.mu.Lock()` for write operations (`Set`, `Purge`, disk saves).
4. **Error Resilience**:
   - Cache errors must never crash the collector. If cache reads/writes fail, log warning/debug messages and permit execution to proceed via direct SSH query.
5. **Lifecycle & Purge Timing**:
   - Never call `Purge()` before verifying SSH reachability. Calling `Purge()` prematurely wipes stale entries needed for emergency fallback if SSH fails.
6. **Stale Age Limit Contract**:
   - Consumers of `GetStale()` must enforce a maximum stale age boundary (`24 * time.Hour`). Stale data older than 24 hours must be rejected.

---

## Testing Guidelines

- Test atomic write and rename behavior on POSIX filesystems.
- Test thread safety with concurrent readers and writers using `go test -race ./internal/cache/...`.
- Test `Get` (cache hit/miss/expired) and `GetStale` (expired cache recovery).
- Test corrupted cache file handling to verify graceful reset.
