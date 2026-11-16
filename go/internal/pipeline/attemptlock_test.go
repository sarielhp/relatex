package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

// An attempt whose lock is held reads as active; once released, or without a lock file, it is idle.
func TestAHeldAttemptIsActive(t *testing.T) {
	attempt := filepath.Join(t.TempDir(), "conversion-1")
	if err := os.Mkdir(attempt, 0o755); err != nil {
		t.Fatal(err)
	}
	if AttemptActive(attempt) {
		t.Fatal("an attempt without a lock file is idle")
	}
	before := len(attemptLocks)
	holdAttempt(attempt)
	if len(attemptLocks) != before+1 {
		t.Fatal("the lock was not taken")
	}
	if !AttemptActive(attempt) {
		t.Fatal("a held attempt must read as active")
	}
	held := attemptLocks[len(attemptLocks)-1]
	attemptLocks = attemptLocks[:before]
	_ = held.Close()
	if AttemptActive(attempt) {
		t.Fatal("a released attempt is idle")
	}
}

// A process that ended its conversions releases its attempts and may then retire them itself.
func TestReleasedAttemptsAreIdle(t *testing.T) {
	attempt := filepath.Join(t.TempDir(), "conversion-2")
	if err := os.Mkdir(attempt, 0o755); err != nil {
		t.Fatal(err)
	}
	saved := attemptLocks
	attemptLocks = nil
	defer func() { attemptLocks = saved }()
	holdAttempt(attempt)
	if !AttemptActive(attempt) {
		t.Fatal("a held attempt must read as active")
	}
	ReleaseAttempts()
	if AttemptActive(attempt) || len(attemptLocks) != 0 {
		t.Fatal("a released attempt is idle")
	}
}
