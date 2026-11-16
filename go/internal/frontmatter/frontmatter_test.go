package frontmatter

import (
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// The expectations are Ruby's PaperTexMetadata results for the same sources.
func TestTexMetadataProject(t *testing.T) {
	cases := []struct{ front, title string }{
		{`\title{A \emph{good} title\thanks{x}}`, "A good title"},
		{`\title{\MakeUppercase{straße} and \MakeLowercase{ΣΑΣ İ}}`, "STRASSE and σασ i̇"},
		{`\title{\oe\ae\ss{} \aa x \AA\o y \L\l}`, "œæß åx Åøy Łł"},
		{`\title{Ens \ensuremath{^{1}} \ensuremath{\beta}-ray Ce$_{17}$}`, "Ens β-ray Ce₁₇"},
		{"\\title{Dash --- and -- ``quoted'' `x'}", "Dash — and – “quoted” ‘x’"},
		{`\title{\i\j \u{g} \v s \c{c} \={a} \.{z}}`, "ijğ š ç ā ż"},
	}
	for _, item := range cases {
		source := "\\documentclass{article}\n" + item.front + " \\author{A}\n\\begin{abstract}Text.\\end{abstract}\n\\maketitle\n"
		if got := NewTexMetadata(source).Project()["title"]; got != item.title {
			t.Errorf("%s: title %q, want %q", item.front, got, item.title)
		}
	}
}

func TestUnicodeWordBoundary(t *testing.T) {
	// Ruby's \b treats é as a word character: \thanksé is not \thanks, and stays unsupported.
	source := "\\documentclass{article}\n\\title{Th\\thanksé x} \\author{A}\n\\maketitle\n"
	message := failure(func() { NewTexMetadata(source).Project() })
	if message != `Unsupported macro in literal ACM metadata: \thanks` {
		t.Errorf("failure %q", message)
	}
}

// A footnote mark beside a name (paper 41's \author{... Marcus Plank$^{2}$*}) types no
// character of the name, as Ruby's without_author_marks.
func TestAuthorNoteMarksAreNotPartOfAName(t *testing.T) {
	source := "\\documentclass{article}\n\\title{X}\\author{Ada Lovelace$^{1}$, Marcus Plank$^{2}$*, Grace Hopper†}\n\\maketitle\n"
	authors := rec.FetchList(NewTexMetadata(source).Project(), "authors")
	want := `[{"affiliations":[],"emails":[],"name":"Ada Lovelace"},{"affiliations":[],"emails":[],"name":"Marcus Plank"},{"affiliations":[],"emails":[],"name":"Grace Hopper"}]`
	if got := marshal(authors); got != want {
		t.Errorf("authors %s", got)
	}
}

// A typeset author mark (paper 73's \textbf{Name}\textsuperscript{\dag}) marks a name line,
// and a line opening with the mark lists affiliations, not names.
func TestTypesetAuthorMarksNameLines(t *testing.T) {
	source := "\\documentclass{article}\n\\title{X}\n\\author{%\n\\textbf{Ann}\\textsuperscript{\\dag}, " +
		"\\textbf{Bob}\\textsuperscript{\\dag} \\\\\n\\textbf{Cid}\\textsuperscript{\\dag}, " +
		"\\textbf{Dan}\\textsuperscript{\\dag} \\\\\n\\textsuperscript{\\dag}Institute \\quad " +
		"\\textsuperscript{\\S}Lab\n}\n\\maketitle\n"
	authors := rec.FetchList(NewTexMetadata(source).Project(), "authors")
	want := `[{"affiliations":["Institute , Lab"],"emails":[],"name":"Ann"},` +
		`{"affiliations":["Institute , Lab"],"emails":[],"name":"Bob"},` +
		`{"affiliations":["Institute , Lab"],"emails":[],"name":"Cid"},` +
		`{"affiliations":["Institute , Lab"],"emails":[],"name":"Dan"}]`
	if got := marshal(authors); got != want {
		t.Errorf("authors %s", got)
	}
}

func TestJMLRAuthors(t *testing.T) {
	source := "\\documentclass{article}\n\\title{X}\\author{\\name Ann \\email ann@x.org \\\\ \\addr Univ \\AND \\name Bob \\email bob@y.org \\\\ Lab}\n\\maketitle\n"
	authors := rec.FetchList(NewTexMetadata(source).Project(), "authors")
	want := `[{"affiliations":["Univ"],"emails":["ann@x.org"],"name":"Ann"},{"affiliations":["Lab"],"emails":["bob@y.org"],"name":"Bob"}]`
	if got := marshal(authors); got != want {
		t.Errorf("authors %s", got)
	}
}

// A contact address list after the names prints under them; it is their address line,
// not three more authors.
func TestAMSAuthorContactListIsNotAName(t *testing.T) {
	source := "\\documentclass{amsart}\n\\begin{document}\n\\author[Short, Names]{%\nReggie Brandt,\nNate Rowe,\n" +
		"Julian Wagner\\\\\n\\MakeLowercase{\\{rbrandt, nrowe, jwagner\\}@example.org\n}}\n\\title{T}\n" +
		"\\maketitle\n\\begin{abstract}Text.\\end{abstract}\n\\end{document}\n"
	authors := rec.FetchList(NewTexMetadata(source).Project(), "authors")
	want := `[{"affiliations":["{rbrandt, nrowe, jwagner}@example.org"],"emails":[],"name":"Reggie Brandt"},` +
		`{"affiliations":["{rbrandt, nrowe, jwagner}@example.org"],"emails":[],"name":"Nate Rowe"},` +
		`{"affiliations":["{rbrandt, nrowe, jwagner}@example.org"],"emails":[],"name":"Julian Wagner"}]`
	if got := marshal(authors); got != want {
		t.Errorf("authors %s", got)
	}
}

// An IEEE author block prints its address line with the email under the name: the email
// belongs to the author it follows.
func TestIEEEAuthorBlockEmail(t *testing.T) {
	source := "\\documentclass{IEEEtran}\n\\title{T}\n\\author{\n\\IEEEauthorblockN{Boris Brandt}\n" +
		"\\IEEEauthorblockA{\\textit{Lab} \\\\\nb.brandt@example.org}\n\\and\n\\IEEEauthorblockN{Ann Other}\n" +
		"\\IEEEauthorblockA{\\textit{Dept} \\\\\nann@example.org}\n}\n\\maketitle\n"
	authors := rec.FetchList(NewTexMetadata(source).Project(), "authors")
	want := `[{"affiliations":[],"emails":["b.brandt@example.org"],"name":"Boris Brandt"},` +
		`{"affiliations":[],"emails":["ann@example.org"],"name":"Ann Other"}]`
	if got := marshal(authors); got != want {
		t.Errorf("authors %s", got)
	}
}

// A \thanks footnote beside a name prints the author's address, email included; the email
// is the author's, the note is not name text.
func TestAuthorThanksEmail(t *testing.T) {
	source := "\\documentclass{article}\n\\title{T}\n\\author{Ada Lovelace\\thanks{Lab. Email: {\\tt ada@example.org}.}" +
		"\\and Bob\\thanks{Dept. Email: {\\tt bob@example.org}.}}\n\\maketitle\n"
	authors := rec.FetchList(NewTexMetadata(source).Project(), "authors")
	want := `[{"affiliations":[],"emails":["ada@example.org"],"name":"Ada Lovelace"},` +
		`{"affiliations":[],"emails":["bob@example.org"],"name":"Bob"}]`
	if got := marshal(authors); got != want {
		t.Errorf("authors %s", got)
	}
}

// A preamble \author is overridden by the body's declaration; only the body sets authors.
func TestBodyAuthorOverridesPreamble(t *testing.T) {
	source := "\\documentclass{article}\n\\author{Preamble Name}\n\\begin{document}\n" +
		"\\title{A Title}\n\\author{Body Name}\n\\maketitle\n\\end{document}\n"
	authors := rec.FetchList(NewTexMetadata(source).Project(), "authors")
	want := `[{"affiliations":[],"emails":[],"name":"Body Name"}]`
	if got := marshal(authors); got != want {
		t.Errorf("authors %s", got)
	}
}

// An \email command after an AMS \author row types the address of every author that row
// lists (Ruby shares that line's email list), while an \address attaches to the last.
func TestAMSEmailRowsBelongToEveryAuthorOfTheLine(t *testing.T) {
	source := "\\documentclass{amsart}\n\\begin{document}\n\\author{Ada Lovelace, Bob Smith}\n" +
		"\\address{Lab}\n\\email{lab@example.org}\n\\title{T}\n\\maketitle\n" +
		"\\begin{abstract}Text.\\end{abstract}\n\\end{document}\n"
	authors := rec.FetchList(NewTexMetadata(source).Project(), "authors")
	want := `[{"affiliations":[],"emails":["lab@example.org"],"name":"Ada Lovelace"},` +
		`{"affiliations":["Lab"],"emails":["lab@example.org"],"name":"Bob Smith"}]`
	if got := marshal(authors); got != want {
		t.Errorf("authors %s", got)
	}
}

// The amsmath manuals: the paper's class is its first \documentclass (amsldoc quotes amsart in
// a verbatim example), \pkg{x} is {\protect\ntt x} with \ntt a family switch, and \LaTeX\
// keeps its control space.
func TestManualFrontMatter(t *testing.T) {
	source := "\\documentclass{book}\n\\newcommand{\\ntt}{\\normalfont\\ttfamily}\n" +
		"\\newcommand{\\pkg}[1]{{\\protect\\ntt#1}}\n\\title{Guide for the \\pkg{amsmath} Package}\n" +
		"\\author{AMS, \\LaTeX\\ Project}\n\\begin{document}\\maketitle\n" +
		"\\begin{verbatim}\n\\documentclass{amsart}\n\\end{verbatim}\n\\end{document}\n"
	metadata := NewTexMetadata(source).Project()
	want := `{"authors":[{"affiliations":[],"emails":[],"name":"AMS"},{"affiliations":[],"emails":[],"name":"LaTeX Project"}],` +
		`"title":"Guide for the amsmath Package"}`
	if got := marshal(metadata); got != want {
		t.Errorf("metadata %s", got)
	}
}

// An author set in a tabular prints its names in columns: the environment, its optional
// position and column arguments print nothing, a cell boundary (&) separates names like a
// comma, and \\ ends a line.
func TestTabularAuthorBlockNames(t *testing.T) {
	cases := []struct {
		name, author string
		want         []string
	}{
		{"optional position and columns", `\begin{tabular}[t]{cc}` +
			"\nPaul Hartner \\quad Stuart Phelps & Ron Schaper\\\\\nAT\\&T Labs & Fairview University\\\\\n" +
			`\end{tabular}`, []string{"Paul Hartner", "Stuart Phelps", "Ron Schaper"}},
		{"columns argument", `\begin{tabular}{cc} Ann & Bob \\ Lab & University \end{tabular}`,
			[]string{"Ann", "Bob"}},
		{"cell boundary separates names", `\begin{tabular}[t]{cc} Ann & Bob \end{tabular}`, []string{"Ann", "Bob"}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			source := "\\documentclass{article}\n\\title{X}\n\\author{" + item.author + "}\n\\maketitle\n"
			authors := rec.FetchList(NewTexMetadata(source).Project(), "authors")
			got := []string{}
			for _, author := range authors {
				got = append(got, author.(rec.Map)["name"].(string))
			}
			if !slices.Equal(got, item.want) {
				t.Errorf("names %q, want %q", got, item.want)
			}
		})
	}
}

// A macro the reader cannot read is still refused inside a tabular author block.
func TestTabularAuthorBlockUnsupportedMacroStillFails(t *testing.T) {
	source := "\\documentclass{article}\n\\title{X}\n\\author{\\begin{tabular}[t]{cc} Ann & \\unknownmacro{Bob} " +
		"\\end{tabular}}\n\\maketitle\n"
	message := failure(func() { NewTexMetadata(source).Project() })
	if message != `Unsupported macro in literal ACM metadata: \unknownmacro` {
		t.Errorf("failure %q", message)
	}
}

func TestValidateWithoutFrontmatter(_ *testing.T) {
	Validate(rec.Map{})
}

func failure(fn func()) (message string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message = recovered.(rec.Failure).Message
		}
	}()
	fn()
	return ""
}

// A typed en dash before a hyphen is one em dash in the PDF: the 8-bit font's ligature program.
func TestTypedEnDashHyphenLigature(t *testing.T) {
	text := NewTexMetadata("\\documentclass{article}\n\\begin{document}\n\\end{document}\n").LiteralText("speedup –- we")
	if text != "speedup — we" {
		t.Errorf("text %q", text)
	}
}
