//go:build windows

package fontbinding

import "os"

// inode is 0 on Windows, whose file information carries no inode: the cached binding's key keeps
// the path, size and modification time.
func inode(os.FileInfo) uint64 {
	return 0
}
