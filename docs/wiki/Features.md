# Features ✨

`flashsystem-collector` provides a comprehensive feature set engineered specifically for enterprise IBM FlashSystem and Spectrum Virtualize environments.

---

## 1. Implemented Capabilities

- **Unified CLI Subcommands**:
  - `collect`: Executes all registered collectors in parallel under concurrency control, returning a comprehensive, single-pass Master JSON payload.
  - `discover`: Executes a targeted collector for a specific resource category, transforming native attributes into Zabbix LLD JSON format (`{"data": [...]}`).
  - `version`: Outputs the compiled collector version string.
- **Strict Configuration Precedence**: Configuration is resolved dynamically following an explicit hierarchy:
  $$\text{CLI Flags} > \text{Environment Variables (FS\_*)} > \text{Default Values}$$
- **Multi-Level Cache Engine**: Atomic disk-backed cache featuring individual TTL management per collector domain.
- **Fail-Safe Fallback**: Serves stale cache data when SSH connectivity drops, annotating output with timestamps and error context.
- **Protection Limits**: Built-in safeguards against oversized JSON payloads (`-max-json-bytes`), excessive volume records (`-max-volumes`), and drive records (`-max-drives`).

---

## 2. Monitoring Coverage (12 Collector Domains)

The collector inspects 12 discrete operational domains through native IBM `svcinfo` commands:

| Collector Domain | IBM CLI Command | Telemetry & Health Coverage |
| :--- | :--- | :--- |
| **System** | `svcinfo lssystem -delim :` | Cluster name, code level (firmware version), operational status, total disk capacity, total free space, statistics collection state. |
| **Nodes** | `svcinfo lsnode -delim :` | Node canister ID, node name, cluster status, assigned I/O group name. |
| **Enclosures** | `svcinfo lsenclosure -delim :` | Enclosure ID, machine type model (`product_MTM`), hardware operational status. |
| **Drives** | `svcinfo lsdrive -delim :` | Physical drive ID, enclosure/slot location, status, drive technology (`tech_type`), capacity. |
| **Storage Pools** | `svcinfo lsmdiskgrp -bytes -delim :` | MDisk group ID, pool name, status, exact capacity, free capacity, used capacity, virtual disk count. |
| **Volumes** | `svcinfo lsvdisk -bytes -delim :` | VDisk ID, volume name, operational status, parent storage pool name. |
| **Fibre Channel** | `svcinfo lsportfc -delim :` | FC port ID, WWPN, port operational status, parent node canister, link speed. |
| **FlashCopy** | `svcinfo lsfcmap -delim :` | FlashCopy mapping ID, mapping name, copy progress (%), mapping status, copy rate. |
| **Replication** | `svcinfo lsreplicationrelationship -delim :` *(fallback: `lsrcrelationship`)* | Remote mirror relationship state, synchronization progress, consistency group membership. |
| **Performance** | `svcinfo lssystemstats -delim :` | System-wide read/write IOPS, read/write bandwidth (MB/s), latency (ms/μs), CPU utilization (%), cache ratios, ambient temperature (°C), power draw (W). |
| **Batteries** | `svcinfo lsenclosurebattery -delim :` | Battery canister ID, enclosure ID, status, state of charge (%), end-of-life warning (`end_of_life_warning`), recondition status. |
| **Power Supplies** | `svcinfo lsenclosurepsu -delim :` | PSU ID, enclosure ID, operational status, AC input failure flag, DC output failure flag, cooling fan failure flag. |

---

## 3. Discovery Capabilities (8 LLD Categories)

Zabbix Low-Level Discovery (LLD) is fully supported across 8 core resource categories via `flashsystem-collector discover -category <NAME>`:

