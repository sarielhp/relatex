package frontmatter

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func projectArticle(t *testing.T, preamble, body string) rec.Map {
	t.Helper()
	source := "\\documentclass{article}\n" + preamble + "\\begin{document}\n" + body + "\n\\end{document}\n"
	return NewTexMetadata(source).Project()
}

func authorNames(projected rec.Map) []string {
	names := []string{}
	for _, author := range rec.FetchList(projected, "authors") {
		names = append(names, rec.FetchString(rec.AsMap(author), "name"))
	}
	return names
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// Spacing and counter commands print nothing; \linebreak takes its optional priority.
func TestSpacingAndCounterCommandsPrintNothing(t *testing.T) {
	projected := projectArticle(t, "", "\\title{Origin of the Blueshift from \\linebreak[3] Pioneer 10}\n"+
		"\\author{Anders Moritz\\thanks{Harbor.} \\and\n\\addtocounter{footnote}{-1}\nSven Zorn\\footnotemark}\n"+
		"\\maketitle\n\\begin{abstract}\nFirst part.\n\\indent Second part.\n\\end{abstract}")
	if title := rec.FetchString(projected, "title"); title != "Origin of the Blueshift from Pioneer 10" {
		t.Fatalf("title = %q", title)
	}
	if names := authorNames(projected); !sameStrings(names, []string{"Anders Moritz", "Sven Zorn"}) {
		t.Fatalf("names = %v", names)
	}
	if abstract := rec.FetchString(projected, "abstract"); abstract != "First part. Second part." {
		t.Fatalf("abstract = %q", abstract)
	}
}

// A title prints its text scripts; a name drops its marks; spconf's \sthanks is a note.
func TestTitleScriptsAndNameMarks(t *testing.T) {
	projected := projectArticle(t, "", "\\title{Spectroscopy of Eu\\textsuperscript{3+}:Y\\textsubscript{2}O\\textsubscript{3}}\n"+
		"\\name{Bruno Casal\\textsuperscript{1}\\sthanks{At Orion Institute.}, Hugo Ertel}\n\\maketitle")
	if title := rec.FetchString(projected, "title"); title != "Spectroscopy of Eu3+:Y2O3" {
		t.Fatalf("title = %q", title)
	}
	if names := authorNames(projected); !sameStrings(names, []string{"Bruno Casal", "Hugo Ertel"}) {
		t.Fatalf("names = %v", names)
	}
}

// A title's math reads as text with its alphabets and scripts: L_∞, P¹ ×P¹, Ba_1-xK_xFe₂As₂.
func TestTitleMathAlphabetsAndScripts(t *testing.T) {
	cases := map[string]string{
		`$L_{\infty}$ bootstrap`:                           "L_∞ bootstrap",
		`for $\mathbb{P}^1 \times \mathbb{P}^1$`:           "for P¹ ×P¹",
		`$\textrm{Ba}_{1-x}\textrm{K}_{x}\textrm{Fe}_{2}$`: "Ba_1-xK_xFe₂",
		`at $T_{c}$ and $s_{++}$`:                          "at T_c and s_++",
	}
	for source, want := range cases {
		projected := projectArticle(t, "", "\\title{"+source+"}\n\\author{A. Author}\n\\maketitle")
		if title := rec.FetchString(projected, "title"); title != want {
			t.Fatalf("%s: title = %q, want %q", source, title, want)
		}
	}
}

// IEEEtran's compsoc author block ends with a footnote list (\IEEEcompsocitemizethanks, each
// item marked \IEEEcompsocthanksitem) and a membership grade (\IEEEmembership): footnotes and
// marks, not name text (papers3/0776, papers4/0991).
func TestCompsocThanksAreNoNameText(t *testing.T) {
	cases := []struct {
		name, author string
		names        []string
		failure      string
	}{
		{"membership grade", `Vera Lin Hoyt,~\IEEEmembership{CS Distinguished Contributor,~IEEE,} and Pavlos Pappas`,
			[]string{"Vera Lin Hoyt", "Pavlos Pappas"}, ""},
		{"affiliation footnote list", `Vera Lin Hoyt, Nora Hoyt\IEEEcompsocitemizethanks{\IEEEcompsocthanksitem Vera Lin Hoyt, Nora Hoyt are with the Department.}`,
			[]string{"Vera Lin Hoyt", "Nora Hoyt"}, ""},
		{"the paper's author block", "Vera Lin Hoyt,\n\tNora Hoyt,\n Tobias Brook Petersen,~\\IEEEmembership{CS Distinguished Contributor,~IEEE,}\n" +
			" and Pavlos Pappas\n\\IEEEcompsocitemizethanks{\\IEEEcompsocthanksitem Vera Lin Hoyt, Nora Hoyt, and Tobias Brook " +
			"Petersen are with Northfield University. \\protect\n E-mail: \\{vlh,nh,tp\\}@example.org.\n\\IEEEcompsocthanksitem " +
			"Pavlos Pappas is with Eastbrook University. \\protect\\\\ E-mail: pp@example.org.}",
			[]string{"Vera Lin Hoyt", "Nora Hoyt", "Tobias Brook Petersen", "Pavlos Pappas"}, ""},
		{"an unknown macro still refuses", `A. \unknownmacro{B}`, nil,
			`Unsupported macro in literal ACM metadata: \unknownmacro`},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			source := "\\documentclass{article}\n\\title{X}\n\\author{" + item.author + "}\n\\maketitle\n"
			if item.failure != "" {
				if message := failure(func() { NewTexMetadata(source).Project() }); message != item.failure {
					t.Fatalf("failure = %q, want %q", message, item.failure)
				}
				return
			}
			if names := authorNames(NewTexMetadata(source).Project()); !sameStrings(names, item.names) {
				t.Fatalf("names = %v, want %v", names, item.names)
			}
		})
	}
}

