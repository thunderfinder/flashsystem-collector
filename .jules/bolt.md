# Bolt ⚡ Performance Journal

## 2025-05-18 - Parser Map Capacity Sizing and Slice Reuse

**Learning:** In `internal/parser`, `buildRecord` constructs a `Record` (`map[string]string`) per row in tabular output (e.g., hundreds or thousands of rows for `lsvdisk`, `lsdrive`). Initializing maps as `Record{}` without capacity hints forces the Go runtime to start with zero buckets and incrementally reallocate and rehash hashmap buckets as keys are populated (typically 5-30 columns per FlashSystem command). Pre-sizing `make(Record, len(headers))` reduces garbage collector pressure and allocation overhead significantly across all 10 collectors.

**Action:** Always pre-size map allocations when row column count or line count is known ahead of time. Filter slices in-place using existing backing arrays when creating filtered views.

## 2026-11-01 - Zero-Allocation Empty/Error Filtering in Tabular Parsers

**Learning:** `isNoResults` in `internal/parser` was calling `strings.ToLower(strings.TrimSpace(line))` on every row of tabular command outputs (such as `lsvdisk` or `lsdrive`, which can have hundreds or thousands of rows). In Go, `strings.ToLower` allocates a brand-new string on the heap for every line examined (~128 bytes/row). Replacing `strings.ToLower` with case-insensitive `strings.EqualFold` and length-checked prefix slicing (`len(line) >= 5 && strings.EqualFold(line[:5], "cmmvc")`) completely eliminates heap allocations (0 allocs/op, 0 B/op vs 1 alloc/op, 128 B/op) and speeds up the check by ~18x (~22.7 ns vs ~416 ns), saving N allocations for an N-row table.

**Action:** Prefer `strings.EqualFold` over `strings.ToLower` when comparing strings case-insensitively in hot parsing loops, and guard prefix checks with string length checks before slicing to preserve zero heap allocations.

## 2026-11-02 - Zero-Allocation Field Scanning in Tabular Parsers

**Learning:** In `internal/parser`, `buildRecord` previously called `strings.SplitN(line, delim, len(headers))` for each data row in colon-separated tabular outputs (`lsvdisk`, `lsdrive`, `lssystemstats`, etc.). For large systems with hundreds of volumes or drives (e.g. 500 rows with 23 columns), `strings.SplitN` allocated a slice header and backing array on the heap for every single row (500 heap slice allocations per parse, totaling ~192 KB and 21.57% of memory churn). Replacing `strings.SplitN` with iterative in-place delimiter index scanning directly into `record[header]` completely eliminates the intermediate slice allocation per row (saving exactly N allocations for N rows, reducing allocations on 500 rows from 2504 to 2004, memory from ~825 KB to ~633 KB (-23.3%), and runtime from ~1.10 ms to ~0.91 ms (-17%)).

**Action:** Avoid allocating intermediate `[]string` slices with `strings.Split` / `strings.SplitN` inside hot row parsing loops; use in-place delimiter scanning via `strings.Index` to populate destination maps or structures directly.
