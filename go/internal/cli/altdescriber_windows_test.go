//go:build windows

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The key file rule reads the file's access list on Windows (decision 129, D2): a file in the
// user's temporary directory is the user's own (SYSTEM, Administrators and the owner); granting
// Everyone read exposes it.
func TestKeyFileCheckedByAccessList(t *testing.T) {
	directory := t.TempDir()
	missing := describerSettings{keyFile: filepath.Join(directory, "none")}
	if got := failure(missing.checkKeyFile); !strings.Contains(got, "does not exist") || !strings.Contains(got, "icacls") {
		t.Errorf("missing key file: %q", got)
	}
	key := filepath.Join(directory, "key")
	if err := os.WriteFile(key, []byte("secret-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := failure(describerSettings{keyFile: key}.checkKeyFile); got != "" {
		t.Fatalf("the user's own key file refused: %q", got)
	}
	icacls, err := exec.LookPath("icacls")
	if err != nil {
		t.Skip("no icacls")
	}
	if output, err := exec.Command(icacls, key, "/grant", "*S-1-1-0:R").CombinedOutput(); err != nil {
		t.Fatalf("icacls: %v: %s", err, output)
	}
	if got := failure(describerSettings{keyFile: key}.checkKeyFile); !strings.Contains(got, "may be read by") ||
		!strings.Contains(got, key) || strings.Contains(got, "secret-key") {
		t.Errorf("a key file Everyone may read: %q", got)
	}
}
