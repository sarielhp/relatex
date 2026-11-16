package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A failed conversion moves the earlier products, with their accessibility claim, out of sight
// and says why; the next successful publish clears both.
func TestFailedConversionSetsAsideStaleProducts(t *testing.T) {
	parent := t.TempDir()
	output := filepath.Join(parent, "output")
	putFile(t, output, "manifest.json", "old")
	putFile(t, output, "accessibility.json", `{"claim":"WCAG 2.2 AA"}`)
	putFile(t, output, "accessibility.html", "old report")
	putFile(t, output, "_work/attempt/log", "work")

	setAsideProducts(output, `LaTeX Error: Command \frq already defined.`)

	for _, gone := range []string{"manifest.json", "accessibility.json", "accessibility.html", "index.html"} {
		if present(output, gone) {
			t.Errorf("%s still looks current", gone)
		}
	}
	for _, kept := range []string{"_stale/accessibility.json", "_stale/manifest.json", "_work/attempt/log", failureNote} {
		if !present(output, kept) {
			t.Errorf("%s is missing", kept)
		}
	}
	note, _ := os.ReadFile(filepath.Join(output, failureNote))
	if !strings.Contains(string(note), `\frq already defined`) || !strings.Contains(string(note), staleDirectory) {
		t.Errorf("note: %q", note)
	}
	if refuses(t, output) {
		t.Error("a failed output is refused by the next conversion")
	}

	setAsideProducts(output, "second failure")
	if !present(output, "_stale/accessibility.json") {
		t.Error("a second failure lost the stale products")
	}

	staged := filepath.Join(parent, ".relatex-publish-3")
	putFile(t, staged, "manifest.json", "new")
	publishInto(staged, output)
	if present(output, "_stale") || present(output, failureNote) || !present(output, "manifest.json") {
		t.Error("a successful publish left the failure behind")
	}
}
