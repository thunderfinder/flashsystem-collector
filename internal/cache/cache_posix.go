//go:build !windows

package cache

import (
	"os"
	"syscall"
)

// checkFileOwnerUID verifies that the file is owned by targetUID or root (UID 0) on POSIX platforms.
func checkFileOwnerUID(info os.FileInfo, targetUID uint32) bool {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Uid == targetUID || stat.Uid == 0
	}
	return false
}
