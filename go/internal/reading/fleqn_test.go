package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func mathBlock(id, kind string, entity any, formula int64, occurrence string) rec.Map {
	return rec.Map{"id": id, "type": kind, "entity": entity, "pages": []any{int64(1)},
		"content": []any{rec.Map{"type": "text", "text": " "}, rec.Map{"type": "math", "formula": formula, "occurrence": occurrence}}}
}

// equationThree: entity 3 is a display the source sets.
var equationThree = equationContext(3, "align")

func tagRecords(formula int64, mathml string) []rec.Map {
	return []rec.Map{{"kind": "math", "formula": formula, "mathml": mathml}, equationThree}
}

// A number too wide to sit beside its flush-left formula is a text row of the equation's alignment,
// below the formula (or above, tags left): the formula becomes the display block, the number its span.
func TestFlushLeftNumberOnItsOwnRow(t *testing.T) {
	cases := map[string]struct {
		blocks []rec.Map
		want   []string
		first  bool
	}{
		"number below": {[]rec.Map{mathBlock("f", "paragraph", int64(3), 1, "o1"), textBlock("n", "paragraph", int64(3), "(18)")},
			[]string{"f"}, false},
		"number above": {[]rec.Map{textBlock("n", "paragraph", int64(3), "(18)"), mathBlock("f", "paragraph", int64(3), 1, "o1")},
			[]string{"f"}, true},
		"another entity": {[]rec.Map{mathBlock("f", "paragraph", int64(3), 1, "o1"), textBlock("n", "paragraph", int64(4), "(18)")},
			[]string{"f", "n"}, false},
		"prose, no number": {[]rec.Map{mathBlock("f", "paragraph", int64(3), 1, "o1"), textBlock("n", "paragraph", int64(3), "as before")},
			[]string{"f", "n"}, false},
	}
	for name, test := range cases {
		e := &Evidence{records: []rec.Map{equationThree}, blocks: test.blocks}
		e.attachFlushLeftNumbers()
		ids := blockIDs(e.blocks)
		if len(ids) != len(test.want) || ids[0] != test.want[0] {
			t.Fatalf("%s: blocks %v, want %v", name, ids, test.want)
		}
		formula := e.blocks[0]
		if len(test.want) > 1 {
			if formula["type"] != "paragraph" {
				t.Errorf("%s: formula promoted: %v", name, formula)
			}
			continue
		}
		content := rec.FetchList(formula, "content")
		number := rec.AsMap(content[len(content)-1])
		if test.first {
			number = rec.AsMap(content[0])
		}
		if formula["type"] != "display_math" || rec.FetchString(number, "text") != "(18)" {
			t.Errorf("%s: block %v", name, formula)
		}
	}
}

func TestFlushLeftNumberJoinsItsFormula(t *testing.T) {
	const tag = `<math display="block" xmlns="http://www.w3.org/1998/Math/MathML"><mtext> (4)</mtext></math>`
	cases := map[string]struct {
		blocks  []rec.Map
		records []rec.Map
		want    []string
	}{
		"flush-left equation": {[]rec.Map{mathBlock("f", "paragraph", int64(3), 1, "o1"), mathBlock("n", "display_math", int64(3), 2, "o2")},
			tagRecords(2, tag), []string{"f"}},
		"another entity": {[]rec.Map{mathBlock("f", "paragraph", int64(4), 1, "o1"), mathBlock("n", "display_math", int64(3), 2, "o2")},
			tagRecords(2, tag), []string{"f", "n"}},
		"a display that is not a tag": {[]rec.Map{mathBlock("f", "paragraph", int64(3), 1, "o1"), mathBlock("n", "display_math", int64(3), 2, "o2")},
			tagRecords(2, `<math><mtext>prose (4)</mtext></math>`), []string{"f", "n"}},
		"a formula with text beside it": {[]rec.Map{textBlock("f", "paragraph", int64(3), "where x"), mathBlock("n", "display_math", int64(3), 2, "o2")},
			tagRecords(2, tag), []string{"f", "n"}},
	}
	for name, test := range cases {
		e := &Evidence{records: test.records, blocks: test.blocks}
		e.attachFlushLeftNumbers()
		if ids := blockIDs(e.blocks); len(ids) != len(test.want) || ids[0] != test.want[0] {
			t.Errorf("%s: blocks %v, want %v", name, ids, test.want)
		}
		if len(test.want) == 1 {
			formula := e.blocks[0]
			content := rec.FetchList(formula, "content")
			if formula["type"] != "display_math" || len(content) != 2 || rec.FetchString(rec.AsMap(content[1]), "text") != " (4)" {
				t.Errorf("%s: block %v", name, formula)
			}
			if len(e.nestedMath) != 1 || e.nestedMath[0] != "o2" {
				t.Errorf("%s: the number's formula was not consumed: %v", name, e.nestedMath)
			}
		}
	}
}

