package mathtokens

import (
	"regexp"
	"strings"
)

// The classic TeX math fonts have no ToUnicode map: the glyph's text is taken from its code,
// which is a letter for the Latin glyphs but wrong or empty for Greek, operators and big
// symbols. Their encodings are fixed (OML for cmmi/lmmi, OMS for cmsy/lmsy, OMX for cmex/lmex),
// so the code decides. The Euler fonts (eurm, eurb) carry the Greek letters at the OML codes.
var (
	mathFont = regexp.MustCompile(`^(?:cm|lm)(mi|sy|ex)\d*|^eu(rm|bm)\d*`)
	oml      = buildOML()
	oms      = buildOMS()
	omx      = buildOMX()
)

// Decode is the text of a math glyph: from the font's encoding table when the font is one of
// the classic math fonts and the table knows the code, else the text the collector gave.
func Decode(fontName string, code int, text string) string {
	match := mathFont.FindStringSubmatch(strings.ToLower(fontName))
	if match == nil {
		return text
	}
	table := map[string]map[int]string{"mi": oml, "sy": oms, "ex": omx, "": oml}[match[1]]
	if decoded, ok := table[code]; ok {
		return decoded
	}
	return text
}

// runes builds a table from consecutive codes starting at first.
func runes(table map[int]string, first int, glyphs string) {
	for offset, glyph := range []rune(glyphs) {
		table[first+offset] = string(glyph)
	}
}

func buildOML() map[int]string {
	table := map[int]string{}
	runes(table, 0, "ΓΔΘΛΞΠΣΥΦΨΩαβγδεζηθικλμνξπρστυφχψωεϑϖϱςφ")
	runes(table, 48, "0123456789")
	table[58], table[59], table[60], table[61], table[62], table[63] = ".", ",", "<", "/", ">", "⋆"
	table[64] = "∂"
	for letter := 0; letter < 26; letter++ {
		table[65+letter] = string(rune('A' + letter))
		table[97+letter] = string(rune('a' + letter))
	}
	table[96], table[123], table[124], table[125] = "ℓ", "ı", "ȷ", "℘"
	return table
}

func buildOMS() map[int]string {
	table := map[int]string{}
	runes(table, 0, "−·×∗÷⋄±∓⊕⊖⊗⊘⊙○∘•≍≡⊆⊇≤≥≼≽∼≈⊂⊃≪≫≺≻←→↑↓↔↗↙≃⇐⇒⇑⇓⇔↖↘∝′∞∈∋△▽/'∀∃¬∅ℜℑ⊤⊥ℵ")
	for letter := 0; letter < 26; letter++ {
		table[65+letter] = string(rune('A' + letter))
	}
	runes(table, 91, "∪∩⊎∧∨⊢⊣⌊⌋⌈⌉{}⟨⟩|‖↕⇕∖≀√∐∇∫⊔⊓⊑⊒§†‡¶♣♢♡♠")
	return table
}

func buildOMX() map[int]string {
	table := map[int]string{}
	big := map[int]string{80: "∑", 88: "∑", 81: "∏", 89: "∏", 82: "∫", 90: "∫", 72: "∮", 73: "∮", 83: "⋃", 91: "⋃", 84: "⋂", 92: "⋂",
		85: "⊔", 93: "⊔", 86: "⋁", 94: "⋁", 87: "⋀", 95: "⋀", 96: "∐", 97: "∐", 112: "√", 113: "√", 114: "√", 115: "√", 116: "√"}
	for code, glyph := range big {
		table[code] = glyph
	}
	return table
}
