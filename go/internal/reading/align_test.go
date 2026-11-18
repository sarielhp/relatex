package reading

import "testing"

// papers5/1443: front-matter lines centred on the page's text column, the abstract filling it.
func TestAlignmentReadsCentreAndRightFromLineExtents(t *testing.T) {
	full := []Extent{{72, 540}, {72, 540}, {72, 540}, {72, 540}, {72, 400}}
	columns := TextColumns([][]Extent{full, full})
	if len(columns) != 1 || columns[0] != (Extent{72, 540}) {
		t.Fatalf("columns %v", columns)
	}
	cases := []struct {
		name  string
		lines []Extent
		want  string
	}{
		{"centred title", []Extent{{102, 510.5}, {180.6, 431.9}}, Centred},
		{"centred within a point of the middle", []Extent{{153.7, 458.4}}, Centred},
		{"right-aligned date", []Extent{{450.3, 539.2}}, Right},
		{"flush-right block", []Extent{{400, 540}, {430, 539.2}, {470, 540}}, Right},
		{"justified paragraph", []Extent{{72, 540}, {72, 540}, {72, 300}}, ""},
		{"flush-left single line", []Extent{{72, 250}}, ""},
		{"indented last line of a justified paragraph", []Extent{{90, 540}}, ""},
		{"uncentred line", []Extent{{120, 400}}, ""},
		{"one centred line among flush ones", []Extent{{102, 510}, {72, 300}}, ""},
		{"no lines", nil, ""},
	}
	for _, c := range cases {
		if got := Alignment(c.lines, columns, ParagraphRoom); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	if got := Alignment([]Extent{{102, 510}}, nil, ParagraphRoom); got != "" {
		t.Errorf("without columns: %q", got)
	}
}

// A two-column page has two columns; a block is centred in the column it sits in.
func TestAlignmentUsesTheColumnOfTheBlock(t *testing.T) {
	left, right := []Extent{{54, 300}}, []Extent{{312, 558}}
	var blocks [][]Extent
	for range 6 {
		blocks = append(blocks, []Extent{left[0], left[0], right[0], right[0]})
	}
	columns := TextColumns(blocks)
	if len(columns) != 2 {
		t.Fatalf("columns %v", columns)
	}
	if got := Alignment([]Extent{{400, 470}}, columns, ParagraphRoom); got != Centred {
		t.Errorf("centred in the right column: %q", got)
	}
	if got := Alignment([]Extent{{142, 212}}, columns, ParagraphRoom); got != Centred {
		t.Errorf("centred in the left column: %q", got)
	}
}

// Authors set in a \tabular (papers2/0512): the cells align inside the table, whose box is centred.
func TestAlignmentOfATableBlockUsesItsHull(t *testing.T) {
	columns := []Extent{{72.3, 542}, {99.6, 514.7}}
	lines := []Extent{{131, 448.6}, {154.5, 465.7}, {108.4, 505.9}, {123.6, 481.6}}
	if got := Alignment(lines, columns, ParagraphRoom); got != "" {
		t.Errorf("lines alone: %q", got)
	}
	if got := Alignment([]Extent{Hull(lines)}, columns, TableRoom); got != Centred {
		t.Errorf("hull %v: %q", Hull(lines), got)
	}
}

// papers5/1403: the author boxes hull is 20 bp inside the column on each side: room enough for a table, not for a paragraph.
func TestTableRoomIsSmallerThanParagraphRoom(t *testing.T) {
	columns, hull := []Extent{{72.3, 542}}, []Extent{{92.5, 521.8}}
	if Alignment(hull, columns, ParagraphRoom) != "" || Alignment(hull, columns, TableRoom) != Centred {
		t.Error("room")
	}
}

// A quote is justified in a measure narrower than the column and symmetric about its centre: not centred.
func TestIndentedJustifiedBlockIsNotCentred(t *testing.T) {
	columns := []Extent{{72, 540}}
	quote := []Extent{{108, 504}, {108, 504}, {108, 504}, {108, 360}}
	full := []Extent{{108, 504}, {108, 504}, {108, 504}}
	if got := Alignment(quote, columns, ParagraphRoom); got != "" {
		t.Errorf("quote %q", got)
	}
	if got := Alignment(full, columns, ParagraphRoom); got != "" {
		t.Errorf("quote with a full last line %q", got)
	}
	title := []Extent{{102, 510}, {180.6, 431.4}}
	if got := Alignment(title, columns, ParagraphRoom); got != Centred {
		t.Errorf("title %q", got)
	}
	if got := Alignment([]Extent{{150, 462}}, columns, ParagraphRoom); got != Centred {
		t.Errorf("one centred line %q", got)
	}
}

// papers/0040: a bibliography entry with a hanging indent whose last line fills the measure is justified.
func TestFullJustifiedEntryIsNotRight(t *testing.T) {
	columns := []Extent{{72, 540}}
	if got := Alignment([]Extent{{72, 540}, {96, 540}}, columns, ParagraphRoom); got != "" {
		t.Errorf("entry %q", got)
	}
}

// papers4/952, papers4/1089: justified bibliography entries in the right column of a two-column page end
// at the full-width column's right edge; they are not flush right.
func TestJustifiedEntriesAreNotRight(t *testing.T) {
	columns := []Extent{{54, 562.1}}
	entries := map[string][]Extent{
		"hanging indent": {{317, 562.1}, {335.3, 562.1}, {335.3, 562.1}},
		"two full lines": {{317, 562.1}, {335.3, 562.1}},
	}
	for name, lines := range entries {
		if got := Alignment(lines, columns, ParagraphRoom); got != "" {
			t.Errorf("%s: %q", name, got)
		}
	}
}

// ason (V-C): five centred lines straddling the gutter share an extent that is no text column; a short
// list item in the right column that happens to end at its right edge is not right-aligned.
func TestAlignmentIgnoresAWiderExtentOfCentredLines(t *testing.T) {
	var blocks [][]Extent
	for range 6 {
		blocks = append(blocks, []Extent{{47, 300}, {47, 300}, {312, 563}, {312, 563}, {151, 470}, {151, 470}})
	}
	columns := TextColumns(blocks)
	if got := Alignment([]Extent{{332.8, 471}}, columns, ParagraphRoom); got != "" {
		t.Errorf("short list item: %q, want flush left", got)
	}
	if got := Alignment([]Extent{{450, 562.5}}, columns, ParagraphRoom); got != Right {
		t.Errorf("flush-right date in the right column: %q", got)
	}
}
