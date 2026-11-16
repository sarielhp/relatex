//go:build !windows

package fspath

func isSharing(error) bool { return false }
