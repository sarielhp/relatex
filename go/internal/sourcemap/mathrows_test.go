package sourcemap

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

const alignSource = "\\begin{align}\nu &= v + w \\\\\nf(t) &= g(s)\n\\end{align}\n"

func alignEntry() *sourceText {
	m := rec.Map{"sources": rec.Map{"main.tex": rec.Map{"text": alignSource}}}
	return loadSource(m, "main.tex", map[string]*sourceText{})
}

// A math origin is honest when it is a formula span or one row of a multi-row display.
func TestRowOfADisplayIsAnHonestMathSpan(t *testing.T) {
	entry := alignEntry()
	row1Start, _ := entry.visible.LineStart(2)
	row2Start, _ := entry.visible.LineStart(3)
	whole := [2]int{0, len(alignSource) - 1} // up to the closing \end{align}, not the newline after it
	word := rec.Map{"text": "zz"}
	for name, span := range map[string][2]int{
		"whole display": whole,
		"row 1":         {row1Start, row1Start + len("u &= v + w")},
		"row 2":         {row2Start, row2Start + len("f(t) &= g(s)")},
	} {
		if !entry.honest(word, "math", span[0], span[1]) {
			t.Errorf("%s %v is not an honest math span", name, span)
		}
	}
	if entry.honest(word, "math", row1Start, row1Start+4) {
		t.Error("a fragment of a row that spells nothing was accepted as a math span")
	}
}

// A token-level origin is honest only when it starts and ends on token boundaries inside one
// formula and the tokens spell the word.
func TestTokenRangeIsAnHonestMathSpanOnlyWhenItSpellsTheWord(t *testing.T) {
	entry := alignEntry()
	row1Start, _ := entry.visible.LineStart(2)
	v := row1Start + strings.Index("u &= v + w", "v")
	if !entry.honest(rec.Map{"text": "v"}, "math", v, v+1) {
		t.Error("the token v was not honest for the word v")
	}
	if entry.honest(rec.Map{"text": "w"}, "math", v, v+1) {
		t.Error("the token v was honest for the word w")
	}
	if !entry.honest(rec.Map{"text": "v+"}, "math", v, v+3) {
		t.Error("the tokens v + were not honest for the word v+")
	}
	if entry.honest(rec.Map{"text": "v"}, "math", v-1, v+1) {
		t.Error("a range starting before the token was honest")
	}
}
