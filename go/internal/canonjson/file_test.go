package canonjson

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Two conversions of one source settle the same cache entry at once: each write renames its own
// temporary file, so neither fails with "no such file" and the entry is one whole write.
func TestWriteFileConcurrentWritersOfOnePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settle.json")
	var group sync.WaitGroup
	errs := make(chan error, 32*20)
	for writer := range 32 {
		group.Go(func() {
			for range 20 {
				errs <- WriteFile(path, map[string]any{"writer": writer, "status": "settled"})
			}
		})
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent write failed: %v", err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("settled file is not one whole write: %q, %v", data, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files left beside the entry: %v, %v", entries, err)
	}
}
