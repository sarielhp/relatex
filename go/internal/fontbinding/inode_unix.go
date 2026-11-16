//go:build !windows

package fontbinding

import (
	"os"
	"syscall"
)

// inode is the file's inode number, part of a cached binding's key.
func inode(info os.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Ino
	}
	return 0
}
