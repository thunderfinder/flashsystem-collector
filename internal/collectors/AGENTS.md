# AGENTS.md — internal/collectors Component Instructions

This directory contains the `Collector` interface, individual resource collectors (12 total), and the concurrent `Runner` orchestration logic.

---

## Technical Responsibilities

- **Collector Interface**: Each IBM storage resource implements `Collector`:
  - `Name() string`: Unique string identifier (e.g., `"system"`, `"volumes"`, `"drives"`, `"batteries"`, `"psus"`).
  - `Collect(client *internalssh.Client) ([]parser.Record, error)`: Executes `svcinfo` command via SSH and returns parsed key-value map records.
  - `TTL() time.Duration`: Defines the cache validity duration for this resource (`metricsTTL`, `discoveryTTL`, or `performanceTTL`).
- **Runner Orchestration**: `Runner.RunAll()` executes registered collectors in parallel with semaphore concurrency throttling (`MaxConcurrent: 3`).
- **Cache Lifecycle**: Checks cache first before executing SSH commands; stores successful results; returns stale cache on SSH failures.
- **Fallback Logic**: Handles firmware differences (e.g., modern `lsreplicationrelationship` falling back to legacy `lsrcrelationship`).

---

## Registered Collectors & Commands

All 12 resource collectors must be instantiated and registered in `AllCollectors(metricsTTL, discoveryTTL, performanceTTL)`:

| Collector | Resource Name | CLI Command | TTL Category |
|-----------|---------------|-------------|--------------|
| System | `system` | `svcinfo lssystem -delim :` | `metricsTTL` (5m) |
| Nodes | `nodes` | `svcinfo lsnode -delim :` | `discoveryTTL` (4h) |
| Enclosures | `enclosures` | `svcinfo lsenclosure -delim :` | `discoveryTTL` (4h) |
| Drives | `drives` | `svcinfo lsdrive -delim :` | `discoveryTTL` (4h) |
| Pools | `pools` | `svcinfo lsmdiskgrp -bytes -delim :` | `discoveryTTL` (4h) |
| Volumes | `volumes` | `svcinfo lsvdisk -bytes -delim :` | `discoveryTTL` (4h) |
| Ports | `ports` | `svcinfo lsportfc -delim :` | `discoveryTTL` (4h) |
| FlashCopy | `flashcopy` | `svcinfo lsfcmap -delim :` | `metricsTTL` (5m) |
| Replication | `replication` | `svcinfo lsreplicationrelationship -delim :` (Fallback: `svcinfo lsrcrelationship -delim :`) | `metricsTTL` (5m) |
| Performance | `performance` | `svcinfo lssystemstats -delim :` | `performanceTTL` (1m) |
| Batteries | `batteries` | `svcinfo lsenclosurebattery -delim :` | `discoveryTTL` (4h) |
| PSUs | `psus` | `svcinfo lsenclosurepsu -delim :` | `discoveryTTL` (4h) |

---

## Engineering Rules

1. **Adding or Modifying a Collector**:
   - Create or update the resource file in `internal/collectors/<resource>.go`.
   - Implement the `Collector` interface (`Name`, `Collect`, `TTL`).
   - Register the constructor in `AllCollectors(...)` in `collector.go`.
   - Update discovery mapping (if applicable) in `main.go` and `internal/zabbix/output.go`.
2. **Concurrency Safety**:
   - Do NOT open new SSH client connections within individual collectors. Use the passed `*internalssh.Client`.
   - Respect `RunnerConfig.MaxConcurrent` (default: 3) to prevent SSH session exhaustion on FlashSystem controllers.
3. **Error Isolation**:
   - A single failing collector must never halt other collectors or cause a panic.
   - Return errors explicitly from `Collect()`; `Runner` captures them into `Result.Error`.
4. **Data Contract**:
   - `Collect()` must return `[]parser.Record` where `parser.Record` is `map[string]string`.
   - If no resources are found (empty system response or CMMVC code), return an empty slice `[]parser.Record{}`, not `nil`.
5. **Exact Capacities**:
   - `pools` (`lsmdiskgrp`) and `volumes` (`lsvdisk`) must execute with `-bytes -delim :` to retrieve exact 64-bit integer capacities.
   - `drives` (`lsdrive`) must format capacities using `parser.FormatCapacity` (base-2 binary: GB if $< 1\text{ TB}$, TB if $\ge 1\text{ TB}$).

---

## Testing Guidelines

- Mock SSH execution responses using `internalssh.Client` test helpers or string parser fixtures.
- Test fallback paths (e.g., replication command fallback on older firmware error codes).
- Test concurrent runner execution and cache hit/miss/stale fallback logic.
- Verify capacity formatting for drives across boundary values (500 GB, 1 TB, 1.92 TB, 3.84 TB).
