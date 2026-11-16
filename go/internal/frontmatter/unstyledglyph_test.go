package frontmatter

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

type stubStyles struct{}

func (stubStyles) Has(int64) bool { return true }

// Runs joins consecutive items of one style, as the textstyle catalog does.
func (stubStyles) Runs(items []any) []rec.Map {
	runs := []rec.Map{}
	for _, value := range items {
		item := rec.AsMap(value)
		text := rec.FetchString(item, "space") + rec.FetchString(item, "text")
		style, _ := item["style"].(rec.Map)
		if len(runs) > 0 && rec.Equal(runs[len(runs)-1]["style"], any(style)) {
			runs[len(runs)-1]["text"] = runs[len(runs)-1]["text"].(string) + text
			continue
		}
		run := rec.Map{"type": "text", "text": text}
		if style != nil {
			run["style"] = style
		}
		runs = append(runs, run)
	}
	return runs
}

// A glyph without a text face (a blackboard R drawn by a math font in a running head) is a run
// of its own without style, never merged into the slanted face around it, and the row validates.
func TestDecorationRunSeparatesAGlyphWithoutATextFace(t *testing.T) {
	slanted := rec.Map{"source": "nfss-font-id", "font_id": int64(106), "font_name": "X-Slanted", "size_sp": int64(786432),
		"encoding": "TU", "family": "X", "series": "m", "shape": "sl", "generic": "serif"}
	observed := []rec.Map{{"text": "S", "style": slanted}, {"text": "O", "style": slanted}, {"text": "ℝ", "math": true}}
	runs := decorationRuns("SOℝ", observed, stubStyles{})
	if len(runs) != 2 || rec.AsMap(runs[0])["text"] != "SO" || rec.AsMap(runs[1])["text"] != "ℝ" {
		t.Fatalf("runs %v", runs)
	}
	if _, styled := rec.AsMap(runs[1])["style"]; styled {
		t.Errorf("the glyph kept a text face: %v", runs[1])
	}
	if math, _ := rec.AsMap(runs[1])["math"].(bool); !math {
		t.Errorf("the glyph run is not marked as math: %v", runs[1])
	}
	validateDecorationRuns(rec.Map{"text": "SOℝ", "runs": runs})
}

// A text glyph that lost its face for another reason (no math marker) is not accepted.
func TestDecorationRunWithoutAFaceOrMathMarkIsRejected(t *testing.T) {
	observed := []rec.Map{{"text": "S"}}
	runs := decorationRuns("S", observed, stubStyles{})
	if message := failure(func() { validateDecorationRuns(rec.Map{"text": "S", "runs": runs}) }); message == "" {
		t.Error("an unstyled non-math run validated")
	}
}
