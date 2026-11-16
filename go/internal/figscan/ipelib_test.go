package figscan

import (
	"strings"
	"testing"
)

// Tests of the reader against ipelib's semantics (~/prog/26/ipe/src/ipelib), one per behaviour.

func readPage(t *testing.T, source string, pdfPage, pdfPages int) *Figure {
	t.Helper()
	figure, err := readIpe([]byte(source), pdfPage, pdfPages, Box{X1: 100, Y1: 100})
	if err != nil {
		t.Fatal(err)
	}
	finish(figure)
	return figure
}

func wantText(t *testing.T, figure *Figure, wants ...string) {
	t.Helper()
	text := figure.Text()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
}

func avoidText(t *testing.T, figure *Figure, avoids ...string) {
	t.Helper()
	text := figure.Text()
	for _, avoid := range avoids {
		if strings.Contains(text, avoid) {
			t.Errorf("unexpected %q in\n%s", avoid, text)
		}
	}
}

// PdfWriter::createPageView writes no PDF page for a page with a NOPDF layer.
func TestNoPdfLayerPageSkipped(t *testing.T) {
	source := `<ipe version="70218"><page><layer name="alpha"/><layer name="NOPDF"/>` +
		`<path stroke="black">0 0 m 10 0 l</path></page>` +
		`<page><path stroke="black">0 0 m 90 90 l</path></page></ipe>`
	figure := readPage(t, source, 1, 0)
	if figure.View != "Ipe page 2" {
		t.Errorf("PDF page 1 is %q, want Ipe page 2", figure.View)
	}
	if _, err := readIpe([]byte(source), 2, 0, Box{X1: 100, Y1: 100}); err == nil {
		t.Error("a second PDF page was read")
	}
}

const ipeMarked = `<ipe version="70218">
<page marked="no"><path stroke="black">0 0 m 10 0 l</path></page>
<page><layer name="a"/><layer name="b"/>
<view layers="a"/><view layers="a b" marked="yes"/><view layers="b"/>
<path stroke="black">0 0 m 50 0 l</path><path layer="b" stroke="red">0 0 m 0 50 l</path></page>
<page><view layers="alpha"/><view layers="alpha"/><path stroke="black">0 0 m 99 99 l</path></page>
</ipe>`

// ipetoipe -markedview (SaveFlag::MarkedView): unmarked pages left out, a page's marked views
// only, its last view when none is marked. Recognized when the PDF's page count says so.
func TestMarkedViewExport(t *testing.T) {
	cases := []struct {
		page, pages int
		view        string
	}{
		{1, 0, "Ipe page 1"}, {2, 0, "Ipe page 2, view 1 of 3"}, {6, 6, "Ipe page 3, view 2 of 2"},
		{1, 2, "Ipe page 2, view 2 of 3"}, {2, 2, "Ipe page 3, view 2 of 2"},
	}
	for _, c := range cases {
		if figure := readPage(t, ipeMarked, c.page, c.pages); figure.View != c.view {
			t.Errorf("PDF page %d of %d is %q, want %q", c.page, c.pages, figure.View, c.view)
		}
	}
	wantText(t, readPage(t, ipeMarked, 1, 2), "stroke red")
}

// A view names the layers it shows, in the page's order; a page without layers shows alpha
// (decision 133: the describer tells views apart by them).
func TestViewLayers(t *testing.T) {
	cases := []struct {
		page   int
		layers string
	}{{2, "a"}, {3, "a b"}, {4, "b"}, {5, "alpha"}, {1, "alpha"}}
	for _, c := range cases {
		if got := strings.Join(readPage(t, ipeMarked, c.page, 0).Layers, " "); got != c.layers {
			t.Errorf("PDF page %d shows layers %q, want %q", c.page, got, c.layers)
		}
	}
}

// ImlParser::parseObject switches the current layer only to a layer the page has.
func TestUnknownLayerKeepsCurrent(t *testing.T) {
	source := `<ipe version="70218"><page><layer name="alpha"/><layer name="beta"/><view layers="alpha"/>` +
		`<path stroke="black">0 0 m 10 0 l</path><path layer="gamma" stroke="red">0 0 m 0 10 l</path></page></ipe>`
	figure := readPage(t, source, 1, 0)
	wantText(t, figure, "stroke red")
}

// A view's <map kind="color"> renames colours (AttributeMap::map).
func TestViewColorMap(t *testing.T) {
	source := `<ipe version="70218"><page><view layers="alpha"><map kind="color" from="red" to="blue"/></view>` +
		`<path stroke="red">0 0 m 10 0 l</path><text pos="5 5" stroke="red">x</text></page></ipe>`
	figure := readPage(t, source, 1, 0)
	wantText(t, figure, "stroke blue", `label "x" (label, blue)`)
	avoidText(t, figure, "red")
}

// Reference::draw draws nothing for a symbol the stylesheets do not define.
func TestUndefinedSymbolNotDrawn(t *testing.T) {
	source := `<ipe version="70218"><ipestyle name="basic"><symbol name="mark/disk(sx)" transformations="translations">` +
		`<path fill="sym-stroke">0.6 0 0 0.6 0 0 e</path></symbol></ipestyle><page>` +
		`<use name="mark/disk(sx)" pos="10 10" size="normal" stroke="black"/>` +
		`<use name="mark/missing(sx)" pos="90 90" size="normal" stroke="black"/></page></ipe>`
	figure := readPage(t, source, 1, 0)
	wantText(t, figure, "disk mark at (10, 10)")
	avoidText(t, figure, "missing")
}

