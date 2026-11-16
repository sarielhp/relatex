// Package figdesc is the document model's figure descriptions (decisions 086, 096, 102): the
// sidecar file relatex/alt.toml beside the paper, the figure files of the export it describes (with
// their caption and citing text), and how its descriptions enter document.json with their
// provenance. The author tool (alttext), the linter, the tagged PDF and the conversion read them
// here. relatex never replaces an entry the author wrote or reviewed.
package figdesc

import (
	"errors"
	"fmt"
	"github.com/sarielhp/relatex/internal/authordir"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// FileName is the sidecar, relative to the paper directory (decision 124).
const FileName = authordir.Alt

// Provenances of an entry: the author's own text (the default when an entry names none), a
// model draft nobody has reviewed, and a model draft a person accepted.
const (
	Author   = "author"
	Model    = "model"
	Reviewed = "reviewed"
	// Source is the provenance of a description the LaTeX source gives (decision 096), which no
	// sidecar entry or model draft replaces.
	Source = "source"
	// SourceMacro is the provenance of a description the source gives with relatex's own
	// \relatexalt (decision 125, amendment 2): the author's or a reviewed sidecar entry replaces it,
	// a model draft does not.
	SourceMacro = "source_macro"
	// Inferred is the provenance of a description relatex derives from what the source draws (a
	// chess diagram's position, decision 163), replaced like SourceMacro; and of a decorative mark
	// relatex inferred (decision 111).
	Inferred = "inferred"
)

// figuresTable is the table that holds one entry per figure: [figs.NAME].
const figuresTable = "figs"

// entryKeys are the settings of a figure entry; approvalKeys those of [approval].
var (
	entryKeys    = []string{"alt", "long", "details", "decorative", "provenance", "model", "sha256", "prompt", "drafted", "rested_on", "reviewed"}
	approvalKeys = []string{"upload", "approved", "command"}
)

// Entry is one figure's description.
type Entry struct {
	Key  string // the figure's name: its file in figs/ without the extension
	Alt  string
	Long string // the short description (aria-describedby; decisions 110, 125)
	// Details are the optional longer description (decision 125), shown on demand in HTML and EPUB
	// only; an entry with details has a short description.
	Details    string
	Decorative bool // the figure is decoration: alt="" and skipped by screen readers
	Provenance string
	Model      string // the drafting model's name, for a draft
	SHA256     string // the figure the text describes
	Prompt     string // the prompt version a draft was made with
	Drafted    string // dates, YYYY-MM-DD
	Reviewed   string
	// RestedOn is what a draft was made from (decision 123): ipe, trace, bitmap or image; empty in
	// a draft from before it was recorded.
	RestedOn string
}

// Approval is the author's recorded consent to send this paper's figures off the machine.
type Approval struct {
	Upload   bool
	Approved string
	Command  string
}

// Sidecar is a read sidecar: its entries and the parsed file, kept so an edit changes only the
// lines it must and the author's comments and order survive.
type Sidecar struct {
	Path     string
	Entries  map[string]*Entry
	Approval Approval
	doc      *document
}

// Read reads the sidecar at path; a missing file is an empty sidecar.
func Read(path string) (*Sidecar, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Sidecar{Path: path, Entries: map[string]*Entry{}, doc: &document{}}, nil
	}
	if err != nil {
		return nil, err
	}
	sidecar, err := Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	sidecar.Path = path
	return sidecar, nil
}

// Exists tells whether the sidecar file is on disk.
func (s *Sidecar) Exists() bool {
	_, err := os.Stat(s.Path)
	return err == nil
}

// Parse reads a sidecar's text.
func Parse(text string) (*Sidecar, error) {
	doc, err := parseTOML(text)
	if err != nil {
		return nil, err
	}
	sidecar := &Sidecar{Entries: map[string]*Entry{}, doc: doc}
	for _, table := range doc.sections {
		switch {
		case len(table.path) == 1 && table.path[0] == "approval":
			err = sidecar.readApproval(table)
		case len(table.path) == 2 && table.path[0] == figuresTable:
			err = sidecar.readEntry(table)
		default:
			err = lineError(table.start, "unknown table ["+displayPath(table.path)+"]; entries are [figs.NAME]")
		}
		if err != nil {
			return nil, err
		}
	}
	return sidecar, nil
}

func (s *Sidecar) readEntry(table *section) error {
	entry := &Entry{Key: table.path[1], Provenance: Author}
	targets := map[string]*string{"alt": &entry.Alt, "long": &entry.Long, "details": &entry.Details, "provenance": &entry.Provenance, "model": &entry.Model,
		"sha256": &entry.SHA256, "prompt": &entry.Prompt, "drafted": &entry.Drafted, "rested_on": &entry.RestedOn, "reviewed": &entry.Reviewed}
	for _, key := range table.order {
		value := table.fields[key]
		if key == "decorative" {
			flag, ok := value.value.(bool)
			if !ok {
				return lineError(value.start, "decorative must be true or false")
			}
			entry.Decorative = flag
			continue
		}
		target, known := targets[key]
		if !known {
			return lineError(value.start, "unknown key "+quoteString(key)+"; an entry takes "+strings.Join(entryKeys, ", "))
		}
		text, isString := value.value.(string)
		if !isString {
			return lineError(value.start, quoteString(key)+" must be a string")
		}
		*target = text
	}
	if err := checkEntry(entry, table); err != nil {
		return err
	}
	s.Entries[entry.Key] = entry
	return nil
}

// checkEntry checks what an entry's keys say together: a known provenance, and details only beside
// the short description they extend (decision 125).
func checkEntry(entry *Entry, table *section) error {
	if !slices.Contains([]string{Author, Model, Reviewed}, entry.Provenance) {
		return lineError(table.fields["provenance"].start, `provenance is "author", "model" or "reviewed"`)
	}
	if Normalize(entry.Details) != "" && Normalize(entry.Long) == "" {
		return lineError(table.fields["details"].start, "details extend the short description: an entry with details also has long")
	}
	return nil
}

func (s *Sidecar) readApproval(table *section) error {
	for _, key := range table.order {
		value := table.fields[key]
		switch key {
		case "upload":
			flag, ok := value.value.(bool)
			if !ok {
				return lineError(value.start, "upload must be true or false")
			}
			s.Approval.Upload = flag
		case "approved", "command":
			text, ok := value.value.(string)
			if !ok {
				return lineError(value.start, quoteString(key)+" must be a string")
			}
			if key == "approved" {
				s.Approval.Approved = text
			} else {
				s.Approval.Command = text
			}
		default:
			return lineError(value.start, "unknown key "+quoteString(key)+"; [approval] takes "+strings.Join(approvalKeys, ", "))
		}
	}
	return nil
}

// Keys are the entries' names, sorted.
func (s *Sidecar) Keys() []string {
	keys := make([]string, 0, len(s.Entries))
	for key := range s.Entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// BySHA is an entry with drafted or reviewed text for the figure identity (its SHA-256, page and
// box, decision 123) other than key; nil when there is none. A renamed figure, or another view of
// the same page, reuses it instead of being sent again; another page or box does not.
func (s *Sidecar) BySHA(identity, key string) *Entry {
	for _, name := range s.Keys() {
		entry := s.Entries[name]
		if name != key && entry.SHA256 == identity && entry.Alt != "" && entry.Provenance != Author {
			return entry
		}
	}
	return nil
}
