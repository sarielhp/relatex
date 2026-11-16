package typstsource

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// Tests of the Typst acceptance's number and count gaps (decision 154, item C-TY).

// A book's appendix chapter set as a paragraph ("Appendix A") above an unnumbered title
// leaves its first section, A.1, to start the lettered segment (books/0006, 0007): Typst then
// prints A.1, not 1.1.
func TestAppendixSectionStartsTheLetteredSegment(t *testing.T) {
	printed := func(text string) *string { return &text }
	plan := planHeadings([]headingEntry{
		{level: 1, printed: printed("1")},
		{level: 2, printed: printed("1.1")},
		{level: 1},
		{level: 2, printed: printed("A.1")},
		{level: 2, printed: printed("A.2")},
	})
	appendix := plan[3]
	if !appendix.hasSegment || appendix.numbering != `"A.1"` {
		t.Errorf("A.1 directive = %+v, want the segment's numbering \"A.1\"", appendix)
	}
	if plan[4].numbering != "" || plan[4].update != nil {
		t.Errorf("A.2 directive = %+v, want none", plan[4])
	}
	// REVTeX's subsection prints only its own letter (II, then A): no new segment.
	revtex := planHeadings([]headingEntry{{level: 1, printed: printed("II")}, {level: 2, printed: printed("A")}})
	if revtex[1].numbering != "" {
		t.Errorf("REVTeX subsection directive = %+v, want no numbering", revtex[1])
	}
}

// A footnote is set with the document's first styles, before the equation numbering's #set:
// a numbered display in it carries its own numbering, so it prints and counts where LaTeX
// counted it (papers2/0442 numbered two lower from (47) on).
func TestEquationInAFootnoteCarriesItsNumbering(t *testing.T) {
	e := &exporter{equations: newSeries("(%s)"), labelNumbers: map[string]string{}, placed: map[string]string{},
		labels: map[string]string{}}
	tag := "(1)"
	if got := e.equation("x", &tag, ""); got != "#set math.equation(numbering: \"(1)\")\n$ x $" {
		t.Errorf("body equation = %q", got)
	}
	e.noteDepth = 1
	tag = "(2)"
	if got := e.equation("y", &tag, ""); got != `#math.equation(block: true, numbering: _ => "(2)", $y$)` {
		t.Errorf("footnote equation = %q", got)
	}
}

// A display numbered by a function prints its whole tag in a reference: "(15a)" is the
// reference itself, not "(" @ref ")" (papers2/0245 printed ((15a))).
func TestReferenceToAnOverriddenTagKeepsOneParenthesis(t *testing.T) {
	e := &exporter{equations: newSeries("(%s)"), labelNumbers: map[string]string{}, placed: map[string]string{},
		labels: map[string]string{"eq:a": "eq:a"}, document: rec.Map{"labels": rec.Map{"eq:a": rec.Map{"number": "15a"}}}}
	tag := "(15a)"
	e.equation("x", &tag, "eq:a")
	e.placedBefore = e.placed
	got := e.reference(rec.Map{"type": "reference", "text": "(15a)", "label": "eq:a"})
	if got != refMark("eq:a") {
		t.Errorf("reference = %q, want %q", got, refMark("eq:a"))
	}
}

// A Beamer title's \footnote (slides/0005) is written once, in the title, not again among the
// notes no text calls.
func TestTitleNoteIsWrittenOnce(t *testing.T) {
	note := rec.Map{"id": "n1", "type": "note", "children": []any{"np"}, "number": "1", "callsites": []any{"t"}}
	noteText := rec.Map{"id": "np", "type": "paragraph", "content": []any{text("Based on a book.")}}
	title := rec.Map{"id": "t", "type": "paragraph", "children": []any{}, "content": []any{text("Finance"),
		rec.Map{"type": "note_reference", "target": "n1"}}}
	document := testDocument(nil, []rec.Map{note, noteText}, nil)
	root := rec.FetchMap(rec.FetchMap(document, "nodes"), "root")
	root["children"] = []any{"t", "section"}
	rec.FetchMap(document, "nodes")["t"] = title
	document["frontmatter"] = rec.Map{"title": "Finance"}
	document["note_order"] = []any{"n1"}
	output, _ := renderTest(t, document)
	if count := strings.Count(output, "Based on a book."); count != 1 {
		t.Errorf("the note is written %d times:\n%s", count, output)
	}
}

