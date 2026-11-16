package fspath

import (
	"errors"
	"io/fs"
	"math/rand/v2"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// LostRace reports whether a rename onto target failed because another process put target
// there first: "exists" or "not empty" on Unix; on Windows renaming a directory onto an
// existing one fails with "access denied", which counts when target now exists.
func LostRace(err error, target string) bool {
	if errors.Is(err, fs.ErrExist) || errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
		return true
	}
	if runtime.GOOS != "windows" || !errors.Is(err, fs.ErrPermission) {
		return false
	}
	_, statErr := os.Lstat(target)
	return statErr == nil
}

// replaceWait bounds Replace's retries; each pause is random between replacePause and twice
// that, so writers that collide do not retry in step.
const (
	replaceWait  = 2 * time.Second
	replacePause = 10 * time.Millisecond
)

// ErrInUse is Replace's error when another program kept the target open: on Windows a file a
// program holds open (Acrobat holding the PDF) cannot be replaced.
var ErrInUse = errors.New("in use by another program")

// Replace renames from onto to. On Unix it is os.Rename. On Windows a target another program
// holds open refuses the rename (sharing violation, access denied); Replace tries again for
// about two seconds, then fails with ErrInUse wrapping the last error.
func Replace(from, to string) error {
	if runtime.GOOS == "windows" {
		lock := targetLock(to)
		lock.Lock()
		defer lock.Unlock()
	}
	err := os.Rename(from, to)
	if err == nil || !retriable(err) {
		return err
	}
	for deadline := time.Now().Add(replaceWait); time.Now().Before(deadline); {
		time.Sleep(replacePause + rand.N(replacePause))
		if err = os.Rename(from, to); err == nil || !retriable(err) {
			return err
		}
	}
	return &inUse{err: err}
}

// replacing holds one lock per target: on Windows a rename onto a file another rename is
// replacing fails, so the writers of one process take turns and only other programs need the
// retries. The map keeps one small mutex per path the process replaced.
var replacing sync.Map

func targetLock(target string) *sync.Mutex {
	key := strings.ToLower(native.normal(target))
	lock, _ := replacing.LoadOrStore(key, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

type inUse struct{ err error }

func (e *inUse) Error() string        { return e.err.Error() + ": " + ErrInUse.Error() }
func (e *inUse) Unwrap() []error      { return []error{ErrInUse, e.err} }
func retriable(err error) bool        { return runtime.GOOS == "windows" && sharingViolation(err) }
func sharingViolation(err error) bool { return errors.Is(err, fs.ErrPermission) || isSharing(err) }
