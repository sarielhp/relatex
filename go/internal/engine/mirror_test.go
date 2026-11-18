package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMirrorDirectoriesCreatesOpenoutTargets(t *testing.T) {
	working, output := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(working, "fragment", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	mirrorDirectories(working, output)
	if info, err := os.Stat(filepath.Join(output, "fragment", "deep")); err != nil || !info.IsDir() {
		t.Fatalf("output lacks the mirrored directory: %v", err)
	}
}
