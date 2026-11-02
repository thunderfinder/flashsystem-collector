# AGENTS.md — internal/parser Component Instructions

This directory contains the parsing logic that converts colon-delimited (`svcinfo -delim :`) CLI output from IBM FlashSystem into structured key-value maps (`[]Record`).

---

## Technical Responsibilities

- **Format Detection**: Detects and parses two distinct FlashSystem CLI output layouts:
  - **Vertical Key-Value Layout**: Used by single-object queries (e.g., `lssystem`). Each line contains `key:value`.
  - **Tabular Matrix Layout**: Used by list queries (e.g., `lsvdisk`, `lsdrive`). Line 1 contains column headers; subsequent lines contain row values.
- **Spectrum Virtualize Normalization**: Normalizes IBM error and empty-result codes (e.g., `CMMVC5845E`, `cmmvc...`, `no results`, `no objects found`) into empty record sets (`[]Record{}`) rather than errors or nil.
- **Binary Capacity Formatting**: Exposes `FormatCapacity(bytesStr)` to format drive capacities into human-readable binary units (base-2, 1024): values $< 1\text{ TB}$ in `GB`, $\ge 1\text{ TB}$ in `TB`.

---

## Parser Logic & Format Detection Rules

1. **Format Detection (`detectFormat`)**:
   - Skips leading blank lines.
   - If line 1 does not contain `:`, it is treated as a single header for tabular layout.
   - If line 1 contains `:`, but line 2 has the same number of colon-delimited fields as line 1, it is detected as **Tabular**.
   - If line 1 contains `:` and line 2 has a different field count, or only 1 line exists with a colon, it is detected as **Vertical**.
2. **Vertical Parsing**:
   - Splits on the *first* colon (`strings.Index(line, ":")`) to preserve subsequent colons within values (e.g., timestamps, firmware build strings like `code_level:8.7.0.0 (build 123.456)`).
3. **Tabular Parsing**:
   - Headers are read from line 1 (`strings.Split(headersLine, ":")`).
   - Slices row values using `strings.SplitN(line, ":", len(headers))` to preserve extra colons in the final column.
   - Missing columns in a row safely default to empty string (`""`).

---

## Performance Rules (Bolt Guidelines)

1. **Map Capacity Sizing**:
   - In `buildRecord`, pre-allocate `Record` maps with known header capacity: `make(Record, len(headers))`.
   - Never initialize `Record{}` without capacity hints in hot row-parsing loops.
2. **Zero-Allocation String Checks**:
   - In `isNoResults`, avoid heap allocations from `strings.ToLower(line)`. Use zero-allocation `strings.EqualFold` and length-guarded prefix checks (`len(line) >= 5 && strings.EqualFold(line[:5], "cmmvc")`).

---

## Engineering Rules

1. **Return Type Consistency**:
   - `Parse()` must always return `Result` with `Records` initialized as `[]Record` (never `nil`).
   - `Record` map values must remain `string`.
2. **Dynamic Header Mapping**:
   - Never assume hardcoded column index offsets. Always resolve columns dynamically by header name to support firmware variations across models.
3. **No Panic Guarantee**:
   - Parsers must be resilient against malformed CLI outputs, truncated lines, or trailing blank lines.

---

## Testing Guidelines

- Provide test fixtures representing realistic `svcinfo` CLI output for all 12 collectors.
- Test edge cases: empty strings, single blank lines, `CMMVC` error messages, lines with trailing/embedded colons.
- Test `ParseVerticalForced` for strict single-record commands.
- Test `FormatCapacity` edge cases (0, 600GB, 1024GB, 1.92TB, 3.84TB, non-numeric input).
