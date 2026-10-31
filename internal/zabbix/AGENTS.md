# AGENTS.md — internal/zabbix Component Instructions

This directory formats and serializes collected metrics and discovery data into Zabbix-compatible JSON representations.

---

## Technical Responsibilities

- **Master Item Output Builder**: `Build()` constructs the master JSON structure (`Output`) containing all 10 resource sections, execution status, and cache statistics.
- **LLD Output Formatter**: `LLDOutput()` converts parser records into Zabbix Low-Level Discovery (`{"data": [{"{#MACRO}": "value"}]}`) structures.
- **Record Truncation & Progressive Reduction**: Enforces `MaxVolumes`, `MaxDrives`, and `MaxJSONBytes` limits. Truncates record arrays or drops optional sections if payload size exceeds Zabbix server buffer limits.
- **Macro Mapping Tables**: Exports LLD macro mapping dictionaries (`DriveLLDFields`, `PoolLLDFields`, `VolumeLLDFields`, `EnclosureLLDFields`, `NodeLLDFields`, `PortLLDFields`).

---

## Output Contracts

### 1. Master Item Payload Structure (`zabbix.Output`)
Top-level keys must remain exact:
`timestamp`, `host`, `version`, `system`, `nodes`, `enclosures`, `drives`, `pools`, `volumes`, `ports`, `flashcopy`, `replication`, `performance`, `status`, `cache_stats`, `batteries`, `psus`, `units_info`.

### 2. Low-Level Discovery (LLD) Structure
Output must follow `{"data": [...]}` format.

Macro Key Contract (Do NOT rename without updating template YAML & docs):
- Drives: `{#DRIVEID}`, `{#ENCLOSUREID}`, `{#SLOTID}`, `{#DRIVESTATUS}`, `{#DRIVETYPE}`, `{#DRIVECAPACITY}`
- Pools: `{#POOLID}`, `{#POOLNAME}`, `{#POOLSTATUS}`
- Volumes: `{#VOLUMEID}`, `{#VOLUMENAME}`, `{#VOLUMESTATUS}`, `{#VOLUMEPOOL}`
- Enclosures: `{#ENCLOSUREID}`, `{#ENCLOSURESTATUS}`, `{#ENCLOSUREMODEL}`
- Nodes: `{#NODEID}`, `{#NODENAME}`, `{#NODESTATUS}`, `{#NODEIOGROUP}`
- Ports: `{#PORTID}`, `{#WWPN}`, `{#PORTSTATUS}`, `{#PORTNODENAME}`, `{#PORTSPEED}`
- Batteries: `{#ENCLOSUREID}`, `{#BATTERYID}`, `{#BATTERYSTATUS}`, `{#BATTERYCHARGE}`, `{#BATTERYEOL}`, `{#BATTERYRECO}`
- PSUs: `{#ENCLOSUREID}`, `{#PSUID}`, `{#PSUSTATUS}`, `{#PSUINPUT}`, `{#PSUOUTPUT}`, `{#PSUFAN}`

---

## Engineering Rules

1. **JSON Safety**:
   - Empty resource sections must marshal as `[]` (`json.RawMessage("[]")`), never as `null` or `{}`.
2. **Partial Errors vs Global Failure**:
   - `status.success` remains `true` if at least one collector succeeded.
   - `status.success` is set to `false` ONLY if all collectors failed.
   - Individual collector errors are collected into `status.errors[]`.
3. **Payload Reduction Strategy**:
   - If output exceeds `MaxJSONBytes`, `reduceOutput()` progressively reduces volume/drive counts and drops secondary sections (`FlashCopy`, `Replication`) before failing.

---

## Testing Guidelines

- Test `Build()` with complete results, partial error results, and total failure results.
- Test truncation handling when `MaxVolumes` or `MaxDrives` limits are hit.
- Test `reduceOutput()` payload size reduction logic.
- Test `LLDOutput()` mapping for all 6 discovery categories.