// \IEEEcompsocthanksitem outside a footnote list is a spacing command: a line break, a space.
func TestCompsocThanksItemPrintsSpace(t *testing.T) {
	projected := projectArticle(t, "", "\\title{Left\\IEEEcompsocthanksitem Right}\n\\author{A. Author}\n\\maketitle")
	if title := rec.FetchString(projected, "title"); title != "Left Right" {
		t.Fatalf("title = %q", title)
	}
}

// Math a text macro prints, or a display, binds the abstract to its observed blocks.
func TestMacroMathAndDisplaysBindTheAbstract(t *testing.T) {
	projected := projectArticle(t, "\\newcommand{\\transition}{${}^5D_0-{}^7F_0$}\n",
		"\\title{A Paper}\n\\author{A. Author}\n\\maketitle\n\\begin{abstract}\nWe measure the \\transition transition.\n\\end{abstract}")
	if source := rec.FetchString(projected, "abstract_source"); source != "We measure the ${}^5D_0-{}^7F_0$ transition." {
		t.Fatalf("abstract_source = %q", source)
	}
	projected = projectArticle(t, "", "\\title{A Paper}\n\\author{A. Author}\n\\maketitle\n"+
		"\\begin{abstract}\nWe show\n\\[\nx < 1,\n\\]\nwhere it holds.\n\\end{abstract}")
	if _, ok := projected["abstract_source"]; !ok {
		t.Fatalf("display abstract not bound: %v", projected)
	}
}

// Displayed, parenthesized and ensured math is left out of the abstract prose; a line break's
// bracket is no display.
func TestAbstractProseLeavesDisplaysOut(t *testing.T) {
	parser := NewTexMetadata("")
	prose := parser.AbstractProse("Let $T$ be\n\\[2|E|\\]\nso $$nd$$ copies and \\(x\\) of \\ensuremath{\\mathsf{DTS}}$(v)$.")
	if prose != "Let be so copies and of ." {
		t.Fatalf("prose = %q", prose)
	}
	if prose := parser.AbstractProse("One\\\\[2pt] Two"); prose != "One Two" {
		t.Fatalf("line break prose = %q", prose)
	}
}

// The plain TeX spelling of the abstract environment.
func TestPlainTexAbstractEnvironment(t *testing.T) {
	projected := projectArticle(t, "", "\\title{A Paper}\n\\author{A. Author}\n\\maketitle\n"+
		"\\abstract\nPlain words here.\n\\endabstract\n\\section{Introduction}")
	if abstract := rec.FetchString(projected, "abstract"); abstract != "Plain words here." {
		t.Fatalf("abstract = %q", abstract)
	}
}

