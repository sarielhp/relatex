package cli

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func mathWord(text string, line, begin, end int64) rec.Map {
	return rec.Map{"text": text, "origins": []any{rec.Map{"file": "main.tex", "line": line, "precision": "math",
		"characters": []any{begin, end}}}}
}

// Clicking on a token also overlaps the formula-wide range of the equation number; the token
// is the answer.
func TestNarrowestDropsAMathWordWhoseRangeHoldsAnother(t *testing.T) {
	token, number := mathWord("g(s)", 12, 17, 21), mathWord("(3)", 12, 0, 25)
	other := mathWord("ds", 12, 23, 25)
	got := narrowest([]any{number, token}, query{file: "main.tex", line: 12})
	if len(got) != 1 || rec.AsMap(got[0])["text"] != "g(s)" {
		t.Errorf("narrowest = %v, want only g(s)", got)
	}
	got = narrowest([]any{token, other}, query{file: "main.tex", line: 12})
	if len(got) != 2 {
		t.Errorf("two disjoint tokens: %d kept, want 2", len(got))
	}
	got = narrowest([]any{number, number}, query{file: "main.tex", line: 12})
	if len(got) != 2 {
		t.Errorf("equal ranges are not nested: %d kept, want 2", len(got))
	}
}

func TestOverlapsAndNarrowestAcceptSlashedSourceQuery(t *testing.T) {
	origin := rec.Map{"file": "sections/intro.tex", "line": int64(10)}
	winQuery := strings.ReplaceAll("sections/intro.tex", "/", `\`)
	normalized := normalizeSourceArg(winQuery, func(p string) string { return strings.ReplaceAll(p, `\`, "/") })
	if !overlaps(origin, normalized, [2]float64{10, 10}, [2]float64{1, 100}) {
		t.Errorf("overlaps failed for normalized %q", normalized)
	}
}
