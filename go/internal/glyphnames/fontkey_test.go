package glyphnames

import "testing"

// TestFontScopedNamesIgnoreCase checks that a reviewed name of one font is found whatever the case
// of the font: the AMS Type 1 fonts name lasy10 and lasy6 "LASY10", "LASY6" (decisions 104/105,
// papers2 0267, 0469, papers4 0864).
func TestFontScopedNamesIgnoreCase(t *testing.T) {
	table := Load("../../../tex/relatex-symbol-glyphs.txt")
	cases := []struct{ font, name, want string }{
		{"LASY10", "a50", "\u25A1"},
		{"LASY10", "a51", "\u25C7"},
		{"LASY6", "a51", "\u25C7"},
		{"LASY10", "a2", "\u22B4"},
		{"lasy10", "a50", "\u25A1"},
		// scoping is kept: one font's name never spells another font's glyph.
		{"CMR10", "a50", ""},
		{"STMARY10", "ssearrow", "\u2199"},
		{"MSAM10", "axisshort", ""},
	}
	for _, c := range cases {
		if got := table.SpellIn(c.font, c.name); got != c.want {
			t.Errorf("SpellIn(%s, %s) = %q, want %q", c.font, c.name, got, c.want)
		}
	}
}