// A superscript inside a title's word is title text; a trailing mark is not; a plain math group
// only groups.
func TestTitleWordSuperscriptsAndMathGroups(t *testing.T) {
	cases := map[string]string{
		`By means of the P$^2$NFFT method$^*$`: "By means of the P2NFFT method",
		`a$^2$b$^{3}$c`:                        "a2b3c",
		`Molecular ${\mathrm{H_2}}$ gas`:       "Molecular H₂ gas",
	}
	for source, want := range cases {
		projected := projectArticle(t, "", "\\title{"+source+"}\n\\author{A. Author}\n\\maketitle")
		if title := rec.FetchString(projected, "title"); title != want {
			t.Fatalf("%s: title = %q, want %q", source, title, want)
		}
	}
}

// \hspace separates names on an author line; IEEEtran's legacy \authorblockN/\authorblockA read
// as their IEEE-prefixed forms.
func TestHorizontalSpaceAndLegacyAuthorBlocks(t *testing.T) {
	projected := projectArticle(t, "", "\\title{A Paper}\n\\author{Martin Tanner\\thanks{Equal.} \\hspace{2cm} Bert Milford "+
		"\\hspace*{1cm} Rae Ngo\\\\\nUniversity of California, Berkeley}\n\\maketitle")
	if names := authorNames(projected); !sameStrings(names, []string{"Martin Tanner", "Bert Milford", "Rae Ngo"}) {
		t.Fatalf("names = %v", names)
	}
	projected = projectArticle(t, "", "\\title{A Paper}\n\\author{\\authorblockN{Lena Wu}\n\\authorblockA{Hillcrest University}}\n\\maketitle")
	if names := authorNames(projected); !sameStrings(names, []string{"Lena Wu"}) {
		t.Fatalf("names = %v", names)
	}
}

// A note ends a name; REVTeX's \surname, primitive glue, a text macro calling an argument macro
// and an unbraced math alphabet read as they print.
func TestNoteSeparatorsSurnamesGlueAndNestedMacros(t *testing.T) {
	projected := projectArticle(t, "\\def\\systemname#1{\\textsf{#1}}\n\\newcommand{\\HOL}{\\systemname{HOL}}\n",
		"\\title{On $\\boldsymbol q$-series for \\HOL}\n"+
			"\\name{Sonia Marsh\\sthanks{At Google.} Hugo Ertel, Dana \\surname{Herrick}}\n\\maketitle\n"+
			"\\begin{abstract}\nWords.\\\\ \\vskip 0.02in\nMore.\n\\end{abstract}")
	if title := rec.FetchString(projected, "title"); title != "On q-series for HOL" {
		t.Fatalf("title = %q", title)
	}
	if names := authorNames(projected); !sameStrings(names, []string{"Sonia Marsh", "Hugo Ertel", "Dana Herrick"}) {
		t.Fatalf("names = %v", names)
	}
	if abstract := rec.FetchString(projected, "abstract"); abstract != "Words. More." {
		t.Fatalf("abstract = %q", abstract)
	}
}

// An empty field (a template's \email{ }) declares nothing.
func TestAnEmptyEmailDeclaresNothing(t *testing.T) {
	source := "\\documentclass{amsart}\n\\title{A Paper}\n\\author{Ivy Lau}\n\\address{Lakeside Normal University}\n\\email{ }\n" +
		"\\begin{document}\n\\begin{abstract}\nWords.\n\\end{abstract}\n\\maketitle\n\\end{document}\n"
	author := rec.AsMap(rec.FetchList(NewTexMetadata(source).Project(), "authors")[0])
	if emails := rec.FetchList(author, "emails"); len(emails) != 0 {
		t.Fatalf("emails = %v", emails)
	}
	if affiliations := rec.FetchList(author, "affiliations"); len(affiliations) != 1 {
		t.Fatalf("affiliations = %v", affiliations)
	}
}

