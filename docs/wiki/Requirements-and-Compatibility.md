# Requirements and Compatibility 📋

This page defines the supported operating systems, Go compilation toolchains, Zabbix server/proxy version compatibility, IBM FlashSystem storage models, and network firewall prerequisites.

---

## 1. Go Toolchain Requirements

- **Mandatory Go Version**: **Go 1.25.0+** (specified directly in `go.mod`).
- **Compilation Toolchain**: Standard `gc` toolchain with static linkage enabled:
  ```bash
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o flashsystem-collector .
  ```
- **Dependencies**: Uses Go modules (`go.mod`, `go.sum`). Relies on `golang.org/x/crypto/ssh` for cryptographic SSH transport.

---

## 2. Operating System Requirements (Monitoring Host)

The binary runs on the host executing the Zabbix External Script (typically a **Zabbix Server** or **Zabbix Proxy** node):

- **Supported Linux Distributions**:
  - Red Hat Enterprise Linux (RHEL) / AlmaLinux / Rocky Linux 8.x, 9.x
  - CentOS Stream 8, 9
  - Debian GNU/Linux 11 (Bullseye), 12 (Bookworm)
  - Ubuntu LTS 20.04, 22.04, 24.04
  - SUSE Linux Enterprise Server (SLES) 15 SP3+
- **System Utilities Required for Wrapper Script**:
  - **Bash 4.0+** (mandated by `mapfile` and associative arrays in `scripts/flashsystem_collector.sh`).
  - **`flock`** (`util-linux` package) for host-level concurrency serialization.
  - **Standard POSIX tools**: `date`, `stat`, `mkdir`, `mv`, `touch`.

---

## 3. Zabbix Version Compatibility

| Zabbix Version | Compatibility Status | Notes |
| :--- | :--- | :--- |
| **Zabbix 7.0 LTS / 7.2** | **Fully Supported** | Recommended. Native support for 1-minute poller intervals, JSONPath preprocessing, and template import. |
| **Zabbix 6.4** | **Fully Supported** | Minimum officially supported version for `template/IBM FlashSystem Collector.yaml`. |
| **Zabbix 6.0 LTS** | **Compatible with Manual Template Tweaks** | Requires template format adjustment if YAML schema features from 6.4 are rejected during import. Preprocessing logic remains identical. |
| **Zabbix 5.0 / 5.4** | **Deprecated / Unsupported** | Lacks advanced JavaScript preprocessing and modern dependent item array transformations. |

### Zabbix Server / Proxy Configuration Requirements
In `/etc/zabbix/zabbix_server.conf` or `/etc/zabbix/zabbix_proxy.conf`:
- **`Timeout=30`** (or `Timeout=55` in Zabbix 7.0+): The default 3-second timeout is insufficient for SSH key negotiation and running multi-domain queries on enterprise arrays.
- **`ExternalScripts`**: Must point to the directory containing `flashsystem_collector.sh` (default: `/usr/lib/zabbix/externalscripts`).

---

## 4. IBM Storage Compatibility Matrix

`flashsystem-collector` is compatible with all storage systems powered by **IBM Spectrum Virtualize** (formerly SAN Volume Controller microcode) running **Spectrum Virtualize OS 8.2.x, 8.3.x, 8.4.x, 8.5.x, 8.6.x+**:

| Storage Family | Compatible Models | Verified CLI Support |
| :--- | :--- | :--- |
| **IBM FlashSystem 5000 Series** | 5015, 5035, 5045, 5200 | All 12 collectors. (Note: Performance stats require `startstats`). |
| **IBM FlashSystem 7000 Series** | 7200, 7300 | All 12 collectors. NVMe-oF and FC ports discovered natively. |
| **IBM FlashSystem 9000 Series** | 9100, 9200, 9500 | All 12 collectors. Full battery, PSU, enclosure, and drive telemetry. |
| **IBM SAN Volume Controller (SVC)**| SV1, SV2, SV3, DH8 | Supported across all managed storage pools and virtualization nodes. |
| **IBM Storwize (Legacy)** | V7000 Gen2/Gen3, V5000 | Supported on Spectrum Virtualize 7.8+ / 8.x. Replication collector falls back cleanly to `lsrcrelationship`. |

---

## 5. Storage Account Permissions and Roles

The collector accesses the storage array via SSH. Security best practices dictate assigning **Least Privilege**:

- **Required Storage Role**: **`Monitor`** (Read-Only).
- The `Monitor` role permits execution of all informational `svcinfo` commands (`lssystem`, `lsmdiskgrp`, `lsvdisk`, `lsnode`, `lsdrive`, `lsenclosure`, `lsenclosurebattery`, `lsenclosurepsu`, `lsportfc`, `lsfcmap`, `lsreplicationrelationship`, `lssystemstats`).
- The `Monitor` role strictly prohibits administrative modifications (`svctask`).

```text
FlashSystem Command Authorization:
[svcinfo *]  ---> PERMITTED (Read-Only inspection)
[svctask *]  ---> BLOCKED / DENIED by storage RBAC
```

---

## 6. Network and Firewall Requirements

- **Network Protocol**: TCP.
- **Destination Port**: **22** (Standard SSH) on the FlashSystem Management Cluster IP.
- **Network Path**: Bidirectional TCP between Zabbix Server/Proxy IP and FlashSystem cluster management IP.
- **Note on Node Canister IPs**: You do **not** need to open port 22 to individual node canister service IPs; queries are directed to the virtual **Cluster Management IP**.
