# Sentinel Journal

## 2025-05-18 - SSH Private Key File Permissions Enforcement

**Vulnerability:** SSH private keys configured via `-keyfile` or `FS_KEYFILE` were only verified for existence, allowing group- or world-accessible keys (e.g., `0644`, `0666`) to be loaded and used by the collector on multi-user systems.

**Learning:** On shared monitoring servers or proxies where Zabbix runs, overly permissive private key files expose FlashSystem storage access credentials to other local users or compromised processes. Unlike the standard OpenSSH client which refuses to use unprotected keys, Go's `golang.org/x/crypto/ssh` does not automatically check filesystem permissions when parsing keys from byte slices.

**Prevention:** Always enforce strict POSIX permissions (`mode.Perm() & 0077 == 0`) and ensure the file is a regular file during configuration validation before reading or parsing the private key.
