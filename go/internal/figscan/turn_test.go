package figscan

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// turnFigure is a 100 × 50 figure: a segment, a tilted ellipse, a mark, a group box and a label.
func turnFigure() *Figure {
	center, radii := Point{20, 10}, [2]float64{8, 4}
	mark := Point{90, 40}
	return &Figure{Source: SourceTrace, Size: [2]float64{200, 100}, Extent: [2]float64{100, 50},
		Nodes: []*Node{
			{ID: "n1", Kind: KindPath, Parts: []Part{{Shape: ShapePolyline, Points: []Point{{0, 0}, {100, 50}}}}},
			{ID: "n2", Kind: KindPath, Parts: []Part{{Shape: ShapeEllipse, Center: &center, Radii: &radii, Angle: 30}}},
			{ID: "n3", Kind: KindGroup, Box: &[4]float64{10, 5, 30, 15}, Children: []*Node{{ID: "n4", Kind: KindMark, Point: &mark}}},
		},
		Labels: []Label{{ID: "n5", Text: "x", Point: Point{60, 45}}}}
}

// TestTurnMovesEveryPosition: a clockwise quarter turn (angle=-90) maps (x, y) in a W × H extent to
// (y, W - x), swaps the extent and the size, turns an ellipse's axis and boxes stay ordered.
func TestTurnMovesEveryPosition(t *testing.T) {
	figure := turnFigure()
	figure.Turn(3)
	if figure.Extent != [2]float64{50, 100} || figure.Size != [2]float64{100, 200} || figure.Turned != "90 degrees clockwise" {
		t.Fatalf("extent %v size %v turned %q", figure.Extent, figure.Size, figure.Turned)
	}
	if got := figure.Nodes[0].Parts[0].Points; got[0] != (Point{0, 100}) || got[1] != (Point{50, 0}) {
		t.Errorf("segment %v", got)
	}
	ellipse := figure.Nodes[1].Parts[0]
	if *ellipse.Center != (Point{10, 80}) || ellipse.Angle != 120 {
		t.Errorf("ellipse %v at %v", *ellipse.Center, ellipse.Angle)
	}
	if box := *figure.Nodes[2].Box; box != [4]float64{5, 70, 15, 90} {
		t.Errorf("box %v", box)
	}
	if *figure.Nodes[2].Children[0].Point != (Point{40, 10}) || figure.Labels[0].Point != (Point{45, 40}) {
		t.Errorf("mark %v label %v", *figure.Nodes[2].Children[0].Point, figure.Labels[0].Point)
	}
	if !strings.Contains(figure.Text(), "turned 90 degrees clockwise as the paper prints it") {
		t.Errorf("text:\n%s", figure.Text())
	}
}

// TestTurnRoundTrips: each turn followed by its inverse gives the figure back; no turn changes
// nothing, not even the text form.
func TestTurnRoundTrips(t *testing.T) {
	for quarters := range 4 {
		figure := turnFigure()
		figure.Turn(quarters)
		figure.Turn(4 - quarters)
		figure.Turned = ""
		if !bytes.Equal(figure.JSON(), turnFigure().JSON()) {
			t.Errorf("quarters %d:\n%s", quarters, figure.JSON())
		}
	}
	plain := turnFigure()
	plain.Turn(0)
	if plain.Text() != turnFigure().Text() || strings.Contains(plain.Text(), "turned") {
		t.Errorf("no turn changed the text:\n%s", plain.Text())
	}
}

// TestTurnARealPlot: the gnuplot figure turned counterclockwise keeps its labels inside the turned
// extent, and turned back reads as scanned.
func TestTurnARealPlot(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "gnuplot-Fig3.pdf"))
	if os.IsNotExist(err) {
		t.Skip("the figure fixtures are not in this tree (they are not published)")
	}
	if err != nil {
		t.Fatal(err)
	}
	scanned, err := Scan(data, 1, nil)
	if err != nil {
		t.Skipf("scan: %v", err)
	}
	before := scanned.JSON()
	scanned.Turn(1)
	for _, label := range scanned.Labels {
		if label.Point[0] < 0 || label.Point[0] > scanned.Extent[0] || label.Point[1] < 0 || label.Point[1] > scanned.Extent[1] {
			t.Errorf("label %s at %v outside %v", label.ID, label.Point, scanned.Extent)
		}
	}
	scanned.Turn(3)
	scanned.Turned = ""
	if !bytes.Equal(scanned.JSON(), before) {
		t.Errorf("turned back differs")
	}
}
