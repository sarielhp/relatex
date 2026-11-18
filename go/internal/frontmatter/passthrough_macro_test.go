package frontmatter

import "testing"

// A one-argument macro the author defines to pass its argument through (\NotESA, the article
// build of an ESA/article switch) wraps the front matter without hiding it; its twin that
// drops the argument (\ESA) hides the other build's.
func TestPassThroughMacroWrapsFrontMatter(t *testing.T) {
	source := "\\documentclass{article}\n" +
		"\\newcommand{\\ESA}[1]{}\n\\newcommand{\\NotESA}[1]{#1}\n" +
		"\\NotESA{\\usepackage{amsmath}}\n" +
		"\\begin{document}\n\\title{T}\n" +
		"\\ESA{\\author{Wrong}{Elsewhere}}\n" +
		"\\newcommand{\\Funded}[1]{\\NotESA{\\thanks{Funded. #1}}}\n\\NotESA{%\n \\author{Ada Lovelace\\Funded{By NSF.}}\n \\date{\\today} }\n" +
		"\\maketitle\n\\begin{abstract}Text.\\end{abstract}\n"
	project := NewTexMetadata(source).Project()
	want := `[{"affiliations":[],"emails":[],"name":"Ada Lovelace"}]`
	if got := marshal(project["authors"]); got != want {
		t.Errorf("authors %s", got)
	}
	if project["title"] != "T" {
		t.Errorf("title %v", project["title"])
	}
}
