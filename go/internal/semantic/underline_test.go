package semantic

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// Text-mode \underline exports as underlined text: the Markdown keeps the page's CSS (as small caps
// does), the Typst wraps the run in #underline[]. A plain run says nothing of an underline.
func TestUnderlinedTextKeepsItsDecoration(t *testing.T) {
	roman := rec.Map{"series": "m", "shape": "n", "size_sp": int64(655360)}
	size := points(int64(655360))
	bold := rec.Map{"series": "bx", "shape": "n", "size_sp": int64(655360)}
	cases := []struct {
		span          rec.Map
		markdown, typ string
	}{
		{rec.Map{"type": "text", "text": " debunked", "style": roman, "decoration": "underline"},
			` <span style="text-decoration:underline">debunked</span>`, `#underline[#text(size: ` + size + `, " debunked")]`},
		{rec.Map{"type": "text", "text": "Total", "style": bold, "decoration": "underline"},
			`<span style="text-decoration:underline">**Total**</span>`, `#strong[#underline[#text(size: ` + size + `, "Total")]]`},
		{rec.Map{"type": "text", "text": "0.57", "decoration": "underline"},
			`<span style="text-decoration:underline">0.57</span>`, `#underline[#text("0.57")]`},
		// soul's \st (decision 134 as extended 2026-11-12): struck-out text.
		{rec.Map{"type": "text", "text": " struck", "style": roman, "decoration": "strike"},
			` <span style="text-decoration:line-through">struck</span>`, `#strike[#text(size: ` + size + `, " struck")]`},
		{rec.Map{"type": "text", "text": "plain", "style": roman}, "plain", `#text(size: ` + size + `, "plain")`},
	}
	md, ty := &markdown{}, &typst{}
	for _, c := range cases {
		if got := md.styledText(c.span); got != c.markdown {
			t.Errorf("markdown %v = %q, want %q", c.span["text"], got, c.markdown)
		}
		if got := ty.styledText(c.span); got != c.typ {
			t.Errorf("typst %v = %q, want %q", c.span["text"], got, c.typ)
		}
	}
}
