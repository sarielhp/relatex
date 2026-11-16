package mathml

import "testing"

// A tag typesetting math (\tag{by $x^2$}) keeps its formula in the label cell; the label's text is
// its text runs and the formula's glyphs, the edge no-break space reading as a space. Other math
// in a label cell (luamml's own tags) is not read, as before.
func TestLabelTextReadsTagFormulas(t *testing.T) {
	root := ParsePaper(`<math xmlns="http://www.w3.org/1998/Math/MathML"><mtable class="align"><mtr><mtd intent=":equation-label"><mtext>(by` + "\u00a0" +
		`</mtext><mrow class="tag-formula"><msup><mi>𝑥</mi><mn>2</mn></msup></mrow><mtext>)</mtext></mtd>` +
		`<mtd><mi>a</mi></mtd></mtr><mtr><mtd intent=":equation-label"><mtext>(1</mtext><mo>′</mo><mtext>)</mtext></mtd>` +
		`<mtd><mi>b</mi></mtd></mtr></mtable></math>`)
	if got := LabelText(root.Row(1).Elements()[0]); got != "(by 𝑥2)" {
		t.Errorf("label text %q, want (by 𝑥2)", got)
	}
	if got := LabelText(root.Row(2).Elements()[0]); got != "(1)" {
		t.Errorf("label text %q, want (1)", got)
	}
}
