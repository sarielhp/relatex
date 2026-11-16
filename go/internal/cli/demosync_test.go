package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDemoSyncNeedsAFullProductAndAnOutput: without the product's source map the command says how to
// get one, and the output directory is required (decision 192).
func TestDemoSyncNeedsAFullProductAndAnOutput(t *testing.T) {
	run := func(arguments ...string) (string, int) {
		errs, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
		if err != nil {
			t.Fatal(err)
		}
		defer errs.Close()
		var out bytes.Buffer
		status := Run(append([]string{"demo-sync"}, arguments...), &out, errs)
		message, _ := os.ReadFile(errs.Name())
		return string(message), status
	}
	product := t.TempDir()
	message, status := run(product, "-o", filepath.Join(t.TempDir(), "demo"))
	if status == 0 || !strings.Contains(message, "--product DIR") {
		t.Errorf("status %d, message %q", status, message)
	}
	if _, status := run(product); status != 2 {
		t.Errorf("without -o: status %d", status)
	}
}
