package mathintent

import (
	"maps"
	"testing"
)

// TestBoldSignNeedsAnOperand: \def\bE{{\mathbf E}} of papers4/1042 names a triple of sets, so a bare
// \bE is a symbol; the same bold sign followed by an operand bracket is an expectation. The lines
// are the paper's.
func TestBoldSignNeedsAnOperand(t *testing.T) {
	macros := Macros{
		"bE":  {Body: `{\mathbf E}`},
		"P":   {Body: `\mathbb{P}`},
		"Var": {Body: `\mathrm{V}`},
	}
	cases := []struct {
		name, tex string
		want      map[Concept]int
	}{
		{"a triple of sets", `\bE=(E_1,E_2,E_3)`, map[Concept]int{}},
		{"a bare symbol", `\Dist(\bE,\scripto(\bEstar))\le C\delta^{1/2} + \scriptt(\bE) \le 1`, map[Concept]int{}},
		{"an operand bracket", `\bE[X^2]`, map[Concept]int{ExpectedValue: 1}},
		{"sized bracket", `\bE\left[X\right]`, map[Concept]int{ExpectedValue: 1}},
		{"subscripted operator", `\bE_{x\sim\mu}[f(x)]`, map[Concept]int{ExpectedValue: 1}},
		{"a roman V", `\Var = (V_1,V_2)`, map[Concept]int{}},
		{"a roman V with an argument", `\Var(X)`, map[Concept]int{Variance: 1}},
	}
	for _, test := range cases {
		if got := Read(test.tex, macros).Counts; !maps.Equal(got, test.want) {
			t.Errorf("%s: %v, want %v", test.name, got, test.want)
		}
	}
	if got := Read(`\P = \P_n`, macros).BlackboardProbability; got != 0 {
		t.Errorf("a bare blackboard P is a symbol: %d", got)
	}
	if got := Read(`\P(A) + \P_x[B]`, macros).BlackboardProbability; got != 2 {
		t.Errorf("a probability with an argument: %d", got)
	}
}

// TestOperatorsAndPairedDelimiters (decision 114): expectation and variance macros, and the bodies
// authormath keeps for mathtools' paired delimiters.
func TestOperatorsAndPairedDelimiters(t *testing.T) {
	macros := Macros{
		"E":     {Body: `\mathbb{E}`},
		"Ex":    {Body: `\mathop{\mathbb{E}}\left[#1\right]`, Parameters: 1},
		"EE":    {Body: `E`},
		"Var":   {Body: `\operatorname{Var}`},
		"var":   {Body: `\mathrm{Variation}`},
		"abs":   {Body: `\mathopen\lvert #1\mathclose\rvert`, Parameters: 1},
		"norm":  {Body: `\mathopen\lVert #1\mathclose\rVert`, Parameters: 1},
		"floor": {Body: `\mathopen\lfloor #1\mathclose\rfloor`, Parameters: 1},
		"inner": {Body: `\mathopen\langle #1,#2\mathclose\rangle`, Parameters: 2},
		"ip":    {Body: `\left<{#1},\,{#2}\right>`, Parameters: 2},
		"pair":  {Body: `\langle #1;#2\rangle`, Parameters: 2},
	}
	cases := []struct {
		name, tex string
		want      map[Concept]int
	}{
		{"expectations", `\E[X] + \Ex{Y}`, map[Concept]int{ExpectedValue: 2}},
		{"an italic E is a variable", `\EE[X]`, map[Concept]int{}},
		{"variance", `\Var(X) = \E[X^2]`, map[Concept]int{Variance: 1, ExpectedValue: 1}},
		{"a longer word is no variance", `\var(f)`, map[Concept]int{}},
		{"paired delimiters", `\abs{x} + \norm{v} + \floor{t}`, map[Concept]int{AbsoluteValue: 1, Norm: 1, Floor: 1}},
		{"inner products", `\inner{u}{v} + \ip{a+b}{c}`, map[Concept]int{InnerProduct: 2}},
		{"an angle pair of another shape", `\pair{u}{v}`, map[Concept]int{}},
	}
	for _, test := range cases {
		if got := Read(test.tex, macros).Counts; !maps.Equal(got, test.want) {
			t.Errorf("%s: %v, want %v", test.name, got, test.want)
		}
	}
}
