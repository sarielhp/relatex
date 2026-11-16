package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// beamerClass matches a beamer \documentclass and captures its options (decision 083).
var beamerClass = regexp.MustCompile(`\\documentclass\s*(?:\[([^\]]*)\])?\s*\{beamer\}`)

// staticModes are the beamer modes that already set one page per frame.
var staticModes = map[string]bool{"handout": true, "trans": true}

// HandoutPrefix asks beamer for its handout mode: one page per frame (decision 083).
const HandoutPrefix = "\\PassOptionsToClass{handout}{beamer}\n"

// OneUpPrefix keeps a deck's pages one frame each: a pgfpages imposition (2 on 1 handouts) is a
// print layout, not content (decision 083).
const OneUpPrefix = "\\AddToHook{package/pgfpages/after}{\\RenewDocumentCommand\\pgfpagesuselayout{mo}{}" +
	"\\RenewDocumentCommand\\pgfpageslogicalpageoptions{mm}{}}\n"

// SlidePrefix is what every build of a source puts before its main file: for a beamer deck, the
// one-up pages and the handout mode unless the deck chooses a static mode itself; nothing for
// any other source.
func SlidePrefix(path string) string {
	match := classLine(path, 2)
	if match == nil {
		return ""
	}
	for _, option := range strings.Split(match[1], ",") {
		if staticModes[strings.TrimSpace(option)] {
			return OneUpPrefix
		}
	}
	return OneUpPrefix + HandoutPrefix
}

// anyClass is any \documentclass line; inputFile is an \input or \include of a named file.
var (
	anyClass  = regexp.MustCompile(`\\documentclass\b`)
	inputFile = regexp.MustCompile(`\\(?:input|include)\s*\{([^{}]+)\}`)
)

// classLine is the beamer class match of a main file, or of a file it \inputs when it has no class
// line of its own (slides/0027: the \documentclass sits in ../preamble.tex), following at most
// depth levels of input.
func classLine(path string, depth int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	text := uncommented(string(data))
	if anyClass.MatchString(text) || depth == 0 {
		return beamerClass.FindStringSubmatch(text)
	}
	for _, input := range inputFile.FindAllStringSubmatch(text, -1) {
		name := filepath.Join(filepath.Dir(path), strings.TrimSpace(input[1]))
		if filepath.Ext(name) == "" {
			name += ".tex"
		}
		if match := classLine(name, depth-1); match != nil {
			return match
		}
	}
	return nil
}

// uncommented drops TeX comments: what follows an unescaped % on each line.
func uncommented(text string) string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = line[:commentStart(line)]
	}
	return strings.Join(lines, "\n")
}

// commentStart is the index of a line's first unescaped %, or its length.
func commentStart(line string) int {
	for at := 0; at < len(line); at++ {
		switch line[at] {
		case '\\':
			at++
		case '%':
			return at
		}
	}
	return len(line)
}

// slidePrefix is SlidePrefix for the probe's main file in its input files.
func (p *Probe) slidePrefix() string {
	return SlidePrefix(filepath.Join(p.Directory, "input", "files", p.main))
}
