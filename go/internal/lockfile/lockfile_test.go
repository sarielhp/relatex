package lockfile

import (
	"os"
	"path/filepath"
	"testing"
)

func open(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

// Two opens of one lock file in one process exclude each other, as two processes do (flock locks
// an open file description, LockFileEx a handle).
func TestTryLockFailsWhileAnotherHoldsIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	first, second := open(t, path), open(t, path)
	if err := Lock(first); err != nil {
		t.Fatal(err)
	}
	held, err := TryLock(second)
	if err != nil || held {
		t.Fatalf("second lock taken while the first holds it: %v %v", held, err)
	}
	if err = Unlock(first); err != nil {
		t.Fatal(err)
	}
	held, err = TryLock(second)
	if err != nil || !held {
		t.Fatalf("free lock not taken: %v %v", held, err)
	}
}

func TestClosingReleasesTheLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	first, second := open(t, path), open(t, path)
	if held, err := TryLock(first); err != nil || !held {
		t.Fatalf("free lock not taken: %v %v", held, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if held, err := TryLock(second); err != nil || !held {
		t.Fatalf("lock of a closed file not released: %v %v", held, err)
	}
}
