# Release and Upgrade 📦

This page documents release versioning, distribution procedures, template synchronization, and rollback strategies for `flashsystem-collector`.

---

## 1. Release Versioning

The project adheres to **Semantic Versioning (SemVer 2.0.0)** (`MAJOR.MINOR.PATCH`):

- **MAJOR**: Incompatible API or output schema changes (e.g., changing root JSON field names or altering required CLI flags).
- **MINOR**: Backward-compatible feature additions (e.g., adding a new collector domain or introducing new LLD macros).
- **PATCH**: Backward-compatible bug fixes, security hardening, or parser corrections.

The binary version is defined directly in `main.go`:
```go
const version = "2.0.0"
```

---

## 2. Upgrade Procedure

Upgrading `flashsystem-collector` in a production Zabbix environment is designed to be zero-downtime:

### Step 1: Compile the New Release Binary
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o flashsystem-collector.new .
```

### Step 2: Swap the Binary Atomically
```bash
# Verify new binary version
./flashsystem-collector.new version

# Atomically install over existing binary
sudo install -m 0755 flashsystem-collector.new /usr/local/bin/flashsystem-collector
rm -f flashsystem-collector.new
```

### Step 3: Update Wrapper Script (if modified)
```bash
sudo install -m 0755 scripts/flashsystem_collector.sh \
    /usr/lib/zabbix/externalscripts/flashsystem_collector.sh
```

### Step 4: Update Zabbix Template (if schema updated)
1. In Zabbix Web UI, navigate to **Data collection** > **Templates**.
2. Click **Import**, select `template/IBM FlashSystem Collector.yaml`.
3. Check **Update existing** and import.

---

## 3. Rollback Strategy

If an unexpected regression occurs after an upgrade:

1. **Rollback Binary**:
   Restore the previous binary from backup or recompile the prior Git release tag.
2. **Clear Disk Cache**:
   Remove existing cache files to ensure new/old schemas do not conflict:
   ```bash
   sudo rm -f /var/tmp/flashsystem_*.cache.json
   ```
3. **Verify Zabbix Ingestion**:
   Run a test collection to verify that the Master Item receives clean JSON and triggers clear.