1. **`pools`**: Discovers all storage pools (`{#POOLID}`, `{#POOLNAME}`, `{#POOLSTATUS}`).
2. **`volumes`**: Discovers virtual disks (`{#VOLUMEID}`, `{#VOLUMENAME}`, `{#VOLUMESTATUS}`, `{#VOLUMEPOOL}`).
3. **`drives`**: Discovers physical storage drives (`{#DRIVEID}`, `{#ENCLOSUREID}`, `{#SLOTID}`, `{#DRIVESTATUS}`, `{#DRIVETYPE}`, `{#DRIVECAPACITY}`).
4. **`enclosures`**: Discovers physical shelf enclosures (`{#ENCLOSUREID}`, `{#ENCLOSURESTATUS}`, `{#ENCLOSUREMODEL}`).
5. **`nodes`**: Discovers canister nodes (`{#NODEID}`, `{#NODENAME}`, `{#NODESTATUS}`, `{#NODEIOGROUP}`).
6. **`ports`**: Discovers Fibre Channel host/SAN interfaces (`{#PORTID}`, `{#WWPN}`, `{#PORTSTATUS}`, `{#PORTNODENAME}`, `{#PORTSPEED}`).
7. **`batteries`**: Discovers enclosure cache backup batteries (`{#ENCLOSUREID}`, `{#BATTERYID}`, `{#BATTERYSTATUS}`, `{#BATTERYCHARGE}`, `{#BATTERYEOL}`, `{#BATTERYRECO}`).
8. **`psus`**: Discovers power supply modules (`{#ENCLOSUREID}`, `{#PSUID}`, `{#PSUSTATUS}`, `{#PSUINPUT}`, `{#PSUOUTPUT}`, `{#PSUFAN}`).

---

## 4. Cache & Resilience Features

- **Multi-Tier Expiration**:
  - `FS_METRICS_TTL` (default `5m`): General inventory and pool capacities.
  - `FS_DISCOVERY_TTL` (default `4h`): Static hardware discovery structures.
  - `FS_PERFORMANCE_TTL` (default `1m`): Real-time I/O performance telemetry.
- **Atomic File Swapping**: Cache files are written to `.tmp` files and committed via POSIX `rename`, eliminating file corruption from partial writes.
- **Automatic Stale-Data Recovery**: If the storage array is temporarily unreachable via SSH, existing expired cache entries are loaded and returned to keep Zabbix populated while alerting on the communication failure.
- **Startup Purge**: Expired entries are automatically sanitized on subsequent collection runs to prevent unbounded cache growth.

---

## 5. Security Features

- **Strict File Permission Checks**: Enforces that SSH private key files (`FS_KEYFILE`) have POSIX permissions restricted to the owner (`chmod 0600`), actively rejecting insecure configurations.
- **Cryptographic Host Key Verification**: Mandates validation against a specified `known_hosts` file (`FS_KNOWN_HOSTS`), preventing MITM attacks. Insecure verification (`FS_INSECURE=true`) is strictly optional for testing.
- **Minimal User Privileges**: Designed to operate exclusively with standard IBM storage user accounts assigned the non-destructive `Monitor` role.
- **Process Isolation**: The bash wrapper utilizes file locking (`flock`) on `/var/lock/flashsystem/` to guarantee that concurrent Zabbix pollers do not spawn overlapping sessions for the same storage array.

---

## 6. Current Limitations

While production-ready and highly resilient, administrators should be aware of several architectural constraints:

- **Per-Execution Connection Lifecycle**: The collector does not maintain a long-running SSH connection daemon between Zabbix polling intervals; each collector run authenticates a fresh SSH connection if cache has expired.
- **Single Host Scoping**: One invocation targets exactly one storage system cluster IP. Multi-cluster environments are monitored by configuring distinct Zabbix hosts.
- **Passphrase-Protected Keys**: Private SSH keys requiring interactive passphrases are unsupported; automated deployments require non-passphrase keys protected by strict operating system ACLs.
- **Unpaged CLI Queries**: `lsvdisk` queries retrieve the full volume list across the cluster. While output truncation limits prevent JSON blowup, environments with more than 5,000 volumes may experience longer SSH command response times.
