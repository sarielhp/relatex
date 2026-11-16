package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/lockfile"
)

// slotWait is the pause between two looks for a free session slot.
const slotWait = 250 * time.Millisecond

// slotPatience is how long a check waits for a free slot before it fails.
const slotPatience = 30 * time.Minute

var unsafeKey = regexp.MustCompile(`[^A-Za-z0-9.-]+`)

// slot is a held session slot: an exclusive lock on one of the endpoint's slot files.
type slot struct{ file *os.File }

// slotDirectory is where an endpoint's slot files live, shared by every relatex process and by
// the Ruby checks (Relatex::BrowserSessions), so the cap holds across all of them.
func slotDirectory(settings Settings) string {
	key := "local"
	if settings.remote() {
		key = unsafeKey.ReplaceAllString(settings.Endpoint, "_")
	}
	return filepath.Join(command.CacheRoot(), "browser-sessions", key)
}

// acquireSlot waits for one of the endpoint's session slots and holds it until release.
func acquireSlot(settings Settings) (*slot, error) {
	directory := slotDirectory(settings)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(slotPatience)
	for {
		held, err := trySlots(directory, settings.sessions())
		if held != nil || err != nil {
			return held, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for a browser session slot in %s", directory)
		}
		time.Sleep(slotWait)
	}
}

func trySlots(directory string, count int) (*slot, error) {
	for index := range count {
		path := filepath.Join(directory, fmt.Sprintf("slot-%d.lock", index))
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
		if err != nil {
			return nil, err
		}
		if held, err := lockfile.TryLock(file); err == nil && held {
			return &slot{file: file}, nil
		}
		file.Close()
	}
	return nil, nil
}

func (s *slot) release() {
	if s == nil || s.file == nil {
		return
	}
	_ = lockfile.Unlock(s.file) // closing releases it too
	s.file.Close()
}
