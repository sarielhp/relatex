package htmlkit

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestEscapeReplacesTheFiveHTMLCharacters(t *testing.T) {
	if got := Escape(`<a href="x">'&'</a>`); got != "&lt;a href=&quot;x&quot;&gt;&#39;&amp;&#39;&lt;/a&gt;" {
		t.Fatalf("Escape = %s", got)
	}
}

func TestRuleCSSStandsTheBoxOnItsDepth(t *testing.T) {
	span := rec.Map{"width_sp": int64(65_536), "height_sp": int64(26_214), "depth_sp": int64(0)}
	want := "display:inline-block;width:1.328352px;height:0.531333px;vertical-align:0.000000px;background:currentColor"
	if got := RuleCSS(span); got != want {
		t.Fatalf("RuleCSS = %s", got)
	}
	span["margin_left_sp"], span["margin_right_sp"], span["color"] = int64(0), 1.5, "#ff0000"
	want = "display:inline-block;width:1.328352px;height:0.531333px;vertical-align:0.000000px;background:#ff0000" +
		";margin:0 0.000030px 0 0.000000px"
	if got := RuleCSS(span); got != want {
		t.Fatalf("RuleCSS with margins = %s", got)
	}
}

func TestSourceFontsCSSLinksEachFamilyOnce(t *testing.T) {
	file := rec.Map{"name": "a.otf", "format": "opentype"}
	document := rec.Map{"fonts": rec.Map{"items": []any{
		rec.Map{"css_family": "A", "file": file},
		rec.Map{"css_family": "A", "file": file},
		rec.Map{"css_family": "B", "file": nil},
	}}}
	want := `@font-face{font-family:"A";src:url("fonts/a.otf") format("opentype");font-display:swap;}`
	if got := SourceFontsCSS(document); got != want {
		t.Fatalf("SourceFontsCSS = %s", got)
	}
	if got := SourceFontsCSS(rec.Map{}); got != "" {
		t.Fatalf("SourceFontsCSS without fonts = %q", got)
	}
}
