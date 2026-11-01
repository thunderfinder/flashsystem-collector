# Monitoring Metrics 📈

This document provides a comprehensive catalog of all telemetry items, capacity metrics, hardware statuses, and performance counters collected by `flashsystem-collector` and ingested by Zabbix via `template/IBM FlashSystem Collector.yaml`.

---

## 1. Collector Execution & Health Metrics

These metrics monitor the operational state of the collector and its interaction with the storage array:

| Metric Name / Description | Zabbix Item Key | Type | Unit | JSONPath Source | Polling Interval |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Collector Master Payload** | `flashsystem_collector.sh[{HOST.IP},collect]` | External Check | Text | *(Raw JSON stdout)* | `{$FS_COLLECT_INTERVAL}` (1m) |
| **Collector Success State** | `flashsystem.collector.success[{HOST.IP}]` | Dependent | Boolean (`1`/`0`) | `$.status.success` | Inherited |
| **Collector Execution Duration** | `flashsystem.collector.duration[{HOST.IP}]` | Dependent | Milliseconds (`ms`) | `$.status.duration_ms` | Inherited |

---

## 2. System-Level Metrics (`lssystem`)

Extracted from the root cluster configuration:

| Metric Name / Description | Zabbix Item Key | Type | Unit | JSONPath & Preprocessing | Notes |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Cluster Name** | `flashsystem.system.name[{HOST.IP}]` | Dependent | Character | `$.system[0].name` | Storage array cluster name. |
| **Cluster Status** | `flashsystem.system.status[{HOST.IP}]` | Dependent | Character | `$.system[0].cluster_status` | Expected: `online`. |
| **Firmware Revision** | `flashsystem.system.firmware[{HOST.IP}]` | Dependent | Character | `$.system[0].code_level` | Spectrum Virtualize microcode version. |
| **Total Raw Capacity** | `flashsystem.system.capacity.total[{HOST.IP}]` | Dependent | Bytes (`B`) | `$.system[0].total_mdisk_capacity` + JS | Total managed disk capacity. |
| **Total Free Capacity** | `flashsystem.system.capacity.free[{HOST.IP}]` | Dependent | Bytes (`B`) | `$.system[0].total_free_space` + JS | Unallocated storage capacity. |
| **Active Node Count** | `flashsystem.system.nodes[{HOST.IP}]` | Dependent | Integer | `$.nodes.length()` | Count of discovered canisters. |
| **Statistics Engine State** | `flashsystem.system.statistics[{HOST.IP}]` | Dependent | Character | `$.system[0].statistics_status` | Must be `on` for performance data. |

---

## 3. Real-Time Storage Performance Telemetry (`lssystemstats`)

Aggregated cluster I/O performance and physical environmental counters:

| Metric Name / Description | Zabbix Item Key | Type | Unit | JSONPath Source | Interpretation Guidance |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Read IOPS** | `flashsystem.perf.read_io[{HOST.IP}]` | Dependent | IOPS | `$.performance[0].read_io` | Read I/O operations per second. |
| **Write IOPS** | `flashsystem.perf.write_io[{HOST.IP}]` | Dependent | IOPS | `$.performance[0].write_io` | Write I/O operations per second. |
| **Read Bandwidth** | `flashsystem.perf.read_mb[{HOST.IP}]` | Dependent | MB/s | `$.performance[0].read_mb` | Storage read throughput. |
| **Write Bandwidth** | `flashsystem.perf.write_mb[{HOST.IP}]` | Dependent | MB/s | `$.performance[0].write_mb` | Storage write throughput. |
| **Read Latency** | `flashsystem.perf.read_ms[{HOST.IP}]` | Dependent | Milliseconds (`ms`) | `$.performance[0].read_ms` | Average read response time. |
| **Write Latency** | `flashsystem.perf.write_ms[{HOST.IP}]` | Dependent | Milliseconds (`ms`) | `$.performance[0].write_ms` | Average write response time. |
| **VDisk Response Time** | `flashsystem.perf.vdisk_ms[{HOST.IP}]` | Dependent | Microseconds ($\mu\text{s}$)| `$.performance[0].vdisk_ms` | Internal virtualization response time. |
| **Canister CPU Utilization** | `flashsystem.perf.cpu_pc[{HOST.IP}]` | Dependent | Percent (`%`) | `$.performance[0].cpu_pc` | Average processor utilization across nodes. |
| **Total Cache Utilization**| `flashsystem.perf.total_cache_pc[{HOST.IP}]`| Dependent | Percent (`%`) | `$.performance[0].total_cache_pc` | Aggregate DRAM cache utilization. |
| **Write Cache Dirty Ratio**| `flashsystem.perf.write_cache_pc[{HOST.IP}]`| Dependent | Percent (`%`) | `$.performance[0].write_cache_pc` | Unwritten cache destage ratio. |
| **FC SAN Port IOPS** | `flashsystem.perf.fc_io[{HOST.IP}]` | Dependent | IOPS | `$.performance[0].fc_io` | Native Fibre Channel operations. |
| **FC SAN Throughput** | `flashsystem.perf.fc_mb[{HOST.IP}]` | Dependent | MB/s | `$.performance[0].fc_mb` | Native Fibre Channel data transfer rate. |
| **System Power Consumption**| `flashsystem.perf.power_w[{HOST.IP}]` | Dependent | Watts (`W`) | `$.performance[0].power_w` | Real-time aggregate power draw. |
| **Ambient Temperature** | `flashsystem.perf.temp_c[{HOST.IP}]` | Dependent | Celsius (`°C`) | `$.performance[0].temp_c` | Enclosure thermal reading. |

