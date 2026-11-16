//go:build !windows

package input

import (
	"path/filepath"
	"syscall"
	"testing"
)

func TestFIFOIsSkipped(t *testing.T) {
	archive := linkedSource(t, func(source, _ string) error {
		return syscall.Mkfifo(filepath.Join(source, "pipe"), 0o644)
	})
	onlySkip(t, archive, "pipe", "input.special_skipped")
}