func equationContext(entity int64, environment string) rec.Map {
	return rec.Map{"kind": "diagnostic", "code": "equation_context", "source": "latex-equation-hooks", "entity": entity, "environment": environment}
}

// A flush-left equation* too wide for its line, given ntheorem's empty end-mark tag, is a row of
// a display alignment with no number row: its lone formula in the equation's entity is the display.
func TestUnnumberedFlushLeftDisplayIsADisplay(t *testing.T) {
	cases := map[string]struct {
		block   rec.Map
		records []rec.Map
		display bool
	}{
		"equation*":      {mathBlock("f", "paragraph", int64(3), 1, "o1"), []rec.Map{equationContext(3, "equation*")}, true},
		"another entity": {mathBlock("f", "paragraph", int64(4), 1, "o1"), []rec.Map{equationContext(3, "equation*")}, false},
		"an alignment":   {mathBlock("f", "paragraph", int64(3), 1, "o1"), []rec.Map{equationContext(3, "align*")}, false},
		"no equation":    {mathBlock("f", "paragraph", int64(3), 1, "o1"), nil, false},
		"text beside it": {textBlock("f", "paragraph", int64(3), "where x"), []rec.Map{equationContext(3, "equation*")}, false},
	}
	for name, test := range cases {
		e := &Evidence{records: test.records, blocks: []rec.Map{test.block}}
		e.attachFlushLeftNumbers()
		if got := e.blocks[0]["type"] == "display_math"; got != test.display {
			t.Errorf("%s: display %v, want %v", name, got, test.display)
		}
	}
}

// A lone formula in a table cell next to a cell that prints "(11,55)" is no flush-left equation:
// the two are different cells (papers2/1182). In one cell they still join.
func TestFlushLeftNumberStaysInItsCell(t *testing.T) {
	for name, cells := range map[string][2]int64{"neighbouring cells": {1, 2}, "one cell": {3, 3}} {
		number := textBlock("n", "paragraph", int64(3), "(11,55)")
		formula := mathBlock("f", "paragraph", int64(3), 1, "o1")
		e := &Evidence{records: []rec.Map{equationThree}, blocks: []rec.Map{number, formula}, origins: map[uintptr]rec.Map{}}
		for index, block := range []rec.Map{number, formula} {
			for _, value := range rec.FetchList(block, "content") {
				e.origins[rec.Identity(rec.AsMap(value))] = rec.Map{"table": int64(1), "cell": cells[index]}
			}
		}
		e.attachFlushLeftNumbers()
		joined := len(e.blocks) == 1
		if joined != (cells[0] == cells[1]) {
			t.Errorf("%s: blocks %v", name, blockIDs(e.blocks))
		}
	}
}

// A formula outside any display the source sets (a table head: $T_{\rm rot}$(X) over (K), papers5/1210)
// takes no neighbouring parenthesized text as its number.
func TestFlushLeftNumberNeedsAnEquation(t *testing.T) {
	e := &Evidence{blocks: []rec.Map{mathBlock("f", "paragraph", int64(3), 1, "o1"), textBlock("n", "paragraph", int64(3), "(K)")}}
	e.attachFlushLeftNumbers()
	if ids := blockIDs(e.blocks); len(ids) != 2 || e.blocks[0]["type"] != "paragraph" {
		t.Errorf("blocks %v", e.blocks)
	}
}
