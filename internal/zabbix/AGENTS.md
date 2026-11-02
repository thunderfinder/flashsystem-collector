# AGENTS.md — internal/zabbix Component Instructions

This directory formats and serializes collected metrics and discovery data into Zabbix-compatible JSON representations, directly corresponding to items and discovery rules defined in `template/IBM FlashSystem Collector.yaml`.

---

## Technical Responsibilities

- **Master Item Output Builder**: `Build()` constructs the master JSON structure (`Output`) containing all 12 resource sections, execution status, and cache statistics.
- **LLD Output Formatter**: `LLDOutput()` converts parser records into Zabbix Low-Level Discovery (`{"data": [{"{#MACRO}": "value"}]}`) structures for 8 supported categories.
- **Record Truncation & Progressive Reduction**: Enforces `MaxVolumes`, `MaxDrives`, and `MaxJSONBytes` limits. Truncates record arrays or drops secondary sections if payload size exceeds Zabbix server buffer limits.
- **Macro Mapping Tables**: Exports LLD macro mapping dictionaries for all 8 categories (`DriveLLDFields`, `PoolLLDFields`, `VolumeLLDFields`, `EnclosureLLDFields`, `NodeLLDFields`, `PortLLDFields`, `BatteryLLDFields`, `PsuLLDFields`).

---

## Output Contracts & Template Alignment

### 1. Master Item Payload Structure (`zabbix.Output`)
Top-level keys must remain exact:
`timestamp`, `host`, `version`, `system`, `nodes`, `enclosures`, `drives`, `pools`, `volumes`, `ports`, `flashcopy`, `replication`, `performance`, `batteries`, `psus`, `units_info`, `status`, `cache_stats`.

Zabbix dependent items extract values from these keys using JSONPath expressions defined in `template/IBM FlashSystem Collector.yaml` (e.g., `$.system[0].status`, `$.performance[?(@.stat_name=='read_io')].stat_current`).

### 2. 8 Low-Level Discovery (LLD) Macro Contracts
Output must strictly follow `{"data": [...]}` format.

Macro Key Contract (Do NOT rename without updating template YAML & docs):
- **Drives**: `{#DRIVEID}`, `{#ENCLOSUREID}`, `{#SLOTID}`, `{#DRIVESTATUS}`, `{#DRIVETYPE}`, `{#DRIVECAPACITY}`
- **Pools**: `{#POOLID}`, `{#POOLNAME}`, `{#POOLSTATUS}`
- **Volumes**: `{#VOLUMEID}`, `{#VOLUMENAME}`, `{#VOLUMESTATUS}`, `{#VOLUMEPOOL}`
- **Enclosures**: `{#ENCLOSUREID}`, `{#ENCLOSURESTATUS}`, `{#ENCLOSUREMODEL}`
- **Nodes**: `{#NODEID}`, `{#NODENAME}`, `{#NODESTATUS}`, `{#NODEIOGROUP}`
- **Ports**: `{#PORTID}`, `{#WWPN}`, `{#PORTSTATUS}`, `{#PORTNODENAME}`, `{#PORTSPEED}`
- **Batteries**: `{#BATTERYID}`, `{#ENCLOSUREID}`, `{#BATTERYSTATUS}`, `{#BATTERYCHARGE}`, `{#BATTERYEOL}`, `{#BATTERYRECO}`
- **PSUs**: `{#PSUID}`, `{#ENCLOSUREID}`, `{#PSUSTATUS}`, `{#PSUINPUT}`, `{#PSUOUTPUT}`, `{#PSUFAN}`

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
4. **Empty LLD Contract**:
   - Discovery failures must emit `{"data":[]}` so Zabbix does not trigger false entity deletion alerts.
5. **Template Compatibility**:
   - Any additions or changes to JSON fields or LLD macros must be mirrored in `template/IBM FlashSystem Collector.yaml` and documented in `README.md`.
   - Zabbix item and discovery rule UUIDs in the template must be exactly 32-character hexadecimal strings.

---

## Testing Guidelines

- Test `Build()` with complete results, partial error results, and total failure results.
- Test LLD formatting for all 8 categories to ensure macro keys and record counts match input fixtures.
- Test payload reduction logic when payload size exceeds `MaxJSONBytes`.
