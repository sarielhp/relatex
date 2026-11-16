package mathintent

import (
	"maps"
	"testing"
)

// diracMacros are the Dirac macros the stores keep, as their papers define them.
var diracMacros = Macros{
	"ket":    {Body: `| #1 \rangle`, Parameters: 1},
	"bra":    {Body: `\langle #1 |`, Parameters: 1},
	"rawket": {Body: `|{#1}\rangle`, Parameters: 1},
	"Ket":    {Body: `\left| #1 \right\rangle`, Parameters: 1},
	"Bra":    {Body: `\left< #1 \right|`, Parameters: 1},
	"lket":   {Body: `\left| #1 \right>`, Parameters: 1},
	"braket": {Body: `\langle #1 | #2 \rangle`, Parameters: 2},
	"Braket": {Body: `\langle #1 \vphantom{#2}\right| \left.\vphantom{#2} #2 \right\rangle`, Parameters: 2},
	// The shapes a concept is not given for: another name, other text, the no-argument macros.
	"state":  {Body: `| #1 \rangle`, Parameters: 1},
	"keti":   {Body: `\left| #1 \right\rangle_i`, Parameters: 1},
	"braphi": {Body: `\langle \phi |`, Parameters: 0},
	"absv":   {Body: `\left| #1 \right|`, Parameters: 1},
}

func TestDiracRead(t *testing.T) {
	cases := []struct {
		name, tex string
		want      map[Concept]int
	}{
		{"a ket", `\ket{\psi}`, map[Concept]int{Ket: 1}},
		{"both", `\ket{\psi} + \bra{\phi}`, map[Concept]int{Ket: 1, Bra: 1}},
		{"a bra of a sum", `\bra{\phi + \chi}`, map[Concept]int{Bra: 1}},
		{"a ket of a sum", `\ket{\psi + \chi}`, map[Concept]int{Ket: 1}},
		{"a bra and a ket", `\bra{\phi}\ket{\psi}`, map[Concept]int{Ket: 1, Bra: 1}},
		{"a bra, an operator and a ket", `\bra{\phi} A \ket{\psi}`, map[Concept]int{Ket: 1, Bra: 1}},
		{"a braket", `\braket{\phi}{\psi}`, map[Concept]int{InnerProduct: 1}},
		{"a braket with phantoms", `\Braket{\phi}{\psi}`, map[Concept]int{InnerProduct: 1}},
		{"a ket beside an absolute value from its own macro", `\ket{\psi} + \absv{x}`,
			map[Concept]int{Ket: 1, AbsoluteValue: 1}},
		{"a bra beside a bare angle", `\bra{\phi} + \langle a,b \rangle`, map[Concept]int{Bra: 1}},
	}
	for _, test := range cases {
		if got := Read(test.tex, diracMacros).Counts; !maps.Equal(got, test.want) {
			t.Errorf("%s: %v, want %v", test.name, got, test.want)
		}
	}
}

func TestDiracRefused(t *testing.T) {
	cases := []struct{ name, tex string }{
		{"a bare ket", `|\psi\rangle`},
		{"a bare bra", `\langle\phi|`},
		{"a ket beside a bare bar", `\ket{\psi} + |x|`},
		{"a macro of another name", `\rawket{\psi}`},
		{"a ket named macro with other text", `\keti{\psi}`},
		{"a macro of no argument", `\braphi`},
		{"a ket of a ket", `\ket{\ket{\psi}}`},
	}
	for _, test := range cases {
		if got := Read(test.tex, diracMacros).Counts; !maps.Equal(got, map[Concept]int{}) {
			t.Errorf("%s: %v", test.name, got)
		}
	}
	if got := Read(`\ket{\psi}`, Macros{}).Counts; !maps.Equal(got, map[Concept]int{}) {
		t.Errorf("without its macro, a ket says nothing: %v", got)
	}
}
