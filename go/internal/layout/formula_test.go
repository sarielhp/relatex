package layout

import (
	"testing"

	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/rec"
)

// A formula's ActualText over three glyphs (its TeX source, papers6/0571): MuPDF's structured text puts
// one character at each glyph and the rest after the last one. The glyphs keep their own Unicode, the
// leftover characters leave the records, and the first glyph carries the whole text, so the last glyph
// binds no long string (the bundle bounds a glyph's components).
func TestWholeFormulaActualTextIsOneSpansText(t *testing.T) {
	source := `x^2 + \frac {1}{y} = \sum _{m=1}^{M} z_m \mathrm {mean}\big (\mathrm {conf}_D^m\big ), \qquad r^P = \frac {1}{M}`
	trace := nativetrace.Parse([]byte(`<document><page>` +
		`<metatext type="actualtext" txt="` + source + `"><fill_text transform="1 0 0 1 0 0">` +
		`<span font="F" wmode="0" trm="10 0 0 10"><g unicode="x" glyph="1" x="40" y="30" adv=".5"/>` +
		`<g unicode="2" glyph="2" x="45" y="30" adv=".5"/><g unicode="y" glyph="3" x="50" y="30" adv=".5"/></span>` +
		`</fill_text></metatext></page></document>`)).Descendants("page")[0]
	paint := newUnicodeGlyphs(trace)
	records := []rec.Map{}
	for index, character := range []rune(source) {
		x := 40.0 + 5*float64(min(index, 2))
		records = append(records, rec.Map{"id": "g", "text": string(character), "font": "F", "line": int64(1), "size": 10.0,
			"baseline": []any{x, 30.0}, "metric_quad": []any{x, 22.0, x, 22.0, x, 31.0, x, 31.0}})
	}
	records, flags := paint.respell(records, make([]int64, len(records)))
	if len(records) != 3 || len(flags) != 3 {
		t.Fatalf("%d records kept, want the 3 glyphs", len(records))
	}
	for index, want := range []string{"x", "2", "y"} {
		if records[index]["text"] != want {
			t.Errorf("glyph %d reads %q, want %q", index, records[index]["text"], want)
		}
	}
	if records[0]["actual_text"] != source || records[1]["actual_text"] != nil {
		t.Errorf("the whole text belongs to the first glyph alone: %v", records[0]["actual_text"])
	}
	for _, bound := range paint.bind(records) {
		if bound["unicode_components"] != nil {
			t.Errorf("glyph %v binds components", bound["text"])
		}
	}
}

// Structured text that keeps the glyphs' own characters leaves the formula alone.
func TestWholeFormulaWithoutASpreadIsLeftAlone(t *testing.T) {
	trace := nativetrace.Parse([]byte(`<document><page>` +
		`<metatext type="actualtext" txt="x2y"><fill_text transform="1 0 0 1 0 0">` +
		`<span font="F" wmode="0" trm="10 0 0 10"><g unicode="a" glyph="1" x="40" y="30" adv=".5"/>` +
		`<g unicode="b" glyph="2" x="45" y="30" adv=".5"/></span></fill_text></metatext></page></document>`)).Descendants("page")[0]
	paint := newUnicodeGlyphs(trace)
	records := []rec.Map{
		{"text": "a", "font": "F", "baseline": []any{40.0, 30.0}},
		{"text": "b", "font": "F", "baseline": []any{45.0, 30.0}},
	}
	kept, _ := paint.respell(records, []int64{0, 0})
	if len(kept) != 2 || kept[0]["text"] != "a" || kept[0]["actual_text"] != nil {
		t.Fatalf("records changed: %v", kept)
	}
}
