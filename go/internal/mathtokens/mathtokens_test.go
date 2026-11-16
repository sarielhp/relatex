package mathtokens

import "testing"

func tokensOf(source string) ([]rune, []Token) {
	text := []rune(source)
	return text, Tokenize(text, 0, len(text))
}

func spelled(tokens []Token) string {
	result := ""
	for _, token := range tokens {
		result += token.Glyph + "|"
	}
	return result
}

func TestTokenizeKeepsGlyphsAndDropsMarkup(t *testing.T) {
	cases := map[string]string{
		`a+b=c`:                               "a|+|b|=|c|",
		`x^2`:                                 "x|2|",
		`\frac{p}{q}`:                         "p|q|",
		`\alpha_i \le \beta`:                  "α|i|≤|β|",
		`\mathbf{v}\,\cdot\,w`:                "v|·|w|",
		`\left( a - b \right)`:                "(|a|-|b|)|",
		`\left. f \right|_0`:                  "f|||0|",
		`\sum_{i=1}^{n} x_i`:                  "∑|i|=|1|n|x|i|",
		`\sqrt[3]{x}`:                         "√|3|x|",
		`\int_0^1 g(s)\,ds`:                   "∫|0|1|g|(|s|)|d|s|",
		`\sin x % comment \alpha`:             "sin|x|",
		`\begin{array}{cc} a & b \end{array}`: "a|b|",
		`\unknownmacro{} y`:                   "y|",
	}
	for source, want := range cases {
		_, tokens := tokensOf(source)
		if got := spelled(tokens); got != want {
			t.Errorf("Tokenize(%q) = %q, want %q", source, got, want)
		}
	}
}

func TestTokenOffsetsAreScalarRanges(t *testing.T) {
	text, tokens := tokensOf(`é + \alpha`)
	if len(tokens) != 3 || tokens[0].Start != 0 || tokens[0].Stop != 1 || tokens[2].Start != 4 || tokens[2].Stop != 10 {
		t.Errorf("tokens = %+v over %q", tokens, string(text))
	}
}

func TestFoldMakesMathFontsAndSignsComparable(t *testing.T) {
	for typeset, want := range map[string]string{"𝐱": "x", "−": "-", "x̂": "x", "∗": "*", "⋅": "·", "ϵ": "ε", "a b": "ab"} {
		if got := Fold(typeset); got != want {
			t.Errorf("Fold(%q) = %q, want %q", typeset, got, want)
		}
	}
}

func TestAlignAssignsClustersToTokensInOrder(t *testing.T) {
	source := `E = mc^2 + \sum_{i=1}^{n} \alpha_i x_i`
	text, tokens := tokensOf(source)
	clusters := []string{"E", "=", "mc2", "+", "n", "∑", "i=1", "αixi"}
	matches := Align(clusters, tokens)
	slice := func(match Match) string { return string(text[match.Start:match.Stop]) }
	want := []string{"E", "=", "mc^2", "+", "n", `\sum`, "i=1", `\alpha_i x_i`}
	for index, match := range matches {
		if !match.OK || slice(match) != want[index] {
			t.Errorf("cluster %q -> %q (ok %v), want %q", clusters[index], slice(match), match.OK, want[index])
		}
	}
}

func TestAlignRefusesAClusterNothingSpellsAndNeverReusesATokenOfAnotherCluster(t *testing.T) {
	_, tokens := tokensOf(`a + b`)
	matches := Align([]string{"a", "z", "a"}, tokens)
	if !matches[0].OK || matches[1].OK || matches[2].OK {
		t.Errorf("matches = %+v, want only the first", matches)
	}
}

func TestSpellsRechecksAClaimedRange(t *testing.T) {
	_, tokens := tokensOf(`mc^2 + x`)
	if !Spells(tokens, 0, 4, "mc2") {
		t.Error("the range of mc^2 does not spell mc2")
	}
	for _, claim := range [][2]int{{0, 3}, {1, 4}, {0, 8}} {
		if Spells(tokens, claim[0], claim[1], "mc2") {
			t.Errorf("range %v was accepted for mc2", claim)
		}
	}
}

func TestDecodeUsesTheEncodingOfTheClassicMathFonts(t *testing.T) {
	for _, c := range []struct {
		font string
		code int
		text string
		want string
	}{
		{"cmmi10", 11, "", "α"},
		{"cmmi10", 97, "a", "a"},
		{"cmex10", 88, "X", "∑"},
		{"cmex10", 90, "Z", "∫"},
		{"cmsy10", 20, "", "≤"},
		{"cmsy7", 0, "", "−"},
		{"EURM10", 22, "\u00af", "μ"},
		{"cmsy10", 1, "", "·"},
		{"LMRoman10-Regular", 97, "a", "a"},
		{"cmmi10", 200, "q", "q"},
	} {
		if got := Decode(c.font, c.code, c.text); got != c.want {
			t.Errorf("Decode(%s, %d, %q) = %q, want %q", c.font, c.code, c.text, got, c.want)
		}
	}
}

func TestSplitGivesEachGlyphItsTokens(t *testing.T) {
	text := []rune(`mc^2 \alpha x`)
	tokens := Tokenize(text, 0, len(text))
	matches, ok := Split([]string{"m", "c", "2"}, tokens, 0, 4)
	if !ok {
		t.Fatalf("split failed: %v", matches)
	}
	want := [][2]int{{0, 1}, {1, 2}, {3, 4}}
	for index, match := range matches {
		if match.Start != want[index][0] || match.Stop != want[index][1] {
			t.Errorf("glyph %d = %d..%d, want %v", index, match.Start, match.Stop, want[index])
		}
	}
	if _, ok := Split([]string{"m", "c"}, tokens, 0, 4); ok {
		t.Error("glyphs that leave a token unspelled must not split")
	}
}
