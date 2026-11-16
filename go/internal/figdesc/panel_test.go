package figdesc

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// panelDocument has a figure with four pictures: one with its subcaption paragraph under it, two
// side by side whose shared subcaption has a mark for each, one without a subcaption; a figure
// with one picture; and a picture outside any figure.
func panelDocument() rec.Map {
	text := func(value string) rec.Map { return rec.Map{"type": "text", "text": value} }
	image := func(id string) rec.Map { return rec.Map{"type": "image", "id": id, "source": id + ".pdf"} }
	paragraph := func(id string, content ...any) rec.Map {
		return rec.Map{"id": id, "type": "paragraph", "children": []any{}, "content": content}
	}
	return rec.Map{
		"block_order": []any{"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8", "p9"},
		"nodes": rec.Map{
			"f1": rec.Map{"id": "f1", "type": "figure_float", "children": []any{"p1", "p2", "p3", "p4", "p5", "c1"}},
			"p1": paragraph("p1", image("i1")),
			"p2": paragraph("p2", text("(a) Scenario 1.")),
			"p3": paragraph("p3", image("i2"), text(" "), image("i3")),
			"p4": paragraph("p4", text("(b) Left plot. (c) Right plot.")),
			"p5": paragraph("p5", image("i4")),
			"c1": rec.Map{"id": "c1", "type": "float_caption", "children": []any{"p6"}},
			"p6": paragraph("p6", text("Figure 1: (a) Scenario 1. (b), (c) Plots. (d) A map.")),
			"f2": rec.Map{"id": "f2", "type": "figure_float", "children": []any{"p7"}},
			"p7": paragraph("p7", image("i5")),
			"p8": paragraph("p8", text("Body text.")),
			"p9": paragraph("p9", image("i6")),
		},
	}
}

func TestPanelPlacesEachPictureOfItsFigure(t *testing.T) {
	reader := newContextReader(panelDocument())
	panels := map[string]Panel{}
	for _, found := range reader.occurrences() {
		panels[rec.ToS(found.span["id"])] = reader.panel(found.block, found.span)
	}
	want := map[string]Panel{
		"i1": {InFigure: true, Index: 1, Count: 4, Column: 1, RowSize: 1, Subcaption: "(a) Scenario 1."},
		"i2": {InFigure: true, Index: 2, Count: 4, Column: 1, RowSize: 2, Subcaption: "(b) Left plot."},
		"i3": {InFigure: true, Index: 3, Count: 4, Column: 2, RowSize: 2, Subcaption: "(c) Right plot."},
		"i4": {InFigure: true, Index: 4, Count: 4, Column: 1, RowSize: 1},
		"i5": {InFigure: true, Index: 1, Count: 1, Column: 1, RowSize: 1},
		"i6": {},
	}
	for id, panel := range want {
		if panels[id] != panel {
			t.Errorf("%s: %+v, want %+v", id, panels[id], panel)
		}
	}
}

func TestPanelSaysWhatIsKnownAndWhatIsNot(t *testing.T) {
	cases := []struct {
		panel Panel
		wants []string
	}{
		{Panel{}, []string{"not in a figure environment; nothing is known about panels."}},
		{Panel{InFigure: true, Index: 1, Count: 1, Column: 1, RowSize: 1}, []string{"the only picture of its figure"}},
		{Panel{InFigure: true, Index: 3, Count: 4, Column: 2, RowSize: 2, Subcaption: "(c) Right plot."},
			[]string{"picture 3 of 4 of its figure", "picture 2 of 2 set in one paragraph", `printed with it in the figure: "(c) Right plot."`}},
		{Panel{InFigure: true, Index: 4, Count: 4, Column: 1, RowSize: 1},
			[]string{"picture 4 of 4", "prints no subcaption for it, so which of the caption's panels it is, is not known."}},
		{Panel{InFigure: true, Index: 1, Count: 3, Column: 1, RowSize: 3, RowText: "(a) and (b)"},
			[]string{`printed with its group of pictures: "(a) and (b)"`}},
	}
	for _, c := range cases {
		described := c.panel.Describe()
		for _, want := range c.wants {
			if !strings.Contains(described, want) {
				t.Errorf("%+v: %q lacks %q", c.panel, described, want)
			}
		}
	}
}

// A group's text splits between its pictures only with one mark per picture.
func TestGroupPartNeedsOneMarkPerPicture(t *testing.T) {
	if got := groupPart("(a) One (b) Two", 1, 2); got != "(b) Two" {
		t.Errorf("second part %q", got)
	}
	if got := groupPart("Both plots", 0, 2); got != "" {
		t.Errorf("unsplittable text gave %q", got)
	}
	if got := groupPart("i) x ii) y iii) z", 2, 3); got != "iii) z" {
		t.Errorf("roman marks %q", got)
	}
	if got := groupPart("Alone.", 0, 1); got != "Alone." {
		t.Errorf("single picture %q", got)
	}
}
