package notes

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestBlankMark(t *testing.T) {
	cases := map[string]bool{"": true, `\@thefnmark`: true, ` \relax `: true, `\protect $\relax $`: true,
		`{}`: true, `\relaxed`: false, `1`: false, `\protect\dagger`: false, `$*$`: false}
	for number, want := range cases {
		if got := blankMark(number); got != want {
			t.Errorf("blankMark(%q) = %v, want %v", number, got, want)
		}
	}
}

func TestMathMLText(t *testing.T) {
	mathml := `<?xml version="1.0"?><math xmlns="http://www.w3.org/1998/Math/MathML"><msup><mi>x</mi>` +
		`<mo>&#x2217;</mo></msup><!-- c --><mtext><![CDATA[a b]]>&amp;</mtext></math>`
	if got := RemoveSpace(MathMLText(mathml)); got != "x∗ab&" {
		t.Errorf("MathMLText = %q", got)
	}
	if got := RemoveSpace("a b c\td"); got != "abcd" {
		t.Errorf("RemoveSpace = %q", got)
	}
}

// A note reports its printed mark: raw TeX resolves, plain numbers stay as recorded.
func TestPrintNumber(t *testing.T) {
	cases := map[string]string{`\protect \mbox {{$\protect \mbox {${^{\star }}$}\relax $}}`: "⋆", "3": "3", `\@thefnmark `: ""}
	for number, want := range cases {
		data := rec.Map{"number": number}
		if printNumber(data); data["number"] != want {
			t.Errorf("printNumber(%q) = %v, want %q", number, data["number"], want)
		}
	}
	data := rec.Map{}
	if printNumber(data); len(data) != 0 {
		t.Errorf("a note without a number gained one: %v", data)
	}
}
