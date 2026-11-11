//go:build windows

package cache

import "os"

// validateCacheFileSecurity on Windows verifies that the cache file is a regular file.
func validateCacheFileSecurity(info os.FileInfo) bool {
	return info.Mode().IsRegular()
}

// isAuthorizedOwner is a no-op on Windows systems.
func isAuthorizedOwner(fileUID, currentUID uint32) bool {
	return true
}
