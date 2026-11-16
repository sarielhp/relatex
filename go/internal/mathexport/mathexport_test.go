package mathexport

import "testing"

// The expectations are Ruby's own results (format('%.Ng'), BrowserMathML.project,
// TeXMathExport and TypstMathExport).
func TestFormatGMatchesRuby(t *testing.T) {
	cases := []struct {
		value     float64
		precision int
		want      string
	}{{0.1 + 0.2, 10, "0.3"}, {1e20, 10, "1e+20"}, {-0.000012345678912, 10, "-1.234567891e-05"},
		{123456789.5, 8, "1.2345679e+08"}, {5e-5, 10, "5e-05"}}
	for _, item := range cases {
		if got := FormatG(item.value, item.precision); got != item.want {
			t.Errorf("FormatG(%v, %d) = %q, want %q", item.value, item.precision, got, item.want)
		}
	}
}

func TestProjectionMatchesRuby(t *testing.T) {
	source := `<math xmlns="http://www.w3.org/1998/Math/MathML" display="block"><mi mathvariant="bold">x</mi><mo>&lt;</mo>` +
		`<mspace width="-0.5em"/></math>`
	// The bold x is a character of its own: mathvariant="normal" would change nothing and goes (decision 100).
	want := "<math display='block' xmlns='http://www.w3.org/1998/Math/MathML'><mi>𝐱</mi><mo>&lt;</mo>" +
		"<mspace style='margin-inline-start:-0.5em' width='0pt'/></math>"
	if got := Project(ParsePaper(source)); got != want {
		t.Errorf("Project = %q, want %q", got, want)
	}
}

func TestConversionsMatchRuby(t *testing.T) {
	fraction := `<math xmlns="http://www.w3.org/1998/Math/MathML"><mfrac><mi>a</mi><mn>2</mn></mfrac></math>`
	if got := NewTeX("tex").Convert(fraction); got != `\frac{a}{2}` {
		t.Errorf("TeX = %q", got)
	}
	power := `<math xmlns="http://www.w3.org/1998/Math/MathML"><msup><mi>x</mi><mn>2</mn></msup></math>`
	if got := NewTypst().Convert(power); got != "scripts(x)^(2)" {
		t.Errorf("Typst = %q", got)
	}
}
