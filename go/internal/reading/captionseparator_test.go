package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A caption line's number may end at the caption's first word (a class with no separator:
// "Table 1 Stellar wind…", papers2/0352) or at a parenthetical title ("Lemma 5.1 (Lovász Local
// Lemma).", papers4/1105), not only at ":" or ".".
func TestACaptionLineNumberEndsAtAWordOrATitle(t *testing.T) {
	cases := []struct{ text, number, counter string }{
		{"Table 1 Stellar wind conditions.", "1", "table"},
		{"Lemma 5.1 (Lovász Local Lemma). Let", "5.1", "lemma"},
		{"Table 2: Line data.", "2", "table"},
	}
	for _, c := range cases {
		e := &Evidence{blocks: []rec.Map{textBlock("b1", "paragraph", int64(7), c.text)}}
		label := rec.Map{"name": "x", "counter": c.counter, "number": c.number, "entity": int64(7), "seq": int64(1)}
		if got := blockIDs(e.captionLineBlocks(label, int64(7))); len(got) != 1 {
			t.Errorf("%q opens no caption line for %s", c.text, c.number)
		}
	}
	// A longer number, or prose that goes on in lower case, is no caption line of the label.
	for _, text := range []string{"Table 12 Stellar wind.", "Table 1 shows the wind."} {
		e := &Evidence{blocks: []rec.Map{textBlock("b1", "paragraph", int64(7), text)}}
		label := rec.Map{"name": "x", "counter": "table", "number": "1", "entity": int64(7), "seq": int64(1)}
		if got := blockIDs(e.captionLineBlocks(label, int64(7))); len(got) != 0 {
			t.Errorf("%q is taken as the caption line of 1: %v", text, got)
		}
	}
}
