package pdftag

import (
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// A page with /Rotate is drawn by the content stream unrotated and laid out as displayed
// (papers4 0919, papers3 0829, papers5 1187: landscape tables), so a position moves between the two.
func TestPageFrameRotation(t *testing.T) {
	box := [4]float64{0, 0, 612, 792}
	cases := []struct {
		rotate int
		x, y   float64 // user space point
		lx, ly float64 // its layout position
		ox, oy float64 // its position in the glyph owners' frame (unrotated, y down)
	}{
		{0, 100, 700, 100, 92, 100, 92},
		{90, 100, 700, 700, 100, 100, 92},
		{180, 100, 700, 512, 700, 100, 92},
		{270, 100, 700, 92, 512, 100, 92},
	}
	for _, c := range cases {
		frame := pageFrame{box: box, rotate: c.rotate}
		if x, y := frame.layout(c.x, c.y); x != c.lx || y != c.ly {
			t.Errorf("rotate %d: layout (%v, %v), want (%v, %v)", c.rotate, x, y, c.lx, c.ly)
		}
		if x, y := frame.user(c.lx, c.ly); x != c.x || y != c.y {
			t.Errorf("rotate %d: user (%v, %v), want (%v, %v)", c.rotate, x, y, c.x, c.y)
		}
		if x, y := frame.tex(c.lx, c.ly); x != c.ox || y != c.oy {
			t.Errorf("rotate %d: owners (%v, %v), want (%v, %v)", c.rotate, x, y, c.ox, c.oy)
		}
	}
}

// A rotated page's first glyph is where the layout has it once the content's position is turned.
func TestMapPaintsRotatedPage(t *testing.T) {
	frame := pageFrame{box: [4]float64{0, 0, 612, 792}, rotate: 90}
	items := []item{{kind: kindShow, first: 0, count: 1, placed: true, origin: [2]float64{90.93, 79.2}}}
	painted := []paintedGlyph{{text: "T", baseline: [2]float64{79.2, 90.93}}}
	m, err := mapPaints(items, painted, frame, identityPaints(1))
	if err != nil || m.mapping[0] != 0 {
		t.Fatalf("mapping %v: %v", m, err)
	}
	if _, err := mapPaints(items, painted, pageFrame{box: frame.box}, identityPaints(1)); err == nil {
		t.Fatal("an unrotated frame found the glyph where the layout has it")
	}
}

// A table set sideways (a landscape page's table, papers4 0919): the owners' glyphs are the
// fragment's turned by a quarter turn, and the fragment still finds its cells.
func TestTableCellsOfASidewaysTable(t *testing.T) {
	for quarters := 1; quarters <= 3; quarters++ {
		grid := cellGrid()
		grid.NativeFragments = []nativeFragment{fragmentAt(1, [2]string{"1", "2"})}
		page := ownersPage([][]string{{"a", "b", "c", "1", "2"}}, [][2]float64{{0, 0}}, 1)
		for index := range page.X {
			point := turned([2]float64{page.X[index], page.Y[index]}, quarters)
			page.X[index], page.Y[index] = round2(point[0]+300), round2(point[1]+400)
		}
		report := TableCells{}
		cells := taggerWith(grid).tableCells(&glyphowner.Owners{Leaves: []string{"p"}, Pages: []glyphowner.Page{page}}, &report)
		if report.Fragments != 1 || cells[1][0] != "h1" || cells[1][2] != "h2" || cells[1][4] != "b2" {
			t.Errorf("%d quarter turns: report %+v, cells %v", quarters, report, cells[1])
		}
	}
}
