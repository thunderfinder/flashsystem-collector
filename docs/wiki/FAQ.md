# Frequently Asked Questions (FAQ) ❓

This page answers frequently asked architectural, security, and operational questions regarding `flashsystem-collector`.

---

### Q1: Why use an SSH CLI collector instead of SNMP or the REST API?
**A**: IBM FlashSystem's CLI (`svcinfo`) provides complete, real-time access to all hardware subsystems (batteries, PSUs, FC optical port states, and system statistics) with zero licensing costs and minimal controller CPU impact. SNMP MIBs lack many of these deep attributes, while REST API sessions require complex token lifecycle management and higher memory overhead on older microcode versions.

---

### Q2: Will running this collector impact my storage array's I/O performance?
**A**: No. The collector queries in-memory storage statistics using a strictly throttled internal worker pool (`MaxConcurrent = 3`). Queries are read-only and cached aggressively on disk. Storage arrays easily service these queries without impacting data-path read/write operations.

---

### Q3: What happens if the storage array's management IP is temporarily offline?
**A**: The collector automatically detects the SSH timeout and invokes **Stale-Data Fallback**. It retrieves the most recent uncorrupted records from the disk cache and emits a valid JSON payload tagged with `"SSH failed (using stale cache): <error>"`. This prevents Zabbix graphs from dropping to zero and stops alert storms.

---

### Q4: Why do performance metrics (`read_io`, `write_io`, `cpu_pc`) return 0 or empty?
**A**: The IBM FlashSystem microcode requires background performance collection to be enabled. Run this command on the storage array CLI:
```bash
svctask startstats -interval 1
```

---

### Q5: Can I run this collector against multiple FlashSystem arrays?
**A**: Yes! In Zabbix, simply create a host for each FlashSystem array and link the `IBM FlashSystem Collector` template. The wrapper script uses independent lock files (`/var/lock/flashsystem/flashsystem_<host>.lock`) and isolated cache files (`/var/tmp/flashsystem_<host>.cache.json`) for each host IP.

---

### Q6: Can I use passwords instead of SSH keys?
**A**: Password authentication is supported for isolated lab networks via `FS_PASSWORD`, but is strongly discouraged in production because passwords are visible in system process tables (`ps -ef`). Public-key authentication (`FS_KEYFILE`) with `chmod 0600` is the recommended standard.

---

### Q7: Why do storage pool capacities in Zabbix differ slightly from GUI values?
**A**: The collector passes `-bytes` to `svcinfo lsmdiskgrp` to capture exact binary byte counts ($1\text{ TiB} = 1,099,511,627,776\text{ Bytes}$). Some storage management GUIs display decimal gigabytes ($1\text{ TB} = 1,000,000,000,000\text{ Bytes}$) or round to the nearest whole gigabyte. Zabbix converts binary bytes into clean binary multipliers (TiB/GiB).
