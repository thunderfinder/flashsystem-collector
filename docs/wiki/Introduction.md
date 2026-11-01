# Introduction 📖

## Project Background

Monitoring enterprise block storage platforms has historically posed a fundamental engineering tradeoff between protocol overhead, storage controller safety, and telemetry granularity. Storage administrators managing **IBM FlashSystem** and **IBM Spectrum Virtualize** (formerly SAN Volume Controller and Storwize) clusters frequently faced several operational challenges:

1. **SNMP Limitations**: Standard MIBs implemented on storage controllers often lack deep operational telemetry (such as detailed battery end-of-life status, power supply unit redundancy, per-port optical states, and near real-time I/O performance statistics). Furthermore, SNMP polling cannot dynamically discover nested topologies with rich contextual metadata.
2. **REST API Overhead**: While modern Spectrum Virtualize releases provide a REST API, deploying external monitoring agents that handle HTTPS session tokens, token expiration, and frequent REST requests across large clusters can consume noticeable storage controller CPU.
3. **SSH Process Flooding**: Naive monitoring scripts often invoke individual SSH sessions for each metric check (e.g., one SSH session for `lssystem`, another for `lsmdiskgrp`, another for `lsnode`). This triggers hundreds of SSH handshakes per minute, quickly exhausting the storage system's SSH daemon session limit (`sshd` max unauthenticated connections / channel limits), driving controller CPU spikes, and triggering audit alert storms.
4. **Monitoring Blindspots During Failover**: When storage management interfaces experience transient network blips or controller failovers, standard monitoring scripts time out and return empty values. In Zabbix, this causes metric graphs to drop to zero and triggers cascading false positive alerts (alert storm) across operations centers.

**`flashsystem-collector`** was created to resolve these challenges through a unified, high-performance Go binary paired with an intelligent Bash wrapper and a modern Zabbix template.

---

## The Problems It Solves

- **Minimizing SSH Overhead**: Instead of executing dozens of separate SSH connections per host, the collector establishes a single authenticated SSH session per run, leveraging an internal worker pool and goroutine semaphore (max 3 concurrent sessions) to execute all storage diagnostic queries in parallel.
- **Cache-First Resiliency**: Query results are cached on local disk with independent TTL tiers (e.g., 5 minutes for general health, 4 hours for static hardware discovery, 1 minute for performance metrics). Subsequent poll requests are served directly from cache without hitting the storage controllers.
- **Stale Cache Fallback**: When an SSH failure occurs (due to transient network timeouts, cluster maintenance, or management IP transitions), the collector automatically falls back to expired cached data while annotating the output with an explicit diagnostic warning. Monitoring dashboards remain populated and threshold triggers do not misfire.
- **Exact Byte Capacity Reporting**: The collector passes native `-bytes` flags to capacity queries (`lsmdiskgrp` and `lsvdisk`), avoiding rough approximations and enabling exact bit-level capacity monitoring in Zabbix.
- **Automated Lifecycle Discovery**: The collector implements Zabbix Low-Level Discovery (LLD) across 8 hardware and logical entities (pools, volumes, drives, enclosures, nodes, FC ports, batteries, and PSUs), eliminating manual inventory synchronization.

---

## Intended Users and Environments

- **Storage Administrators & SAN Engineers**: Teams managing on-premises IBM FlashSystem arrays requiring high-visibility dashboards for IOPS, latency, cache utilization, pool depletion forecasting, and hardware health.
- **Enterprise Operations Centers (NOC / SRE)**: Organizations utilizing Zabbix 6.4+ or 7.x to monitor mission-critical infrastructure with strict service level agreements (SLAs) and zero tolerance for false alarms during maintenance windows.
- **High-Security Regulated Environments**: Financial, healthcare, and governmental environments that enforce strict SSH public-key authentication, forbid cleartext passwords, require hardened file permissions (`chmod 0600`), and enforce cryptographic host-key verification (`known_hosts`) to mitigate MITM threats.

---

## Design Goals

- **Zero-Dependency Production Deployment**: Built with `CGO_ENABLED=0` as a fully static, standalone Linux executable with no dynamic C library dependencies.
- **Predictable Resource Footprint**: Controlled memory utilization, deterministic JSON output sizing, and protective payload truncation limits (`MaxJSONBytes`, `MaxVolumes`, `MaxDrives`).
- **Strict Separation of Concerns**:
  - The Go binary handles configuration parsing, SSH transport, parser normalization, caching, and JSON generation.
  - The Bash wrapper handles environment injection, file locking (`flock`), and process execution monitoring.
  - The Zabbix template handles ingestion, preprocessing, trigger logic, and visualization.
- **Crash-Resistant Output Guarantees**: Under no circumstances does the collector exit silently or emit malformed output. Every failure mode emits structured JSON with clear diagnostic error messages so Zabbix pollers never enter an unsupported state.

---

## Scope and Non-Goals

### What the Project Does (In Scope)
- Read-only data collection from IBM FlashSystem and Spectrum Virtualize CLI (`svcinfo`).
- Health and status monitoring of hardware components (canisters, drives, enclosures, batteries, PSUs, FC ports).
- Capacity reporting for storage pools and virtual disks (volumes).
- Real-time aggregate performance tracking via `lssystemstats` (IOPS, bandwidth, latency, CPU utilization).
- FlashCopy mapping and replication relationship status reporting.
- Low-Level Discovery (LLD) generation formatted for Zabbix server ingestion.

### What the Project Does NOT Do (Out of Scope / Non-Goals)
- **No Storage Configuration Modification**: The collector is strictly read-only and uses storage accounts with the `Monitor` role. It will never execute administrative modification commands (`svctask`).
- **No Long-Running Daemon or Agent Service**: The collector is an ephemeral execution tool invoked on-demand by Zabbix external scripts; it is not a background system daemon listening on network sockets.
- **No Direct Database Ingestion**: The collector does not connect directly to relational databases, InfluxDB, or Prometheus; it interfaces natively with Zabbix via External Check scripts and standard JSON stdout streams.
- **No Active Firmware Management**: The collector detects and reports cluster firmware revisions (`code_level`), but does not perform software updates or orchestrate node reboots.
