//go:build windows

package lockfile

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// The whole file's range, as far as LockFileEx can name it: a lock may extend past the end.
const (
	rangeLow  = ^uint32(0)
	rangeHigh = ^uint32(0)
)

// lock is LockFileEx, exclusive, failing at once unless wait: held false when another holds it.
func lock(file *os.File, wait bool) (bool, error) {
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if !wait {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	overlapped := new(windows.Overlapped)
	err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, rangeLow, rangeHigh, overlapped)
	if err == nil {
		return true, nil
	}
	if !wait && errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return false, err
}

func unlock(file *os.File) error {
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, rangeLow, rangeHigh, new(windows.Overlapped))
}
