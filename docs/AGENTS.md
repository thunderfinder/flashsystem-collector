# AGENTS.md — docs Component Instructions

This directory contains project documentation, including the comprehensive 24-page GitHub Wiki mirror under `docs/wiki/` and architectural references.

---

## Technical Responsibilities

- **GitHub Wiki Architecture**: Source of truth for the project's official GitHub Wiki (`flashsystem-collector.wiki.git`).
- **Comprehensive Topic Coverage**: Maintains 24 dedicated topic pages plus navigation sidebar (`_Sidebar.md`), covering:
  - Architecture, How-It-Works, Features, Introduction, Home
  - Configuration, Installation, Operations-and-Maintenance, Zabbix-Integration
  - Low-Level-Discovery, Monitoring-Metrics, Cache-and-Resilience, SSH-and-Security
  - Performance-and-Scaling, Requirements-and-Compatibility, Testing-and-Quality, Troubleshooting
  - Contributing, Development, Release-and-Upgrade, Limitations-and-Roadmap, FAQ, Glossary, References
- **Zabbix & IBM Alignment**: Ensures metric names, LLD macro keys, CLI command syntax, and configuration variables match actual code and template definitions.

---

## Documentation Standards & Editing Rules (DocMaster Guidelines)

1. **GitHub Wiki Syntax & Cross-Linking**:
   - Internal page links must use GitHub Wiki syntax: `[[Page-Name]]` or `[[Link Title|Page-Name]]`.
   - Never use relative markdown file paths (such as `[Architecture](Architecture.md)`) for internal wiki links, as GitHub Wiki flattens all pages to the repository root.
   - When adding or removing pages, always update `_Sidebar.md` to keep the navigation hierarchy synchronized.
2. **Technical Accuracy & Zero Invention**:
   - **Go Toolchain**: Must document Go `1.25.0` (as required by `go.mod`), not older versions.
   - **12 Collectors**: Always reflect all 12 supported collectors (`system`, `nodes`, `enclosures`, `drives`, `pools`, `volumes`, `ports`, `flashcopy`, `replication`, `performance`, `batteries`, `psus`).
   - **8 LLD Discovery Categories**: Always reflect all 8 supported discovery categories (`drives`, `pools`, `volumes`, `enclosures`, `nodes`, `ports`, `batteries`, `psus`).
   - **Security Defaults**: Always document strict host key verification using `known_hosts` (`FS_KNOWN_HOSTS`) and private key permissions (`0600`) as standard. Document `FS_INSECURE=true` strictly as an opt-in override for isolated lab/testing environments with explicit warnings.
   - **TTL Hierarchy**: Document all three TTL tiers: Metrics TTL (`5m`), Discovery TTL (`4h`), and Performance TTL (`1m` via `FS_PERFORMANCE_TTL` or `FS_PERF_TTL`).
3. **Diagrams**:
   - Use standard Mermaid (`mermaid`) diagram blocks for flowcharts, architecture diagrams, and state machines.

---

## Validation Guidelines

- Check markdown syntax and link integrity across all modified wiki documents.
- Verify that command flags, configuration environment variables (`FS_*`), and JSON output examples match the code implementation in `main.go`, `internal/config/config.go`, and `internal/zabbix/output.go`.
- Ensure no broken links or orphaned pages exist in `docs/wiki/`.
