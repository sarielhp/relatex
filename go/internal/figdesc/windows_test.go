package figdesc

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// citationAsset is one PNG asset the figures of these tests share.
func citationAsset() (string, rec.Map) {
	original := mediamodel.Encoded([]byte("png bytes"), "image/png")
	sha := original["sha256"].(string)
	return sha, rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": sha, "representations": rec.Map{"original": original}}
}

// citationDocument has one captioned figure cited in four blocks: the second reference opens
// its paragraph and a long formula follows it, and the fourth comes after a quiet block.
func citationDocument() rec.Map {
	sha, asset := citationAsset()
	text := func(value string) rec.Map { return rec.Map{"type": "text", "text": value} }
	reference := rec.Map{"type": "reference", "text": " Figure 1", "target": "cap"}
	paragraph := func(id string, content ...any) rec.Map {
		return rec.Map{"id": id, "type": "paragraph", "children": []any{}, "content": content}
	}
	formula := strings.Repeat("a", 600)
	return rec.Map{
		"assets":      rec.Map{"items": rec.Map{sha: asset}},
		"block_order": []any{"p1", "ifig", "cap", "c1", "c2", "c3", "quiet", "c4"},
		"nodes": rec.Map{
			"p1": paragraph("p1", text("Earlier context sentence.")),
			"f1": rec.Map{"id": "f1", "type": "figure_float", "children": []any{"ifig", "cap"}},
			"ifig": paragraph("ifig", rec.Map{"type": "image", "id": "i1", "asset": sha, "source": "figures/disk.png",
				"width_sp": int64(10), "height_sp": int64(10)}),
			"cap": paragraph("cap", text("Figure 1: A disk.")),
			"c1":  paragraph("c1", text("Earlier "+strings.Repeat("filler ", 60)), reference, text(". It is flat.")),
			"c2": paragraph("c2", reference, text(" shows the boundary. The radius is "),
				rec.Map{"type": "math", "node": "m1"}, text(" and it grows.")),
			"m1":    rec.Map{"id": "m1", "type": "math", "tex": rec.Map{"status": "verified", "text": formula}},
			"c3":    paragraph("c3", text("Third, "), reference, text(" again; the colour is blue.")),
			"quiet": paragraph("quiet", text(strings.Repeat("Nothing cites the figure here. ", 40))),
			"c4":    paragraph("c4", text("Fourth, "), reference, text(" and this one must not appear.")),
		},
	}
}

// TestCitationWindowsAreCentredOnEachReference: the first three citing blocks give one window
// each, crossing paragraph boundaries, with the reference marked, the formula whole and the
// fourth reference beyond the limit (decision 123).
func TestCitationWindowsAreCentredOnEachReference(t *testing.T) {
	figure := Figures(citationDocument())[0]
	if len(figure.Citations) != citingLimit {
		t.Fatalf("%d windows, want %d: %q", len(figure.Citations), citingLimit, figure.Citations)
	}
	formula := "$" + strings.Repeat("a", 600) + "$"
	for i, window := range figure.Citations {
		if !strings.Contains(window, "→Figure 1←") {
			t.Errorf("window %d does not mark its reference: %q", i, window)
		}
		if strings.Count(window, "$")%2 != 0 {
			t.Errorf("window %d cuts inside a formula: %q", i, window)
		}
		if strings.Contains(window, "Fourth") || strings.Contains(window, "must not appear") {
			t.Errorf("window %d reaches the fourth reference: %q", i, window)
		}
		if len([]rune(window)) > windowLimit {
			t.Errorf("window %d has %d runes: %q", i, len([]rune(window)), window)
		}
	}
	if !strings.Contains(figure.Citations[1], "Earlier") {
		t.Errorf("the window at a paragraph start does not cross the paragraph boundary: %q", figure.Citations[1])
	}
	if !strings.Contains(figure.Citations[1], formula) {
		t.Errorf("the window cuts the formula: %q", figure.Citations[1])
	}
	if !strings.Contains(figure.Context, figure.Citations[0]) || !strings.Contains(figure.Context, figure.Citations[2]) {
		t.Errorf("context is not the joined windows: %q", figure.Context)
	}
}

// TestCitationWindowWithoutAReferenceIsTheTextBefore: a figure nothing cites keeps the paragraph
// before it as its one window.
func TestCitationWindowWithoutAReferenceIsTheTextBefore(t *testing.T) {
	sha, asset := citationAsset()
	text := func(value string) rec.Map { return rec.Map{"type": "text", "text": value} }
	document := rec.Map{
		"assets":      rec.Map{"items": rec.Map{sha: asset}},
		"block_order": []any{"p1", "ifig", "cap"},
		"nodes": rec.Map{
			"p1":   rec.Map{"id": "p1", "type": "paragraph", "children": []any{}, "content": []any{text("Before the figure.")}},
			"f1":   rec.Map{"id": "f1", "type": "figure_float", "children": []any{"ifig", "cap"}},
			"ifig": rec.Map{"id": "ifig", "type": "paragraph", "children": []any{}, "content": []any{rec.Map{"type": "image", "id": "i1", "asset": sha, "source": "figures/disk.png", "width_sp": int64(10), "height_sp": int64(10)}}},
			"cap":  rec.Map{"id": "cap", "type": "paragraph", "children": []any{}, "content": []any{text("Figure 1: A disk.")}},
		},
	}
	figure := Figures(document)[0]
	if len(figure.Citations) != 1 || figure.Citations[0] != "Before the figure." || figure.Context != "Before the figure." {
		t.Errorf("fallback citations %q, context %q", figure.Citations, figure.Context)
	}
}
