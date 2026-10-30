# AGENTS.md — internal/parser Component Instructions

This directory handles text parsing of colon-delimited IBM FlashSystem CLI (`svcinfo -delim :`) command outputs.

---

## Technical Responsibilities

- **Dual-Format Detection**: Automatically distinguishes between **Vertical** key-value format (e.g., `lssystem`) and **Tabular** multi-column matrix format (e.g., `lsvdisk`, `lsdrive`).
- **Delimiter Parsing**: Parses colon (`:`) delimited strings into `Record` instances (`map[string]string`).
- **Line Ending Normalization**: Handles Linux (`\n`), Windows (`\r\n`), and legacy Mac (`\r`) line endings transparently.
- **Error Line Filtering**: Identifies IBM error strings and empty responses (e.g., `"CMMVC5845E"`, `"no results"`) and normalizes them to empty record sets instead of failing.

---

## Output Formats & Parsing Strategies

### 1. Vertical Format (`FormatVertical`)
- **Structure**: Each line contains `key:value`.
- **Example (`lssystem -delim :`)**:
  ```text
  id:0
  name:FlashSystem_5045
  location:local
  ```
- **Special Logic**: Split line only on the *first* colon (`strings.Index(line, ":")`). This preserves colons inside values (e.g., timestamps or firmware string versions `code_level:8.7.0.0 (build 123.456)`).

### 2. Tabular Format (`FormatTabular`)
- **Structure**: First line contains column headers separated by colons (`:`). Remaining lines contain resource row values.
- **Example (`lsdrive -delim :`)**:
  ```text
  id:enclosure_id:slot_id:status:capacity
  0:1:1:online:1.2TB
  1:1:2:online:1.2TB
  ```
- **Special Logic**: `strings.SplitN(line, ":", len(headers))` splits row values up to the number of headers. Any additional colons in the last column are preserved.

---

## Engineering Rules

1. **Return Type Consistency**:
   - `Parse()` must always return `Result` with `Records` initialized as `[]Record` (never `nil`).
   - `Record` map values must remain `string`. Collectors retain responsibility for string formatting.
2. **Dynamic Header Mapping**:
   - Never assume column indices in Tabular output. Dynamic column mapping using header row names is required to support firmware version differences across FlashSystem models.
3. **No Panic Guarantee**:
   - Parsers must be resilient against malformed CLI outputs, truncated lines, or blank lines.
4. **IBM Storage CLI Error Codes**:
   - Extend `isNoResults()` if new IBM storage CLI empty/error response prefixes are encountered (e.g., `CMMVC...`).

---

## Testing Guidelines

- Provide test fixtures representing realistic `svcinfo` CLI output for all 10 collectors.
- Test edge cases: empty strings, single blank lines, `CMMVC` error messages, lines with trailing/embedded colons.
- Test `ParseVerticalForced` for strict single-record commands.
