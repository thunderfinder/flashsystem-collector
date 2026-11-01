# Installation 🚀

This guide provides end-to-end instructions for compiling, installing, configuring, and verifying `flashsystem-collector` on your Zabbix Server or Zabbix Proxy host.

---

## 1. Prerequisites

Before installing the collector, ensure the target system meets these requirements:

- **Host System**: Zabbix Server or Zabbix Proxy running on Linux (RHEL 8/9, Rocky/AlmaLinux, Debian 11/12, or Ubuntu 20.04/22.04/24.04).
- **Zabbix User**: The standard `zabbix` system user must exist.
- **Packages**:
  - `bash` (version 4.0 or higher)
  - `util-linux` (provides `flock`)
  - `openssh-clients` (provides `ssh-keygen` and `ssh-keyscan`)

---

## 2. Building the Go Binary

The binary should be compiled as a static executable with all symbols trimmed and dynamic C library links disabled (`CGO_ENABLED=0`).

### Compiling from Source
On any system with Go 1.25.0+ installed:
```bash
# Clone the repository
git clone https://github.com/thunderfinder/flashsystem-collector.git
cd flashsystem-collector

# Verify dependencies and run tests
go mod download
go test -race ./...

# Build statically linked binary
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o flashsystem-collector .
```

### Verifying Binary Static Linkage
Check that the binary has no shared object dependencies:
```bash
file flashsystem-collector
# Expected output:
# flashsystem-collector: ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked, stripped
```

---

## 3. Directory Structure & Permissions

Create the required operational directories with strict ownership and permission boundaries:

```bash
# Configuration directory (readable only by root and zabbix group)
sudo mkdir -p /etc/zabbix/flashsystem
sudo chown root:zabbix /etc/zabbix/flashsystem
sudo chmod 0750 /etc/zabbix/flashsystem

# SSH credentials directory (strictly private to zabbix user)
sudo mkdir -p /etc/zabbix/ssh
sudo chown zabbix:zabbix /etc/zabbix/ssh
sudo chmod 0700 /etc/zabbix/ssh

# Host locking directory
sudo mkdir -p /var/lock/flashsystem
sudo chown zabbix:zabbix /var/lock/flashsystem
sudo chmod 0755 /var/lock/flashsystem

# Ensure log file destination exists and is writable
sudo touch /var/log/zabbix/flashsystem_collector.log
sudo chown zabbix:zabbix /var/log/zabbix/flashsystem_collector.log
sudo chmod 0644 /var/log/zabbix/flashsystem_collector.log

# Ensure cache directory is writable (default /var/tmp is 1777 sticky)
sudo mkdir -p /var/tmp
```

---

## 4. Installing the Binary and Wrapper Script

Copy the compiled binary and the bash wrapper to their target locations:

```bash
# 1. Install binary to system binary path
sudo install -m 0755 flashsystem-collector /usr/local/bin/flashsystem-collector

# 2. Install wrapper script to Zabbix ExternalScripts directory
# Note: On RHEL/Debian, this is typically /usr/lib/zabbix/externalscripts
sudo install -m 0755 scripts/flashsystem_collector.sh \
    /usr/lib/zabbix/externalscripts/flashsystem_collector.sh
```

---

## 5. Setting Up Environment Configuration

The wrapper script loads settings from `/etc/zabbix/flashsystem/env`. Create this file based on `env/example.env`:

```bash
sudo tee /etc/zabbix/flashsystem/env > /dev/null << 'EOF'
# =============================================================================
# IBM FlashSystem Collector - Operational Environment Configuration
# =============================================================================

# FlashSystem Storage User (requires Monitor role on storage)
FS_USER=zabbix_monitor

# SSH Authentication: Path to private key (chmod 0600)
FS_KEYFILE=/etc/zabbix/ssh/flashsystem_key

# SSH Host Key Verification: Path to known_hosts file
FS_KNOWN_HOSTS=/etc/zabbix/ssh/known_hosts

# Set to true ONLY in isolated testing environments
FS_INSECURE=false

# Cache directory (defaults to /var/tmp)
FS_CACHE_DIR=/var/tmp

# Cache TTL policies
FS_METRICS_TTL=5m
FS_DISCOVERY_TTL=4h
FS_PERFORMANCE_TTL=1m

# Output limits
FS_MAX_JSON_BYTES=1048576
FS_MAX_VOLUMES=500
FS_MAX_DRIVES=500

# Timeouts
FS_DIAL_TIMEOUT=10s
FS_CMD_TIMEOUT=30s
EOF

# Harden permissions on the environment file
sudo chown root:zabbix /etc/zabbix/flashsystem/env
sudo chmod 0640 /etc/zabbix/flashsystem/env
```

---

## 6. Verifying the Installation

Run these manual test commands as the `zabbix` system user:

### Step 1: Check Version
```bash
sudo -u zabbix /usr/local/bin/flashsystem-collector version
# Expected: flashsystem-collector 2.0.0
```

### Step 2: Validate Storage Connectivity & Master Collection
Replace `<FLASHYSTEM_IP>` with your storage array's management IP:
```bash
sudo -u zabbix /usr/lib/zabbix/externalscripts/flashsystem_collector.sh <FLASHYSTEM_IP> collect
```
Verify that the output is valid JSON and contains `"success": true`:
```bash
sudo -u zabbix /usr/lib/zabbix/externalscripts/flashsystem_collector.sh <FLASHYSTEM_IP> collect \
    | grep -o '"success":\s*[a-z]*'
# Expected: "success": true
```

### Step 3: Validate Discovery
Test LLD discovery for storage pools:
```bash
sudo -u zabbix /usr/lib/zabbix/externalscripts/flashsystem_collector.sh <FLASHYSTEM_IP> discover pools
```
Verify that it outputs valid Zabbix LLD JSON containing `{#POOLNAME}` macros:
```json
{
  "data": [
    {
      "{#POOLID}": "0",
      "{#POOLNAME}": "Pool_NVMe",
      "{#POOLSTATUS}": "online"
    }
  ]
}
```

If all three tests succeed, your installation is complete and ready for Zabbix template linking.
