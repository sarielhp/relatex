package pipeline

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// A layer left out is a recorded diagnostic, counted and printed with the others, not a stderr
// line alone: books/0003 shipped without its layout and no report said so.
func TestLeftOutLayersAreDiagnostics(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := diagnostics.LoadProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	c := &conversion{degradations: []any{}, diagnostics: diagnostics.New(profile, nil, false)}
	reason := "Missing or ambiguous native glyph paint: p375-g39 (2 matches)"
	if !c.degrade("layout", "no layout view", func() { rec.Fail("%s", reason) }) {
		t.Fatal("the layout failure was not left out")
	}
	if !c.degrade("synchronize", "no source synchronization", func() { rec.Fail("no map") }) {
		t.Fatal("the synchronization failure was not left out")
	}
	if c.degrade("layout", "no layout view", func() {}) {
		t.Fatal("a layout that succeeds is left out")
	}
	if got := c.diagnostics.Counts()["alert"]; got != int64(2) {
		t.Fatalf("alerts %v, want 2", got)
	}
	codes := c.diagnosticCodes()
	if codes.Get("layout.absent") != int64(1) || codes.Get("synchronization.absent") != int64(1) || len(codes) != 2 {
		t.Fatalf("codes %v", codes)
	}
	lines := strings.Join(c.diagnostics.Lines("warning"), "\n")
	if !strings.Contains(lines, "alert[layout.absent]: "+reason) {
		t.Fatalf("lines %q", lines)
	}
	if len(c.degradations) != 2 || c.degradations[0].(ojson.Object).Get("level") != "alert" {
		t.Fatalf("degradations %v", c.degradations)
	}
}
