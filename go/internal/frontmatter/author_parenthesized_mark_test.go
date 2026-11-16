package frontmatter

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A name's affiliation mark may hold parenthesized numbers or a letter ($^{(1)}$,
// $^{(a)}$), as paper 245's \author{Rod Chase$^{(1)}$} does. Without it the second line of
// the author block was read as an affiliation and the names were glued to it.
func TestParenthesizedAuthorMarksNameLines(t *testing.T) {
	cases := []struct{ name, mark string }{
		{"plain digits", `A. Alpha$^{1}$`},
		{"digits and commas", `A. Alpha$^{1,2}$`},
		{"star", `A. Alpha$^*$`},
		{"parenthesized digits", `A. Alpha$^{(1)}$`},
		{"parenthesized letter", `A. Alpha$^{(a)}$`},
	}
	for _, item := range cases {
		source := "\\documentclass{article}\n\\title{X}\\author{" + item.mark + "}\n\\maketitle\n"
		if names := authorNames(NewTexMetadata(source).Project()); !sameStrings(names, []string{"A. Alpha"}) {
			t.Errorf("%s: names = %v", item.name, names)
		}
	}
}

// Paper 245 breaks the author list with \\ between two names that both carry $^{(n)}$ marks;
// every line is a name line, so no affiliation is read from the author block.
func TestParenthesizedMarksKeepALineBreakBetweenNames(t *testing.T) {
	source := "\\documentclass{article}\n\\title{X}\n" +
		"\\author{Rod Chase$^{(1)}$, Tomas Benson$^{(2)}$,\\\\ Gunther Hallmann$^{(1)}$, Robin Kleist$^{(1)}$}\n" +
		"\\maketitle\n"
	projected := NewTexMetadata(source).Project()
	names := authorNames(projected)
	if !sameStrings(names, []string{"Rod Chase", "Tomas Benson", "Gunther Hallmann", "Robin Kleist"}) {
		t.Fatalf("names = %v", names)
	}
	for _, author := range rec.FetchList(projected, "authors") {
		if affiliations := rec.FetchList(rec.AsMap(author), "affiliations"); len(affiliations) != 0 {
			t.Fatalf("affiliations = %v", affiliations)
		}
	}
}

// An unknown macro in an author is still refused with the same message.
func TestParenthesizedMarkKeepsUnknownMacroRefused(t *testing.T) {
	source := "\\documentclass{article}\n\\title{X}\\author{A. \\unknownmacro{B}}\n\\maketitle\n"
	message := failure(func() { NewTexMetadata(source).Project() })
	if message != `Unsupported macro in literal ACM metadata: \unknownmacro` {
		t.Errorf("failure %q", message)
	}
}

// A line whose math holds a longer parenthesized superscript ($C^{(ab)}$) is no name line:
// it stays an affiliation of the author above it.
func TestLongParenthesizedSuperscriptKeepsAnAffiliationLine(t *testing.T) {
	source := "\\documentclass{article}\n\\title{X}\n" +
		"\\author{Ann Author$^{1}$\\\\\nInstitute of $C^{(ab)}$ Studies, City}\n\\maketitle\n"
	projected := NewTexMetadata(source).Project()
	if names := authorNames(projected); !sameStrings(names, []string{"Ann Author"}) {
		t.Fatalf("names = %v", names)
	}
	authors := rec.FetchList(projected, "authors")
	if len(authors) != 1 || len(rec.FetchList(rec.AsMap(authors[0]), "affiliations")) != 1 {
		t.Fatalf("authors = %v", authors)
	}
}
