package mathexport

import (
	"strings"
	"testing"
)

// piece is a text piece as the collector's placed() writes it: an mpadded of zero size pinned at
// its x offset, holding one mtext.
func piece(offset, text string) string {
	return `<mpadded depth='0pt' height='0pt' lspace='` + offset + `' voffset='0.000000pt' width='0pt'><mtext>` +
		text + `</mtext></mpadded>`
}

// textBox is the box the collector writes around \text: its size, and a row of placed pieces.
func textBox(pieces ...string) string {
	return `<mpadded depth='0.099632pt' height='7.471980pt' width='42.719821pt'><mrow>` +
		strings.Join(pieces, "") + `</mrow></mpadded>`
}

// TestJoinTextRuns: a word the collector split at a font kern is one mtext again (sample 81:
// \text{otherwise} read "other wise"); a word space and an explicit \kern are a space inside one
// piece and stay, and nothing joins across an atom, a box or a space. The offsets are the
// collector's own: 27.365382pt is Roboto's r-w kern at 12pt (the sample), 22.804498pt at 10pt.
func TestJoinTextRuns(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"a kerned word is one mtext", textBox(piece("0.000000pt", "other"), piece("27.365382pt", "wise")),
			textBox(piece("0.000000pt", "otherwise"))},
		{"a kerned word at another size is one mtext", textBox(piece("0.000000pt", "other"), piece("22.804498pt", "wise")),
			textBox(piece("0.000000pt", "otherwise"))},
		{"a \\quad gap keeps two words apart", textBox(piece("0.000000pt", "other"), piece("39.365382pt", "wise")),
			textBox(piece("0.000000pt", "other"), piece("39.365382pt", "wise"))},
		{"a \\ gap keeps two words apart", textBox(piece("0.000000pt", "other"), piece("30.365382pt", "wise")),
			textBox(piece("0.000000pt", "other"), piece("30.365382pt", "wise"))},
		{"a word before a bracket stays apart (books_0001)",
			textBox(piece("0.000000pt", "by"), piece("16.605727pt", "(3.24)")),
			textBox(piece("0.000000pt", "by"), piece("16.605727pt", "(3.24)"))},
		{"a bracket against a word stays apart even with no gap",
			textBox(piece("0.000000pt", "Output"), piece("0.000000pt", "(Atari")),
			textBox(piece("0.000000pt", "Output"), piece("0.000000pt", "(Atari"))},
		{"a number and a dash join", textBox(piece("0.000000pt", "9"), piece("4.764300pt", "–"), piece("9.528599pt", "11")),
			textBox(piece("0.000000pt", "9–11"))},
		{"a word space is a space inside one piece", textBox(piece("0.000000pt", "two words")),
			textBox(piece("0.000000pt", "two words"))},
		{"an explicit kern is a space inside one piece", textBox(piece("0.000000pt", "a b")),
			textBox(piece("0.000000pt", "a b"))},
		{"a space at the boundary keeps two pieces", textBox(piece("0.000000pt", "other "), piece("27.365382pt", "wise")),
			textBox(piece("0.000000pt", "other "), piece("27.365382pt", "wise"))},
		{"a no-break space at the boundary keeps two pieces",
			textBox(piece("0.000000pt", "other\u00A0"), piece("27.365382pt", "wise")),
			textBox(piece("0.000000pt", "other\u00A0"), piece("27.365382pt", "wise"))},
		{"a piece of spaces keeps the pieces apart",
			textBox(piece("0.000000pt", "if"), piece("4.000000pt", " "), piece("9.000000pt", "x")),
			textBox(piece("0.000000pt", "if"), piece("4.000000pt", " "), piece("9.000000pt", "x"))},
		{"an atom between the pieces keeps them apart",
			textBox(piece("0.000000pt", "other"), `<mi>x</mi>`, piece("22.804498pt", "wise")),
			textBox(piece("0.000000pt", "other"), `<mi>x</mi>`, piece("22.804498pt", "wise"))},
		{"pieces of two boxes do not join",
			textBox(piece("0.000000pt", "a")) + `<mspace width='9.963pt'/>` + textBox(piece("0.000000pt", "b")),
			textBox(piece("0.000000pt", "a")) + `<mspace width='9.963pt'/>` + textBox(piece("0.000000pt", "b"))},
		{"pieces outside a box row do not join", `<mrow>` + piece("0pt", "other") + piece("22pt", "wise") + `</mrow>`,
			`<mrow>` + piece("0pt", "other") + piece("22pt", "wise") + `</mrow>`},
		{"a sized box holding text is no placed piece",
			textBox(`<mpadded depth='0pt' height='1pt' width='5pt'><mtext>a</mtext></mpadded>` +
				`<mpadded depth='0pt' height='1pt' width='5pt'><mtext>b</mtext></mpadded>`),
			textBox(`<mpadded depth='0pt' height='1pt' width='5pt'><mtext>a</mtext></mpadded>` +
				`<mpadded depth='0pt' height='1pt' width='5pt'><mtext>b</mtext></mpadded>`)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := project(t, testCase.body); got != testCase.want {
				t.Errorf("\n got %s\nwant %s", got, testCase.want)
			}
		})
	}
}
