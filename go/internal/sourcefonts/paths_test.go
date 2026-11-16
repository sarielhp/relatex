package sourcefonts

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func windowsSlash(path string) string { return strings.ReplaceAll(path, `\`, "/") }

func TestLinkedFontFileUsesForwardSlashes(t *testing.T) {
	sha := strings.Repeat("a", 64)
	winPath := `C:\texlive\2026\texmf-dist\fonts\opentype\public\lm\lmroman10-regular.otf`
	got := linkedFontFile(winPath, sha, "opentype", 1234, windowsSlash)
	wantSource := "C:/texlive/2026/texmf-dist/fonts/opentype/public/lm/lmroman10-regular.otf"
	if got["source"] != wantSource {
		t.Errorf("file.source = %q, want %q", got["source"], wantSource)
	}
	wantName := sha[:16] + "-lmroman10-regular.otf"
	if got["name"] != wantName {
		t.Errorf("file.name = %q, want %q", got["name"], wantName)
	}
}

func TestFileFontBindingUsesForwardSlashes(t *testing.T) {
	sha := strings.Repeat("b", 64)
	winPath := `C:\Users\author\fonts\Custom Font.otf`
	got := fileFontBinding("CustomFont", "CustomFont", winPath, sha, []any{}, windowsSlash)
	want := "C:/Users/author/fonts/Custom Font.otf"
	if got["full_source_path"] != want {
		t.Errorf("full_source_path = %q, want %q", got["full_source_path"], want)
	}
	deps := rec.FetchMap(got, "dependencies")
	if _, ok := deps[want]; !ok || len(deps) != 1 {
		t.Errorf("dependencies = %v, want key %q", deps, want)
	}
}