---

## 4. Discovered Entity Metrics (LLD Prototypes)

These items are dynamically created by Zabbix Low-Level Discovery rules:

### Storage Pools (`lsmdiskgrp`)
- **Discovery Rule**: `flashsystem_collector.sh[{HOST.IP},discover,pools]`
- **Macros**: `{#POOLID}`, `{#POOLNAME}`, `{#POOLSTATUS}`

| Item Name | Zabbix Item Key | Type | Unit | Preprocessing / Source |
| :--- | :--- | :--- | :--- | :--- |
| **Pool Operational Status** | `flashsystem.pool.status[{HOST.IP},{#POOLNAME}]` | Dependent | Character | `$.pools[?(@.name=='{#POOLNAME}')].status.first()` |
| **Total Pool Capacity** | `flashsystem.pool.capacity[{HOST.IP},{#POOLNAME}]` | Dependent | Bytes (`B`) | `$.pools[?(@.name=='{#POOLNAME}')].capacity.first()` + JS |
| **Free Pool Space** | `flashsystem.pool.free[{HOST.IP},{#POOLNAME}]` | Dependent | Bytes (`B`) | `$.pools[?(@.name=='{#POOLNAME}')].free_capacity.first()` + JS |
| **Used Pool Space** | `flashsystem.pool.used[{HOST.IP},{#POOLNAME}]` | Dependent | Bytes (`B`) | JavaScript: `capacity - free` |
| **Pool Space Utilization %** | `flashsystem.pool.pct[{HOST.IP},{#POOLNAME}]` | Dependent | Percent (`%`)| JavaScript: `(used / capacity) * 100` |
| **Assigned VDisk Count** | `flashsystem.pool.vdisk_count[{HOST.IP},{#POOLNAME}]` | Dependent | Integer | `$.pools[?(@.name=='{#POOLNAME}')].vdisk_count.first()` |

### Virtual Volumes (`lsvdisk`)
- **Discovery Rule**: `flashsystem_collector.sh[{HOST.IP},discover,volumes]`
- **Macros**: `{#VOLUMEID}`, `{#VOLUMENAME}`, `{#VOLUMESTATUS}`, `{#VOLUMEPOOL}`

| Item Name | Zabbix Item Key | Type | Unit | Preprocessing / Source |
| :--- | :--- | :--- | :--- | :--- |
| **Volume Status** | `flashsystem.volume.status[{HOST.IP},{#VOLUMEID}]` | Dependent | Character | `$.volumes[?(@.id=='{#VOLUMEID}')].status.first()` |

### Physical Drives (`lsdrive`)
- **Discovery Rule**: `flashsystem_collector.sh[{HOST.IP},discover,drives]`
- **Macros**: `{#DRIVEID}`, `{#ENCLOSUREID}`, `{#SLOTID}`, `{#DRIVESTATUS}`, `{#DRIVETYPE}`, `{#DRIVECAPACITY}`

