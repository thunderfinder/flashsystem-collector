# IBM FlashSystem Collector for Zabbix

[![Go Report Card](https://goreportcard.com/badge/github.com/flashsystem-collector)](https://goreportcard.com/report/github.com/flashsystem-collector)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

A high-performance Go monitoring collector and Zabbix integration for **IBM FlashSystem** and **IBM Spectrum Virtualize** storage systems.

`flashsystem-collector` connects to FlashSystem management nodes over SSH, executes non-intrusive native CLI commands (`svcinfo`), and aggregates hardware status, capacity, and performance statistics into structured JSON payloads for Zabbix Server or Proxy.

---

## Key Capabilities

- **Single SSH Connection per Cycle**: Minimizes storage controller load by executing required queries concurrently over a single SSH session pool (max 3 concurrent queries).
- **Dual Operating Modes**:
  - `collect`: Gathers all system, component, and performance metrics in one structured JSON payload for a Zabbix **Master Item**.
  - `discover`: Outputs RFC-compliant Zabbix **Low-Level Discovery (LLD)** JSON with macro mappings (`{#MACRO}`) for automated entity discovery.
- **Persistent Atomic Disk Cache**: JSON cache backed by atomic file replacement prevents SSH polling storms and serves cached data when intervals overlap.
- **Stale Cache Resilience**: Automatically serves stale metrics when storage management interfaces are temporarily unreachable or undergoing maintenance, preventing false positive alarms.
- **Turnkey Zabbix 6.4+ / 7.x Template**: Ready-to-import YAML template featuring Master/Dependent item architecture, JavaScript byte conversions, pre-tuned triggers, and LLD discovery rules.
- **Strict Host Key Verification**: Supports strict `known_hosts` verification by default to defend against Man-In-The-Middle (MITM) attacks.

---

## Architecture Overview

```text
┌─────────────────────────────────────────────────────────────┐
│                        Zabbix Server / Proxy                │
│                                                             │
│   ┌─────────────────────────────────────────────────────┐   │
│   │ Template: IBM FlashSystem Collector                 │   │
│   │                                                     │   │
│   │  - Master Item:  flashsystem_collector.sh collect    │   │
│   │    └─ Dependent Items (JSONPath & JS preprocessing) │   │
│   │  - LLD Rules:    flashsystem_collector.sh discover  │   │
│   │    └─ Pools, Drives, Volumes, Enclosures, Nodes,    │   │
│   │       Ports, Batteries, PSUs                        │   │
│   └──────────────────────────┬──────────────────────────┘   │
│                              │ executes                     │
│                              ▼                              │
│         /usr/lib/zabbix/externalscripts/flashsystem_collector.sh
│                              │ loads /etc/zabbix/flashsystem/env
│                              ▼                              │
│               /usr/local/bin/flashsystem-collector          │
│                              │                              │
│              ┌───────────────┴───────────────┐              │
│              ▼                               ▼              │
│       Disk JSON Cache                 SSH Client Session    │
│   /var/tmp/flashsystem_*.cache.json   (Port 22, Monitor)    │
└──────────────────────────────────────────────┬──────────────┘
                                               │ svcinfo commands
                                               ▼
                                  ┌─────────────────────────┐
                                  │   IBM FlashSystem / SVC │
                                  │   Management Interface  │
                                  └─────────────────────────┘
```

---

## Requirements

### Runtime Requirements
- **Operating System**: Linux x86_64 (Debian/Ubuntu, RHEL/Rocky Linux/AlmaLinux, or compatible).
- **Zabbix**: Zabbix Server or Zabbix Proxy 6.4+, 7.0 LTS, or 7.2+.
- **Storage System**: IBM FlashSystem family (5000, 5200, 7200, 7300, 9200, 9500) or IBM Spectrum Virtualize / SVC running supported firmware.
- **Storage Privileges**: FlashSystem user account with the **`Monitor`** role (read-only access).
- **Network**: TCP port 22 reachable from Zabbix Server/Proxy to the FlashSystem management IP.

### Build Requirements
- **Go Compiler**: Go **1.25.0** or newer (as specified in `go.mod`).
- **Build Tools**: Standard GNU toolchain (`make`, `file`, etc.).

---

## 1. Build from Source

Compile a statically linked Linux AMD64 binary with symbol table and DWARF debugging information stripped:

```bash
# Clone the repository
git clone https://github.com/thunderfinder/flashsystem-collector.git
cd flashsystem-collector

# Verify dependencies
go mod verify

# Compile static binary for Linux x86_64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -trimpath \
    -o flashsystem-collector .

# Validate binary architecture
file flashsystem-collector
# Expected output: ELF 64-bit LSB executable, x86-64, statically linked
```

---

## 2. Install Collector Binary and Wrapper Script

Install the compiled binary to `/usr/local/bin` and the wrapper script to the Zabbix `ExternalScripts` directory:

```bash
# Install binary
sudo cp flashsystem-collector /usr/local/bin/
sudo chown root:root /usr/local/bin/flashsystem-collector
sudo chmod 755 /usr/local/bin/flashsystem-collector

# Install Zabbix external script wrapper
sudo cp scripts/flashsystem_collector.sh /usr/lib/zabbix/externalscripts/
sudo chown zabbix:zabbix /usr/lib/zabbix/externalscripts/flashsystem_collector.sh
sudo chmod 755 /usr/lib/zabbix/externalscripts/flashsystem_collector.sh
```

---

## 3. Directory and Permission Setup

Create the required directories for credentials, logs, locks, and cache:

```bash
# Create configuration and credential directories
sudo mkdir -p /etc/zabbix/flashsystem
sudo mkdir -p /etc/zabbix/ssh
sudo mkdir -p /var/log/zabbix
sudo mkdir -p /var/lock/flashsystem

# Set ownership and permissions
sudo chown -R zabbix:zabbix /etc/zabbix/flashsystem
sudo chown -R zabbix:zabbix /etc/zabbix/ssh
sudo chown -R zabbix:zabbix /var/lock/flashsystem
sudo chown -R zabbix:zabbix /var/log/zabbix

sudo chmod 750 /etc/zabbix/flashsystem
sudo chmod 700 /etc/zabbix/ssh
sudo chmod 755 /var/lock/flashsystem
```

---

## 4. SSH Authentication and Security Hardening

The collector strictly enforces secure authentication. Password-less SSH key authentication using ED25519 is strongly recommended.

### Step 4.1: Generate SSH Key Pair

Generate an ED25519 key pair owned by the `zabbix` user without a passphrase (passphrases are not supported by the collector):

```bash
sudo -u zabbix ssh-keygen -t ed25519 \
    -C "zabbix-flashsystem-monitor" \
    -f /etc/zabbix/ssh/flashsystem_key \
    -N ""

# Ensure restrictive permissions on the private key
sudo chmod 600 /etc/zabbix/ssh/flashsystem_key
sudo chown zabbix:zabbix /etc/zabbix/ssh/flashsystem_key
```

### Step 4.2: Provision the Public Key on IBM FlashSystem

Copy the public key content to the FlashSystem storage monitor user:

```bash
cat /etc/zabbix/ssh/flashsystem_key.pub
```

On IBM FlashSystem GUI or CLI:
1. Navigate to **Settings** > **Security** > **Users**.
2. Create or select a dedicated monitoring user (e.g., `monitor`).
3. Assign the user role **`Monitor`** (strictly read-only).
4. Paste the public key into the user's authorized SSH keys.

Alternatively, via FlashSystem CLI:
```bash
svctask chuser -keyfile /path/to/key.pub monitor
```

### Step 4.3: Secure Host-Key Verification (`known_hosts`)

By default, `flashsystem-collector` requires valid host key verification. Disabling host key verification leaves monitoring traffic vulnerable to Man-In-The-Middle (MITM) attacks.

Retrieve and store the FlashSystem SSH host key into a dedicated `known_hosts` file:

```bash
# Populate known_hosts using ssh-keyscan (replace 10.10.10.50 with your FlashSystem IP)
sudo -u zabbix ssh-keyscan -p 22 10.10.10.50 | sudo tee /etc/zabbix/ssh/known_hosts > /dev/null

# Secure known_hosts permissions
sudo chmod 644 /etc/zabbix/ssh/known_hosts
sudo chown zabbix:zabbix /etc/zabbix/ssh/known_hosts
```

### Step 4.4: Create the Environment File

Create `/etc/zabbix/flashsystem/env` containing runtime configuration for the wrapper script:

```bash
sudo tee /etc/zabbix/flashsystem/env > /dev/null << 'EOF'
# /etc/zabbix/flashsystem/env
# Runtime environment for flashsystem-collector

# --- Authentication ---
FS_USER=monitor
FS_KEYFILE=/etc/zabbix/ssh/flashsystem_key

# --- Host Key Verification (Production Default) ---
FS_KNOWN_HOSTS=/etc/zabbix/ssh/known_hosts

# WARNING: Only set FS_INSECURE=true for isolated lab environments where known_hosts cannot be maintained.
# FS_INSECURE=false

# --- Cache and Polling Intervals ---
FS_CACHE_DIR=/var/tmp
FS_METRICS_TTL=5m
FS_DISCOVERY_TTL=4h
FS_PERFORMANCE_TTL=1m

# --- Logging & Limits ---
FS_VERBOSE=false
FS_DIAL_TIMEOUT=10s
FS_CMD_TIMEOUT=30s
EOF

# Restrict permissions so only zabbix can read credentials
sudo chmod 640 /etc/zabbix/flashsystem/env
sudo chown zabbix:zabbix /etc/zabbix/flashsystem/env
```

> **Security Note on Password Authentication**: While `FS_PASSWORD` is supported as an alternative when `FS_KEYFILE` cannot be used, passwords can be exposed in process tables or command logs. SSH private keys (`FS_KEYFILE`) should always be preferred.

---

## 5. Verification and Testing

Verify connectivity and execution step-by-step before enabling items in Zabbix:

### Step 5.1: Test SSH Connectivity
```bash
sudo -u zabbix ssh \
    -i /etc/zabbix/ssh/flashsystem_key \
    -o UserKnownHostsFile=/etc/zabbix/ssh/known_hosts \
    monitor@10.10.10.50 \
    "svcinfo lssystem -delim :"
```
*Expected result*: Colon-delimited cluster properties (`id:0`, `name:...`).

### Step 5.2: Test Go Binary Directly
```bash
sudo -u zabbix \
    FS_USER=monitor \
    FS_KEYFILE=/etc/zabbix/ssh/flashsystem_key \
    FS_KNOWN_HOSTS=/etc/zabbix/ssh/known_hosts \
    /usr/local/bin/flashsystem-collector collect \
    -host 10.10.10.50 | head -c 500
```
*Expected result*: Valid JSON containing `status.success: true` and populated resource arrays.

### Step 5.3: Test External Script Wrapper
```bash
# Test Master Item collection
sudo -u zabbix \
    /usr/lib/zabbix/externalscripts/flashsystem_collector.sh \
    10.10.10.50 collect | head -c 500

# Test Low-Level Discovery (LLD)
sudo -u zabbix \
    /usr/lib/zabbix/externalscripts/flashsystem_collector.sh \
    10.10.10.50 discover drives
```
*Expected result*: Valid LLD JSON payload (`{"data": [{"{#DRIVEID}": "0", ...}]}`).

---

## 6. Zabbix Template Configuration

The repository includes a production-grade template located at:
`template/IBM FlashSystem Collector.yaml`

### Step 6.1: Import Template
1. In the Zabbix Web UI, navigate to **Data collection** > **Templates**.
2. Click **Import** in the upper right corner.
3. Select `template/IBM FlashSystem Collector.yaml` and confirm the import.

### Step 6.2: Link Template to Host
1. Navigate to **Data collection** > **Hosts** and select your FlashSystem host.
2. In the **Templates** tab, link **`IBM FlashSystem Collector`**.
3. In the **Interfaces** section, ensure the management IP address is configured as the primary interface (used by `{HOST.IP}`).

### Step 6.3: Template User Macros
Adjust the following template macros under the host's **Macros** tab if custom thresholds or intervals are required:

| Macro | Default | Description |
|---|---|---|
| `{$FS_COLLECT_INTERVAL}` | `60` | Polling interval for the Master Item (in seconds). |
| `{$FS_TIMEOUT}` | `55` | Execution timeout for the Master Item external script (must be `< {$FS_COLLECT_INTERVAL}`). |
| `{$FS_DISCOVERY_INTERVAL}` | `4h` | Polling interval for LLD rules. Matches `FS_DISCOVERY_TTL`. |
| `{$FS_POOL_WARN_PCT}` | `80` | Pool utilization warning threshold percentage (0–100). |
| `{$FS_POOL_CRIT_PCT}` | `90` | Pool utilization disaster/critical threshold percentage (0–100). |
| `{$FS_LATENCY_READ_WARN}` | `10` | Average read latency warning threshold in milliseconds. |
| `{$FS_LATENCY_WRITE_WARN}` | `10` | Average write latency warning threshold in milliseconds. |
| `{$FS_CPU_WARN}` | `80` | Node canister CPU utilization warning threshold percentage. |

---

## 7. Configuration Reference

Configuration options follow strict precedence:
**CLI Flags > Environment Variables > Default Values**

| Flag | Environment Variable | Default | Allowed Values / Validation | Description |
|---|---|---|---|---|
| `-host` | `FS_HOST` | *(empty)* | String (Required) | FlashSystem management IP or hostname. |
| `-port` | `FS_PORT` | `22` | Integer (1–65535) | SSH management port. |
| `-user` | `FS_USER` | *(empty)* | String (Required) | Storage user account (`Monitor` role). |
| `-keyfile` | `FS_KEYFILE` | *(empty)* | File path | Path to SSH private key. Mutually exclusive with `-password`. |
| `-password` | `FS_PASSWORD` | *(empty)* | String | SSH password (insecure; visible in process listing). |
| `-known-hosts` | `FS_KNOWN_HOSTS` | *(empty)* | File path | Path to `known_hosts` file for host verification. |
| `-insecure` | `FS_INSECURE` | `false` | `true`, `false`, `1`, `0` | Disables SSH host-key verification. **Use only in lab testing**. |
| `-cache-dir` | `FS_CACHE_DIR` | `/var/tmp` | Writable directory path | Directory storing host JSON cache files. |
| `-metrics-ttl` | `FS_METRICS_TTL` | `5m` | Duration (>= `1s`, e.g., `5m`, `300s`) | Cache validity duration for operational metrics. |
| `-discovery-ttl` | `FS_DISCOVERY_TTL` | `4h` | Duration (>= `1m`, e.g., `4h`, `240m`) | Cache validity duration for LLD discovery. |
| `-performance-ttl` | `FS_PERFORMANCE_TTL` | `1m` | Duration (>= `1s`, e.g., `1m`, `60s`) | Cache validity duration for performance stats (alias: `-perf-ttl` / `FS_PERF_TTL`). |
| `-dial-timeout` | `FS_DIAL_TIMEOUT` | `10s` | Duration (>= `1s`, e.g., `10s`) | Timeout for TCP dial and SSH handshake. |
| `-cmd-timeout` | `FS_CMD_TIMEOUT` | `30s` | Duration (>= `1s`, e.g., `30s`) | Timeout per individual SSH command execution. |
| `-max-json-bytes`| `FS_MAX_JSON_BYTES` | `1048576` (1MB)| Integer (>= `1024`) | Maximum bytes for stdout JSON payload. |
| `-max-volumes` | `FS_MAX_VOLUMES` | `500` | Integer (>= `1`) | Maximum volume records included in JSON output. |
| `-max-drives` | `FS_MAX_DRIVES` | `500` | Integer (>= `1`) | Maximum drive records included in JSON output. |
| `-verbose` | `FS_VERBOSE` | `false` | `true`, `false`, `1`, `0` | Enables debug logging to `stderr`. |
| `-category` | *(none)* | *(empty)* | `drives`, `pools`, `volumes`, `enclosures`, `nodes`, `ports`, `batteries`, `psus` | **Required** for `discover` subcommand. |

---

## 8. IBM FlashSystem Commands & Metrics Reference

The collector executes the following native CLI commands via `svcinfo` using colon-delimited formatting (`-delim :`):

| Resource | CLI Command | Monitored Attributes & Discovery Macros |
|---|---|---|
| **System** | `svcinfo lssystem` | Cluster name, code level (firmware), cluster status, total raw capacity, total free space, statistics status. |
| **Nodes** | `svcinfo lsnode` | Node status, name, IO group (`{#NODEID}`, `{#NODENAME}`, `{#NODESTATUS}`, `{#NODEIOGROUP}`). |
| **Enclosures** | `svcinfo lsenclosure` | Enclosure status, machine model (`{#ENCLOSUREID}`, `{#ENCLOSURESTATUS}`, `{#ENCLOSUREMODEL}`). |
| **Drives** | `svcinfo lsdrive` | Physical drive status, tier type, capacity (`{#DRIVEID}`, `{#ENCLOSUREID}`, `{#SLOTID}`, `{#DRIVESTATUS}`, `{#DRIVETYPE}`, `{#DRIVECAPACITY}`). |
| **Storage Pools** | `svcinfo lsmdiskgrp` | Pool status, capacity, free capacity, used capacity, volume count (`{#POOLID}`, `{#POOLNAME}`, `{#POOLSTATUS}`). |
| **Volumes** | `svcinfo lsvdisk` | Volume status, assigned pool (`{#VOLUMEID}`, `{#VOLUMENAME}`, `{#VOLUMESTATUS}`, `{#VOLUMEPOOL}`). |
| **Fibre Channel** | `svcinfo lsportfc` | FC port status, WWPN, speed, canister node (`{#PORTID}`, `{#WWPN}`, `{#PORTSTATUS}`, `{#PORTNODENAME}`, `{#PORTSPEED}`). |
| **FlashCopy** | `svcinfo lsfcmap` | Copy map progress, status, copy rate, source/target disks. |
| **Replication** | `svcinfo lsreplicationrelationship` *(legacy fallback: `lsrcrelationship`)* | Remote copy status, sync progress, consistency group state. |
| **Performance** | `svcinfo lssystemstats` | Read/write IOPS, read/write MB/s, read/write latency (ms), CPU usage (%), FC throughput, FC IOPS. |
| **Enclosure Batteries** | `svcinfo lsenclosurebattery` | Enclosure battery status, charge level (%), EOL warning, recondition needed (`{#ENCLOSUREID}`, `{#BATTERYID}`, `{#BATTERYSTATUS}`, `{#BATTERYCHARGE}`, `{#BATTERYEOL}`, `{#BATTERYRECO}`). |
| **Power Supply Units** | `svcinfo lsenclosurepsu` | PSU status, input/output failure flags, fan failure flags (`{#ENCLOSUREID}`, `{#PSUID}`, `{#PSUSTATUS}`, `{#PSUINPUT}`, `{#PSUOUTPUT}`, `{#PSUFAN}`). |

> **Enabling Storage Statistics**:
> The `lssystemstats` command requires background statistics collection to be enabled on the storage cluster. If statistics are disabled (`statistics_status: off`), execute the following command on the FlashSystem CLI:
> ```bash
> svctask startstats -interval 1
> ```

---

## 9. Cache and Fallback Behavior

- **Cache File Location**: `<cache-dir>/flashsystem_<sanitized_host>.cache.json` (e.g., `/var/tmp/flashsystem_10.10.10.50.cache.json`).
- **Atomic Writes**: Written to a temporary file (`.flashsystem_cache_*.tmp`) and renamed atomically (`os.Rename`), preventing file corruption if interrupted.
- **Cache Purge**: Expired entries are automatically purged at the start of each `collect` execution.
- **Stale Fallback**: If an SSH connection failure occurs during `collect`, the collector reads existing cache entries regardless of expiration. It produces a valid JSON response tagged with `"SSH failed (stale cache from <timestamp>)"`, ensuring monitoring graphs do not drop to zero and triggers with evaluation windows do not falsely fire.

---

## 10. Troubleshooting

| Symptom | Probable Cause | Diagnostic Command | Corrective Action |
|---|---|---|---|
| `SSH host key verification not configured` | Neither `FS_KNOWN_HOSTS` nor `FS_INSECURE` is specified. | Check `/etc/zabbix/flashsystem/env` | Run `ssh-keyscan` to populate `/etc/zabbix/ssh/known_hosts` and set `FS_KNOWN_HOSTS`. |
| `Host key mismatch` / `known_hosts: key mismatch` | FlashSystem controller reinstalled or management IP moved. | `ssh-keygen -l -f /etc/zabbix/ssh/known_hosts` | Refresh host key entry using `ssh-keyscan -p 22 <IP> > /etc/zabbix/ssh/known_hosts`. |
| `SSH key file not accessible` | Permission denied or key missing. | `ls -la /etc/zabbix/ssh/flashsystem_key` | Verify ownership `zabbix:zabbix` and mode `chmod 600`. |
| `SSH authentication required` | Neither `FS_KEYFILE` nor `FS_PASSWORD` is configured. | `sudo -u zabbix env \| grep FS_` | Ensure `/etc/zabbix/flashsystem/env` defines `FS_KEYFILE` or `FS_PASSWORD`. |
| `command ... timed out after 30s` | FlashSystem CLI overloaded or slow network. | `ping <IP>` and check storage CPU | Increase timeout via `FS_CMD_TIMEOUT=60s` or `-cmd-timeout 60s`. |
| `empty response from lssystemstats` | Performance stats collection is inactive on storage. | Run `svcinfo lssystem \| grep statistics_status` | Enable stats on FlashSystem: `svctask startstats -interval 1`. |
| Zabbix item status: `Unsupported` | Timeout reached before external script finished. | `tail -n 50 /var/log/zabbix/zabbix_server.log` | Increase script timeout in `zabbix_server.conf` (`Timeout=30` or `55`) and template macro `{$FS_TIMEOUT}`. |
| Discovery returns `{"data":[]}` | Invalid category or SSH failure. | `tail -n 50 /var/log/zabbix/flashsystem_collector.log` | Validate category name (`drives`, `pools`, `volumes`, `enclosures`, `nodes`, `ports`, `batteries`, `psus`) and verify SSH key access. |
| `cache directory error` | `/var/tmp` or custom cache dir not writable. | `sudo -u zabbix touch /var/tmp/test_write` | Fix permissions on cache directory (`chown zabbix:zabbix /var/tmp` or use dedicated dir). |
| Collector output truncated | Total JSON payload exceeds `MaxJSONBytes` (1MB). | Check `status.truncated_at` in output JSON | Adjust `-max-volumes` or increase `FS_MAX_JSON_BYTES=2097152`. |

---

## 11. Repository Structure

```text
flashsystem-collector/
├── go.mod                              # Go module definition (Go 1.25.0)
├── go.sum                              # Checksums for dependencies
├── main.go                             # Application entrypoint and CLI routing
├── internal/
│   ├── cache/                          # Persistent atomic disk cache implementation
│   ├── collectors/                     # Resource collectors for FlashSystem CLI
│   │   ├── collector.go                # Collector interface and concurrent Runner
│   │   ├── system.go                   # lssystem collector
│   │   ├── nodes.go                    # lsnode collector
│   │   ├── enclosures.go               # lsenclosure collector
│   │   ├── drives.go                   # lsdrive collector
│   │   ├── pools.go                    # lsmdiskgrp collector
│   │   ├── volumes.go                  # lsvdisk collector
│   │   ├── ports.go                    # lsportfc collector
│   │   ├── flashcopy.go                # lsfcmap collector
│   │   ├── replication.go              # lsreplicationrelationship collector
│   │   ├── batteries.go                # lsenclosurebattery collector
│   │   ├── psus.go                     # lsenclosurepsu collector
│   │   └── performance.go              # lssystemstats collector
│   ├── config/                         # CLI flag and environment variable parsing
│   ├── parser/                         # Colon-delimited svcinfo parser
│   ├── ssh/                            # SSH connection pooling and timeout handling
│   └── zabbix/                         # Output formatting and LLD builders
├── scripts/
│   └── flashsystem_collector.sh        # Zabbix External Script wrapper with flock
├── template/
│   └── IBM FlashSystem Collector.yaml # Production Zabbix 6.4+ / 7.x template
├── env/
│   └── example.env                     # Sample environment file template
├── LICENSE                             # Apache 2.0 license
├── instructions.md                     # Deep technical notes
└── README.md                           # Main installation and configuration guide
```

---

## License

This project is licensed under the Apache License 2.0. See the [LICENSE](LICENSE) file for details.
