package mathml

import "testing"

func TestSplitEquationTag(t *testing.T) {
	cases := []struct{ name, source, formula, tag string }{
		{"number inside the line box", `<math display="block"><mpadded><mrow><mi>x</mi><mtext>(2)</mtext></mrow></mpadded></math>`,
			`<math display="block"><mpadded><mrow><mi>x</mi></mrow></mpadded></math>`, "(2)"},
		{"number with a space", `<math display="block"><mi>x</mi><mtext> (1.2)</mtext></math>`, `<math display="block"><mi>x</mi></math>`, "(1.2)"},
		{"bracketed number", `<math display="block"><mi>x</mi><mtext>[7]</mtext></math>`, `<math display="block"><mi>x</mi></math>`, "[7]"},
		{"only the number", `<math display="block"><mtext> (1)</mtext></math>`, `<math display="block"><mtext> (1)</mtext></math>`, ""},
		{"inline formula", `<math><mi>x</mi><mtext>(2)</mtext></math>`, `<math><mi>x</mi><mtext>(2)</mtext></math>`, ""},
		{"prose after the formula", `<math display="block"><mi>x</mi><mtext>(see above)</mtext></math>`,
			`<math display="block"><mi>x</mi><mtext>(see above)</mtext></math>`, ""},
		{"number before the formula", `<math display="block"><mtext>(2)</mtext><mi>x</mi></math>`,
			`<math display="block"><mtext>(2)</mtext><mi>x</mi></math>`, ""},
		// amsldoc: an align row numbered by its label cell ends in \text{by \eqref{eq:C}}.
		{"a labelled row's annotation", `<math display="block"><mtable><mtr><mtd intent=":equation-label"><mtext>(3.15)</mtext></mtd>` +
			`<mtd><mi>x</mi><mtext>(3.24)</mtext></mtd></mtr></mtable></math>`,
			`<math display="block"><mtable><mtr><mtd intent=":equation-label"><mtext>(3.15)</mtext></mtd>` +
				`<mtd><mi>x</mi><mtext>(3.24)</mtext></mtd></mtr></mtable></math>`, ""},
	}
	for _, c := range cases {
		formula, tag := SplitEquationTag(c.source)
		if formula != c.formula || tag != c.tag {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", c.name, formula, tag, c.formula, c.tag)
		}
	}
}

func TestSplitEquationTagLeavesAnUnderbraceLabel(t *testing.T) {
	source := `<math display="block"><mrow><munder><mi>x</mi><mpadded><mrow><mtext>(I)</mtext></mrow></mpadded></munder></mrow></math>`
	if formula, tag := SplitEquationTag(source); formula != source || tag != "" {
		t.Errorf("an underbrace label is no number: (%q, %q)", formula, tag)
	}
}

func TestSplitEquationTagReadsAPositionedNumber(t *testing.T) {
	box := func(glyph string) string {
		return `<mpadded depth="0pt" lspace="1pt"><mtext>` + glyph + `</mtext></mpadded>`
	}
	source := `<math display="block"><mrow><mi>x</mi>` + box("(") + box("2") + box(")") + `</mrow></math>`
	formula, tag := SplitEquationTag(source)
	if formula != `<math display="block"><mrow><mi>x</mi></mrow></math>` || tag != "(2)" {
		t.Errorf("got (%q, %q)", formula, tag)
	}
	if formula, tag := SplitEquationTag(`<math display="block"><mrow><mi>x</mi>` + box("a") + box("b") + box("c") + `</mrow></math>`); tag != "" ||
		formula == "" {
		t.Errorf("a word is no number: (%q, %q)", formula, tag)
	}
}

func TestSplitEquationTagSkipsEmptyBoxesAfterAPositionedNumber(t *testing.T) {
	box := func(glyph string) string {
		return `<mpadded depth="0pt" lspace="1pt"><mtext>` + glyph + `</mtext></mpadded>`
	}
	empty := func(width string) string {
		return `<mpadded depth="0pt" width="0pt"><mspace depth="3pt" height="8pt" width="` + width + `"/></mpadded>`
	}
	number := box("(") + box("1") + box(")")
	source := `<math display="block"><mrow><mi>x</mi>` + number + empty("0.000000pt") + `</mrow></math>`
	if formula, tag := SplitEquationTag(source); formula != `<math display="block"><mrow><mi>x</mi></mrow></math>` || tag != "(1)" {
		t.Errorf("got (%q, %q)", formula, tag)
	}
	wide := `<math display="block"><mrow><mi>x</mi>` + number + empty("2.000000pt") + `</mrow></math>`
	if formula, tag := SplitEquationTag(wide); tag != "" || formula != wide {
		t.Errorf("a space after the number keeps it in the formula: (%q, %q)", formula, tag)
	}
}

func TestSplitEquationTagReadsAPositionedNumberRun(t *testing.T) {
	run := `<mpadded depth="0pt" lspace="9pt"><mtext>(4.18)</mtext></mpadded>`
	empty := `<mpadded depth="0pt" width="0pt"><mspace depth="3pt" height="8pt" width="0.000000pt"/></mpadded>`
	source := `<math display="block"><mrow><mi>x</mi>` + run + empty + `</mrow></math>`
	if formula, tag := SplitEquationTag(source); formula != `<math display="block"><mrow><mi>x</mi></mrow></math>` || tag != "(4.18)" {
		t.Errorf("got (%q, %q)", formula, tag)
	}
	words := `<math display="block"><mrow><mi>x</mi><mpadded lspace="9pt"><mtext>for all</mtext></mpadded></mrow></math>`
	if formula, tag := SplitEquationTag(words); tag != "" || formula != words {
		t.Errorf("words are no number: (%q, %q)", formula, tag)
	}
}
