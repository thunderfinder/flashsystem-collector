# Sentinel Journal

## 2025-05-18 - SSH Private Key File Permissions Enforcement

**Vulnerability:** SSH private keys configured via `-keyfile` or `FS_KEYFILE` were only verified for existence, allowing group- or world-accessible keys (e.g., `0644`, `0666`) to be loaded and used by the collector on multi-user systems.

**Learning:** On shared monitoring servers or proxies where Zabbix runs, overly permissive private key files expose FlashSystem storage access credentials to other local users or compromised processes. Unlike the standard OpenSSH client which refuses to use unprotected keys, Go's `golang.org/x/crypto/ssh` does not automatically check filesystem permissions when parsing keys from byte slices.

**Prevention:** Always enforce strict POSIX permissions (`mode.Perm() & 0077 == 0`) and ensure the file is a regular file during configuration validation before reading or parsing the private key.

## 2025-05-19 - SSH Known Hosts File Permissions and Integrity Enforcement

**Vulnerability:** SSH host key databases configured via `-known-hosts` or `FS_KNOWN_HOSTS` (and the fallback `~/.ssh/known_hosts`) were not checked for file type or write permissions, allowing group- or world-writable files (e.g., `0666`, `0664`, `0777`) or non-regular files to be trusted for host-key verification.

**Learning:** On shared monitoring systems running Zabbix server or proxy, any unprivileged local user or process could append or overwrite host keys in a permissive `known_hosts` file. Unlike standard OpenSSH which warns or rejects untrusted files, Go's `golang.org/x/crypto/ssh/knownhosts` parses files without checking filesystem permissions or verifying that the path is a regular file. An attacker modifying `known_hosts` could poison trusted host keys and execute a man-in-the-middle (MITM) attack to steal FlashSystem credentials during collector polling.

**Prevention:** Always enforce regular file validation (`info.Mode().IsRegular()`) and strict POSIX write permissions (`info.Mode().Perm() & 0022 == 0`, forbidding group and other write access) on host-key database files before passing them to `knownhosts.New`.
