//go:build windows

package fspath

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isSharing reports a file another program holds open: a sharing or lock violation.
func isSharing(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
