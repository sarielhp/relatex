package figdesc

import (
	"fmt"
	"github.com/sarielhp/relatex/internal/authordir"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// fileHeader opens a sidecar relatex creates.
const fileHeader = `# Figure descriptions (text alternatives) for relatex: one [figs.NAME] entry per figure, NAME
# being its file in figs/ without the extension. See "Figure descriptions" in the relatex guide.
# provenance: "author" (your own text; the default when absent), "model" (a draft nobody has
# reviewed yet), "reviewed" (a draft a person accepted). relatex never changes an author or
# reviewed entry; edit the alt text freely, then accept it with: relatex alt accept NAME`

// PutDraft writes a model draft for entry.Key: a new entry, or the replacement of an earlier
// draft. An entry the author wrote or reviewed is never replaced.
func (s *Sidecar) PutDraft(entry Entry) error {
	if existing, ok := s.Entries[entry.Key]; ok && existing.Provenance != Model {
		return fmt.Errorf("figure %s has an entry by the author (%s); it is kept", entry.Key, existing.Provenance)
	}
	entry.Provenance = Model
	rendered := renderEntry(entry)
	if table := s.table(figuresTable, entry.Key); table != nil {
		s.replace(table.start, fieldsEnd(table), rendered)
	} else {
		s.appendLines(rendered)
	}
	return s.reparse()
}

// Accept marks a model draft reviewed on date: its text becomes the author's. An entry that is
// already the author's or reviewed is left as it is (accepted reports false).
func (s *Sidecar) Accept(key, date string) (accepted bool, err error) {
	entry, ok := s.Entries[key]
	if !ok {
		return false, fmt.Errorf("no entry for figure %s in %s", key, FileName)
	}
	if entry.Provenance != Model {
		return false, nil
	}
	if strings.TrimSpace(entry.Alt) == "" {
		return false, fmt.Errorf("figure %s has no alt text to accept", key)
	}
	return true, s.mark(key, Reviewed, date)
}

// mark sets key's provenance and its review date: the provenance line is replaced (or added after
// the entry's settings) by the provenance and the date, and an earlier review date goes.
func (s *Sidecar) mark(key, provenance, date string) error {
	table := s.table(figuresTable, key)
	marked := []string{"provenance = " + quoteString(provenance), "reviewed = " + quoteString(date)}
	line, has := table.fields["provenance"]
	if !has {
		end := fieldsEnd(table)
		line = field{start: end, end: end}
	}
	if existing, has := table.fields["reviewed"]; has {
		// An earlier review date goes; the new one follows the provenance line.
		s.replace(existing.start, existing.end, nil)
		if existing.start < line.start {
			shift := existing.end - existing.start
			line.start, line.end = line.start-shift, line.end-shift
		}
	}
	s.replace(line.start, line.end, marked)
	return s.reparse()
}

// Set writes the text of one field of key's entry, alt, long (decision 110) or details (decision
// 125), keeping the rest of the entry, its comments and its provenance (a model draft stays a draft
// until accepted); an empty text removes a long description or details. A figure without an entry
// gets one by the author. Details need a short description (long) beside them.
func (s *Sidecar) Set(key, field, text string) error {
	if !slices.Contains([]string{"alt", "long", "details"}, field) {
		return fmt.Errorf("an entry's text is alt, long or details, not %s", field)
	}
	text = Normalize(text)
	if err := s.checkSet(key, field, text); err != nil {
		return err
	}
	table := s.table(figuresTable, key)
	switch {
	case table == nil && text == "":
		return nil
	case table == nil:
		entry := Entry{Key: key, Provenance: Author}
		if field == "alt" {
			entry.Alt = text
		} else {
			entry.Long = text // details are refused above without an entry
		}
		s.appendLines(renderEntry(entry))
	default:
		line := []string{field + " = " + quoteString(text)}
		if text == "" {
			line = nil
		}
		if existing, has := table.fields[field]; has {
			s.replace(existing.start, existing.end, line)
		} else {
			end := fieldsEnd(table)
			s.replace(end, end, line)
		}
	}
	return s.reparse()
}

// SetTexts sets several texts of key's entry (alt, long, details) with Set, in an order that never
// leaves details without the short description they extend: details go first when they are
// removed and last when they are written.
func (s *Sidecar) SetTexts(key string, texts map[string]string) error {
	order := []string{"alt", "long", "details"}
	if details, has := texts["details"]; has && Normalize(details) == "" {
		order = []string{"details", "alt", "long"}
	}
	for _, field := range order {
		text, has := texts[field]
		if !has {
			continue
		}
		if err := s.Set(key, field, text); err != nil {
			return err
		}
	}
	return nil
}

// checkSet refuses a Set that would leave details without the short description they extend.
func (s *Sidecar) checkSet(key, field, text string) error {
	entry := s.Entries[key]
	switch {
	case field == "details" && text != "" && (entry == nil || Normalize(entry.Long) == ""):
		return fmt.Errorf("figure %s has no short description (long); details extend it, so write long first", key)
	case field == "long" && text == "" && entry != nil && Normalize(entry.Details) != "":
		return fmt.Errorf("figure %s has details, which extend its short description; remove them first (details \"\")", key)
	}
	return nil
}

// RecordApproval writes the [approval] table: the author's consent to send figures off the machine.
func (s *Sidecar) RecordApproval(approval Approval) error {
	lines := []string{"[approval]", "# You approved sending this paper's figures to the drafting command's service.",
		"upload = " + fmt.Sprint(approval.Upload), "approved = " + quoteString(approval.Approved),
		"command = " + quoteString(approval.Command)}
	if table := s.table("approval"); table != nil {
		s.replace(table.start, fieldsEnd(table), lines)
	} else if len(s.doc.sections) > 0 {
		s.replace(s.doc.sections[0].start, s.doc.sections[0].start, append(lines, ""))
	} else {
		s.appendLines(lines)
	}
	return s.reparse()
}

// Bytes is the sidecar's text, with the line ends the author's file had (decision 129).
func (s *Sidecar) Bytes() []byte {
	text := strings.Join(s.doc.lines, "\n")
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if s.doc.crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	return []byte(text)
}

// Write saves the sidecar, replacing the file in one step so an interrupted write never leaves
// half a file. relatex/ is created on the first write there (decision 124).
func (s *Sidecar) Write() error {
	if err := authordir.Ensure(s.Path); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.Path), ".relatex-alt-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(s.Bytes()); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporary.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), s.Path)
}

