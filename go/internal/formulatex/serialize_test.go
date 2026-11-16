package formulatex

import (
	"strings"
	"testing"
)

const mathNS = `<math xmlns="http://www.w3.org/1998/Math/MathML"`

func inline(body string) string { return mathNS + ">" + body + "</math>" }

func TestWriteTokensAndScripts(t *testing.T) {
	cases := map[string]string{
		`<mi>𝑥</mi><mo>+</mo><mn>1</mn>`:                                     "x+1",
		`<msub><mi>𝑥</mi><mi>𝑖</mi></msub>`:                                  "x_i",
		`<msup><mi>𝜎</mi><mo>∗</mo></msup>`:                                  "\\sigma^{*}",
		`<msup><mi>𝑓</mi><mo>′</mo></msup>`:                                  "f'",
		`<msub><mi>𝑥</mi><mrow><mi>𝑖</mi><mo>+</mo><mn>1</mn></mrow></msub>`: "x_{i+1}",
		`<mfrac><mn>1</mn><mn>2</mn></mfrac>`:                                "\\frac{1}{2}",
		`<msqrt><mi>𝑥</mi></msqrt>`:                                          "\\sqrt{x}",
		`<mi mathvariant="double-struck">R</mi>`:                             "\\mathbb{R}",
		`<mi>ℝ</mi>`:                                                         "\\mathbb{R}",
		`<mover><mi>𝑥</mi><mo>^</mo></mover>`:                                "\\hat{x}",
	}
	for body, want := range cases {
		formula, err := Write(inline(body))
		if err != nil || formula.Text != want {
			t.Errorf("Write(%s) = %q, %v; want %q", body, formula.Text, err, want)
		}
	}
}

func TestWriteOperatorLimits(t *testing.T) {
	body := `<munderover><mo movablelimits="false">∫</mo><mn>0</mn><mi>𝑡</mi></munderover><mi>𝑓</mi>`
	formula, err := Write(inline(body))
	if err != nil || formula.Text != "\\int\\limits_0^tf" {
		t.Errorf("Write = %q, %v", formula.Text, err)
	}
}

func TestWriteFencesAndOperatorNames(t *testing.T) {
	body := `<mrow><mi mathvariant="normal">s</mi><mi mathvariant="normal">i</mi><mi mathvariant="normal">n</mi></mrow>` +
		`<mspace width="0.167em"/><mrow><mo fence="true">(</mo><mi>𝑥</mi><mo fence="true">)</mo></mrow>`
	formula, err := Write(inline(body))
	if err != nil || formula.Text != "\\sin\\left(x\\right)" {
		t.Errorf("Write = %q, %v", formula.Text, err)
	}
}

func TestWriteAlignWithTag(t *testing.T) {
	mathml := mathNS + ` display="block"><mtable class="align"><mtr>` +
		`<mtd intent=":equation-label"><mtext>(3)</mtext></mtd>` +
		`<mtd columnalign="right"><mrow><mi>𝑥</mi></mrow></mtd>` +
		`<mtd columnalign="left"><mrow><mstyle displaystyle="true" scriptlevel="0"><mrow><mo>=</mo><mn>1</mn></mrow></mstyle></mrow></mtd>` +
		`</mtr></mtable></math>`
	formula, err := Write(mathml)
	if err != nil {
		t.Fatal(err)
	}
	if formula.Environment != "align*" || !strings.Contains(formula.Text, "x & =1") || !strings.Contains(formula.Text, "\\tag{3}") {
		t.Errorf("Write = %+v", formula)
	}
	if standalone := formula.Standalone(); !strings.HasPrefix(standalone, "\\begin{align*}") {
		t.Errorf("Standalone = %q", standalone)
	}
}

func TestWriteRaisedAnchor(t *testing.T) {
	body := `<mpadded depth="0pt" height="0pt" width="0pt"><mrow><mpadded voffset="12.5pt" width="0pt">` +
		`<mspace depth="0pt" height="0pt" width="0pt"/></mpadded></mrow></mpadded><mi>𝑥</mi>`
	formula, err := Write(inline(body))
	if err != nil || formula.Text != "\\smash{\\raisebox{12.5bp}{}}x" {
		t.Errorf("Write = %q, %v", formula.Text, err)
	}
}

func TestWriteUnsupportedIsAClass(t *testing.T) {
	_, err := Write(inline(`<mpadded><mfrac><mn>1</mn><mn>2</mn></mfrac></mpadded>`))
	if err == nil || !strings.HasPrefix(err.Error(), "mpadded around") {
		t.Errorf("err = %v", err)
	}
}

func TestOneGroup(t *testing.T) {
	cases := map[string]bool{"{a b}": true, "{a}{b}": false, "{\\{}": true, "a": false, "{}": true}
	for text, want := range cases {
		if got := oneGroup(text); got != want {
			t.Errorf("oneGroup(%q) = %v", text, got)
		}
	}
	if got := group("{a b}"); got != "{a b}" {
		t.Errorf("group = %q", got)
	}
}
