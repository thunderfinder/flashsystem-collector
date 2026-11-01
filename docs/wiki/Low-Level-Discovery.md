# Low-Level Discovery (LLD) 🔍

Zabbix Low-Level Discovery (LLD) allows automated creation of items, triggers, and graphs based on discovered infrastructure components. `flashsystem-collector` implements dynamic LLD across 8 distinct hardware and logical storage categories.

---

## 1. Discovery Architecture

LLD requests are executed via the `discover` subcommand:
```bash
flashsystem-collector discover -host <HOST> -category <CATEGORY> [flags]
```

Or via the wrapper script:
```bash
flashsystem_collector.sh <HOST> discover <CATEGORY>
```

```mermaid
flowchart LR
    ZabbixPoller["Zabbix Discovery Poller"] -->|Every {$FS_DISCOVERY_INTERVAL}| Wrapper["flashsystem_collector.sh"]
    Wrapper -->|Executes discover -category <cat>| Collector["flashsystem-collector"]
    Collector -->|Check LLD Cache| CacheEngine[("Disk Cache (4h TTL)")]

    alt Cache Hit
        CacheEngine -->|Cached LLD JSON| Collector
    else Cache Miss
        Collector -->|Run Specific Collector| Storage["IBM FlashSystem CLI"]
        Storage -->|Colon-delimited data| Parser["internal/parser"]
        Parser -->|fieldMap Transformation| LLDBuilder["internal/zabbix.LLDOutput"]
        LLDBuilder -->|Persist to Cache| CacheEngine
    end

    Collector -->|{"data": [...]}| Wrapper
    Wrapper -->|stdout stream| ZabbixPoller
```

---

## 2. Macro Mappings across 8 Discovery Categories

The collector maps native IBM `svcinfo` attributes directly to Zabbix macros (`{#MACRO}`) using defined field maps in `internal/zabbix/output.go`:

### 1. Storage Pools (`pools`)
- **Native Command**: `svcinfo lsmdiskgrp -delim :`
- **Field Map**:
  - `id` $\rightarrow$ `{#POOLID}`
  - `name` $\rightarrow$ `{#POOLNAME}`
  - `status` $\rightarrow$ `{#POOLSTATUS}`

### 2. Virtual Volumes (`volumes`)
- **Native Command**: `svcinfo lsvdisk -delim :`
- **Field Map**:
  - `id` $\rightarrow$ `{#VOLUMEID}`
  - `name` $\rightarrow$ `{#VOLUMENAME}`
  - `status` $\rightarrow$ `{#VOLUMESTATUS}`
  - `mdisk_grp_name` $\rightarrow$ `{#VOLUMEPOOL}`

### 3. Physical Drives (`drives`)
- **Native Command**: `svcinfo lsdrive -delim :`
- **Field Map**:
  - `id` $\rightarrow$ `{#DRIVEID}`
  - `enclosure_id` $\rightarrow$ `{#ENCLOSUREID}`
  - `slot_id` $\rightarrow$ `{#SLOTID}`
  - `status` $\rightarrow$ `{#DRIVESTATUS}`
  - `tech_type` $\rightarrow$ `{#DRIVETYPE}`
  - `capacity` $\rightarrow$ `{#DRIVECAPACITY}`

### 4. Physical Enclosures (`enclosures`)
- **Native Command**: `svcinfo lsenclosure -delim :`
- **Field Map**:
  - `id` $\rightarrow$ `{#ENCLOSUREID}`
  - `status` $\rightarrow$ `{#ENCLOSURESTATUS}`
  - `product_MTM` $\rightarrow$ `{#ENCLOSUREMODEL}`

### 5. Canister Nodes (`nodes`)
- **Native Command**: `svcinfo lsnode -delim :`
- **Field Map**:
  - `id` $\rightarrow$ `{#NODEID}`
  - `name` $\rightarrow$ `{#NODENAME}`
  - `status` $\rightarrow$ `{#NODESTATUS}`
  - `IO_group_name` $\rightarrow$ `{#NODEIOGROUP}`

### 6. Fibre Channel Ports (`ports`)
- **Native Command**: `svcinfo lsportfc -delim :`
- **Field Map**:
  - `id` $\rightarrow$ `{#PORTID}`
  - `WWPN` $\rightarrow$ `{#WWPN}`
  - `status` $\rightarrow$ `{#PORTSTATUS}`
  - `node_name` $\rightarrow$ `{#PORTNODENAME}`
  - `port_speed` $\rightarrow$ `{#PORTSPEED}`

### 7. Backup Batteries (`batteries`)
- **Native Command**: `svcinfo lsenclosurebattery -delim :`
- **Field Map**:
  - `enclosure_id` $\rightarrow$ `{#ENCLOSUREID}`
  - `battery_id` $\rightarrow$ `{#BATTERYID}`
  - `status` $\rightarrow$ `{#BATTERYSTATUS}`
  - `percent_charged` $\rightarrow$ `{#BATTERYCHARGE}`
  - `end_of_life_warning` $\rightarrow$ `{#BATTERYEOL}`
  - `recondition_needed` $\rightarrow$ `{#BATTERYRECO}`

### 8. Power Supply Units (`psus`)
- **Native Command**: `svcinfo lsenclosurepsu -delim :`
- **Field Map**:
  - `enclosure_id` $\rightarrow$ `{#ENCLOSUREID}`
  - `psu_id` $\rightarrow$ `{#PSUID}`
  - `status` $\rightarrow$ `{#PSUSTATUS}`
  - `input_failed` $\rightarrow$ `{#PSUINPUT}`
  - `output_failed` $\rightarrow$ `{#PSUOUTPUT}`
  - `fan_failed` $\rightarrow$ `{#PSUFAN}`

---

## 3. Empty Discovery & Error Resilience

When an error occurs during discovery, returning malformed JSON or throwing an uncaught script error causes Zabbix discovery rules to enter an **Unsupported** state, which can trigger discovery deletion timers and purge existing history items.

To prevent this, `flashsystem-collector` implements strict fail-safe discovery behavior:

```go
// Function in main.go guaranteeing valid LLD envelope
func emitEmptyLLD() {
    fmt.Println(`{"data":[]}`)
}
```

- **Unknown Category**: If an invalid category name is requested, the collector logs the error to `stderr` and emits `{"data":[]}`.
- **SSH Failure**: If SSH connectivity fails and no cached discovery data exists, the collector logs the failure to `stderr` and emits `{"data":[]}`.
- **Result in Zabbix**: Zabbix receives a valid JSON object with zero items. Existing discovered prototypes enter the "Lost resources" grace period (governed by Zabbix's `Keep lost resources period` setting, typically 30 days) instead of being prematurely purged.
