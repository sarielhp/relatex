package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/textstyle"
)

// A flush-left equation set in a smaller size (papers2/0254, fleqn equations at 9pt in a 10pt
// paper): its number is text in the style its glyphs are set in (the collector's tag_style), not the
// body's most common style. Without that evidence the body style stays the fallback.
func TestFlushLeftNumberKeepsItsGlyphStyle(t *testing.T) {
	const tag = `<math display="block" xmlns="http://www.w3.org/1998/Math/MathML"><mtext> (4)</mtext></math>`
	body := rec.Map{"font_id": int64(1), "family": "ptm", "series": "m", "shape": "n", "size_sp": int64(655360)}
	small := rec.Map{"font_id": int64(2), "family": "ptm", "series": "m", "shape": "n", "size_sp": int64(589824)}
	for name, item := range map[string]rec.Map{
		"glyph style": {"tag": true, "formula": int64(2), "tag_style": int64(2)},
		"no evidence": {"tag": true, "formula": int64(2)},
	} {
		prose := rec.Map{"id": "p", "type": "paragraph", "entity": int64(1), "pages": []any{int64(1)}, "content": []any{rec.Map{"type": "text", "text": "Body", "style": body}}}
		e := &Evidence{records: tagRecords(2, tag), textStyles: textstyle.Catalog{1: body, 2: small},
			blocks: []rec.Map{prose, mathBlock("f", "paragraph", int64(3), 1, "o1"), mathBlock("n", "display_math", int64(3), 2, "o2")}}
		e.markTagFormula(item)
		e.attachFlushLeftNumbers()
		content := rec.FetchList(e.blocks[1], "content")
		number := rec.AsMap(content[len(content)-1])
		want := body
		if item["tag_style"] != nil {
			want = small
		}
		if number["text"] != " (4)" || !rec.Equal(number["style"], want) {
			t.Errorf("%s: number %v, want style %v", name, number, want)
		}
	}
}

// An equation whose formula sets its own number (amsmath's split inside equation, papers2/0441
// "(1.2)" at 10.95pt in an 11pt paper whose abstract is \small): the number is read from the
// formula's MathML when the formula opens, before its tag glyphs arrive. When the formula closes,
// the number takes the style its glyphs named, not the most common style read so far.
func TestDisplayTagTakesItsGlyphStyleWhenTheFormulaCloses(t *testing.T) {
	small := rec.Map{"font_id": int64(1), "family": "lmr", "series": "m", "shape": "n", "size_sp": int64(655360)}
	body := rec.Map{"font_id": int64(2), "family": "lmr", "series": "m", "shape": "n", "size_sp": int64(717619)}
	e := &Evidence{
		textStyles: textstyle.Catalog{1: small, 2: body},
		retention:  rec.Map{"occurrences": []any{rec.Map{"id": "m-6", "formula": int64(6), "fragments": []any{"f6"}}}},
		fragments:  omap.New[any, rec.Map](),
		mathNodes:  []nodeKey{{int64(6), "f6", int64(1)}, {int64(6), "f6", int64(2)}},
	}
	e.fragments.Set("f6", rec.Map{"nodes": int64(2)})
	tag := rec.Map{"type": "text", "text": " (1.2)", "style": small}
	e.pending = &pendingMath{formula: int64(6), remaining: 2, nested: map[any]int64{}, block: rec.Map{"pages": []any{}}, tag: tag}
	e.observeMath(rec.Map{"formula": int64(6), "fragment": "f6", "position": int64(1)}, rec.Map{}, int64(1))
	e.observeMath(rec.Map{"formula": int64(6), "fragment": "f6", "position": int64(2), "tag": true, "tag_style": int64(2)},
		rec.Map{}, int64(1))
	if e.pending != nil || !rec.Equal(tag["style"], body) {
		t.Errorf("number style %v, want %v (pending %v)", tag["style"], body, e.pending)
	}
}