func renderEntry(entry Entry) []string {
	lines := []string{"[" + displayPath([]string{figuresTable, entry.Key}) + "]", "alt = " + quoteString(entry.Alt)}
	if entry.Long != "" {
		lines = append(lines, "long = "+quoteString(entry.Long))
	}
	if entry.Details != "" {
		lines = append(lines, "details = "+quoteString(entry.Details))
	}
	lines = append(lines, "provenance = "+quoteString(entry.Provenance))
	for _, pair := range [][2]string{{"model", entry.Model}, {"sha256", entry.SHA256}, {"prompt", entry.Prompt},
		{"drafted", entry.Drafted}, {"rested_on", entry.RestedOn}, {"reviewed", entry.Reviewed}} {
		if pair[1] != "" {
			lines = append(lines, pair[0]+" = "+quoteString(pair[1]))
		}
	}
	return lines
}

// table is the section with this header path, or nil.
func (s *Sidecar) table(path ...string) *section {
	for _, table := range s.doc.sections {
		if slices.Equal(table.path, path) {
			return table
		}
	}
	return nil
}

// fieldsEnd is the line after a section's last setting: the comments and blank lines after it
// stay with whatever follows.
func fieldsEnd(table *section) int {
	end := table.start + 1
	for _, value := range table.fields {
		end = max(end, value.end)
	}
	return end
}

func (s *Sidecar) replace(start, end int, lines []string) {
	s.doc.lines = slices.Concat(s.doc.lines[:start:start], lines, s.doc.lines[end:])
}

func (s *Sidecar) appendLines(lines []string) {
	existing := s.doc.lines
	for len(existing) > 0 && strings.TrimSpace(existing[len(existing)-1]) == "" {
		existing = existing[:len(existing)-1]
	}
	if len(existing) == 0 {
		existing = strings.Split(fileHeader, "\n")
	}
	s.doc.lines = slices.Concat(existing, []string{""}, lines, []string{""})
}

func (s *Sidecar) reparse() error {
	parsed, err := Parse(strings.Join(s.doc.lines, "\n"))
	if err != nil {
		return fmt.Errorf("relatex wrote an unreadable %s: %w", FileName, err)
	}
	parsed.Path = s.Path
	parsed.doc.crlf = s.doc.crlf
	*s = *parsed
	return nil
}
