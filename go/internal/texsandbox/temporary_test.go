package texsandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/command"
)

// A sandboxed program's TMPDIR is the run's own: Ghostscript (epstopdf) cannot start without a
// writable one, and relatex's /tmp/USER/relatex does not exist inside the sandbox's /tmp.
func TestSandboxTemporaryIsTheRunsOwn(t *testing.T) {
	previous := command.Workspace
	defer func() { command.Workspace = previous }()
	command.Workspace = ""
	attempt, _ := filepath.Abs(filepath.FromSlash("/attempt/eps"))
	if got := sandboxTemporary(map[string]string{"TEXMFVAR": filepath.Join(attempt, "texmf-var")}); got != filepath.Join(attempt, "tmp") {
		t.Errorf("without a workspace: %q", got)
	}
	command.Workspace = filepath.FromSlash("/workspace")
	if got := sandboxTemporary(map[string]string{"TEXMFVAR": "/attempt/eps/texmf-var"}); got != filepath.Join(command.Workspace, "_tmp") {
		t.Errorf("with a workspace: %q", got)
	}
}

// More readable directories are mounted after the sandbox's /tmp (so one under /tmp stays visible),
// read-only, and only when they exist.
func TestWrapWithMountsExtraDirectoriesReadOnly(t *testing.T) {
	engine := command.Executable("lualatex")
	if command.Executable("bwrap") == "" || engine == "" {
		t.Skip("no bubblewrap or lualatex")
	}
	root := t.TempDir()
	extra := filepath.Join(root, "styles")
	if err := os.Mkdir(extra, 0o755); err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{"TEXMFVAR": filepath.Join(root, "var"), "TEXMFCACHE": filepath.Join(root, "cache")}
	source := filepath.Join(root, "src")
	wrapped := strings.Join(Current(engine).WrapWith([]string{"true"}, source, filepath.Join(root, "out"), source,
		environment, []string{extra, filepath.Join(root, "missing")}), " ")
	mount := "--ro-bind " + extra + " " + extra
	if !strings.Contains(wrapped, mount) || strings.Index(wrapped, mount) < strings.Index(wrapped, "--tmpfs /tmp") {
		t.Fatalf("the extra directory is not mounted after /tmp: %s", wrapped)
	}
	if strings.Contains(wrapped, "missing") {
		t.Fatal("a missing directory is not mounted")
	}
}
