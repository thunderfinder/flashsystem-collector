# Bolt ⚡ Performance Journal

## 2025-05-18 - Parser Map Capacity Sizing and Slice Reuse

**Learning:** In `internal/parser`, `buildRecord` constructs a `Record` (`map[string]string`) per row in tabular output (e.g., hundreds or thousands of rows for `lsvdisk`, `lsdrive`). Initializing maps as `Record{}` without capacity hints forces the Go runtime to start with zero buckets and incrementally reallocate and rehash hashmap buckets as keys are populated (typically 5-30 columns per FlashSystem command). Pre-sizing `make(Record, len(headers))` reduces garbage collector pressure and allocation overhead significantly across all 10 collectors.

**Action:** Always pre-size map allocations when row column count or line count is known ahead of time. Filter slices in-place using existing backing arrays when creating filtered views.
