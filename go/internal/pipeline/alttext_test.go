package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/input"
	"github.com/sarielhp/relatex/internal/rec"
)

// books/0003 (decision 136): a ZIP that carries relatex/alt.toml beside its main file (a store
// item's author files) is read as the paper's sidecar; a ZIP input once read none, and the
// author's 411 reviewed descriptions never reached the export.
func TestAZipCarriesItsSidecarBesideTheMainFile(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := diagnostics.LoadProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	zip := filepath.Join(work, "paper.zip")
	if err := os.WriteFile(zip, []byte("PK"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(work, "input")
	sidecar := filepath.Join(archive, "files", "book", "relatex", "alt.toml")
	if err := os.MkdirAll(filepath.Dir(sidecar), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecar, []byte("[figs.absent]\nalt = \"A figure the paper does not have\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &conversion{input: zip, diagnostics: diagnostics.New(profile, nil, false),
		document: rec.Map{"nodes": rec.Map{}, "block_order": []any{}, "assets": rec.Map{"items": rec.Map{}}}}
	if got := c.altTextPath(); got != "" {
		t.Fatalf("before the input is captured: %q, want none", got)
	}
	c.archive = &input.Archive{Directory: archive, Manifest: rec.Map{"main": "book/book.tex"}}
	if got := c.altTextPath(); got != sidecar {
		t.Fatalf("sidecar %q, want %q", got, sidecar)
	}
	c.applyAltText()
	if codes := c.diagnosticCodes(); codes.Get("alt.unmatched") != int64(1) {
		t.Fatalf("the ZIP's sidecar was not read: codes %v", codes)
	}
}

// Two figures whose names differ only in case give the warning figures.case_twins instead of
// stopping the export (decision 129, amendment 5).
func TestCaseTwinFiguresWarn(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := diagnostics.LoadProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	image := func(source, asset string) rec.Map {
		return rec.Map{"type": "image", "asset": asset, "source": source}
	}
	nodes := rec.Map{"n-1": rec.Map{"id": "n-1", "content": []any{image("figs/Plot.pdf", "a"), image("img/plot.pdf", "b")}}}
	c := &conversion{diagnostics: diagnostics.New(profile, nil, false), document: rec.Map{"nodes": nodes, "block_order": []any{"n-1"}}}
	c.figureNameDiagnostics()
	if codes := c.diagnosticCodes(); codes.Get("figures.case_twins") != int64(1) {
		t.Fatalf("codes %v", codes)
	}
}
