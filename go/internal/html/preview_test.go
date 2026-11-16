package html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// The page loads the preview script from beside it, and its policy allows only local scripts.
func TestPageLoadsThePreviewScriptBesideIt(t *testing.T) {
	if previewScriptTag != `<script src="preview.js" defer="defer"></script>` {
		t.Errorf("script tag %s", previewScriptTag)
	}
	policy := (&Paper{document: rec.Map{"nodes": rec.Map{}, "fonts": []any{}}}).contentSecurityPolicy()
	if !strings.Contains(policy, "script-src 'self' file:") || strings.Contains(policy, "script-src 'unsafe-inline'") {
		t.Errorf("policy %s", policy)
	}
}

// WritePreviewScript copies the repository's preview.js next to the page.
func TestWritePreviewScriptCopiesTheResource(t *testing.T) {
	previous := Root
	Root = filepath.Join("..", "..", "..")
	defer func() { Root = previous }()
	directory := t.TempDir()
	WritePreviewScript(directory)
	script, err := os.ReadFile(filepath.Join(directory, "preview.js"))
	if err != nil || !strings.Contains(string(script), "template[data-split-previews]") {
		t.Fatalf("script %v: %.60s", err, script)
	}
}
