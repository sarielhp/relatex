package mathexport

import "testing"

// The text alternative is a linear form of the MathML (decision 088).
func TestAltTextIsALinearForm(t *testing.T) {
	const namespace = `<math xmlns="http://www.w3.org/1998/Math/MathML">`
	cases := map[string]string{
		`<mi>𝑁</mi><mo>=</mo><mn>50</mn>`:                                                                                  "N=50",
		`<msubsup><mi>𝑥</mi><mi>𝑖</mi><mn>2</mn></msubsup>`:                                                                "x_i^2",
		`<mfrac><mrow><mi>𝑎</mi><mo>+</mo><mn>1</mn></mrow><mi>𝑏</mi></mfrac>`:                                             "(a+1)/b",
		`<mfrac><mrow><mn>10</mn></mrow><mn>3</mn></mfrac>`:                                                                "10/3",
		`<msqrt><mi>𝛼</mi></msqrt><mspace width="1em"/><mroot><mi>𝑥</mi><mn>3</mn></mroot>`:                                "√(α) √[3](x)",
		`<munderover><mo>∑</mo><mrow><mi>𝑖</mi><mo>=</mo><mn>1</mn></mrow><mi>𝑛</mi></munderover>`:                         "∑_(i=1)^n",
		`<mtable><mtr><mtd><mi>𝑎</mi></mtd><mtd><mo>=</mo><mi>𝑏</mi></mtd></mtr><mtr><mtd><mn>1</mn></mtd></mtr></mtable>`: "a =b; 1",
		"<mi>sin</mi><mo>\u2061</mo><mi>ℎ</mi><mphantom><mi>𝑦</mi></mphantom><mi>𝐱</mi>":                                   "sinh𝐱",
		`<mtext>if  </mtext><mi>𝛤</mi><mi>𝜕</mi>`:                                                                          "if Γ∂",
	}
	for body, want := range cases {
		if got := AltText(ParsePaper(namespace + body + "</math>")); got != want {
			t.Errorf("%s: %q, want %q", body, got, want)
		}
	}
}
