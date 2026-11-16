package a11ylint

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/figures"
)

// IncludedFigure is a figure the last conversion included from a file, with the places in the
// source TeX recorded for it (decision 115): where its \includegraphics ran, or the call of the
// author's macro that wraps it. The lint finds through them what no reading of the source can
// (macros that build the file name, chapters input by the author's commands).
type IncludedFigure struct {
	Key    string // the export's name for it (the sidecar's key)
	Source string // the included file, from the source root
	Places []figdesc.Place
}

// locateWindow bounds how many lines before the recorded one the call that names the file may
// start (a macro's arguments are read whole before its \includegraphics runs).
const locateWindow = 60

// AddPlaces reads the files of the conversion's places that the source's inputs did not reach,
// so their items have a file to be marked in.
func AddPlaces(directory string, paper *Paper) {
	loaded := map[string]bool{}
	for _, file := range paper.Files {
		loaded[file.Path] = true
	}
	places := []figdesc.Place{}
	for _, figure := range paper.Included {
		places = append(places, figure.Places...)
	}
	for _, drawing := range paper.Drawings {
		places = append(places, drawing.Places...)
	}
	for _, place := range places {
		if loaded[place.File] {
			continue
		}
		loaded[place.File] = true
		if data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(place.File))); err == nil {
			paper.Files = append(paper.Files, newFile(place.File, string(data)))
		}
	}
}

// staticGraphic is an \includegraphics the source names literally: its file, its key and the name
// without commands ("" when a command builds it, as in \File{figs/a}, which only TeX resolves).
type staticGraphic struct {
	file, key, plain string
}

var command = regexp.MustCompile(`\\[A-Za-z@]+\*?\s*`)

// cleanName is a graphic's name with its commands and braces removed (\File{figs/a} is figs/a).
func cleanName(name string) string {
	return strings.TrimSpace(strings.NewReplacer("{", "", "}", "").Replace(command.ReplaceAllString(name, "")))
}

