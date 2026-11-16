package glyphnames

import "testing"

func TestSpellCodesRejectsPrivateUseAndControls(t *testing.T) {
	cases := map[string]string{"2032": "\u2032", "F8F1": "", "0007": "", "1F6D1": "\U0001F6D1", "zz": ""}
	for code, want := range cases {
		if got := SpellCodes([]string{code}); got != want {
			t.Errorf("SpellCodes(%s) = %q, want %q", code, got, want)
		}
	}
}

func TestGlyphTableReadsAndSpells(t *testing.T) {
	table := Table{Adobe: map[string]string{"a": "a", "bracelefttp": "", "minus": "\u2212"},
		TeX: map[string]string{"prime": "\u2032", "bracelefttp": "\u23A7"}}
	if !table.Readable("a") || table.Readable("prime") || table.Readable("bracelefttp") || table.Readable("uni2032") {
		t.Fatalf("readable: a is, prime (not in the list), bracelefttp (private use) and uni2032 are not")
	}
	if table.Spell("bracelefttp") != "\u23A7" || table.Spell("minus") != "\u2212" || table.Spell("uni00E9") != "é" ||
		table.Spell("unknown") != "" {
		t.Fatalf("spell: TeX's names first, then the Adobe list, then uniXXXX")
	}
}

func TestCodeNamesSpellCodePoints(t *testing.T) {
	cases := map[string]string{"uni2032": "′", "uni00660069": "fi", "u1D45D": "\U0001D45D", "union": "", "uogonek": "",
		"uniF8F1": ""}
	for name, want := range cases {
		if got := CodeName(name); got != want {
			t.Errorf("CodeName(%s) = %q, want %q", name, got, want)
		}
	}
}

func TestFontAliasesReachTheirTFMsNames(t *testing.T) {
	table := Table{Fonts: map[string]map[string]string{"bbding10": {"enc-97": "●"}}}
	// bbding10.pfb's FontName is bbding: its reviewed names are the TFM's (books 0004).
	if table.SpellIn("bbding", "enc-97") != "●" || table.SpellIn("bbding10", "enc-97") != "●" ||
		table.SpellIn("cmr10", "enc-97") != "" {
		t.Fatal("bbding's glyphs spelled only through the font bbding10 stands for")
	}
}

func TestSymbolFontNamesAreReviewedByTheirPaintedGlyph(t *testing.T) {
	table := Load("../../../tex/relatex-symbol-glyphs.txt")
	cases := []struct{ font, name, want string }{
		{"txexs", "parenlefttpA", "⎛"}, {"txexs", "parenrightbtA", "⎠"}, {"txexs", "barex", "⏐"},
		{"txexs", "hat5", "ˆ"}, {"txexs", "tilde6", "˜"}, {"txexa", "iiintegtext", "∭"},
		{"txexa", "intersectionsqdisplay", "⨅"}, {"txexas", "dblbracketleftBig", "⟦"},
		{"txexa", "lbagBiggg", "⟅"}, {"txsyc", "nparallel", "∦"}, {"txsyc", "Wreathproduct", "≀"},
		{"wasy10", "varprop", "∝"},
		// stmaryrd's Type 1 names at slots 36-38 are shifted against the glyphs: ssearrow paints
		// the south-west arrow, curlywedgeuparrow the south-east one, sswarrow no character.
		{"stmary10", "ssearrow", "↙"}, {"stmary7", "curlywedgeuparrow", "↘"}, {"stmary10", "sswarrow", ""},
		{"cmr10", "ssearrow", ""},
		// pieces of \dashrightarrow stay unmapped.
		{"MSAM10", "arrowaxisright", ""}, {"MSAM10", "axisshort", ""},
		{"txexs", "bardblex", ""},
		// The rest of txsyc, stmaryrd and wasy (accessibility round 2): \strictif ⥽, \lJoin ⋉,
		// \openJoin ⪤, \bbslash ⑊, \APLlog ⍟, \pointer ⇨, \zh ʒ; \strictiff (two fish tails), the
		// other solid multimaps, txexa's arrowed integrals, \moo and \kreuz stay unmapped.
		{"txsyc", "strict", "⥽"}, {"pxsyc", "strictinverse", "⥼"}, {"txsyc", "multicloseleft1", "⋉"},
		{"txsyc", "multicloseboth1", "⨝"}, {"txsyc", "multiopenboth", "⪤"}, {"txsyc", "parallel1", "∥"},
		{"stmary10", "bbslash", "⑊"}, {"wasy10", "logof", "⍟"}, {"wasy7", "pointer", "⇨"},
		{"wasy10", "APLdown", "∇"}, {"wasy10", "rightturn", "↻"}, {"wasy10", "zh", "ʒ"},
		{"wasy10", "inve", "ə"}, {"wasy10", "dh", "ð"}, {"wasy10", "bell", "🔔"},
		{"txsyc", "rsolidmultimapboth", "⊶"}, {"pxsyc", "lsolidmultimapboth", "⊷"},
		{"txsyc", "strictboth", ""}, {"txsyc", "solidmultimap", ""}, {"txexa", "clclwdblcontintegtext", ""},
		{"stmary10", "moo", ""}, {"wasy10", "kreuz", ""},
	}
	for _, c := range cases {
		if got := table.SpellIn(c.font, c.name); got != c.want {
			t.Errorf("SpellIn(%s, %s) = %q, want %q", c.font, c.name, got, c.want)
		}
	}
}
