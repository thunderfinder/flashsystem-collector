# AGENTS.md — internal/collectors Component Instructions

This directory contains the `Collector` interface, individual resource collectors, and the concurrent `Runner` orchestration logic.

---

## Technical Responsibilities

- **Collector Interface**: Each IBM storage resource implements `Collector`:
  - `Name() string`: Unique string identifier (e.g., `"system"`, `"volumes"`, `"drives"`).
  - `Collect(client *internalssh.Client) ([]parser.Record, error)`: Executes `svcinfo` command via SSH and returns parsed key-value map records.
  - `TTL() time.Duration`: Defines the cache validity duration for this resource.
- **Runner Orchestration**: `Runner.RunAll()` executes registered collectors in parallel with semaphore concurrency throttling (`MaxConcurrent: 3`).
- **Cache Lifecycle**: Checks cache first before executing SSH commands; stores successful results; returns stale cache on SSH failures.
- **Fallback Logic**: Handles firmware differences (e.g., modern `lsreplicationrelationship` falling back to legacy `lsrcrelationship`).

---

## Registered Collectors & Commands

All resource collectors must be instantiated and registered in `AllCollectors(metricsTTL, discoveryTTL, performanceTTL)`:

| Collector | Resource Name | CLI Command |
|-----------|---------------|-------------|
| System | `system` | `svcinfo lssystem -delim :` |
| Nodes | `nodes` | `svcinfo lsnode -delim :` |
| Enclosures | `enclosures` | `svcinfo lsenclosure -delim :` |
| Drives | `drives` | `svcinfo lsdrive -delim :` |
| Pools | `pools` | `svcinfo lsmdiskgrp -delim :` |
| Volumes | `volumes` | `svcinfo lsvdisk -delim :` |
| Ports | `ports` | `svcinfo lsportfc -delim :` |
| FlashCopy | `flashcopy` | `svcinfo lsfcmap -delim :` |
| Replication | `replication` | `svcinfo lsreplicationrelationship -delim :` (Fallback: `svcinfo lsrcrelationship -delim :`) |
| Performance | `performance` | `svcinfo lssystemstats -delim :` |
| Batteries | `batteries` | `svcinfo lsenclosurebattery -delim :` |
| PSUs | `psus` | `svcinfo lsenclosurepsu -delim :` |

---

## Engineering Rules

1. **Adding a New Collector**:
   - Create a new file in `internal/collectors/<resource>.go`.
   - Implement the `Collector` interface (`Name`, `Collect`, `TTL`).
   - Register the constructor in `AllCollectors(...)` in `collector.go`.
   - Add the discovery mapping (if applicable) in `main.go` and `internal/zabbix/output.go`.
2. **Concurrency Safety**:
   - Do NOT open new SSH client connections within individual collectors. Use the passed `*internalssh.Client`.
   - Respect `RunnerConfig.MaxConcurrent` (default: 3) to prevent SSH session exhaustion on FlashSystem controllers.
3. **Error Isolation**:
   - A single failing collector must never halt other collectors or cause a panic.
   - Return errors explicitly from `Collect()`; `Runner` captures them into `Result.Error`.
4. **Data Contract**:
   - `Collect()` must return `[]parser.Record` where `parser.Record` is `map[string]string`.
   - If no resources are found (empty system response), return an empty slice `[]parser.Record{}`, not `nil`.

---

## Testing Guidelines

- Mock SSH execution responses using `internalssh.Client` test helpers or string parser fixtures.
- Test fallback paths (e.g., replication command fallback on older firmware error codes).
- Test concurrent runner execution and cache hit/miss/stale fallback logic.
