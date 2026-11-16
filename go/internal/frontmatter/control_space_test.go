package frontmatter

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// An author block that separates its names from the affiliations with \\ \ \\ (paper 162):
// the control space alone on its line prints a space, not an unreadable macro.
func TestAuthorBlockWithALoneControlSpaceLine(t *testing.T) {
	source := "\\documentclass{article}\n\\begin{document}\n\\author{\nC. Vlachou$^{1}$ and \nA. Souto$^{2}$\n\\\\ \\ \\\\\n" +
		"${}^1$ SQIG\\\\\n${}^2$ LaSige}\n\\title{Quantum walks}\n\\maketitle\n\\end{document}\n"
	projected := NewTexMetadata(source).Project()
	authors := rec.FetchList(projected, "authors")
	if len(authors) != 2 || rec.FetchString(rec.AsMap(authors[0]), "name") != "C. Vlachou" {
		t.Fatalf("authors = %v", projected)
	}
}

// \s inside a character class is Ruby's whitespace characters, not a nested class: an author
// mark after a name ($^{1,2}$) and an email in parentheses are found.
func TestRubyWhitespaceInsideACharacterClass(t *testing.T) {
	if !authorMarkLine.MatchString(`N. Paunkovi\'c$^{1,2,6}$ and`) {
		t.Fatal("author mark line not found")
	}
	if !emailPattern.MatchString("(a.b@example.org)") || rx(`[^}\s]x`).MatchString(" x") {
		t.Fatal("whitespace class inside brackets")
	}
}
