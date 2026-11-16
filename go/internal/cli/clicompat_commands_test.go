package cli

import (
	"slices"
	"testing"
)

// commandCase is a command line of the other commands (not convert, which clicompat_test.go pins
// option by option): the command it reaches and the words left for it, or that it is a usage error.
type commandCase struct {
	name      string
	arguments []string
	path      []string
	words     []string
}

// commandCases are the invocations of the other commands found in tools/, lib/, test/ and the
// documentation (the 2026-11 option inventory), and their options.
func commandCases() []commandCase {
	return []commandCase{
		{"package_smoke export", []string{"export", "DOC.json.gz", "--output", "D", "--format", "typst", "--compile-typst", "--report"},
			[]string{"export"}, []string{"DOC.json.gz"}},
		{"check_epub", []string{"export", "BUNDLE", "--format", "epub", "--output", "D", "--report"}, []string{"export"}, []string{"BUNDLE"}},
		{"check_typst", []string{"export", "DOC", "--output", "O", "--format", "typst-source", "--report"}, []string{"export"}, []string{"DOC"}},
		{"documented export", []string{"export", "reading/document.json", "--format", "markdown", "--output", "markdown"},
			[]string{"export"}, []string{"reading/document.json"}},
		{"reconstruct", []string{"reconstruct", "B", "-o", "D", "--allow-fallback", "image"}, []string{"reconstruct"}, []string{"B"}},
		{"validate", []string{"validate", "document.json.gz"}, []string{"validate"}, []string{"document.json.gz"}},
		{"inspect", []string{"inspect", "B", "--node", "n1", "--projection", "html", "--raw"}, []string{"inspect"}, []string{"B"}},
		{"sync forward", []string{"sync", "B", "--source", "a.tex", "--line", "3", "--column", "2"}, []string{"sync"}, []string{"B"}},
		{"sync inverse", []string{"sync", "B", "--page", "1", "--x", "10.5", "--y", "20"}, []string{"sync"}, []string{"B"}},
		{"layers", []string{"layers", "A", "--stage", "input"}, []string{"layers"}, []string{"A"}},
		{"config", []string{"config", "d", "--init-project"}, []string{"config"}, []string{"d"}},
		{"capabilities", []string{"capabilities"}, []string{"capabilities"}, nil},
		{"doctor", []string{"doctor", "--json"}, []string{"doctor"}, nil},
		{"bibstyle_compare all", []string{"bibclean", "--all", "--report", "r.json", "a.bib", "b.bib"}, []string{"bibclean"},
			[]string{"a.bib", "b.bib"}},
		{"bibclean keys", []string{"bibclean", "--keys", "a,b", "--keys", "c", "x.bib"}, []string{"bibclean"}, []string{"x.bib"}},
		{"lint", []string{"lint", "--accessibility", "PAPER", "--output", "D", "--annotate", "A", "--review-page", "R", "--patch", "P",
			"--accept", "T", "--strict", "--json", "--math", "--main", "m.tex"}, []string{"lint"}, []string{"PAPER"}},
		{"lint review", []string{"lint", "PAPER", "--accessibility", "--output", "D"}, []string{"lint"}, []string{"PAPER"}},
		{"synctex", []string{"synctex", "PAPER", "--main", "main.tex", "--for", "PDF", "--wait", "300"}, []string{"synctex"},
			[]string{"PAPER"}},
		{"synctex replace", []string{"synctex", "--for", "PDF", "--replace-pdf", "--engine", "e"}, []string{"synctex"}, nil},
		{"tag-pdf", []string{"tag-pdf", "out", "--pdf", "p.pdf", "--target", "t.pdf", "--web-report", "w.json"}, []string{"tag-pdf"},
			[]string{"out"}},
		{"alt draft", []string{"alt", "draft", "--output", "D", "--allow-upload", "--limit", "3", "--redraft", "P"},
			[]string{"alt", "draft"}, []string{"P"}},
		{"alt review", []string{"alt", "review", "P", "-o", "D"}, []string{"alt", "review"}, []string{"P"}},
		{"alt accept", []string{"alt", "accept", "a", "b", "--paper", "P"}, []string{"alt", "accept"}, []string{"a", "b"}},
		{"alt import", []string{"alt", "import", "saved.toml", "P"}, []string{"alt", "import"}, []string{"saved.toml", "P"}},
		{"alt set", []string{"alt", "set", "a", "--alt", "A tree.", "--long", "", "--details", "d"}, []string{"alt", "set"}, []string{"a"}},
		{"alt geometry", []string{"alt", "geometry", "--pdf", "f.pdf", "--page", "2", "--json"}, []string{"alt", "geometry"}, nil},
		{"replay", []string{"replay", "attempt", "--force", "export,publish", "--no-cache"}, []string{"replay"}, []string{"attempt"}},
		{"convert command", []string{"convert", "paper.zip", "--html"}, []string{"convert"}, []string{"paper.zip"}},
	}
}

// TestCommandInvocationsKeepTheirParse: every invocation of the other commands the tools, tests and
// documentation make reaches its command with its words.
func TestCommandInvocationsKeepTheirParse(t *testing.T) {
	for _, c := range commandCases() {
		current, err := parseLine(c.arguments...)
		if err != nil {
			t.Errorf("%s: relatex %v: %v", c.name, c.arguments, err)
			continue
		}
		if !slices.Equal(current.path, c.path) || !slices.Equal(current.words, c.words) {
			t.Errorf("%s: relatex %v reached %v with %v, wants %v with %v", c.name, c.arguments, current.path, current.words, c.path, c.words)
		}
	}
}

// TestCommandUsageErrorsKeepTheirStatus: a wrong option, a missing required one or a wrong number of
// words is status 2 for every command.
func TestCommandUsageErrorsKeepTheirStatus(t *testing.T) {
	for _, arguments := range [][]string{
		{"export", "DOC"}, {"export", "--output", "D"}, {"export", "DOC", "--output", "D", "--format", "gif"},
		{"validate"}, {"validate", "a", "b"}, {"inspect", "--node"}, {"sync"}, {"sync", "B", "--line", "x"},
		{"layers", "A", "--stage", "nowhere"}, {"capabilities", "extra"}, {"doctor", "extra"}, {"doctor", "--yes"},
		{"bibclean", "--all"}, {"lint", "PAPER"}, {"lint", "--accessibility", "--accept", "T"},
		{"synctex", "--main", "m.tex"}, {"synctex", "--for", "P", "--wait", "soon"}, {"tag-pdf", "--bogus"},
		{"alt", "draft", "--limit", "many"}, {"alt", "set"}, {"alt", "nonsense"}, {"replay"}, {"replay", "a", "b"},
	} {
		if status, _, stderr := run(t, arguments...); status != 2 || stderr == "" {
			t.Errorf("relatex %v: status %d, stderr %q", arguments, status, stderr)
		}
	}
}
