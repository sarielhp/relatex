package papersource

import (
	"path"
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

var (
	// styleDeclaration: the class and the packages a source loads, by name.
	styleDeclaration = regexp.MustCompile(`\\(documentclass|usepackage|RequirePackage)\s*(?:\[[^\]]*\]\s*)?\{([^{}]+)\}`)
	styleComment     = regexp.MustCompile(`(?m)(^|[^\\])%.*$`)
)

// ownStyles: the text of the class and package files the paper itself ships, as the source
// loads them (\documentclass{X} reads X.cls, \usepackage{a,b} reads a.sty and b.sty). Front
// matter reads class author forms from them (decision 091); a system style is not read.
func (s *Source) ownStyles(text string) string {
	var builder strings.Builder
	for _, match := range styleDeclaration.FindAllStringSubmatch(styleComment.ReplaceAllString(text, "$1"), -1) {
		extension := ".sty"
		if match[1] == "documentclass" {
			extension = ".cls"
		}
		for _, name := range strings.Split(match[2], ",") {
			file := s.inputName(rec.Strip(name) + extension)
			if _, ok := s.manifest["input/files/"+file]; ok && file != "" {
				builder.Write(s.sourceArtifact(file))
				builder.WriteByte('\n')
			}
		}
	}
	return builder.String()
}

// inputName is the archive name of a file the main file's source names (\input, a class or a
// package): TeX runs in the main file's directory, so a name is relative to it ("../preamble"
// from slides/deck.tex is "preamble"); "" for a name that leaves the source.
func (s *Source) inputName(name string) string {
	directory := path.Dir(s.main)
	if directory == "." {
		return name
	}
	joined := path.Join(directory, name)
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return ""
	}
	return joined
}
