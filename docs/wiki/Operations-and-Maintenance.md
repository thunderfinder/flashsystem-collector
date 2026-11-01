# Operations and Maintenance 🔧

This guide outlines standard procedures for day-to-day administration, routine health verification, manual metric collection, log management, and cache maintenance for `flashsystem-collector`.

---

## 1. Running Manual Metric Collections

When verifying storage configuration or debugging a connectivity issue, execute the collector directly from the command line as the `zabbix` system user.

### Master Collection via Wrapper Script
```bash
sudo -u zabbix /usr/lib/zabbix/externalscripts/flashsystem_collector.sh <FLASHSYSTEM_IP> collect
```

### Inspecting Formatted JSON Output
Format the output with Python's JSON tool to verify overall success and inspect specific metrics:
```bash
sudo -u zabbix /usr/lib/zabbix/externalscripts/flashsystem_collector.sh <FLASHSYSTEM_IP> collect \
    | python3 -c "
import sys, json
data = json.load(sys.stdin)
print('Overall Success:', data['status']['success'])
print('Execution Duration (ms):', data['status']['duration_ms'])
print('Storage System Name:', data['system'][0].get('name', 'N/A') if data['system'] else 'N/A')
print('Firmware Level:', data['system'][0].get('code_level', 'N/A') if data['system'] else 'N/A')
print('Discovered Nodes:', len(data['nodes']))
print('Discovered Pools:', len(data['pools']))
print('Discovered Drives:', len(data['drives']))
print('Discovered Volumes:', len(data['volumes']))
if data['status']['errors']:
    print('Errors:', data['status']['errors'])
"
```

### Manual LLD Discovery Verification
Test Low-Level Discovery across any supported category:
```bash
sudo -u zabbix /usr/lib/zabbix/externalscripts/flashsystem_collector.sh <FLASHSYSTEM_IP> discover drives
sudo -u zabbix /usr/lib/zabbix/externalscripts/flashsystem_collector.sh <FLASHSYSTEM_IP> discover pools
sudo -u zabbix /usr/lib/zabbix/externalscripts/flashsystem_collector.sh <FLASHSYSTEM_IP> discover batteries
```

---

## 2. Log Management and Rotation

The collector and wrapper emit logs through two distinct channels:

1. **Zabbix Server / Proxy Log (`/var/log/zabbix/zabbix_server.log`)**:
   - The Go binary writes diagnostic error messages to `stderr`. Zabbix captures this output and records it in the main server/proxy log if a script fails.
2. **Wrapper Execution Log (`/var/log/zabbix/flashsystem_collector.log`)**:
   - The shell wrapper records timestamps, exit codes, output sizes, and lock events.

### Built-in Wrapper Log Rotation
`scripts/flashsystem_collector.sh` includes automatic size-based rotation:
- Threshold: **10 MB** (`LOG_MAX_BYTES=10485760`).
- When the log file exceeds 10 MB, it is moved to `flashsystem_collector.log.1` and a fresh log file is created.

### Production `logrotate` Configuration
For enterprise environments requiring compression and multi-week log retention, configure `logrotate` in `/etc/logrotate.d/flashsystem-collector`:

```text
/var/log/zabbix/flashsystem_collector.log {
    weekly
    rotate 8
    missingok
    notifempty
    compress
    delaycompress
    create 0644 zabbix zabbix
}
```

---

## 3. Cache Maintenance and Purge Procedures

Under normal operations, the collector automatically purges expired cache records at the start of each run. However, manual cache clearing may be desirable after major storage reconfigurations (e.g., mass drive replacements or storage pool consolidations).

### Inspecting Cache Files
Cache files are named by sanitized host in `/var/tmp`:
```bash
ls -lh /var/tmp/flashsystem_*.cache.json
```

### Inspecting Cache Stats Directly
```bash
python3 -c "
import json
with open('/var/tmp/flashsystem_10.10.10.50.cache.json') as f:
    cache = json.load(f)
print('Total Cached Keys:', len(cache))
for key, item in cache.items():
    print(f'  {key:30} CachedAt: {item.get(\"cached_at\")} TTL: {item.get(\"ttl_seconds\")}s')
"
```

### Flushing the Cache
To force an immediate, full hardware re-scan across all 12 collector domains, delete the target cache file:
```bash
sudo rm -f /var/tmp/flashsystem_<FLASHSYSTEM_IP>.cache.json
```
On the next poll, the collector will connect via SSH and rebuild all cache tiers from live storage data.

---

## 4. Routine Maintenance Checklist

Perform these routine checks quarterly or after storage firmware upgrades:

| Check Item | Action | Expected State |
| :--- | :--- | :--- |
| **Log Health** | `tail -n 100 /var/log/zabbix/flashsystem_collector.log` | Clean execution entries, no unhandled exit codes. |
| **Lock Cleanup** | `ls -la /var/lock/flashsystem/` | Lock files exist with 0-byte size; no orphaned processes. |
| **SSH Key Expiration** | Check age of `/etc/zabbix/ssh/flashsystem_key` | Key intact; permissions remain `0600`. |
| **Storage Stats State** | Check `statistics_status` in `lssystem` | Must remain `on` on the FlashSystem. |
| **Zabbix Item Queue** | Review Zabbix **Administration** > **Queue** | External checks executing on time without backlog. |
