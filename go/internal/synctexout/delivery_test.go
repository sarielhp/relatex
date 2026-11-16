package synctexout

import (
	"strings"
	"testing"
)

func TestExternalSourceEntryUsesForwardSlashes(t *testing.T) {
	winOriginal := `C:\texlive\2026\texmf-dist\tex\latex\base\article.cls`
	entry := externalSourceEntry(1, winOriginal, func(p string) string { return strings.ReplaceAll(p, `\`, "/") })
	want := "C:/texlive/2026/texmf-dist/tex/latex/base/article.cls"
	if entry.Get("native_path") != want {
		t.Errorf("native_path = %q, want %q", entry.Get("native_path"), want)
	}
}