// Painter::untransform: translations keep only the matrix's translation, rigid its rotation too.
func TestPathTransformations(t *testing.T) {
	source := `<ipe version="70218"><page>` +
		`<path stroke="black" matrix="2 0 0 2 10 10" transformations="translations">0 0 m 10 0 l</path>` +
		`<path stroke="red" matrix="0 2 -2 0 50 50" transformations="rigid">0 0 m 10 0 l</path>` +
		`<path stroke="blue" matrix="2 0 0 2 0 0">0 0 m 10 0 l</path></page></ipe>`
	figure := readPage(t, source, 1, 0)
	wantText(t, figure, "segment (10, 10)-(20, 10)", "segment (50, 50)-(50, 60)", "segment (0, 0)-(20, 0)")
}

// Group::draw clips its children to the clip attribute's path.
func TestGroupClip(t *testing.T) {
	source := `<ipe version="70218"><page><group clip="0 0 m 50 0 l 50 50 l 0 50 l h" matrix="1 0 0 1 10 10">` +
		`<path stroke="black">0 0 m 90 0 l</path><path stroke="black">0 5 m 90 5 l</path></group></page></ipe>`
	figure := readPage(t, source, 1, 0)
	wantText(t, figure, "clipped to")
	if len(figure.Nodes) != 1 || len(figure.Nodes[0].Clip) != 1 || figure.Nodes[0].Clip[0].Points[0] != (Point{10, 10}) {
		t.Errorf("clip not read in its group's frame:\n%s", figure.Text())
	}
}

// Path(XmlAttributes): neither stroke nor fill is stroked black; stroke-opacity defaults to opacity.
func TestPathDefaults(t *testing.T) {
	source := `<ipe version="70218"><page><path>0 0 m 10 0 l</path>` +
		`<path stroke="black" opacity="50%">0 10 m 10 10 l</path>` +
		`<path stroke="black" opacity="50%" stroke-opacity="opaque">0 20 m 10 20 l</path>` +
		`<path stroke="black" pen="heavier">0 30 m 10 30 l</path></page></ipe>`
	figure, err := readIpe([]byte(source), 1, 0, Box{X1: 100, Y1: 100})
	if err != nil {
		t.Fatal(err)
	}
	nodes := figure.Nodes
	if nodes[0].Stroke != "black" || nodes[0].Pen != 0.4 {
		t.Errorf("unstyled path: stroke %q pen %v", nodes[0].Stroke, nodes[0].Pen)
	}
	if nodes[1].contrast >= nodes[2].contrast {
		t.Errorf("stroke opacity: %v (opacity 50%%) vs %v (stroke opaque)", nodes[1].contrast, nodes[2].contrast)
	}
	if nodes[3].Pen != 0.8 {
		t.Errorf("basic pen heavier: %v", nodes[3].Pen)
	}
}

// Path::makeArrowData: arrows only on one open subpath; arrow/mid-* shapes sit halfway along.
func TestArrows(t *testing.T) {
	source := `<ipe version="70218"><page>` +
		`<path stroke="black" arrow="normal/normal">0 0 m 10 0 l 10 10 l h</path>` +
		`<path stroke="black" arrow="normal/normal">0 20 m 10 20 l 0 30 m 10 30 l</path>` +
		`<path stroke="black" arrow="mid-normal/normal">0 40 m 50 40 l</path>` +
		`<path stroke="black" arrow="normal/normal" rarrow="normal/normal">0 60 m 50 60 l</path></page></ipe>`
	figure, err := readIpe([]byte(source), 1, 0, Box{X1: 100, Y1: 100})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, n := range figure.Nodes {
		got = append(got, n.Arrows)
	}
	if strings.Join(got, "|") != "||at middle|both ends" {
		t.Errorf("arrows %q", got)
	}
}

// Text(XmlAttributes): a width without a type attribute makes a minipage.
func TestMinipageByWidth(t *testing.T) {
	source := `<ipe version="70005"><page><text pos="10 90" width="80">Some words</text></page></ipe>`
	wantText(t, readPage(t, source, 1, 0), "(minipage)")
}

// Shape::load: "u" starts a subpath of its own; "*" in an L splits spiro knots from Bézier points.
func TestPathSyntaxSubpaths(t *testing.T) {
	parts := parsePath("0 0 m 10 0 l 20 20 30 20 30 30 u", identity)
	if len(parts) != 2 || parts[0].Shape != ShapePolyline || parts[1].Shape != ShapeSpline || !parts[1].Closed {
		t.Errorf("u merged into the subpath before it: %+v", parts)
	}
	parts = parsePath("0 0 m 10 10 20 0 * 1 1 2 2 3 3 4 4 5 5 6 6 L", identity)
	if len(parts) != 1 || len(parts[0].Points) != 3 {
		t.Errorf("precomputed spiro: %+v", parts)
	}
}
