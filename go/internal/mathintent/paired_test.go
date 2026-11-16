package mathintent

import (
	"maps"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// pairedMacros are mathtools paired delimiters as authormath keeps them (decision 114): the plain
// form's body, marked Paired.
var pairedMacros = Macros{
	"abs":   {Body: `\mathopen\lvert #1\mathclose\rvert`, Parameters: 1, Paired: true},
	"card":  {Body: `\mathopen|#1\mathclose|`, Parameters: 1, Paired: true},
	"fcomp": {Body: `\mathopen\lvert #1\mathclose\rvert`, Parameters: 1, Paired: true},
	"nrm":   {Body: `\mathopen\lVert #1\mathclose\rVert`, Parameters: 1, Paired: true},
	"ceil":  {Body: `\mathopen\lceil #1\mathclose\rceil`, Parameters: 1, Paired: true},
	"floor": {Body: `\mathopen\lfloor #1\mathclose\rfloor`, Parameters: 1, Paired: true},
	"inner": {Body: `\mathopen\langle #1\mathclose\rangle`, Parameters: 1, Paired: true},
	"ip":    {Body: `\mathopen\langle #1,#2\mathclose\rangle`, Parameters: 2, Paired: true},
	"gen":   {Body: `\mathopen\langle #1\mathclose\rangle`, Parameters: 1, Paired: true},
	"vnorm": {Body: `\mathopen\| #1\mathclose\|`, Parameters: 1},
}

// TestPairedDelimiterIntent: a paired delimiter's concept comes from its pair (floor, ceiling, a
// norm's double bars) or, where the pair means several things (a bar, angle brackets), from its
// name; the star and the optional size change nothing (decision 114, amendment 2026-11-13).
func TestPairedDelimiterIntent(t *testing.T) {
	cases := []struct {
		name, tex string
		want      map[Concept]int
	}{
		{"plain, starred and sized absolute values", `\abs{x} + \abs*{\frac{a}{b}} + \abs[\big]{y} + \abs [\Bigg] {z}`,
			map[Concept]int{AbsoluteValue: 4}},
		{"a starred cardinality", `\card*{S}`, map[Concept]int{Cardinality: 1}},
		{"a bar pair of another name says nothing", `\fcomp*{x} + \fcomp[\big]{y}`, map[Concept]int{}},
		{"double bars are a norm by the pair", `\nrm{v} + \nrm*{w} + \nrm[\Big]{u}`, map[Concept]int{Norm: 3}},
		{"a plain double-bar macro is a norm by the pair without paired or a norm name", `\vnorm{v}`, map[Concept]int{Norm: 1}},
		{"floor and ceiling by the pair, starred and sized", `\ceil*{\frac{n}{2}} + \floor[\big]{x} + \floor{y}`,
			map[Concept]int{Ceiling: 1, Floor: 2}},
		{"an inner product of one parameter, two sides", `\inner{u, v} + \inner*{x,y} + \inner[\big]{a, {b, c}}`,
			map[Concept]int{InnerProduct: 3}},
		{"angle brackets without one comma", `\inner{u} + \inner{a, b, c}`, map[Concept]int{}},
		{"angle brackets of another name", `\gen{a, b}`, map[Concept]int{}},
		{"an inner product of two parameters", `\ip*{u}{v} + \ip[\big]{x}{y}`, map[Concept]int{InnerProduct: 2}},
		{"the size's brackets are not an argument", `\abs[\big]{x} + [0, 1]`, map[Concept]int{AbsoluteValue: 1}},
		{"a bare bar still makes the bars ambiguous", `\abs*{x} + |y|`, map[Concept]int{}},
	}
	for _, test := range cases {
		if got := Read(test.tex, pairedMacros).Counts; !maps.Equal(got, test.want) {
			t.Errorf("%s: %v, want %v", test.name, got, test.want)
		}
	}
}

// TestMacrosOfPaired: a kept macro with a paired declaration is read as a paired delimiter.
func TestMacrosOfPaired(t *testing.T) {
	declaration := rec.Map{"before": "", "open": `\lvert`, "close": `\rvert`, "after": "", "inner": "#1"}
	document := rec.Map{"math_macros": rec.Map{
		"abs": rec.Map{"body": `\mathopen\lvert #1\mathclose\rvert`, "parameters": int64(1), "paired": declaration}}}
	if got := MacrosOf(document)["abs"]; !got.Paired {
		t.Fatalf("not paired: %+v", got)
	}
}
