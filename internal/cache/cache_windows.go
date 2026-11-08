//go:build windows

package cache

import "os"

// checkFileOwnerUID is a no-op on Windows where POSIX numeric UIDs are not present.
func checkFileOwnerUID(info os.FileInfo, targetUID uint32) bool {
	return true
}
