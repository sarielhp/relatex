package figscan

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/command"
)

// goldenCases are real Ipe figures of books/0003 (decision 123), chosen to cover multiple views
// (balanced views 1 and 2), groups, marks, arrows, ellipses and arcs, splines, math labels, named
// and numeric colours, opacity, and Ipe XML 7.0.5, 7.0.10, 7.2.6 and 7.2.18.
var goldenCases = []struct {
	file string
	page int
}{
	{"balanced", 1}, {"balanced", 2}, {"Minkowski_sum", 1}, {"blocking-source", 1}, {"bridge_basic", 1},
	{"barycentric", 3}, {"after_2", 2}, {"creation_time", 1}, {"colors", 2}, {"exponential", 1}, {"disks", 4},
}

func scanCase(t *testing.T, file string, page int) *Figure {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", file+".pdf"))
	if os.IsNotExist(err) {
		t.Skip("the figure fixtures are not in this tree (they are not published)")
	}
	if err != nil {
		t.Fatal(err)
	}
	figure, err := Scan(data, page, nil)
	if err != nil {
		t.Fatalf("%s page %d: %v", file, page, err)
	}
	return figure
}

// TestGolden compares the text and JSON forms with the checked outputs (checked by eye against
// renders of the pages). FIGSCAN_UPDATE=1 rewrites them.
func TestGolden(t *testing.T) {
	for _, c := range goldenCases {
		figure := scanCase(t, c.file, c.page)
		if figure.Source != SourceIpe {
			t.Fatalf("%s: source %q", c.file, figure.Source)
		}
		base := filepath.Join("testdata", "golden", fmt.Sprintf("%s-%d", c.file, c.page))
		compareGolden(t, base+".txt", []byte(figure.Text()))
		compareGolden(t, base+".json", figure.JSON())
	}
}

func compareGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if os.Getenv("FIGSCAN_UPDATE") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from its golden output:\n%s", path, got)
	}
}

func TestDeterministic(t *testing.T) {
	for _, c := range goldenCases {
		first, second := scanCase(t, c.file, c.page), scanCase(t, c.file, c.page)
		if first.Text() != second.Text() || !bytes.Equal(first.JSON(), second.JSON()) {
			t.Errorf("%s page %d: two scans differ", c.file, c.page)
		}
	}
}

// TestBalancedViews checks the view mapping on the five-view file: view 1 shows no gamma lines,
// view 2 shows the 8 red points the vision models miscounted.
func TestBalancedViews(t *testing.T) {
	one, two := scanCase(t, "balanced", 1), scanCase(t, "balanced", 2)
	if one.View != "Ipe page 1, view 1 of 5" || two.View != "Ipe page 1, view 2 of 5" {
		t.Fatalf("views %q, %q", one.View, two.View)
	}
	marks := 0
	for _, n := range flatten(two.Nodes) {
		if n.Kind == KindMark {
			marks++
		}
		marks += n.Count
	}
	if marks != 8 {
		t.Errorf("view 2 marks = %d, want 8", marks)
	}
	if strings.Contains(one.Text(), "stroke blue") {
		t.Errorf("view 1 shows the gamma layer's blue lines:\n%s", one.Text())
	}
	if _, err := Scan(mustRead(t, "balanced"), 6, nil); err == nil {
		t.Error("page 6 of a five-page file scanned")
	}
}

