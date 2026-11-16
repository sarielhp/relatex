package mathexport

import "testing"

// The operator names the stores abbreviate most, which a text-to-speech voice mispronounces
// (decision 114, amendment 2026-11-10): each applied name gets U+2061 and a literal intent.
func TestSpeechOperatorNames(t *testing.T) {
	words := []struct{ name, word string }{
		{"Stab", "stabilizer"},
		{"rk", "rank"},
		{"Aut", "automorphism-group"},
		{"dom", "domain"},
		{"argmax", "arg-max"},
		{"argmin", "arg-min"},
		{"im", "image"},
		{"conv", "convex-hull"},
		{"Lip", "lipschitz-constant"},
		{"coker", "cokernel"},
		{"codim", "codimension"},
		{"Hess", "hessian"},
		{"proj", "projection"},
		{"cl", "closure"},
	}
	for _, test := range words {
		body := `<mi>` + test.name + `</mi><mi>X</mi>`
		want := `<mi intent='` + test.word + `'>` + test.name + `</mi><mo>` + functionApplication + `</mo><mi>X</mi>`
		if got := project(t, body); got != want {
			t.Errorf("%s applied:\n got %s\nwant %s", test.name, got, want)
		}
	}
}

func TestSpeechOperatorNamesStayAlone(t *testing.T) {
	cases := []struct{ name, body, want string }{
		// A name in a script has no argument: it is a label, not the operator.
		{"cl as a subscript label", `<msub><mi>𝑇</mi><mrow><mi>cl</mi></mrow></msub>`,
			`<msub><mi>𝑇</mi><mrow><mi>cl</mi></mrow></msub>`},
		// Capital Im stays unmapped: imaginary part or image, ambiguous.
		{"Im is not applied", `<mi>Im</mi><mi>z</mi>`, `<mi>Im</mi><mi>z</mi>`},
		// A name followed by an operator gets neither the application nor the intent.
		{"a name before an operator", `<mi>Stab</mi><mo>+</mo>`, `<mi>Stab</mi><mo>+</mo>`},
	}
	for _, test := range cases {
		if got := project(t, test.body); got != test.want {
			t.Errorf("%s:\n got %s\nwant %s", test.name, got, test.want)
		}
	}
}
