// Package authorhints reads the author's hints for speech and alt text (decision 122): a comment
// in any source file the paper loads, or relatex/hints.json beside the main file. The two sections are
// read the same way, so a later job reuses them for alt text keyed by a figure's label.
//
// A hint never changes what the paper typesets: speech hints become MathML intent (decision 109)
// and alt hints become an image's text alternative.
package authorhints

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/authordir"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/schema"
)

// FileName is the hint file in the paper's relatex/ directory (decision 124).
const FileName = authordir.Hints

// Sections are the hint sections decisions 122 and 131 define.
var Sections = []string{"speech", "alt", TablesSection, ColourCaptionsSection}

// ColourCaptionsSection holds the author's attestations of captions that name colours (decision 188): a
// float's \label (or its caption number) and the sha256 of the caption text, `%% relatex colour_captions:
// fig:a = "SHA"` or {"colour_captions": {"fig:a": "SHA"}} in relatex/hints.json.
const ColourCaptionsSection = "colour_captions"

// Hint is one author hint: what it says and where it was read.
type Hint struct {
	Value string
	File  string
	Line  int
}

// Conflict is a hint overridden by a later one saying something else.
type Conflict struct {
	Section, Key string
	Winner       Hint
	Loser        Hint
}

// Unknown is a hint section relatex does not know.
type Unknown struct {
	Section string
	File    string
	Line    int
}

// Hints are the author's hints by section and key. A key is a macro name without its backslash
// (speech) or a figure's label (alt).
type Hints struct {
	Sections  map[string]map[string]Hint
	Conflicts []Conflict
	Unknown   []Unknown
}

// Source is one file the paper loads: its path relative to the paper directory and its text as
// written, comments included.
type Source struct {
	Path, Text string
}

// Reader reads the sources of a paper's main file, in input order (a11ylint.Load).
type Reader func(directory, main string) ([]Source, error)

// Get is the hint for a key of a section, if the author gave one.
func (h *Hints) Get(section, key string) (Hint, bool) {
	if h == nil {
		return Hint{}, false
	}
	hint, ok := h.Sections[section][key]
	return hint, ok
}

// Parse reads hints from the paper's source files, in input order (a later file wins), then from
// relatex/hints.json (which wins over every comment). contract is schemas/relatex-hints.schema.json.
func Parse(sources []Source, data []byte, contract rec.Map) (*Hints, error) {
	hints := &Hints{Sections: map[string]map[string]Hint{}}
	for _, source := range sources {
		hints.comments(source)
	}
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := hints.json(data, contract); err != nil {
			return nil, err
		}
	}
	return hints, nil
}

// Read reads the paper's hints from directory: its main file and the files it loads, then
// relatex/hints.json beside it. load follows the paper's inputs (a11ylint.Load).
func Read(directory, main string, load Reader, contract rec.Map) (*Hints, error) {
	sources, err := load(directory, main)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(directory, FileName))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return Parse(sources, data, contract)
}

var (
	header  = regexp.MustCompile(`^\s*%+\s*relatex\s+([A-Za-z][\w-]*)\s*:(.*)$`)
	entry   = regexp.MustCompile(`^\s*\\?([^=\s]+)\s*=\s*"(.*)"\s*$`)
	comment = regexp.MustCompile(`^\s*%+\s*(.*)$`)
)

// comments reads the hint comments of one source file.
func (h *Hints) comments(source Source) {
	section, open := "", false
	for index, line := range strings.Split(source.Text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if match := header.FindStringSubmatch(line); match != nil {
			section, open = match[1], strings.TrimSpace(match[2]) == ""
			h.add(section, strings.TrimSpace(match[2]), source.Path, index+1)
			continue
		}
		if !open || !comment.MatchString(line) {
			open = false
			continue
		}
		h.add(section, comment.FindStringSubmatch(line)[1], source.Path, index+1)
	}
}

// add reads one hint entry ("\Im = "image"") and records it; a header's empty tail is no entry.
func (h *Hints) add(section, text, file string, line int) {
	match := entry.FindStringSubmatch(text)
	if match == nil {
		return
	}
	value, err := strconv.Unquote(`"` + match[2] + `"`)
	if err != nil {
		return
	}
	key := strings.TrimPrefix(match[1], `\`)
	h.set(section, key, Hint{Value: value, File: file, Line: line})
}

// set records a hint, noting a conflict when it overrides a different value.
func (h *Hints) set(section, key string, hint Hint) {
	if !knownSection(section) {
		h.Unknown = append(h.Unknown, Unknown{Section: section, File: hint.File, Line: hint.Line})
	}
	if h.Sections[section] == nil {
		h.Sections[section] = map[string]Hint{}
	}
	if previous, ok := h.Sections[section][key]; ok && previous.Value != hint.Value {
		h.Conflicts = append(h.Conflicts, Conflict{Section: section, Key: key, Winner: hint, Loser: previous})
	}
	h.Sections[section][key] = hint
}

// json reads relatex/hints.json and validates it against the hints schema.
func (h *Hints) json(data []byte, contract rec.Map) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			failure, ok := recovered.(rec.Failure)
			if !ok {
				panic(recovered)
			}
			err = errors.New(failure.Message)
		}
	}()
	value, decodeErr := canonjson.Decode(strings.NewReader(string(data)))
	if decodeErr != nil {
		return decodeErr
	}
	schema.Validate(value, contract, "$")
	object, ok := value.(rec.Map)
	if !ok {
		return errors.New("relatex/hints.json is not an object")
	}
	for section, value := range object {
		entries, ok := value.(rec.Map)
		if !ok {
			return errors.New("relatex/hints.json section " + section + " is not an object")
		}
		for key, value := range entries {
			text, err := hintText(section, key, value)
			if err != nil {
				return err
			}
			h.set(section, key, Hint{Value: text, File: FileName})
		}
	}
	return nil
}

// hintText is a hints.json entry as a hint's text: a string, or a table entry in its comment form.
func hintText(section, key string, value any) (string, error) {
	if section == TablesSection {
		return tableValue(key, value)
	}
	text, ok := value.(string)
	if !ok {
		return "", errors.New("relatex/hints.json hint " + key + " is not a string")
	}
	return text, nil
}

func knownSection(section string) bool {
	for _, name := range Sections {
		if section == name {
			return true
		}
	}
	return false
}
