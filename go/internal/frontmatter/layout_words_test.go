package frontmatter

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// Layout words inside an author or address block print as the line breaks, font switches and
// author-block starts they are; they are not metadata the reader must refuse.
func TestAuthorLayoutWordsRead(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{
			"endgraf ends a line in an address",
			"\\documentclass{amsart}\n\\title{X}\n\\author{A. Author}\n\\address{Dept\\endgraf City}\n" +
				"\\begin{abstract}T.\\end{abstract}\n\\maketitle\n",
			`[{"affiliations":["Dept City"],"emails":[],"name":"A. Author"}]`,
		},
		{
			"break ends a line in an address",
			"\\documentclass{amsart}\n\\title{X}\n\\author{A. Author}\n\\address{A. Author \\hfill\\break Institute}\n" +
				"\\begin{abstract}T.\\end{abstract}\n\\maketitle\n",
			`[{"affiliations":["A. Author Institute"],"emails":[],"name":"A. Author"}]`,
		},
		{
			"alignauthor starts the next author block",
			"\\documentclass{article}\n\\title{X}\\author{\\alignauthor Ann\\\\ \\affaddr{One} " +
				"\\alignauthor Bob\\\\ \\affaddr{Two}}\n\\maketitle\n",
			`[{"affiliations":["One"],"emails":[],"name":"Ann"},{"affiliations":["Two"],"emails":[],"name":"Bob"}]`,
		},
		{
			"a nested address is the author's affiliation",
			"\\documentclass{amsart}\n\\title{X}\n\\author{Yuri Dolan \\address{Dept\\\\ e.mail: y@x}}\n" +
				"\\begin{abstract}T.\\end{abstract}\n\\maketitle\n",
			`[{"affiliations":["Dept e.mail: y@x"],"emails":[],"name":"Yuri Dolan"}]`,
		},
	}
	for _, item := range cases {
		authors := rec.FetchList(NewTexMetadata(item.source).Project(), "authors")
		if got := marshal(authors); got != item.want {
			t.Errorf("%s: authors %s, want %s", item.name, got, item.want)
		}
	}
}

// An unknown macro in an author is still refused with the same message.
func TestUnknownAuthorMacroStillRefuses(t *testing.T) {
	source := "\\documentclass{article}\n\\title{X}\\author{A. \\unknownmacro{B}}\n\\maketitle\n"
	message := failure(func() { NewTexMetadata(source).Project() })
	if message != `Unsupported macro in literal ACM metadata: \unknownmacro` {
		t.Errorf("failure %q", message)
	}
}