| Item Name | Zabbix Item Key | Type | Unit | Preprocessing / Source |
| :--- | :--- | :--- | :--- | :--- |
| **Drive Status** | `flashsystem.drive.status[{HOST.IP},{#DRIVEID}]` | Dependent | Character | `$.drives[?(@.id=='{#DRIVEID}')].status.first()` |

### Enclosure Canister Nodes (`lsnode`)
- **Discovery Rule**: `flashsystem_collector.sh[{HOST.IP},discover,nodes]`
- **Macros**: `{#NODEID}`, `{#NODENAME}`, `{#NODESTATUS}`, `{#NODEIOGROUP}`

| Item Name | Zabbix Item Key | Type | Unit | Preprocessing / Source |
| :--- | :--- | :--- | :--- | :--- |
| **Node Canister Status** | `flashsystem.node.status[{HOST.IP},{#NODEID}]` | Dependent | Character | `$.nodes[?(@.id=='{#NODEID}')].status.first()` |

### Physical Enclosures (`lsenclosure`)
- **Discovery Rule**: `flashsystem_collector.sh[{HOST.IP},discover,enclosures]`
- **Macros**: `{#ENCLOSUREID}`, `{#ENCLOSURESTATUS}`, `{#ENCLOSUREMODEL}`

| Item Name | Zabbix Item Key | Type | Unit | Preprocessing / Source |
| :--- | :--- | :--- | :--- | :--- |
| **Enclosure Status** | `flashsystem.enclosure.status[{HOST.IP},{#ENCLOSUREID}]` | Dependent | Character | `$.enclosures[?(@.id=='{#ENCLOSUREID}')].status.first()` |

### Fibre Channel Ports (`lsportfc`)
- **Discovery Rule**: `flashsystem_collector.sh[{HOST.IP},discover,ports]`
- **Macros**: `{#PORTID}`, `{#WWPN}`, `{#PORTSTATUS}`, `{#PORTNODENAME}`, `{#PORTSPEED}`

| Item Name | Zabbix Item Key | Type | Unit | Preprocessing / Source |
| :--- | :--- | :--- | :--- | :--- |
| **FC Port Status** | `flashsystem.port.status[{HOST.IP},{#PORTID}]` | Dependent | Character | `$.ports[?(@.id=='{#PORTID}')].status.first()` |

### Enclosure Cache Batteries (`lsenclosurebattery`)
- **Discovery Rule**: `flashsystem_collector.sh[{HOST.IP},discover,batteries]`
- **Macros**: `{#ENCLOSUREID}`, `{#BATTERYID}`, `{#BATTERYSTATUS}`, `{#BATTERYCHARGE}`, `{#BATTERYEOL}`, `{#BATTERYRECO}`

| Item Name | Zabbix Item Key | Type | Unit | Preprocessing / Source |
| :--- | :--- | :--- | :--- | :--- |
| **Battery Status** | `flashsystem.battery.status[{HOST.IP},{#BATTERYID}]` | Dependent | Character | `$.batteries[?(@.battery_id=='{#BATTERYID}')].status.first()` |
| **Battery Health** | `flashsystem.battery.health[{HOST.IP},{#BATTERYID}]` | Dependent | Character | `$.batteries[?(@.battery_id=='{#BATTERYID}')].end_of_life_warning.first()` |

### Power Supply Units (`lsenclosurepsu`)
- **Discovery Rule**: `flashsystem_collector.sh[{HOST.IP},discover,psus]`
- **Macros**: `{#ENCLOSUREID}`, `{#PSUID}`, `{#PSUSTATUS}`, `{#PSUINPUT}`, `{#PSUOUTPUT}`, `{#PSUFAN}`

| Item Name | Zabbix Item Key | Type | Unit | Preprocessing / Source |
| :--- | :--- | :--- | :--- | :--- |
| **PSU Status** | `flashsystem.psu.status[{HOST.IP},{#PSUID}]` | Dependent | Character | `$.psus[?(@.psu_id=='{#PSUID}')].status.first()` |
