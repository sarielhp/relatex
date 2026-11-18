package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// BibTeX takes the first byte of "Željko" as its initial (fresh arXiv run, 2611.18355); the .bbl
// must be valid UTF-8 for LuaTeX, the loss recorded once, and a clean .bbl left alone.
func TestBibtexUTF8CutIsRepairedAndRecorded(t *testing.T) {
	bbl := filepath.Join(t.TempDir(), "paper.bbl")
	cut := "Samec, \xC5.~Stipčević, R.~Ursin\n"
	if err := os.WriteFile(bbl, []byte(cut), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &Probe{}
	p.repairBibtexUTF8(bbl)
	p.repairBibtexUTF8(bbl)
	got, err := os.ReadFile(bbl)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Samec, �.~Stipčević, R.~Ursin\n"; string(got) != want {
		t.Fatalf("repaired %q, want %q", got, want)
	}
	if len(p.degradations) != 1 || !strings.Contains(rec.AsMap(p.degradations[0])["reason"].(string), "multibyte") {
		t.Fatalf("degradations %v", p.degradations)
	}
	clean := &Probe{}
	clean.repairBibtexUTF8(bbl)
	clean.repairBibtexUTF8(filepath.Join(t.TempDir(), "missing.bbl"))
	if len(clean.degradations) != 0 {
		t.Fatalf("a valid or missing .bbl was recorded: %v", clean.degradations)
	}
}
