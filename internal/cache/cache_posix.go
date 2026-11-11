//go:build !windows

package cache

import (
	"os"
	"syscall"
)

// validateCacheFileSecurity verifies that a cache file on POSIX systems is a regular file,
// has restrictive permissions (0600 or stricter, no group/other access), and is owned by
// the current process UID or root (UID 0). This prevents loading untrusted or poisoned
// cache files planted by other local users in shared directories like /var/tmp.
func validateCacheFileSecurity(info os.FileInfo) bool {
	if !info.Mode().IsRegular() {
		return false
	}
	if info.Mode().Perm()&0077 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return isAuthorizedOwner(stat.Uid, uint32(os.Getuid()))
}

// isAuthorizedOwner returns true if the file owner UID matches the current process UID
// or root (UID 0).
func isAuthorizedOwner(fileUID, currentUID uint32) bool {
	return fileUID == currentUID || fileUID == 0
}
