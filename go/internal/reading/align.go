package reading

import (
	"math"
	"slices"
)

// Paragraph alignment (decision 116): read from the typeset lines' extents on the PDF page against the
// text column the paragraph sits in. Only centred and right are recorded; flush-left and justified
// paragraphs are the default.

const (
	// Centred is the recorded alignment of a centred paragraph.
	Centred = "center"
	// Right is the recorded alignment of a flush-right paragraph.
	Right = "right"
	// alignTolerance is how far (bp) a line's centre or right edge may be from its column's.
	alignTolerance = 1.5
	// ParagraphRoom is the least space (bp) a line must leave at the column's left edge: a paragraph
	// indent is not an alignment.
	ParagraphRoom = 24.0
	// TableRoom is the least for a table's box, which has no indent to mistake.
	TableRoom = 6.0
	// columnLines is how many identical full lines make a text column.
	columnLines = 5
)

// Extent is where one typeset line starts and ends on its page, in bp.
type Extent struct{ Left, Right float64 }

// TextColumns are the text columns of a document: the extents shared by at least columnLines lines
// that are not the last line of their paragraph (a justified line fills its column).
func TextColumns(paragraphs [][]Extent) []Extent {
	counts := map[Extent]int{}
	for _, lines := range paragraphs {
		for _, line := range lines[:max(len(lines)-1, 0)] {
			counts[Extent{math.Round(line.Left), math.Round(line.Right)}]++
		}
	}
	columns := []Extent{}
	for column, count := range counts {
		if count >= columnLines {
			columns = append(columns, column)
		}
	}
	slices.SortFunc(columns, func(a, b Extent) int {
		if a.Left != b.Left {
			return int(a.Left - b.Left)
		}
		return int(a.Right - b.Right)
	})
	return columns
}

// Alignment is Centred when every line of the paragraph is centred in one text column and some line
// leaves more than room (bp) on its side, Right when every line ends at one column's right edge, leaves room and the left edges are ragged (a justified entry, hanging indent or not, is not right-aligned), else "".
func Alignment(lines, columns []Extent, room float64) string {
	if len(lines) == 0 || justified(lines) {
		return ""
	}
	narrowest := tightest(lines, columns)
	for _, column := range columns {
		if !within(lines, column) {
			continue
		}
		spare := slices.ContainsFunc(lines, func(line Extent) bool { return line.Left-column.Left > room })
		if !spare {
			continue
		}
		if all(lines, func(line Extent) bool {
			return math.Abs(float64((line.Left+line.Right)/2)-float64((column.Left+column.Right)/2)) <= alignTolerance
		}) {
			return Centred
		}
		// A right margin is the right edge of the tightest column holding the lines: a wider extent shared by
		// centred lines (one straddling the gutter) is no margin (ason V-C: a short bullet read as right-aligned).
		if column == narrowest && all(lines, func(line Extent) bool {
			return math.Abs(line.Right-column.Right) <= alignTolerance && line.Left-column.Left > room
		}) && ragged(lines) {
			return Right
		}
	}
	return ""
}

// tightest is the narrowest of the columns holding all the lines (the zero Extent when none does).
func tightest(lines, columns []Extent) Extent {
	best := Extent{}
	for _, column := range columns {
		if within(lines, column) && (best == (Extent{}) || column.Right-column.Left < best.Right-best.Left) {
			best = column
		}
	}
	return best
}

func within(lines []Extent, column Extent) bool {
	return all(lines, func(line Extent) bool {
		return line.Left >= column.Left-alignTolerance && line.Right <= column.Right+alignTolerance
	})
}

func all(lines []Extent, test func(Extent) bool) bool {
	return !slices.ContainsFunc(lines, func(line Extent) bool { return !test(line) })
}

// Hull is the extent covering all the lines.
func Hull(lines []Extent) Extent {
	hull := lines[0]
	for _, line := range lines[1:] {
		hull = Extent{min(hull.Left, line.Left), max(hull.Right, line.Right)}
	}
	return hull
}

// justified: a paragraph of several lines whose lines but the last (all, of two) share one extent is set flush in
// its own measure (a quote, an abstract with \\leftskip and \\rightskip): centred lines vary in width.
func justified(lines []Extent) bool {
	if len(lines) < 2 {
		return false
	}
	first, body := lines[0], lines[:len(lines)-1]
	if len(lines) == 2 {
		body = lines
	}
	return all(body, func(line Extent) bool {
		return math.Abs(line.Left-first.Left) <= 1 && math.Abs(line.Right-first.Right) <= 1
	})
}

// ragged: the left edges of a flush-right block differ line by line. One line has none to compare; a
// justified entry, hanging indent or not, has its first line apart and the others at one edge, so the
// lines after the first must show two different left edges.
func ragged(lines []Extent) bool {
	if len(lines) == 1 {
		return true
	}
	rest := lines[1:]
	return !all(rest, func(line Extent) bool { return math.Abs(line.Left-rest[0].Left) <= 1 })
}
