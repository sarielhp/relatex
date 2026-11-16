package paperdoc

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A bibitem that opens with \line (content with no entry of its own) still belongs to its item.
func TestOwnsEntryIgnoresContentWithoutEntry(t *testing.T) {
	g := &citationGraph{
		nodes: rec.Map{"b": rec.Map{"id": "b", "content": []any{
			rec.Map{"bibliography_entry": nil}, rec.Map{"bibliography_entry": "e1"}}}},
		entries: map[any]rec.Map{"e1": {"number": int64(1)}},
	}
	if !g.ownsEntry("b", "e1", g.entries["e1"]) {
		t.Fatal("block should own its entry")
	}
}

// paperdoc accepts the same bibliography labels as the evidence reader: a year a letter tells apart is a label.
func TestBibliographyLabelAcceptsYearLetter(t *testing.T) {
	for _, label := range []string{"1", "ADM25", "2012b", "Cheng et al., 2021", "Gualtieri and Platt, 2021a"} {
		if !bibliographyLabel.MatchString(label) {
			t.Errorf("label %q rejected", label)
		}
	}
	for _, label := range []string{"0", "[1]", "2012bc", `\protect Cheng \bgroup et al.\egroup , 2021`, "Cheng {et al.}, 2021"} {
		if bibliographyLabel.MatchString(label) {
			t.Errorf("label %q accepted", label)
		}
	}
}
