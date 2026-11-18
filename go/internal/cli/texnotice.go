package cli

import (
	"os"
	"strings"
)

// whyLuaLaTeXURL is the page that explains why relatex needs a paper that builds on LuaLaTeX.
const whyLuaLaTeXURL = "https://github.com/sarielhp/relatex/blob/main/docs/why-lualatex.md"

// luaLaTeXNotice is printed before the details of a failure whose cause is the paper's own
// LaTeX build; the URL (possibly a hyperlink) follows it.
const luaLaTeXNotice = "This program requires a paper that compiles correctly on LuaLaTeX (no errors). " +
	"It does not work on papers that compile only with other LaTeX engines (such as xelatex, " +
	"pdflatex or latex). This is because of non-trivial technical issues. For more details see: "

// texBuildFailures are the messages of the engine's refusals of a paper whose LaTeX build fails
// (engine.engineFailure): a TeX error, a build that did not compile, or builds that never settle.
var texBuildFailures = []string{
	"LuaLaTeX failed on the paper",
	"Ordinary/external builds did not both compile and converge",
	"The builds did not converge",
}

func isTeXBuildFailure(message string) bool {
	for _, marker := range texBuildFailures {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// hyperlinkable: an OSC 8 link only on a colour terminal that is not "dumb" (colour is false
// when stderr is no terminal or NO_COLOR is set).
func hyperlinkable(colour bool) bool {
	term := os.Getenv("TERM")
	return colour && term != "dumb" && term != ""
}

// luaLaTeXNoticeText is the notice with its URL, an OSC 8 hyperlink when hyperlinkable.
func luaLaTeXNoticeText(colour bool) string {
	url := whyLuaLaTeXURL
	if hyperlinkable(colour) {
		url = "\x1b]8;;" + whyLuaLaTeXURL + "\x1b\\" + whyLuaLaTeXURL + "\x1b]8;;\x1b\\"
	}
	return luaLaTeXNotice + url
}

// withTeXNotice puts the notice before the details of a TeX-build failure; any other failure
// is returned as it is.
func withTeXNotice(message, text string, colour bool) string {
	if !isTeXBuildFailure(message) {
		return text
	}
	return luaLaTeXNoticeText(colour) + "\n\n" + text
}
