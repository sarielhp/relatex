// Package lockfile takes and releases exclusive locks on open files, one way on every platform:
// flock(2) on Unix, LockFileEx on Windows (decision 129). A lock is advisory on Unix and
// mandatory on Windows; relatex only ever locks files that hold nothing to read, so the two
// behave alike for it. The lock ends with Unlock, with closing the file, or with the process.
package lockfile

import (
	"fmt"
	"os"
)

// Lock waits until this process holds the file's exclusive lock.
func Lock(file *os.File) error {
	if _, err := lock(file, true); err != nil {
		return fmt.Errorf("lock %s: %w", file.Name(), err)
	}
	return nil
}

// TryLock takes the file's exclusive lock when it is free; false, with no error, when another
// open file holds it.
func TryLock(file *os.File) (bool, error) {
	held, err := lock(file, false)
	if err != nil {
		return false, fmt.Errorf("lock %s: %w", file.Name(), err)
	}
	return held, nil
}

// Unlock releases the file's lock.
func Unlock(file *os.File) error {
	if err := unlock(file); err != nil {
		return fmt.Errorf("unlock %s: %w", file.Name(), err)
	}
	return nil
}
