//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// The key file rule reads Unix permission bits; Windows reads the access list (decision 129, D2).
func TestKeyFileCheckedByModeOnly(t *testing.T) {
	directory := t.TempDir()
	missing := describerSettings{keyFile: filepath.Join(directory, "none")}
	if got := failure(missing.checkKeyFile); !strings.Contains(got, "does not exist") || !strings.Contains(got, "chmod 600") {
		t.Errorf("missing key file: %q", got)
	}
	open := filepath.Join(directory, "open")
	if err := os.WriteFile(open, []byte("secret-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(open, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := failure(describerSettings{keyFile: open}.checkKeyFile); !strings.Contains(got, open) ||
		!strings.Contains(got, "chmod 600") || strings.Contains(got, "secret-key") {
		t.Errorf("0644 key file: %q", got)
	}
	// A FIFO and a file without read permission pass: relatex stats the key file and never opens
	// it (opening the FIFO would block, reading the other would fail).
	fifo := filepath.Join(directory, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(directory, "unreadable")
	if err := os.WriteFile(unreadable, []byte("k"), 0o200); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{fifo, unreadable} {
		if got := failure(describerSettings{keyFile: path}.checkKeyFile); got != "" {
			t.Errorf("%s refused: %q", path, got)
		}
	}
}
