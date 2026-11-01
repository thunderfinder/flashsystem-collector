# Bolt ⚡ Performance Journal

## 2025-05-18 - Parser Map Capacity Sizing and Slice Reuse

**Learning:** In `internal/parser`, `buildRecord` constructs a `Record` (`map[string]string`) per row in tabular output (e.g., hundreds or thousands of rows for `lsvdisk`, `lsdrive`). Initializing maps as `Record{}` without capacity hints forces the Go runtime to start with zero buckets and incrementally reallocate and rehash hashmap buckets as keys are populated (typically 5-30 columns per FlashSystem command). Pre-sizing `make(Record, len(headers))` reduces garbage collector pressure and allocation overhead significantly across all 10 collectors.

**Action:** Always pre-size map allocations when row column count or line count is known ahead of time. Filter slices in-place using existing backing arrays when creating filtered views.

## 2026-11-01 - Zero-Allocation Empty/Error Filtering in Tabular Parsers

**Learning:** `isNoResults` in `internal/parser` was calling `strings.ToLower(strings.TrimSpace(line))` on every row of tabular command outputs (such as `lsvdisk` or `lsdrive`, which can have hundreds or thousands of rows). In Go, `strings.ToLower` allocates a brand-new string on the heap for every line examined (~128 bytes/row). Replacing `strings.ToLower` with case-insensitive `strings.EqualFold` and length-checked prefix slicing (`len(line) >= 5 && strings.EqualFold(line[:5], "cmmvc")`) completely eliminates heap allocations (0 allocs/op, 0 B/op vs 1 alloc/op, 128 B/op) and speeds up the check by ~18x (~22.7 ns vs ~416 ns), saving N allocations for an N-row table.

**Action:** Prefer `strings.EqualFold` over `strings.ToLower` when comparing strings case-insensitively in hot parsing loops, and guard prefix checks with string length checks before slicing to preserve zero heap allocations.
