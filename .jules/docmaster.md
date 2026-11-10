# DocMaster Journal 📚

## 2026-03-17 - SSH Host Key Verification and Security Defaults

**Learning:** The previous documentation recommended `FS_INSECURE=true` and `-o StrictHostKeyChecking=no` as the standard setup. In reality, `internal/ssh/client.go` defaults to `InsecureHostKey: false` and explicitly returns an error if neither `KnownHostsFile` nor `InsecureHostKey` is configured. Recommending insecure mode by default exposes monitoring environments to Man-In-The-Middle (MITM) attacks and obscures proper production hardening using `known_hosts` files via `FS_KNOWN_HOSTS`.

**Action:** Production documentation must always default to verified SSH host key checking using `ssh-keyscan` and `FS_KNOWN_HOSTS`. Document `FS_INSECURE=true` strictly as an opt-in override for isolated lab/testing environments with explicit warnings about its security implications.

## 2026-03-17 - Go Version Toolchain Alignment

**Learning:** `README.md` and initial notes cited `Go 1.21+` as the compiler requirement, but `go.mod` explicitly mandates `go 1.25.0`. Compiling on older toolchains causes build failures or unmanaged automatic toolchain upgrades.

**Action:** Standardize all build documentation, CI workflows, and prerequisite lists to Go `1.25.0` to reflect actual dependency requirements and static build guarantees (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`).

## 2026-03-17 - Official Zabbix Template Discovery and Integration

**Learning:** Previous notes in `instructions.md` stated that no Zabbix template was included in the repository. In fact, an official Zabbix 6.4+ YAML template exists at `template/IBM FlashSystem Collector.yaml` with preconfigured Master Item, 6 LLD discovery rules (`pools`, `drives`, `volumes`, `enclosures`, `nodes`, `ports`), JavaScript capacity byte conversions, dependent items, and customizable macros.

**Action:** Document the official template import process in `README.md`, including required user macros (`{$FS_COLLECT_INTERVAL}`, `{$FS_TIMEOUT}`, `{$FS_DISCOVERY_INTERVAL}`, thresholds), eliminating the need for manual item creation in Zabbix.

## 2026-11-01 - GitHub Wiki Architecture & Technical Documentation

**Learning:** The project lacked an organized, comprehensive GitHub Wiki detailing the inner workings, concurrency model (goroutine semaphore), multi-tier cache TTL architecture, atomic disk operations (`rename`), stale-data fallback behavior, LLD discovery definitions across all 8 supported categories, and complete metric catalogs. Keeping documentation solely in `README.md` or unversioned markdown files causes drift and operational opacity.

**Action:** Designed and created a 24-page GitHub Wiki structure (plus `_Sidebar.md`) synchronized directly in `docs/wiki/` and prepared in the GitHub Wiki repository (`thunderfinder/flashsystem-collector.wiki.git`). Validated all cross-links and structure using a dedicated validator tool (`validate_wiki.py`).

## 2026-11-02 - Alignment of 12 Collectors, 8 LLD Discovery Categories, and Performance TTL

**Learning:** A significant documentation gap existed between `README.md` and the actual implementation: `README.md` omitted 2 collectors (`batteries` / `lsenclosurebattery` and `psus` / `lsenclosurepsu`) and restricted the documented discovery categories to 6 instead of the 8 supported by `main.go`, `scripts/flashsystem_collector.sh`, and `template/IBM FlashSystem Collector.yaml`. Additionally, `README.md` and `env/example.env` omitted `-performance-ttl` / `FS_PERFORMANCE_TTL`, and `env/example.env` previously defaulted to `FS_INSECURE=true`, which conflicted with project security standards.

**Action:** Kept `README.md` synchronized with the source code by documenting all 12 collectors, all 8 LLD categories (`drives`, `pools`, `volumes`, `enclosures`, `nodes`, `ports`, `batteries`, `psus`), and `FS_PERFORMANCE_TTL` (default `1m`, duration >= 1s) with backward-compatible alias `FS_PERF_TTL`. Hardened `env/example.env` to default to `FS_KNOWN_HOSTS`.

## 2026-11-03 - SSH known_hosts Permission Hardening and Callback Mechanics

**Learning:** Documentation across the wiki and `README.md` omitted the security enforcement introduced for SSH `known_hosts` files in `internal/config/config.go` and `internal/ssh/client.go` (`info.Mode().IsRegular()` and `perm & 0022 == 0`, forbidding group and world write permissions, e.g. `chmod 0644` or `0600`). Additionally, `docs/wiki/How-It-Works.md` inaccurately documented host key callback construction as using `ssh.NewPublicKeyAuthMethod()` instead of `knownhosts.New()` returning `ssh.HostKeyCallback`, and cited command termination via `ssh.SIGKILL` instead of `ssh.SIGTERM`.

**Action:** Documented the regular file and POSIX non-writable permission requirements (`0644`/`0600`) for `known_hosts`, the automatic fallback to `~/.ssh/known_hosts`, the proper callback mechanics with `golang.org/x/crypto/ssh/knownhosts`, the `SIGTERM` cancellation signal, and added troubleshooting entries for `known_hosts` permission and regularity errors across `README.md` and `docs/wiki/`.

## 2026-11-04 - Zabbix Template Macro Alignment and Threshold Synchronization

**Learning:** A significant discrepancy existed between the documented Zabbix template user macros and the actual template definition in `template/IBM FlashSystem Collector.yaml`. `README.md` and `docs/wiki/Zabbix-Integration.md` only documented 8 macros and used outdated defaults for read and write latency (`10` ms instead of the template's flash-tuned values of `2` ms and `5` ms per IBM Redbook SG248543). Furthermore, 8 production template user macros were missing entirely from documentation: critical latency (`{$FS_LATENCY_READ_CRIT}`, `{$FS_LATENCY_WRITE_CRIT}`), write cache saturation (`{$FS_WRITE_CACHE_WARN}`, `{$FS_WRITE_CACHE_CRIT}`), temperature limits (`{$FS_TEMP_WARN}`, `{$FS_TEMP_CRIT}`), and battery health (`{$FS_BATTERY_HEALTH_WARN}`, `{$FS_BATTERY_HEALTH_CRIT}`). Also, `docs/wiki/Home.md` mistakenly suggested configuring credentials as host macros (`{$FS_USER}`, `{$FS_KEYFILE}`) rather than via `/etc/zabbix/flashsystem/env`, and `docs/wiki/Monitoring-Metrics.md` misidentified `flashsystem.battery.health` as returning character text instead of numeric percentage charge (`percent_charged`).

**Action:** Synchronized all 16 user macros, their flash-optimized defaults, and corresponding trigger thresholds across `README.md` and `docs/wiki/Zabbix-Integration.md`. Clarified environment-file-based credential configuration in `docs/wiki/Home.md` and corrected `flashsystem.battery.health` type and source in `docs/wiki/Monitoring-Metrics.md`.

## 2026-11-05 - Cache Purge Timing and 24-Hour Stale Age Bound Synchronization

**Learning:** Documentation across `README.md`, `instructions.md`, and several wiki articles incorrectly claimed that `cache.Purge()` executes at the start of every collection run, and described stale cache fallback as serving expired data indefinitely ("regardless of expiration"). In reality, `main.go` invokes `c.Purge()` only after a successful SSH connection is established, which ensures that expired cache entries are preserved on disk if SSH connectivity fails. Furthermore, `main.go` and `internal/collectors/collector.go` enforce a strict 24-hour upper bound (`maxStaleAge = 24 * time.Hour`), rejecting stale entries older than 24 hours to prevent reporting obsolete metrics or decommissioned infrastructure. Additionally, `runDiscover` leverages stale discovery cache within the 24-hour window before falling back to empty LLD `{"data":[]}`.

**Action:** Updated `README.md`, `instructions.md`, and wiki pages (`Cache-and-Resilience.md`, `How-It-Works.md`, `Architecture.md`, `Low-Level-Discovery.md`, `Operations-and-Maintenance.md`, and `Features.md`) to document post-connection cache purge execution, the 24-hour stale age bound (`maxStaleAge = 24h`), and discovery-mode stale recovery.

## 2026-11-06 - Technical Instructions Hardening and Implementation Alignment

**Learning:** `instructions.md` contained several discrepancies with the actual codebase and project security policies:
1. It cited `Go 1.21` as the minimum Go compiler requirement, conflicting with `go.mod`, GitHub Actions workflows, `README.md`, and `docs/wiki/` which require `Go 1.25.0+`.
2. It incorrectly claimed in Section 10 ("Limitaciones detectadas") that the binary aborted without checking `~/.ssh/known_hosts`. In reality, `internal/ssh/client.go` includes an automatic fallback to `~/.ssh/known_hosts` (with regular file and POSIX `perm & 0022 == 0` validation) when `FS_KNOWN_HOSTS` is unset before aborting.
3. Examples across the document consistently promoted `-insecure`, `FS_INSECURE=true`, and `-o StrictHostKeyChecking=no` as standard invocation patterns, obscuring secure production workflows with host-key verification (`FS_KNOWN_HOSTS=/etc/zabbix/ssh/known_hosts`).
4. Section 3 ("Requisitos identificados") and Section 5 omitted explicit documentation of the strict POSIX permission constraints on private keys (`0600`, `perm & 0077 == 0`) and `known_hosts` (`0644`/`0600`, `perm & 0022 == 0`).

**Action:** Updated `instructions.md` to reflect Go 1.25.0+, accurately document the `~/.ssh/known_hosts` fallback resolution path, detail strict file permission checks for credentials and host keys, and replace insecure defaults with hardened `FS_KNOWN_HOSTS` examples throughout execution, troubleshooting, and verification sections.

## 2026-11-10 - Full Technical Documentation & Wiki Architecture Audit

**Learning:** Conducted a comprehensive audit of all 24 GitHub Wiki topic pages plus `_Sidebar.md` under `docs/wiki/` against the authoritative source code, IBM FlashSystem command references, and Zabbix templates. Verified that every technical dimension (Go 1.25.0, 12 resource collectors, 8 LLD discovery categories, 3-tier TTL architecture, post-connect cache purge timing, 24-hour maximum stale age bound, POSIX key and known_hosts permissions, and 16 Zabbix template user macros) is consistently and rigorously documented. Because direct push access to `flashsystem-collector.wiki.git` is restricted in the execution environment, the full 25-file wiki suite remains prepared in `docs/wiki/` for synchronization.

**Action:** Confirmed publication status as `PREPARED` per WikiBuilder documentation guidelines. Verified 47 internal wiki links (`[[Page]]` format), 10 Mermaid architectural diagrams, zero broken links, and full alignment with project security and reliability contracts.
