# References 📚

This page indexes official IBM technical publications, Zabbix documentation, protocol standards, and repository resources.

---

## 1. IBM FlashSystem & Spectrum Virtualize References

The following IBM Redbooks and command manuals provide authoritative guidance on storage architecture and CLI syntax (included in `base_source_docs/`):

- **IBM SAN Volume Controller and FlashSystem CLI Command Guide**: Detailed syntax for `svcinfo` commands (`lssystem`, `lsmdiskgrp`, `lsvdisk`, `lsnode`, `lsdrive`, `lsenclosure`, `lsenclosurebattery`, `lsenclosurepsu`, `lsportfc`, `lsfcmap`, `lsreplicationrelationship`, `lssystemstats`).
- **IBM Redbooks SG24-8543**: *Implementing the IBM FlashSystem with IBM Spectrum Virtualize V8.5*. Comprehensive guide to FlashSystem architecture, clustering, and storage pool provisioning.
- **IBM Redbooks SG24-8561**: *IBM FlashSystem 7300 Architecture and Implementation*. Deep dive into NVMe architecture, canister redundancy, and high-performance flash controllers.
- **IBM Redbooks REDP-5668**: *IBM FlashSystem 5200 Product Guide*. Small-form-factor enterprise NVMe storage architecture.
- **IBM Redbooks REDP-5736**: *IBM FlashSystem 9500 Architecture and Implementation Guide*. High-end enterprise multi-canister performance and replication topologies.
- **IBM Redbooks REDP-5741**: *IBM FlashSystem Cyber Vault and Safeguarded Copy Architecture*. Snapshot resiliency and cyber recovery.

---

## 2. Zabbix Monitoring Documentation

- **Zabbix Official Documentation (7.0 LTS / 7.4)**: *Zabbix Manual - External Checks, Low-Level Discovery, and Master/Dependent Items*.
  - Reference: `base_source_docs/Zabbix_Documentation_7.4.en.txt`
- **JSONPath Preprocessing in Zabbix**: Using JSONPath filters (`$.pools[?(@.name=='{#POOLNAME}')]`) for extracting array values.
- **JavaScript Preprocessing**: Executing ECMA-compatible scripts within the Zabbix server engine for capacity multiplier normalization.

---

## 3. Standards and Cryptographic RFCs

- **RFC 4253**: *The Secure Shell (SSH) Transport Layer Protocol*.
- **RFC 4252**: *The Secure Shell (SSH) Authentication Protocol*.
- **OpenSSH Format Specifications**: Specification for OpenSSH public and private key formats.
- **POSIX.1-2017**: Standards for file permissions (`chmod`), file locking (`flock`), and atomic file operations (`rename`).

---

## 4. Repository Resources

- **`template/IBM FlashSystem Collector.yaml`**: The production Zabbix template for importing Master Items, Dependent Items, and LLD rules.
- **`scripts/flashsystem_collector.sh`**: The official shell wrapper implementing file locking and environment loading.
- **`env/example.env`**: Production environment configuration template.
- **`AGENTS.md`**: Architectural guidelines and agent instructions for the repository.
