// Package formulatex is each formula's TeX written from its MathML (decision 141): standard LaTeX
// (amsmath, amssymb and bm, no author macros), proven by typesetting it again without the author's
// preamble and getting the same MathML. A formula whose TeX does not round-trip gets none.
package formulatex

// Preamble is the round trip's whole preamble: the formula TeX may use nothing else.
const Preamble = "\\documentclass{article}\n\\usepackage{amsmath,amssymb,bm}\n"

// symbolKey is a token element: its name, text and mathvariant.
type symbolKey struct{ element, text, variant string }

// Formula is a formula's TeX: its body, and the display environment that holds it ("" for an
// inline formula or a plain display).
type Formula struct {
	Text        string
	Environment string
	Display     bool
}

// Standalone is the formula as a reader pastes it into a LaTeX document's math: an environment
// with its \begin and \end, else the body alone.
func (f Formula) Standalone() string {
	if f.Environment == "" {
		return f.Text
	}
	return "\\begin{" + f.Environment + "}\n" + f.Text + "\n\\end{" + f.Environment + "}"
}

// candidateSource is the formula as the round trip typesets it. Each candidate ends a paragraph
// (TeX stops after 100 errors within one), and an inline formula is boxed, so no pages are built.
func candidateSource(formula Formula) string {
	switch {
	case !formula.Display:
		return "\\leavevmode\\setbox0\\hbox{$" + formula.Text + "$}\\par\n"
	case formula.Environment != "":
		return formula.Standalone() + "\\par\n"
	default:
		return "\\[\n" + formula.Text + "\n\\]\\par\n"
	}
}
