package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A build that still asks for a rerun did not converge; the failure names what it left undefined.
// (Undefined references alone, on stable aux files, are a draft that converged: the probe passes it.)
func TestUnconvergedBuildsNameTheUndefinedReferences(t *testing.T) {
	directory := t.TempDir()
	output := filepath.Join(directory, "control", "output")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	log.WriteString("LaTeX Warning: Reference `sec:gone' on page 1 undefined on input line 7.\n")
	log.WriteString("Package natbib Warning: Citation `knuth84' on page 1 undefined on input line 9.\n")
	log.WriteString("LaTeX Warning: Reference `sec:gone' on page 2 undefined on input line 12.\n")
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		log.WriteString("LaTeX Warning: Citation `" + name + "' on page 3 undefined on input line 20.\n")
	}
	log.WriteString("LaTeX Warning: There were undefined references.\n")
	log.WriteString("LaTeX Warning: Label(s) may have changed. Rerun to get cross-references right.\n")
	if err := os.WriteFile(filepath.Join(output, "pass-3.log"), []byte(log.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	build := rec.Map{"compiled": true, "passes": int64(3), "unresolved_references_or_rerun": true, "errors": []any{}}
	got := engineFailure(directory, rec.Map{"control": build, "external": build})
	want := "The builds did not converge: LaTeX still asked for another run after 3 LaTeX passes; undefined: " +
		"reference sec:gone, citation knuth84, citation a, citation b, citation c and 2 more"
	if got != want {
		t.Errorf("got %q", got)
	}
	if err := os.WriteFile(filepath.Join(output, "pass-3.log"), []byte("Rerun to get cross-references right.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = engineFailure(directory, rec.Map{"control": build, "external": build})
	if got != "The builds did not converge: LaTeX still asked for another run after 3 LaTeX passes (see pass-3.log)" {
		t.Errorf("got %q", got)
	}
}