func mustRead(t *testing.T, file string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", file+".pdf"))
	if os.IsNotExist(err) {
		t.Skip("the figure fixtures are not in this tree (they are not published)")
	}
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestNonIpe(t *testing.T) {
	if command.Resolve("mutool") == "" {
		t.Skip("mutool is not installed")
	}
	pdf := []byte("%PDF-1.4\n1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n" +
		"2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 50] >> endobj\n" +
		"3 0 obj << /Type /Page /Parent 2 0 R >> endobj\ntrailer << /Root 1 0 R >>\n%%EOF\n")
	figure, err := Scan(pdf, 1, nil)
	if err != nil || figure.Source != SourceTrace || figure.Extent != [2]float64{100, 50} {
		t.Fatalf("got %v, %v", figure, err)
	}
	if !strings.Contains(figure.Text(), "source trace") || !strings.Contains(string(figure.JSON()), `"nodes": []`) {
		t.Errorf("text %q json %s", figure.Text(), figure.JSON())
	}
	file := parsePDF(pdf)
	if boxes := file.pageBoxes(); len(boxes) != 1 || boxes[0] != (Box{X1: 100, Y1: 50}) {
		t.Errorf("inherited MediaBox: %v", boxes)
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestMatrixComposition(t *testing.T) {
	translate := matrix{1, 0, 0, 1, 10, 0}
	scale := matrix{2, 0, 0, 2, 0, 0}
	// Translate first, then scale: (1, 1) -> (11, 1) -> (22, 2).
	if p := translate.then(scale).apply(1, 1); p != (Point{22, 2}) {
		t.Errorf("translate then scale: %v", p)
	}
	if p := scale.then(translate).apply(1, 1); p != (Point{12, 2}) {
		t.Errorf("scale then translate: %v", p)
	}
	rotate := matrix{0, 1, -1, 0, 0, 0}
	m := rotate.then(translate).then(scale)
	back := m.then(m.inverse()).apply(3, 4)
	if !near(back[0], 3) || !near(back[1], 4) {
		t.Errorf("inverse: %v", back)
	}
	major, minor, angle := matrix{0, 3, -1, 0, 0, 0}.axes()
	if !near(major, 3) || !near(minor, 1) || !near(angle, 90) {
		t.Errorf("axes: %v %v %v", major, minor, angle)
	}
	if parseMatrix("1 2 3") != identity {
		t.Error("a malformed matrix is not the identity")
	}
}

func TestParsePath(t *testing.T) {
	cases := []struct {
		text  string
		want  []string
		check func(t *testing.T, parts []Part)
	}{
		{"0 0 m 10 0 l 10 10 l h", []string{ShapePolygon}, nil},
		{"0 0 m 10 0 l 10 10 l", []string{ShapePolyline}, nil},
		{"0 0 m 10 0 l h 20 20 m 30 30 l", []string{ShapePolygon, ShapePolyline}, nil},
		{"5 0 0 5 50 50 e", []string{ShapeEllipse}, func(t *testing.T, parts []Part) {
			if *parts[0].Center != (Point{50, 50}) || *parts[0].Radii != [2]float64{5, 5} {
				t.Errorf("circle %v %v", *parts[0].Center, *parts[0].Radii)
			}
		}},
		{"10 0 m 10 0 0 10 0 0 0 10 a", []string{ShapeArc}, func(t *testing.T, parts []Part) {
			// Counterclockwise from (10, 0) to (0, 10): through 45 degrees.
			through := *parts[0].Through
			if !near(through[0], 7.1) || !near(through[1], 7.1) {
				t.Errorf("arc through %v", through)
			}
		}},
		{"0 0 m 10 10 20 0 30 10 s", []string{ShapeSpline}, func(t *testing.T, parts []Part) {
			if len(parts[0].Points) != 4 || parts[0].Closed {
				t.Errorf("spline %+v", parts[0])
			}
		}},
		{"0 0 10 10 20 0 u", []string{ShapeSpline}, func(t *testing.T, parts []Part) {
			if len(parts[0].Points) != 3 || !parts[0].Closed {
				t.Errorf("closed spline %+v", parts[0])
			}
		}},
		{"0 0 m 10 0 l 10 0 0 10 0 0 0 10 a h", []string{ShapeCurve}, nil},
		{"0 0 m 1 1 2 2 3 3 c", []string{ShapeSpline}, nil},
	}
	for _, c := range cases {
		parts := parsePath(c.text, identity)
		shapes := make([]string, len(parts))
		for i, p := range parts {
			shapes[i] = p.Shape
		}
		if strings.Join(shapes, " ") != strings.Join(c.want, " ") {
			t.Errorf("%q: shapes %v, want %v", c.text, shapes, c.want)
			continue
		}
		if c.check != nil {
			c.check(t, parts)
		}
	}
}

const ipeTwoPages = `<?xml version="1.0"?>
<ipe version="70218" creator="test">
<ipestyle name="s"><color name="mine" value="0 0.5 0"/><pen name="fat" value="1.2"/></ipestyle>
<page><layer name="alpha"/><layer name="beta"/>
<view layers="alpha"/><view layers="alpha beta"/>
<path stroke="mine" pen="fat">0 0 m 100 0 l</path>
<path layer="beta" stroke="black">0 10 m 100 10 l</path>
<path stroke="black">0 20 m 100 20 l</path>
<group layer="alpha" matrix="1 0 0 1 10 0"><use name="mark/disk(sx)" pos="0 50" size="normal" stroke="black"/></group>
</page>
<page><text pos="50 50" type="label" style="math">x_1</text></page>
</ipe>`

func TestViewsAndLayers(t *testing.T) {
	box := Box{X1: 100, Y1: 100}
	cases := []struct {
		page   int
		view   string
		shapes int
	}{{1, "Ipe page 1, view 1 of 2", 3}, {2, "Ipe page 1, view 2 of 2", 5}, {3, "Ipe page 2", 0}}
	for _, c := range cases {
		figure, err := readIpe([]byte(ipeTwoPages), c.page, 0, box)
		if err != nil {
			t.Fatal(err)
		}
		finish(figure)
		if figure.View != c.view || figure.Total != c.shapes {
			t.Errorf("page %d: view %q shapes %d, want %q %d\n%s", c.page, figure.View, figure.Total, c.view, c.shapes, figure.Text())
		}
	}
	figure, _ := readIpe([]byte(ipeTwoPages), 1, 0, box)
	finish(figure)
	text := figure.Text()
	for _, want := range []string{"stroke mine pen 1.2", "disk mark at (10, 50)"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	last, _ := readIpe([]byte(ipeTwoPages), 3, 0, box)
	finish(last)
	if len(last.Labels) != 1 || !strings.Contains(last.Text(), `"$x_1$" at (50, 50)`) {
		t.Errorf("label page:\n%s", last.Text())
	}
	if _, err := readIpe([]byte(ipeTwoPages), 4, 0, box); err == nil {
		t.Error("page 4 of three views read")
	}
}

// TestSizeRule checks that a crowded figure lists at most budget shapes and counts the rest.
func TestSizeRule(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<ipe version="70010"><page>`)
	for i := range 60 {
		fmt.Fprintf(&b, `<path stroke="black">0 %d m %d %d l</path>`, i, 10+i, i)
	}
	b.WriteString(`<text pos="0 0">a</text></page></ipe>`)
	figure, err := readIpe([]byte(b.String()), 1, 0, Box{X1: 100, Y1: 100})
	if err != nil {
		t.Fatal(err)
	}
	finish(figure)
	if figure.Total != 60 || figure.Shown != budget || figure.Cut["paths"] != 20 || len(figure.Labels) != 1 {
		t.Errorf("total %d shown %d cut %v labels %d", figure.Total, figure.Shown, figure.Cut, len(figure.Labels))
	}
	// The longest lines stay; the label's nearest primitive (the shortest line at y 0) also gets a bonus.
	if !strings.Contains(figure.Text(), "segment (0, 59)-(69, 59)") || !strings.Contains(figure.Text(), "plus 20 more not listed (20 paths)") {
		t.Errorf("size rule:\n%s", figure.Text())
	}
}

func TestCollapseAndRepeats(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<ipe version="70010"><page>`)
	for i := range 5 {
		fmt.Fprintf(&b, `<use name="mark/fdisk(sfx)" pos="%d 10" size="normal" stroke="black" fill="red"/>`, 10*i)
	}
	b.WriteString(`<use name="mark/fdisk(sfx)" pos="0 90" size="large" stroke="black" fill="red"/>`)
	b.WriteString(`<path stroke="black">0 0 m 50 50 l</path><path stroke="black">0 0 m 50 50 l</path>`)
	b.WriteString(`</page></ipe>`)
	figure, err := readIpe([]byte(b.String()), 1, 0, Box{X1: 100, Y1: 100})
	if err != nil {
		t.Fatal(err)
	}
	finish(figure)
	text := figure.Text()
	for _, want := range []string{"5 alike fdisk marks; stroke black, fill red, normal, box x 0-40 y 10-10",
		"fdisk mark at (0, 90)", "segment (0, 0)-(50, 50); stroke black pen 0.4 (drawn 2 times over each other)"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
}
