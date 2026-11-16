package pipeline

import (
	"os"

	"github.com/sarielhp/relatex/internal/lockfile"
)

// A conversion attempt holds an exclusive lock on a file beside its directory for as long as its
// process runs, so another conversion of the same project never retires or compresses it while it
// works (two conversions of one paper at once: an agent's run and the release smoke test, 2026-10-29,
// lost files mid-run). The lock lives beside the directory, not in it, so no inventory of the
// attempt's files sees it; the kernel releases it when the process ends, however it ends.

// attemptLocks keeps the held lock files open for the life of the process.
var attemptLocks []*os.File

// AttemptLockPath is the lock file of an attempt directory.
func AttemptLockPath(attempt string) string {
	return attempt + ".lock"
}

// holdAttempt takes the attempt's lock for the rest of the process.
func holdAttempt(attempt string) {
	file, err := os.OpenFile(AttemptLockPath(attempt), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return
	}
	if lockfile.Lock(file) != nil {
		_ = file.Close()
		return
	}
	attemptLocks = append(attemptLocks, file)
}

// AttemptActive reports whether another process still works in an attempt: its lock file exists and
// is held. An attempt without a lock file (made before locks existed, or already finished) is idle.
func AttemptActive(attempt string) bool {
	file, err := os.OpenFile(AttemptLockPath(attempt), os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer file.Close()
	if held, err := lockfile.TryLock(file); err != nil || !held {
		return true
	}
	_ = lockfile.Unlock(file) // closing releases it too
	return false
}

// ReleaseAttempts lets go of the attempts this process holds, once their conversions ended and the
// process is about to retire them itself: a held attempt counts as another process's (AttemptActive
// opens its lock anew), so rlx kept the attempt of each successful run until the next one.
func ReleaseAttempts() {
	for _, file := range attemptLocks {
		_ = file.Close()
	}
	attemptLocks = nil
}
