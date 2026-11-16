package bibclean

import (
	"reflect"
	"strings"
	"testing"
)

// Decision 172 (the plan's decision A): a MathRoute writes every inline formula of a printed field, in
// field order per entry; verbatim fields keep theirs, and a formula it declines keeps the cleaner's reading.
func TestMathRouteWritesFormulas(t *testing.T) {
	input := `@article{k1,
  title = {On $\varepsilon$-nets and \(k\)-sets},
  note = {$x^2$},
  url = {http://a.example/$x$},
}`
	calls := []string{}
	route := func(key string, index int, tex string) (string, bool) {
		calls = append(calls, key+":"+tex)
		if index == 2 {
			return "", false
		}
		return "$M" + string(rune('0'+index)) + "$", true
	}
	out, report, err := CleanInputs([]Input{{Name: "r.bib", Data: input}}, Options{All: true, Math: route})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "title = {On $M0$-nets and $M1$-sets}") || !strings.Contains(out, "note = {$x^2$}") ||
		!strings.Contains(out, "url = {http://a.example/$x$}") {
		t.Errorf("cleaned:\n%s", out)
	}
	if want := []string{`k1:\varepsilon`, "k1:k", "k1:x^2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("calls %q, want %q", calls, want)
	}
	if report.Entries[0].Status != StatusConverted {
		t.Errorf("report %+v", report.Entries[0])
	}
}

// \ensuremath{..} typesets its argument as an inline formula wherever it stands (books/0003's s-pgpd-89 has
// one in its abstract), so it is one more formula of the entry: the route writes it, and without a route a
// formula with commands leaves the entry unconverted, as $..$ does.
func TestEnsureMathIsAFormula(t *testing.T) {
	input := `@article{s89,
  title = {Planar graphs},
  abstract = {the order on \ensuremath{V \cup E} whose},
}`
	calls := []string{}
	route := func(_ string, _ int, tex string) (string, bool) {
		calls = append(calls, tex)
		return "", true
	}
	out, report, err := CleanInputs([]Input{{Name: "e.bib", Data: input}}, Options{All: true, Math: route})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "abstract = {the order on  whose}") || report.Entries[0].Status != StatusConverted {
		t.Errorf("routed:\n%s\n%+v", out, report.Entries[0])
	}
	if want := []string{`V \cup E`}; !reflect.DeepEqual(calls, want) {
		t.Errorf("calls %q, want %q", calls, want)
	}
	_, plain, err := CleanInputs([]Input{{Name: "e.bib", Data: input}}, Options{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if !hasReasonKind(plain.Entries[0].Reasons, "math_command", `\cup`) {
		t.Errorf("without a route: %+v", plain.Entries[0])
	}
}