// A caption whose head is set in bold with no separator ("Fig. 9 Comparisons", papers3/0837)
// is a figure; an algorithm float among the figures names its own supplement.
func TestBareCaptionHeadMakesAFigure(t *testing.T) {
	float := func(id, caption, head, rest string) []rec.Map {
		return []rec.Map{
			{"id": id, "type": "figure_float", "children": []any{caption}},
			{"id": caption, "type": "float_caption", "children": []any{caption + "-text"}},
			{"id": caption + "-text", "type": "paragraph", "content": []any{boldText(head), text(rest)}},
		}
	}
	first, second := float("f1", "c1", "Fig. 1", " A picture."), float("f2", "c2", "Algorithm 1", " Steps.")
	// An empty \caption{} leaves the head alone, in the class's plain font (papers4/1108).
	third := float("f3", "c3", "Fig. 2", "")
	third[2]["content"] = []any{text("Fig. 2")}
	extra := slices.Concat(first[1:], second[1:], third[1:])
	document := testDocument([]rec.Map{first[0], second[0], third[0]}, extra, nil)
	output, _ := renderTest(t, document)
	if !strings.Contains(output, "caption: [],\n  kind: image,") {
		t.Errorf("empty caption:\n%s", output)
	}
	if !strings.Contains(output, "caption: [A picture.],\n  kind: image,") {
		t.Errorf("figure:\n%s", output)
	}
	if !strings.Contains(output, "caption: [Steps.],\n  kind: image, supplement: [Algorithm],") {
		t.Errorf("algorithm:\n%s", output)
	}
	if !strings.Contains(output, "set figure(supplement: [Fig.])") {
		t.Errorf("supplement:\n%s", output)
	}
}

// An uncaptioned float emits an unnumbered #figure so counts see it while numbered figures
// before and after keep LaTeX's numbers (books/0007).
func TestUncaptionedFloatDoesNotShiftNumberedFigures(t *testing.T) {
	captioned := func(id, caption, head string) []rec.Map {
		return []rec.Map{
			{"id": id, "type": "figure_float", "children": []any{id + "-body", caption}},
			{"id": id + "-body", "type": "paragraph", "content": []any{text(id + " body")}},
			{"id": caption, "type": "float_caption", "children": []any{caption + "-text"}},
			{"id": caption + "-text", "type": "paragraph", "content": []any{text(head)}},
		}
	}
	first := captioned("f1", "c1", "Figure 1: First.")
	second := captioned("f2", "c2", "Figure 2: Second.")
	bareFig := []rec.Map{
		{"id": "fb", "type": "figure_float", "children": []any{"fb-body"}},
		{"id": "fb-body", "type": "paragraph", "content": []any{text("Uncaptioned figure")}},
	}
	bareTab := []rec.Map{
		{"id": "tb", "type": "table_float", "children": []any{"tb-body"}},
		{"id": "tb-body", "type": "paragraph", "content": []any{text("Uncaptioned table")}},
	}
	extra := slices.Concat(first[1:], bareFig[1:], bareTab[1:], second[1:])
	document := testDocument([]rec.Map{first[0], bareFig[0], bareTab[0], second[0]}, extra, nil)
	output, _ := renderTest(t, document)
	for _, want := range []string{
		"#figure(\n  [Uncaptioned figure],\n  kind: image, numbering: none,\n)",
		"#figure(\n  [Uncaptioned table],\n  kind: table, numbering: none,\n)",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q in:\n%s", want, output)
		}
	}
	if strings.Contains(output, ".update(") {
		t.Errorf("unexpected counter update after uncaptioned float:\n%s", output)
	}
	typstBin, typstErr := exec.LookPath("typst")
	if typstErr != nil {
		t.Skip("typst not installed")
	}
	workDir := t.TempDir()
	mainPath := filepath.Join(workDir, "main.typ")
	if writeErr := os.WriteFile(mainPath, []byte(output), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	writeSupportTest(t, workDir)
	queried, queryErr := exec.Command(typstBin, "eval", "--ignore-system-fonts", "--format", "json", "--in", mainPath,
		`query(figure).map(f => if f.numbering != none { counter(figure.where(kind: f.kind)).at(f.location()) } else { none })`).CombinedOutput()
	if queryErr != nil {
		t.Fatalf("typst eval failed: %v\n%s\n%s", queryErr, queried, output)
	}
	if got := strings.TrimSpace(string(queried)); got != "[[1],null,null,[2]]" {
		t.Errorf("figure numbers = %s, want [[1],null,null,[2]]", got)
	}
}
