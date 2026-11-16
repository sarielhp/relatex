package fontmodel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
)

func linkedFont(t *testing.T, content string) (rec.Map, string) {
	t.Helper()
	source := filepath.Join(t.TempDir(), "face.otf")
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	file := rec.Map{"name": "0123456789abcdef-face.otf", "sha256": command.FileDigest(source), "source": source}
	return rec.Map{"fonts": rec.Map{"items": []any{rec.Map{"file": file}, rec.Map{"file": file}, rec.Map{}}}}, source
}

func TestWriteFilesCopiesEachLinkedFontOnceBesideThePage(t *testing.T) {
	document, _ := linkedFont(t, "font bytes")
	directory := t.TempDir()
	WriteFiles(document, directory)
	data, err := os.ReadFile(filepath.Join(directory, "fonts", "0123456789abcdef-face.otf"))
	if err != nil || string(data) != "font bytes" {
		t.Errorf("font file = %q, %v", data, err)
	}
}

func TestWriteFilesWritesNothingWithoutLinkedFonts(t *testing.T) {
	directory := t.TempDir()
	WriteFiles(rec.Map{"fonts": rec.Map{"items": []any{rec.Map{}}}}, directory)
	if _, err := os.Stat(filepath.Join(directory, "fonts")); err == nil {
		t.Error("a fonts directory was written for a document without linked fonts")
	}
}

func TestWriteFilesRefusesAFontThatChanged(t *testing.T) {
	document, source := linkedFont(t, "font bytes")
	if err := os.WriteFile(source, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recover() == nil {
			t.Error("a changed font was copied")
		}
	}()
	WriteFiles(document, t.TempDir())
}

func TestReadSourcePathAcceptsForwardSlashExportPaths(t *testing.T) {
	got := readSourcePath("C:/texlive/2026/face.otf", func(p string) string { return strings.ReplaceAll(p, "/", `\`) })
	want := `C:\texlive\2026\face.otf`
	if got != want {
		t.Errorf("readSourcePath = %q, want %q", got, want)
	}
	document, source := linkedFont(t, "slashed bytes")
	items := rec.FetchList(rec.FetchMap(document, "fonts"), "items")
	rec.FetchMap(rec.AsMap(items[0]), "file")["source"] = filepath.ToSlash(source)
	directory := t.TempDir()
	WriteFiles(document, directory)
	data, err := os.ReadFile(filepath.Join(directory, "fonts", "0123456789abcdef-face.otf"))
	if err != nil || string(data) != "slashed bytes" {
		t.Errorf("slashed WriteFiles = %q, %v", data, err)
	}
}
