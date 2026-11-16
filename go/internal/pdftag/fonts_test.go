package pdftag

import (
	"bytes"
	"testing"
)

// A show operator whose glyph's width grew is written as one TJ with the kern that undoes the
// change after that glyph, the existing kerns kept: every glyph stays where it was.
func TestCompensatedShowOperators(t *testing.T) {
	entry := item{op: operation{operator: "TJ"}, kind: kindShow, font: "1 0 R", elements: []element{
		{codes: [][]byte{{0x41}, {0x42}}}, {raw: []byte("-120")}, {codes: [][]byte{{0x41}}}}}
	moved := map[int]float64{0x41: 1.4000000001}
	if !movesGlyphs(entry, moved) || movesGlyphs(entry, map[int]float64{0x43: 2}) {
		t.Fatal("movesGlyphs")
	}
	var out bytes.Buffer
	writeCompensated(&out, entry, moved, nil)
	if got := out.String(); got != "[<41> 1.4 <42> -120 <41> 1.4 ] TJ\n" {
		t.Fatalf("got %q", got)
	}
}
