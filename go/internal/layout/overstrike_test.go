package layout

import (
	"testing"

	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/rec"
)

// "Rafii" set with an "fi" ligature (papers4/1123): MuPDF drops the second "i" as an overstrike of
// the ligature's continuation and leaves a synthetic space in its advance box.
func TestOverstruckGlyphIsRecovered(t *testing.T) {
	char := func(c string, x0, x1 float64, flags string) string {
		quad := rec.ToS(x0) + ` 0 ` + rec.ToS(x1) + ` 0 ` + rec.ToS(x0) + ` 10 ` + rec.ToS(x1) + ` 10`
		return `<char c="` + c + `" quad="` + quad + `" x="` + rec.ToS(x0) + `" y="8" flags="` + flags + `"/>`
	}
	structured := func(spaceFlags string) *nativetrace.Node {
		return nativetrace.Parse([]byte(`<document><page><block><line><font name="Times" size="8">` +
			char("f", 0, 4, "16") + char("i", 4, 4, "24") + char(" ", 4, 6, spaceFlags) +
			`</font></line></block></page></document>`)).Descendants("page")[0]
	}
	painted := func(text string, x, advance float64) rec.Map {
		return rec.Map{"text": text, "font": "ABCDEF+Times", "baseline": []any{x, 8.0}, "advance": []any{advance, 0.0}}
	}
	paints := []rec.Map{painted("fi", 0, 4), painted("i", 4, 2)}
	page := structured("20")
	glyphs := glyphsFor(page, 1)
	recoverOverstruck(glyphs, charFlags(page), paints)
	if glyphs[2]["text"] != "i" || glyphs[1]["text"] != "i" || glyphs[0]["text"] != "f" {
		t.Fatalf("recovered %v %v %v", glyphs[0]["text"], glyphs[1]["text"], glyphs[2]["text"])
	}
	// A space MuPDF did not synthesize, or one the paint does not fill exactly, stays a space.
	for name, check := range map[string]func() []rec.Map{
		"real space": func() []rec.Map {
			spaced := structured("16")
			list := glyphsFor(spaced, 1)
			recoverOverstruck(list, charFlags(spaced), paints)
			return list
		},
		"short paint": func() []rec.Map {
			list := glyphsFor(page, 1)
			recoverOverstruck(list, charFlags(page), []rec.Map{painted("fi", 0, 4), painted("i", 4, 1.5)})
			return list
		},
	} {
		if text := check()[2]["text"]; text != " " {
			t.Errorf("%s: text %q", name, text)
		}
	}
}
