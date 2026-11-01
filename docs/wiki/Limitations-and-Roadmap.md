# Limitations and Roadmap 🗺️

This document details explicit technical limitations of the current implementation and outlines future engineering milestones.

---

## 1. Verified Limitations

- **Connection Lifecycle**:
  - A new SSH connection is established on cache misses. There is no long-lived daemon maintaining persistent SSH tunnels between polling runs.
- **Unpaged CLI Queries**:
  - `svcinfo lsvdisk` and `svcinfo lsdrive` do not support server-side pagination via the CLI. In massive clusters (>10,000 volumes), the storage CLI command itself can take several seconds to generate output.
- **Strict POSIX Permissions**:
  - The security check requiring `chmod 0600` on private keys applies to POSIX environments (Linux/Unix). On Windows, permission checking is skipped.
- **Passphrase-Protected Keys**:
  - Private SSH keys with passphrases cannot be unlocked automatically; keys must be unencrypted on disk and protected by strict filesystem permissions.
- **Single Host Scope**:
  - Each binary invocation queries exactly one FlashSystem IP. Monitoring multi-cluster installations requires setting up distinct Zabbix hosts.

---

## 2. Technical Debt & Agent Recommendations

From architectural reviews conducted across the codebase, several future improvements have been identified:

- **LogicForge Recommendation**: Investigate implementing a persistent daemon mode (e.g., lightweight UNIX socket or HTTP endpoint) for ultra-high-frequency environments ($<10\text{s}$ polling) to eliminate SSH handshake overhead entirely.
- **Sentinel Recommendation**: Support PKCS#11 hardware security modules (HSM) or SSH agent forwarding to allow automated authentication without unencrypted private keys on disk.
- **Bolt Recommendation**: Explore streaming JSON decoders to parse large multi-megabyte `lsvdisk` outputs with lower peak memory consumption.

---

## 3. Product Roadmap

### Milestone 2.1 (Target: Q3 2026)
- [ ] Add collector domain for storage host mappings (`svcinfo lshost`, `svcinfo lshostvdiskmap`).
- [ ] Implement native Prometheus exporter mode as an optional flag (`-export-prometheus`).
- [ ] Add support for encrypted SSH keys using password-protected environment secrets.

### Milestone 2.2 (Target: Q4 2026)
- [ ] Dynamic adaptive polling (automatically increasing collection frequency during detected storage error states).
- [ ] Integration with IBM Safeguarded Copy and Cyber Vault snapshot verification telemetry.