// staticGraphics are the \includegraphics the source files name literally.
func staticGraphics(files []*File) []staticGraphic {
	found := []staticGraphic{}
	for _, file := range files {
		for _, match := range graphic.FindAllStringIndex(file.Text, -1) {
			_, after, _ := argument(file.Text, match[1], '[', ']')
			name, _, ok := argument(file.Text, after, '{', '}')
			if !ok || strings.Contains(name, "#") {
				continue
			}
			name = strings.TrimSpace(name)
			entry := staticGraphic{file: file.Path, key: figures.SourceName(cleanName(name))}
			if !strings.Contains(name, `\`) {
				entry.plain = name
			}
			found = append(found, entry)
		}
	}
	return found
}

// covered: the source's own \includegraphics of this file is already linted, either in the file
// the conversion recorded or, named plainly, wherever it is (a macro's definition, the qed sign).
func covered(graphics []staticGraphic, source, file string) bool {
	key := figures.SourceName(source)
	for _, graphic := range graphics {
		if graphic.key != key {
			continue
		}
		if graphic.file == file || graphic.plain != "" && sameFile(graphic.plain, source) {
			return true
		}
	}
	return false
}

// sameFile: name (as \includegraphics writes it, maybe without its extension) is source.
func sameFile(name, source string) bool {
	name = path.Clean(strings.TrimPrefix(name, "./"))
	return name == source || name == strings.TrimSuffix(source, path.Ext(source))
}

// includedItems: every place of the last conversion's included figures that has no description
// and that the source's literal \includegraphics does not already report, once per place.
func includedItems(paper Paper, graphics []staticGraphic, commands *regexp.Regexp) []Item {
	items := []Item{}
	files := map[string]*File{}
	for _, file := range paper.Files {
		files[file.Path] = file
	}
	done := map[string]bool{}
	for _, figure := range paper.Included {
		for _, place := range figure.Places {
			file := files[place.File]
			if place.Described || file == nil || covered(graphics, figure.Source, place.File) {
				continue
			}
			item := locate(file, place.Line, figure, commands)
			if id := fmt.Sprintf("%s\x00%s\x00%d", figure.Key, item.File, item.Line); !done[id] {
				done[id] = true
				items = append(items, includedFound(item, paper.Sidecar)...)
			}
		}
	}
	return items
}

// includedFound is the item of a figure place, unless the sidecar describes it since the conversion.
func includedFound(item Item, sidecar *figdesc.Sidecar) []Item {
	var entry *figdesc.Entry
	if sidecar != nil {
		entry = sidecar.Entries[item.Key]
	}
	found := sidecarItems(item, entry, false)
	for index := range found {
		if found[index].Kind == "figure" {
			found[index] = includedNote(found[index])
		}
	}
	return found
}

// includedNote is what to write for a figure an author's macro includes: alt in the macro's
// options when it passes them on to \includegraphics, else the sidecar entry.
func includedNote(item Item) Item {
	item.NoteLine = item.Line
	sidecar := fmt.Sprintf("[figs.%s] with alt = \"…\" in %s beside the main file (relatex alt draft drafts it, "+
		"relatex alt accept %s keeps a checked draft)", item.Key, figdesc.FileName, item.Key)
	if item.Offset >= 0 {
		item.Note = "ready to fill: alt={…} in the options of this call, which passes them on to \\includegraphics, or " + sidecar
		item.Fix = "alt={…} in the options of this call, or [figs." + item.Key + "] alt = \"…\" in " + figdesc.FileName +
			" (relatex alt draft drafts one); artifact (or alt={}) if it is decoration"
		return item
	}
	item.Note = "this figure is included through a command of the paper (TeX recorded it here); ready to fill: " + sidecar
	item.Fix = "[figs." + item.Key + "] alt = \"…\" in " + figdesc.FileName + " (relatex alt draft drafts one), or decorative = true; " +
		"or alt={…} on the \\includegraphics inside the command when every use shows the same figure"
	return item
}

// locate is the item of a figure at the line TeX recorded: moved back to the line that names its
// file when that is within locateWindow lines, with the offset of the call that takes the alt key
// there (-1 when none does).
func locate(file *File, line int, figure IncludedFigure, commands *regexp.Regexp) Item {
	item := Item{File: file.Path, Line: line, Offset: -1, Key: figure.Key, Target: figure.Source}
	if line < 1 || line > len(file.lines) {
		return item
	}
	end := len(file.Text)
	if line < len(file.lines) {
		end = file.lines[line]
	}
	start := file.lines[max(0, line-1-locateWindow)]
	at := lastSpelling(file.Text[start:end], spellings(file.Path, figure.Source))
	if at < 0 {
		return item
	}
	at += start
	if insideDefinition(file.Text, at) {
		return item // a command of the file names it for each call (a page each): the call is the place
	}
	item.Line = file.Line(at)
	item.Offset = callAround(file.Text, start, at, commands)
	return item
}

// insideDefinition: offset at lies in the body of a command the text defines.
func insideDefinition(text string, at int) bool {
	for _, match := range definition.FindAllStringIndex(text[:at], -1) {
		_, after, _ := argument(text, match[1], '[', ']')
		if _, end, ok := argument(text, after, '{', '}'); ok && end > at {
			return true
		}
	}
	return false
}

// spellings are the ways a source may name source, longest first: from the including file's
// directory, from the root, and its stem.
func spellings(including, source string) []string {
	bare := strings.TrimSuffix(source, path.Ext(source))
	relative := strings.TrimPrefix(bare, path.Dir(including)+"/")
	result := []string{}
	for _, spelling := range []string{bare, relative, path.Base(bare)} {
		if spelling != "" && (len(result) == 0 || result[len(result)-1] != spelling) {
			result = append(result, spelling)
		}
	}
	return result
}

// lastSpelling is the offset of the last whole occurrence in text of the first spelling found.
func lastSpelling(text string, spellings []string) int {
	for _, spelling := range spellings {
		pattern := regexp.MustCompile(`(?:^|[^A-Za-z0-9_])(` + regexp.QuoteMeta(spelling) + `)(?:[^A-Za-z0-9_]|$)`)
		matches := pattern.FindAllStringSubmatchIndex(text, -1)
		if len(matches) > 0 {
			return matches[len(matches)-1][2]
		}
	}
	return -1
}

// callAround is the start of the call of \includegraphics, or of a command passing its options on,
// whose file argument holds offset at; -1 when none does.
func callAround(text string, from, at int, commands *regexp.Regexp) int {
	found := -1
	for _, match := range commands.FindAllStringIndex(text[from:at], -1) {
		start := from + match[0]
		_, after, _ := argument(text, from+match[1], '[', ']')
		// The file is one of its first arguments (\IncGraphPage[opts]{figs}{name}{2}).
		for range 4 {
			_, end, ok := argument(text, after, '{', '}')
			if !ok || after > at {
				break
			}
			if end > at {
				found = start
				break
			}
			after = end
		}
	}
	return found
}
