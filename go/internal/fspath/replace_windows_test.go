//go:build windows

package fspath

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A viewer holding the PDF open blocks its replacement on Windows: Replace waits for it to let
// go, and names the program in use when it never does.
func TestReplaceWaitsForAnOpenTarget(t *testing.T) {
	directory := t.TempDir()
	from, to := filepath.Join(directory, "new.pdf"), filepath.Join(directory, "paper.pdf")
	for _, path := range []string{from, to} {
		if err := os.WriteFile(path, []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	held, err := os.Open(to)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		held.Close()
	}()
	if err := Replace(from, to); err != nil {
		t.Fatalf("Replace after the viewer let go: %v", err)
	}
	if err := os.WriteFile(from, []byte("again"), 0o644); err != nil {
		t.Fatal(err)
	}
	held, err = os.Open(to)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if err := Replace(from, to); !errors.Is(err, ErrInUse) {
		t.Fatalf("Replace onto a file held open throughout: %v, want ErrInUse", err)
	}
}
