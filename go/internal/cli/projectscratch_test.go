package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The harness puts a conversion's project cache in its run's scratch (decision 090).
func TestProjectScratchFollowsProjectsDirectory(t *testing.T) {
	input := filepath.Join(t.TempDir(), "paper.zip")
	if err := os.WriteFile(input, []byte("zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("RELATEX_PROJECTS_DIR", "")
	if got := projectScratch(input); !strings.HasPrefix(got, filepath.Join(os.Getenv("XDG_CACHE_HOME"), "relatex", "projects", "paper.zip-")) {
		t.Fatalf("default project scratch %s", got)
	}
	runs := t.TempDir()
	t.Setenv("RELATEX_PROJECTS_DIR", runs)
	if got := projectScratch(input); !strings.HasPrefix(got, filepath.Join(runs, "paper.zip-")) {
		t.Fatalf("project scratch %s is not under %s", got, runs)
	}
}
