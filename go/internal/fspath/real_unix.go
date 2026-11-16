//go:build !windows

package fspath

import "path/filepath"

// Real is the path with every symbolic link resolved: filepath.EvalSymlinks.
func Real(path string) (string, error) { return filepath.EvalSymlinks(path) }
