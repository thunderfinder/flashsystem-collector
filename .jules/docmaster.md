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
