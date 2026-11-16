package synchronize

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/mathsource"
	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/sourcetext"
)

const alignText = "\\begin{align}\nu &= v + w \\\\\nf(t) &= g(s)\n\\end{align}\n"

func placementFor(text string) *mathPlacement {
	spans := mathsource.NewSourceText(text, nil).Spans
	masks := []sourcetext.Span{}
	for _, span := range spans {
		masks = append(masks, sourcetext.Span{Start: span.Start, Stop: span.Stop})
	}
	texts := omap.New[string, *source]()
	texts.Set("main.tex", &source{text: text, spans: spans, visible: sourcetext.New(text, masks)})
	return newMathPlacement(texts, nil, map[string]string{"main.tex": "sha"})
}

func originOf(m *mathPlacement, formula int64) (line int64, characters []any) {
	origin := m.origin(formula, "main.tex", 4, 4)
	if origin == nil {
		return 0, nil
	}
	return origin["line"].(int64), origin["characters"].([]any)
}

// The rows of an align are typeset as separate formulas with the same bracketing lines; each
// takes its own source row.
func TestRowsOfADisplayTakeTheirOwnSourceRange(t *testing.T) {
	m := placementFor(alignText)
	m.prepare([]formulaSight{{7, "main.tex", 4, 4}, {7, "main.tex", 4, 4}, {8, "main.tex", 4, 4}})
	line, characters := originOf(m, 7)
	if line != 2 || characters[0] != int64(0) || characters[1] != int64(10) {
		t.Errorf("row 1 = line %d %v, want line 2 [0 10] (u &= v + w)", line, characters)
	}
	line, characters = originOf(m, 8)
	if line != 3 || characters[0] != int64(0) || characters[1] != int64(12) {
		t.Errorf("row 2 = line %d %v, want line 3 [0 12] (f(t) &= g(s))", line, characters)
	}
}

// When the number of formulas is not the number of source rows the rows cannot be told
// apart, and every formula keeps the whole display: no guess.
func TestRowsAreNotGuessedWhenTheCountsDiffer(t *testing.T) {
	m := placementFor(alignText)
	m.prepare([]formulaSight{{7, "main.tex", 4, 4}, {8, "main.tex", 4, 4}, {9, "main.tex", 4, 4}})
	for _, formula := range []int64{7, 8, 9} {
		if line, characters := originOf(m, formula); line != 1 || characters[0] != int64(0) {
			t.Errorf("formula %d = line %d %v, want the whole display from line 1", formula, line, characters)
		}
	}
}

// A single-row display (equation) and an inline formula are left as they are.
func TestSingleFormulaDisplaysKeepTheirSpan(t *testing.T) {
	m := placementFor("\\begin{equation}\nE = mc^2\n\\end{equation}\n")
	m.prepare([]formulaSight{{7, "main.tex", 2, 2}})
	if line, _ := originOf(m, 7); line != 1 {
		t.Errorf("equation = line %d, want 1", line)
	}
	if len(m.rows) != 0 {
		t.Errorf("rows were assigned to a single-row display: %v", m.rows)
	}
}

func TestAlignatSkipsItsColumnCount(t *testing.T) {
	text := "\\begin{alignat}{2}\na &= b \\\\\nc &= d\n\\end{alignat}\n"
	m := placementFor(text)
	m.prepare([]formulaSight{{7, "main.tex", 4, 4}, {8, "main.tex", 4, 4}})
	if line, characters := originOf(m, 7); line != 2 || characters[1] != int64(6) {
		t.Errorf("alignat row 1 = line %d %v, want line 2 [0 6] (a &= b)", line, characters)
	}
}

// TeX can bracket a display's first row with the last line of the display before it; the one
// span inside the bracket is the row's.
func TestBracketReachingIntoThePreviousDisplayStillFindsTheFormula(t *testing.T) {
	text := "\\begin{align}\nu &= v \\\\\nf &= g\n\\end{align}\n\nx\n\n\\begin{gather}\np = q \\\\\nr \\ge s\n\\end{gather}\n"
	m := placementFor(text)
	// Lines 1-4 are the align, 8-11 the gather; the bracket 4..11 overlaps both.
	found := m.bracketed("main.tex", 4, 11)
	if found.span == nil || found.span.Environment != "gather" {
		t.Errorf("bracket 4..11 resolved to %+v, want the gather", found.span)
	}
	m.prepare([]formulaSight{{7, "main.tex", 4, 11}, {8, "main.tex", 11, 11}})
	if line, _ := originOf(m, 7); line != 9 {
		t.Errorf("first gather row = line %d, want 9", line)
	}
}

// A math word of several glyphs gets one range per glyph, so a click on the m of mc^2 lands on
// the m, not on the whole word.
func TestGlyphsOfAMathWordTakeTheirOwnTokens(t *testing.T) {
	text := "$E = mc^2$\n"
	m := placementFor(text)
	m.prepare([]formulaSight{{3, "main.tex", 1, 1}})
	spelled := &word{id: "w1", glyphs: []*glyph{{text: "m", formula: 3}, {text: "c", formula: 3}, {text: "2", formula: 3}}}
	m.alignWords([]*word{spelled})
	origins := m.letterOrigins("w1")
	if len(origins) != 3 {
		t.Fatalf("letters = %d, want 3", len(origins))
	}
	want := [][2]int64{{5, 6}, {6, 7}, {8, 9}}
	for index, origin := range origins {
		characters := origin["characters"].([]any)
		if characters[0] != want[index][0] || characters[1] != want[index][1] {
			t.Errorf("glyph %d = %v, want %v", index, characters, want[index])
		}
	}
}

func TestExpandRootNormalizesWindowsPaths(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{`sections\a\..\intro.tex`, "/sections/intro.tex"},
		{`C:\paper\main.tex`, "C:/paper/main.tex"},
		{`/sections/intro.tex`, "/sections/intro.tex"},
		{`sections/intro.tex`, "/sections/intro.tex"},
	}
	for _, c := range cases {
		if got := expandRootWith(c.input, func(p string) string { return strings.ReplaceAll(p, `\`, "/") }); got != c.want {
			t.Errorf("expandRootWith(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
