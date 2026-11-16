//go:build !windows

package lockfile

import (
	"errors"
	"os"
	"syscall"
)

// lock is flock(2) with LOCK_EX, and LOCK_NB unless wait: held false when another holds it.
func lock(file *os.File, wait bool) (bool, error) {
	mode := syscall.LOCK_EX
	if !wait {
		mode |= syscall.LOCK_NB
	}
	err := syscall.Flock(int(file.Fd()), mode)
	if err == nil {
		return true, nil
	}
	if !wait && errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return false, err
}

func unlock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
