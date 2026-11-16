// Package a11ylint is relatex lint --accessibility (plan accessibility C3, C6; decision 096): a
// fast report, read from the LaTeX source without converting it, of what only the author can
// supply for an accessible export: descriptions of figures, the document's language, header rows
// of tables, link texts that name their target, and (opt-in) a document-level reading of
// ambiguous math. relatex never edits the source; each item says where and how to fix it, and
// Annotate writes a marked copy of the source tree for the author to fill in (decision 098).
package a11ylint

import (
	"cmp"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// File is one source file of the paper: its path relative to the paper directory, its text as
// read (Source) and the same text with each comment blanked to spaces (Text), so an offset of Text
// is the same place in Source and line breaks are kept.
type File struct {
	Path   string
	Text   string
	Source string
	lines  []int // the offset where each line starts
}

// inputDepth bounds how deep \input and \include are followed.
const inputDepth = 6

var (
	// The import package names a directory first: \subimport*{figures/}{a.pdf_t}.
	inputCommand = regexp.MustCompile(`\\(?:input|include|subfile|(?:sub)?(?:import|inputfrom|includefrom)\*?\s*\{([^}]*)\})\s*\{([^}]+)\}`)
	comment      = regexp.MustCompile(`(?m)(^|[^\\])%.*$`)
)

// Load reads the main file and the files it inputs (\input, \include, \subfile, and the author's
// commands that input a file named by their arguments, decision 115), in reading order, each once.
func Load(directory, main string) ([]*File, error) {
	l := &loader{directory: directory, seen: map[string]bool{}, defined: map[string]macro{}, templates: map[string][]string{}}
	err := l.visit(main, 0)
	return l.files, err
}

// loader follows a paper's inputs: the files read so far, the commands they define (with each
// command's input templates once known) and the directories of \input@path.
type loader struct {
	directory string
	files     []*File
	seen      map[string]bool
	defined   map[string]macro
	templates map[string][]string
	paths     []string
}

// inclusion is a file a text inputs, at offset.
type inclusion struct {
	offset int
	name   string
}

func (l *loader) visit(name string, depth int) error {
	if l.seen[name] || depth > inputDepth {
		return nil
	}
	l.seen[name] = true
	data, err := os.ReadFile(filepath.Join(l.directory, filepath.FromSlash(name)))
	if err != nil {
		if depth == 0 {
			return err
		}
		return nil
	}
	file := newFile(name, string(data))
	l.files = append(l.files, file)
	addMacros(l.defined, file.Text)
	clear(l.templates)
	l.paths = append(l.paths, inputPaths(file.Text)...)
	// An input may define the commands that input the next ones (a book's preamble file defines
	// \InputChap), so the inclusions after each one are found again once it is read.
	for next := l.nextInclusion(file.Text, 0); next != nil; next = l.nextInclusion(file.Text, next.offset+1) {
		for _, input := range next.names {
			if child := resolve(l.directory, path.Dir(name), input, l.paths); child != "" {
				if err := l.visit(child, depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// pending are the files the first inclusion at or after an offset inputs.
type pending struct {
	offset int
	names  []string
}

// nextInclusion is the first inclusion of text at or after from, nil when there is none.
func (l *loader) nextInclusion(text string, from int) *pending {
	var next *pending
	for _, input := range l.inclusions(text) {
		switch {
		case input.offset < from:
			continue
		case next == nil:
			next = &pending{offset: input.offset, names: []string{input.name}}
		case input.offset == next.offset:
			next.names = append(next.names, input.name)
		}
	}
	return next
}

// inclusions are the files text inputs, in the order they appear.
func (l *loader) inclusions(text string) []inclusion {
	found := []inclusion{}
	for _, match := range inputCommand.FindAllStringSubmatchIndex(text, -1) {
		directory := ""
		if match[2] >= 0 {
			directory = text[match[2]:match[3]]
		}
		if name := inputName(directory, text[match[4]:match[5]]); !strings.Contains(name, "#") {
			found = append(found, inclusion{offset: match[0], name: name})
		}
	}
	// A package of the paper's own (diffyqssetup.sty beside the main file) defines its commands; a
	// package TeX Live provides is not in the paper and resolves to nothing (decision 167).
	for _, match := range packageUse.FindAllStringSubmatchIndex(text, -1) {
		for _, name := range strings.Split(text[match[2]:match[3]], ",") {
			if name = strings.TrimSpace(name); name != "" && !strings.Contains(name, "#") {
				found = append(found, inclusion{offset: match[0], name: name + ".sty"})
			}
		}
	}
	for _, name := range innerCommands(text, l.defined, "") {
		definition := l.defined[name]
		templates := inputTemplates(name, l.defined, l.templates, inputDepth)
		if len(templates) == 0 {
			continue
		}
		for _, use := range calls(text, name, definition) {
			for _, template := range templates {
				found = append(found, inclusion{offset: use.start, name: strings.TrimSpace(substitute(template, use.arguments))})
			}
		}
	}
	slices.SortStableFunc(found, func(a, b inclusion) int { return cmp.Compare(a.offset, b.offset) })
	return found
}

// inputName is the file an input names: its name, in the directory the import package gives it.
func inputName(directory, name string) string {
	directory, name = strings.TrimSpace(directory), strings.TrimSpace(name)
	if directory == "" {
		return name
	}
	return strings.TrimSuffix(directory, "/") + "/" + name
}

// resolve is an input's file relative to the paper directory, in TeX's order: as written (TeX's
// working directory is the main file's), then in the directories of \input@path, and last beside
// the inputting file (base), with .tex added when it has no extension; "" when missing or when the
// name still holds a command. A chapter's \input{shared/prefix} is the main directory's
// shared/prefix.tex even when the chapter's directory holds its own copy (decision 167).
func resolve(directory, base, name string, paths []string) string {
	if strings.Contains(name, `\`) || name == "" {
		return ""
	}
	prefixes := append(append([]string{""}, paths...), base)
	for _, prefix := range prefixes {
		// Names are the source's own, "/" between their parts on every platform.
		for _, candidate := range []string{path.Join(prefix, name), path.Join(prefix, name+".tex")} {
			clean := path.Clean(candidate)
			if strings.HasPrefix(clean, "..") {
				continue
			}
			if info, err := os.Stat(filepath.Join(directory, filepath.FromSlash(clean))); err == nil && !info.IsDir() {
				return clean
			}
		}
	}
	return ""
}

func newFile(path, source string) *File {
	blanked := []byte(source)
	for _, match := range comment.FindAllStringSubmatchIndex(source, -1) {
		for index := match[3]; index < match[1]; index++ {
			blanked[index] = ' '
		}
	}
	text := string(blanked)
	file := &File{Path: path, Text: text, Source: source, lines: []int{0}}
	for index, char := range text {
		if char == '\n' {
			file.lines = append(file.lines, index+1)
		}
	}
	return file
}

// Line is the 1-based line of offset.
func (f *File) Line(offset int) int {
	low, high := 0, len(f.lines)
	for low+1 < high {
		middle := (low + high) / 2
		if f.lines[middle] <= offset {
			low = middle
		} else {
			high = middle
		}
	}
	return low + 1
}

// companions are the top-level files the conversion never takes for the main one (input.Companions,
// the default main.ignore; the lint, a source layer, does not import the input package).
var companions = []string{"prefix.tex", "prelim.tex"}

// FindMain is the paper's main file in directory, as the conversion chooses it: requested when
// given, else the only top-level .tex file besides the companions (prefix.tex, prelim.tex), else
// the only top-level .tex file that begins a document.
func FindMain(directory, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", err
	}
	candidates, topLevel := []string{}, []string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".tex") {
			continue
		}
		if !slices.Contains(companions, strings.ToLower(entry.Name())) {
			topLevel = append(topLevel, entry.Name())
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err == nil && strings.Contains(string(data), `\begin{document}`) {
			candidates = append(candidates, entry.Name())
		}
	}
	if len(topLevel) == 1 {
		return topLevel[0], nil
	}
	if len(candidates) != 1 {
		return "", fmt.Errorf("cannot choose the main file in %s (%d candidates: %s); name it with --main",
			directory, len(candidates), strings.Join(candidates, ", "))
	}
	return candidates[0], nil
}

// argument is a balanced group at offset (after white space), opened by open and closed by closing: its text and the
// offset after it; ok is false when none is there.
func argument(text string, offset int, open, closing byte) (string, int, bool) {
	start := offset
	for start < len(text) && strings.IndexByte(" \t\r\n", text[start]) >= 0 {
		start++
	}
	if start >= len(text) || text[start] != open {
		return "", offset, false
	}
	depth := 0
	for index := start; index < len(text); index++ {
		switch text[index] {
		case '{':
			depth++
		case '}':
			depth--
		}
		if open == '{' && depth == 0 || open == '[' && text[index] == closing && depth == 0 {
			return text[start+1 : index], index + 1, true
		}
	}
	return "", offset, false
}

// keyValues are key=value options split at top-level commas; a value loses one pair of braces.
func keyValues(options string) map[string]string {
	result := map[string]string{}
	depth, start := 0, 0
	add := func(part string) {
		key, value, _ := strings.Cut(part, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
			value = value[1 : len(value)-1]
		}
		if key != "" {
			result[key] = value
		}
	}
	for index := 0; index < len(options); index++ {
		switch options[index] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				add(options[start:index])
				start = index + 1
			}
		}
	}
	add(options[start:])
	return result
}
