package mathintent

import (
	"maps"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

var paperMacros = Macros{
	"abs":   {Body: `\left| #1 \right|`, Parameters: 1},
	"card":  {Body: `\lvert #1\rvert`, Parameters: 1},
	"norm":  {Body: `\left\| #1 \right\|`, Parameters: 1},
	"size":  {Body: `\left| #1 \right|`, Parameters: 1},
	"floor": {Body: `\left \lfloor #1 \right \rfloor`, Parameters: 1},
	"bin":   {Body: `\binom{#1}{#2}`, Parameters: 2},
	"T":     {Body: `\top`, Parameters: 0},
	"Prob":  {Body: `\Pr`, Parameters: 0},
	"PP":    {Body: `\mathbb{P}`, Parameters: 0},
	"QQ":    {Body: `\mathbb{P}`, Parameters: 0},
	"dd":    {Body: `\mathrm{d}`, Parameters: 0},
}

func TestRead(t *testing.T) {
	cases := []struct {
		name, tex string
		want      map[Concept]int
	}{
		{"binomials", `\binom{n}{k} + \tbinom{n}{2} + \dbinom mj + {a \choose b}`, map[Concept]int{Binomial: 4}},
		{"a macro using \\binom", `\bin{n}{k}`, map[Concept]int{Binomial: 1}},
		{"floor and ceiling", `\lfloor x \rfloor + \left\lceil y \right\rceil + \floor{z}`, map[Concept]int{Floor: 2, Ceiling: 1}},
		{"absolute values", `\abs{x} + \abs{\frac{a}{b}}`, map[Concept]int{AbsoluteValue: 2}},
		{"cardinality", `\card{S}`, map[Concept]int{Cardinality: 1}},
		{"a bar macro of another name says nothing", `\size{x}`, map[Concept]int{}},
		{"a bare bar makes the bars ambiguous", `\abs{x} + |y|`, map[Concept]int{}},
		{"\\lvert alone says nothing", `\lvert x \rvert`, map[Concept]int{}},
		{"bare bars say nothing (decision 114, item 5)", `|x| + \left| y \right| + \|v\| + \Vert w \Vert`, map[Concept]int{}},
		{"nested absolute values", `\abs{\abs{x} - 1}`, map[Concept]int{}},
		{"absolute value and cardinality together", `\abs{x} \card{S}`, map[Concept]int{}},
		{"norms", `\norm{v} + \lVert w \rVert + \bigl\| u \bigr\|`, map[Concept]int{Norm: 3}},
		{"a bare double bar", `\norm{v} + \|u\|`, map[Concept]int{}},
		{"norm inside an absolute value", `\abs{\norm{v}}`, map[Concept]int{AbsoluteValue: 1, Norm: 1}},
		{"transposes", `A^\top B^{\top} C^\intercal D^{\mathsf{T}} E^\mathrm{T} F^\T`, map[Concept]int{Transpose: 6}},
		{"an italic T is a power", `A^T x^{T}`, map[Concept]int{}},
		{"inverse functions", `\sin^{-1} x + \cosh^{-1}y`, map[Concept]int{Inverse: 2}},
		{"other inverses are powers", `f^{-1}(y) A^{-1}`, map[Concept]int{}},
		{"factorials, not negative space", `n! (n+1)! \! x`, map[Concept]int{Factorial: 2}},
		{"probability", `\Pr[X] + \Prob(A) + \operatorname{Pr}(B)`, map[Concept]int{Probability: 3}},
		{"derivatives", `\frac{dy}{dx} + \frac{d^2 y}{d x^2} + \dfrac{\dd f}{\dd t}`, map[Concept]int{Derivative: 3}},
		{"partial derivatives", `\frac{\partial f}{\partial x} + \frac{\partial^2 f}{\partial x\,\partial y} + \frac{\partial^2 u}{\partial t^2}`,
			map[Concept]int{PartialDerivative: 3}},
		{"the operator form names no function", `\frac{d}{dx} f`, map[Concept]int{}},
		{"orders must add up", `\frac{d^2 y}{dx} + \frac{\partial^3 f}{\partial x \partial y}`, map[Concept]int{}},
		{"a fraction of d's is not always a derivative", `\frac{d-1}{d x} + \frac{dy}{dx dz}`, map[Concept]int{}},
		{"comments are not read", "\\binom{n}{k} % \\binom{a}{b}\n", map[Concept]int{Binomial: 1}},
	}
	for _, test := range cases {
		if got := Read(test.tex, paperMacros).Counts; !maps.Equal(got, test.want) {
			t.Errorf("%s: %v, want %v", test.name, got, test.want)
		}
	}
}

func TestBlackboardProbability(t *testing.T) {
	if got := Read(`\PP(A) + \QQ(B) + \mathbb{P}(C)`, paperMacros).BlackboardProbability; got != 1 {
		t.Fatalf("only a probability name defined as \\mathbb{P} counts: %d", got)
	}
}

func TestMacrosOf(t *testing.T) {
	document := rec.Map{"math_macros": rec.Map{"abs": rec.Map{"body": `\left|#1\right|`, "parameters": int64(1)}}}
	if got := MacrosOf(document); got["abs"] != (Macro{Body: `\left|#1\right|`, Parameters: 1}) {
		t.Fatalf("macros: %v", got)
	}
	if got := MacrosOf(rec.Map{}); len(got) != 0 {
		t.Fatalf("no macros: %v", got)
	}
}

func TestExpansionTerminates(t *testing.T) {
	loop := Macros{"a": {Body: `\a\binom{n}{k}`}}
	if got := Read(`\a`, loop).Count(Binomial); got != maxDepth {
		t.Fatalf("a self-using macro expands %d times: %d", maxDepth, got)
	}
}

// TestNewcommandDelimiters checks Part 1: plain \newcommand double-bar macros get Norm by their
// delimiter pair regardless of name, \abs with single bars and \floor/\ceil with brackets work when
// the body is just the pair around #1, and partial or multi-parameter bodies keep no intent.
func TestNewcommandDelimiters(t *testing.T) {
	macros := Macros{
		"norm":    {Body: `\lVert#1\rVert`, Parameters: 1},
		"bars":    {Body: `\|#1\|`, Parameters: 1},
		"bigbars": {Body: `\left\|#1\right\|`, Parameters: 1},
		"abs":     {Body: `\lvert#1\rvert`, Parameters: 1},
		"absbar":  {Body: `|#1|`, Parameters: 1},
		"floor":   {Body: `\lfloor#1\rfloor`, Parameters: 1},
		"ceil":    {Body: `\lceil#1\rceil`, Parameters: 1},
		"sqbars":  {Body: `\|#1\|_2`, Parameters: 1},
		"plusone": {Body: `\lvert#1\rvert + 1`, Parameters: 1},
		"absplus": {Body: `|#1|+1`, Parameters: 1},
		"other":   {Body: `|#1|`, Parameters: 1},
		"twodiff": {Body: `\|#1 - #2\|`, Parameters: 2},
		"box":     {Body: `[#1]`, Parameters: 1},
	}
	// Resolve absbar under the name "abs" as well so both |#1| and \lvert#1\rvert are tested on \abs.
	pipeAbs := Macros{"abs": macros["absbar"]}
	cases := []struct {
		name   string
		tex    string
		macros Macros
		want   map[Concept]int
	}{
		{"norm \\lVert#1\\rVert", `\norm{v}`, macros, map[Concept]int{Norm: 1}},
		{"unnamed double bars \\|#1\\|", `\bars{v}`, macros, map[Concept]int{Norm: 1}},
		{"unnamed left-right double bars", `\bigbars{v}`, macros, map[Concept]int{Norm: 1}},
		{"abs with \\lvert...\\rvert", `\abs{x}`, macros, map[Concept]int{AbsoluteValue: 1}},
		{"abs with |...|", `\abs{x}`, pipeAbs, map[Concept]int{AbsoluteValue: 1}},
		{"floor and ceil brackets", `\floor{x} + \ceil{y}`, macros, map[Concept]int{Floor: 1, Ceiling: 1}},
		{"nested macro norm inside abs", `\abs{\bars{v}}`, macros, map[Concept]int{AbsoluteValue: 1, Norm: 1}},
		{"double-bar body with subscript gets no intent", `\sqbars{v}`, macros, map[Concept]int{}},
		{"single-bar body with trailing term gets no intent", `\plusone{x}`, macros, map[Concept]int{}},
		{"plain bar body with trailing term gets no intent", `\absplus{x}`, macros, map[Concept]int{}},
		{"unnamed single bars get no intent", `\other{x}`, macros, map[Concept]int{}},
		{"two-parameter norm difference gets no intent", `\twodiff{x}{y}`, macros, map[Concept]int{}},
		{"brackets are not a norm or abs", `\box{x}`, macros, map[Concept]int{}},
	}
	for _, test := range cases {
		if got := Read(test.tex, test.macros).Counts; !maps.Equal(got, test.want) {
			t.Errorf("%s: %v, want %v", test.name, got, test.want)
		}
	}
}

// TestTransposeMacroBody (decision 187): the body decides, never the name.
func TestTransposeMacroBody(t *testing.T) {
	bodies := map[string]bool{
		`^\top`: true, `^{\top}`: true, `^\intercal`: true, `^T`: true, `^{\mathsf T}`: true, `^{\mathrm{T}}`: true,
		`^{\mkern -1.5mu\mathsf {T}}`: true, `\ensuremath{^{\top}}`: true, `{}^\top`: true, `^{\kern1pt\top}`: true,
		`^2`: false, `^{\top}x`: true, `^T x`: false, `\top`: false, `^{\mathit{T}}`: false, `^{TT}`: false,
	}
	for body, want := range bodies {
		macros := Macros{"tr": {Body: body}}
		if got := Read(`(AB)\tr`, macros).Count(Transpose) == 1; got != want {
			t.Errorf("body %s: transpose %v, want %v", body, got, want)
		}
	}
	if got := Read(`A\tr`, Macros{"tr": {Body: `^T`}}); got.ItalicTranspose != 1 || got.Count(Transpose) != 1 {
		t.Errorf("an italic T body: %+v", got)
	}
	if got := Read(`A\tr`, Macros{"tr": {Body: `^\top`}}); got.ItalicTranspose != 0 {
		t.Errorf("a top body: %+v", got)
	}
}