// The paper's own environments print their code; a formula of stars is a mark.
func TestOwnEnvironmentsAndStarMarks(t *testing.T) {
	projected := projectArticle(t, "\\newenvironment{IEEEkeywords}\n  {\\textbf{Keywords:}}\n  {}\n",
		"\\title{A Paper}\n\\author{A. Author$^*$\\\\\n$*$Department of Mathematics}\n\\maketitle\n"+
			"\\begin{abstract}\nWords.\n\\begin{IEEEkeywords}\nnetworks\n\\end{IEEEkeywords}\n\\end{abstract}")
	if abstract := rec.FetchString(projected, "abstract"); abstract != "Words. Keywords: networks" {
		t.Fatalf("abstract = %q", abstract)
	}
	author := rec.AsMap(rec.FetchList(projected, "authors")[0])
	if affiliations := rec.FetchList(author, "affiliations"); len(affiliations) != 1 || affiliations[0] != "Department of Mathematics" {
		t.Fatalf("affiliations = %v", affiliations)
	}
}

// An abstract's display environments, \PACS codes and own heading line are decorations.
func TestAbstractProseDropsDisplaysPACSAndHeading(t *testing.T) {
	parser := NewTexMetadata("")
	cases := map[string]string{
		"Let\n\\begin{equation*}\nx^2\n\\end{equation*}\nbe so.":                "Let be so.",
		"One two.\n\\PACS{{03.67.Mn}{Entanglement} \\and {02.50.Sk}{Analysis}}": "One two.",
		"\\begin{center}\n\\textbf{Abstract} \\\\\n\\end{center}\nThe text.":    "The text.",
	}
	for source, want := range cases {
		if got := parser.AbstractProse(source); got != want {
			t.Errorf("AbstractProse(%q) = %q, want %q", source, got, want)
		}
	}
}

// A structured abstract's \paragraph{Heading} prints as its heading; elsewhere \paragraph is
// still unsupported literal metadata.
func TestAbstractParagraphHeadings(t *testing.T) {
	parser := NewTexMetadata("")
	cases := map[string]string{
		"\\paragraph{Objective}\nTo assess the ability.": "Objective. To assess the ability.",
		"\\paragraph*{Design} Knee lateral view.":        "Design. Knee lateral view.",
	}
	for source, want := range cases {
		if got := parser.AbstractProse(source); got != want {
			t.Errorf("AbstractProse(%q) = %q, want %q", source, got, want)
		}
	}
	projected := projectArticle(t, "", "\\title{A Paper}\n\\author{A. Author}\n\\maketitle\n"+
		"\\begin{abstract}\n\\paragraph{Objective}\nTo assess the ability.\n\\paragraph{Design}\nKnee lateral view.\n\\end{abstract}")
	want := "Objective. To assess the ability. Design. Knee lateral view."
	if abstract := rec.FetchString(projected, "abstract"); abstract != want {
		t.Fatalf("abstract = %q, want %q", abstract, want)
	}
	source := "\\documentclass{article}\n\\title{A \\paragraph{No} title}\n\\author{A. Author}\n\\maketitle\n"
	if message := failure(func() { NewTexMetadata(source).Project() }); message != `Unsupported macro in literal ACM metadata: \paragraph` {
		t.Fatalf("failure = %q", message)
	}
}

// Names above one minipage share the affiliation the minipage holds.
func TestNamesAboveOneMinipageShareItsAffiliation(t *testing.T) {
	projected := projectArticle(t, "", "\\title{A Paper}\n\\author{Ann One, Bob Two, \\\\ Cy Three, and Di Four\\\\[0.5em]\n{\\small\n"+
		"\\begin{minipage}{\\linewidth}\n\\begin{center}\nSome University \\\\\n"+
		"\\url{{ann, bob}@some.edu}\n\\end{center}\n\\end{minipage}\n}\n}\n\\maketitle")
	if names := authorNames(projected); !sameStrings(names, []string{"Ann One", "Bob Two", "Cy Three", "Di Four"}) {
		t.Fatalf("names = %v", names)
	}
	first := rec.AsMap(rec.FetchList(projected, "authors")[0])
	if affiliations := rec.FetchList(first, "affiliations"); len(affiliations) != 1 || affiliations[0] != "Some University" {
		t.Fatalf("affiliations = %v", affiliations)
	}
}
