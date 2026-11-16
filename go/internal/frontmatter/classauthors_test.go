package frontmatter

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

const (
	spconfStyle = "\\def\\name#1{\\gdef\\@name{{\\em #1}\\\\}}\n\\def\\address#1{\\gdef\\@address{#1}}\n" +
		"\\def\\twoauthors#1#2#3#4{\\gdef\\@address{}\n\\gdef\\@name{{\\em #1} #2 {\\em #3} #4}}\n"
	wileyClass = "\\LoadClass{article}\n\\renewcommand{\\maketitle}{\n\\huge\n\\@title\n}\n" +
		"\\renewenvironment{author} {\n\\large\n\\emph\n}\n"
)

func styledAuthors(t *testing.T, source, styles string) string {
	t.Helper()
	return marshal(rec.FetchList(NewTexMetadata(source).WithStyles(styles).Project(), "authors"))
}

// spconf's \twoauthors{name}{address}{name}{address} (paper 559): each author keeps the
// address block printed under the name; an address line alone is the email.
func TestSpconfTwoAuthors(t *testing.T) {
	source := "\\documentclass{article}\n\\usepackage{spconf}\n\\title{A Title}\n" +
		"\\twoauthors\n  {Ada Lovelace\\sthanks{Work done elsewhere.}}\n" +
		"  {Analytical Lab\\sthanks{A joint lab.}\\\\ 1 Engine Street, London\\\\ada@example.org}\n" +
		"  {Grace Hopper}\n  {Navy\\\\ Arlington, USA\\\\ grace@example.org}\n" +
		"\\begin{document}\n\\maketitle\n\\begin{abstract}Text.\\end{abstract}\n"
	want := `[{"affiliations":["Analytical Lab","1 Engine Street, London"],"emails":["ada@example.org"],"name":"Ada Lovelace"},` +
		`{"affiliations":["Navy","Arlington, USA"],"emails":["grace@example.org"],"name":"Grace Hopper"}]`
	if got := styledAuthors(t, source, "% \\threeauthors\n"+spconfStyle); got != want {
		t.Errorf("authors %s", got)
	}
	// Without the paper's style defining it, \twoauthors declares nothing (unchanged).
	if got := styledAuthors(t, source, ""); got != "[]" {
		t.Errorf("undefined \\twoauthors read: %s", got)
	}
}

func TestSpconfThreeAuthorsNeedTheirDefinition(t *testing.T) {
	source := "\\documentclass{article}\n\\title{T}\n\\threeauthors{A One}{X}{B Two}{Y}{C Three}{Z}\n\\maketitle\n"
	style := "\\def\\threeauthors#1#2#3#4#5#6{\\gdef\\@name{#1#2#3#4#5#6}}\n"
	want := `[{"affiliations":["X"],"emails":[],"name":"A One"},{"affiliations":["Y"],"emails":[],"name":"B Two"},` +
		`{"affiliations":["Z"],"emails":[],"name":"C Three"}]`
	if got := styledAuthors(t, source, style); got != want {
		t.Errorf("authors %s", got)
	}
	// A definition of another arity is no name/address list.
	if got := styledAuthors(t, source, "\\newcommand{\\threeauthors}[3]{#1 #2 #3}\n"); got != "[]" {
		t.Errorf("authors %s", got)
	}
}

// WileyMSP-template prints \author{Name} where it stands (paper 523: after \maketitle).
func TestAuthorsTheClassPrintsAfterMaketitle(t *testing.T) {
	source := "\\documentclass{wiley}\n\\begin{document}\n\\title{A Title}\n\\maketitle\n" +
		"\\author{Ada Lovelace},\n\\author{Grace Hopper*}\n\\begin{affiliations}Lab\\end{affiliations}\n" +
		"\\begin{abstract}Text.\\end{abstract}\n\\section{Intro}\n\\author{Not An Author}\n"
	want := `[{"affiliations":[],"emails":[],"name":"Ada Lovelace"},{"affiliations":[],"emails":[],"name":"Grace Hopper"}]`
	if got := styledAuthors(t, source, wileyClass); got != want {
		t.Errorf("authors %s", got)
	}
	// Generic LaTeX keeps \author for \maketitle: one given after it prints nothing (unchanged).
	if got := styledAuthors(t, source, ""); got != "[]" {
		t.Errorf("article read a late \\author: %s", got)
	}
	storing := "\\renewcommand{\\author}[1]{\\gdef\\@author{#1}}\n"
	if got := styledAuthors(t, source, storing); got != "[]" {
		t.Errorf("a storing \\author read in place: %s", got)
	}
}

// An ordinary \author before \maketitle is read as before, whatever the styles say.
func TestOrdinaryAuthorsUnchangedByStyles(t *testing.T) {
	source := "\\documentclass{article}\n\\title{T}\n\\author{Ada Lovelace \\and Grace Hopper}\n" +
		"\\twoauthors{X}{Y}{Z}{W}\n\\begin{document}\n\\maketitle\n\\author{Late}\n"
	want := `[{"affiliations":[],"emails":[],"name":"Ada Lovelace"},{"affiliations":[],"emails":[],"name":"Grace Hopper"}]`
	for _, styles := range []string{"", spconfStyle + wileyClass} {
		if got := styledAuthors(t, source, styles); got != want {
			t.Errorf("authors %s", got)
		}
	}
}
