package browser

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestStartUsesRunScratchForUserData verifies Chromium's temporary user-data-dir is created under
// RELATEX_RUNS_DIR via command.Scratch and removed by Close (decision 138 E).
func TestStartUsesRunScratchForUserData(t *testing.T) {
	base := t.TempDir()
	t.Setenv("RELATEX_RUNS_DIR", base)
	seen := filepath.Join(t.TempDir(), "seen")
	t.Setenv("RELATEX_CHROMIUM", stubChromium(t, t.TempDir(), seen))

	b := &Browser{pending: map[int64]chan reply{}, done: make(chan struct{})}
	_, _ = b.start()
	userData := b.userData
	b.Close()

	resolved, _ := filepath.EvalSymlinks(base)
	if !strings.HasPrefix(userData, base) && !strings.HasPrefix(userData, resolved) {
		t.Fatalf("user-data-dir %s outside RELATEX_RUNS_DIR %s", userData, base)
	}
	recorded, err := os.ReadFile(seen)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(recorded), "--user-data-dir="+userData) {
		t.Fatalf("Chromium arguments %q missing --user-data-dir=%s", recorded, userData)
	}
	if _, statErr := os.Stat(userData); !os.IsNotExist(statErr) {
		t.Fatalf("Close left user-data-dir %s behind (err=%v)", userData, statErr)
	}
}

func stubChromium(t *testing.T, dir, seen string) string {
	t.Helper()
	line := "DevTools listening on ws://127.0.0.1:1/devtools/browser/00000000-0000-0000-0000-000000000000"
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "chromium.bat")
		body := "@echo off\r\necho %*> \"" + seen + "\"\r\necho " + line + " 1>&2\r\n"
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	path := filepath.Join(dir, "chromium")
	body := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"" + seen + "\"\nprintf '%s\\n' '" + line + "' >&2\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
