package browser

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestValidateFloatsExpectsInlineCaptionsAsDivisionsWithoutNamingTheFigure(t *testing.T) {
	figure := rec.Map{"id": "f", "type": "figure_float", "children": []any{"p", "c", "q"}}
	caption := rec.Map{"id": "c", "type": "float_caption", "children": []any{}}
	nodes := rec.Map{"f": figure, "c": caption, "p": rec.Map{"type": "paragraph"}, "q": rec.Map{"type": "paragraph"}}
	structures := []rec.Map{figure, caption}
	inventory := func(tag string) rec.Map {
		return rec.Map{"floats": []any{
			rec.Map{"id": "f", "type": "figure_float", "tag": "figure", "caption": nil, "role": nil, "float_parent": nil},
			rec.Map{"id": "c", "type": "float_caption", "tag": tag, "caption": nil, "role": nil, "float_parent": "f"}}}
	}
	validateFloats(inventory("div"), structures, nodes)
	defer func() {
		if recover() == nil {
			t.Error("a figcaption between the figure's blocks was accepted")
		}
	}()
	validateFloats(inventory("figcaption"), structures, nodes)
}

// TestValidateFloatsLetsAFigcaptionNameItsFigure: a figcaption names its figure natively
// (decision 183), so the figure carries no aria-labelledby.
func TestValidateFloatsLetsAFigcaptionNameItsFigure(t *testing.T) {
	figure := rec.Map{"id": "f", "type": "figure_float", "children": []any{"c", "p"}}
	caption := rec.Map{"id": "c", "type": "float_caption", "children": []any{}}
	nodes := rec.Map{"f": figure, "c": caption, "p": rec.Map{"type": "paragraph"}}
	structures := []rec.Map{figure, caption}
	inventory := func(label any) rec.Map {
		return rec.Map{"floats": []any{
			rec.Map{"id": "f", "type": "figure_float", "tag": "figure", "caption": label, "role": nil, "float_parent": nil},
			rec.Map{"id": "c", "type": "float_caption", "tag": "figcaption", "caption": nil, "role": nil, "float_parent": "f"}}}
	}
	validateFloats(inventory(nil), structures, nodes)
	defer func() {
		if recover() == nil {
			t.Error("a figure labelled by its figcaption was accepted")
		}
	}()
	validateFloats(inventory("c"), structures, nodes)
}
