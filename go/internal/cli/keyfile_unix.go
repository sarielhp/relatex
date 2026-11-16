//go:build !windows

package cli

import (
	"fmt"
	"io/fs"
)

// keyFileExposure is why others may read the key file, or "" when only its owner can: a group or
// other permission bit.
func keyFileExposure(_ string, info fs.FileInfo) string {
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Sprintf("may be read by others (mode %#o)", info.Mode().Perm())
	}
	return ""
}

// keyFileRemedy is how to make the key file the owner's alone.
func keyFileRemedy(path string) string { return "chmod 600 " + path }
