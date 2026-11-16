package figdesc

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

// The bulk review (decision 112, the author's decision 3 of 2026-11-02): the review page lets the
// author accept or edit each figure's description and saves the decisions as a review file; relatex
// alt import takes it back into the sidecar. Only what the author accepted or edited changes: an
// accepted draft becomes reviewed, an edited text the author's own. The file records what the page
// showed, so a decision about a text that has changed in the sidecar since is refused, not applied
// to the new text.

// ReviewTable heads a review file; it is not a sidecar table, so a review file read as the sidecar
// is refused instead of being taken for the author's descriptions.
const ReviewTable = "review"

// ReviewKeys are the settings of a figure in a review file.
var ReviewKeys = []string{"alt", "long", "details", "shown", "shown_long", "shown_details", "sha256"}

// Decision is one figure the author accepted or edited on the review page: the text now, the text
// the page showed, and the figure's bytes.
type Decision struct {
	Key, Alt, Long, Details, Shown, ShownLong, ShownDetails, SHA256 string
}

// Edited tells whether the author changed the text the page showed.
func (d Decision) Edited() bool {
	return Normalize(d.Alt) != Normalize(d.Shown) || Normalize(d.Long) != Normalize(d.ShownLong) ||
		Normalize(d.Details) != Normalize(d.ShownDetails)
}

// ReadReview reads a review file's decisions, in file order.
func ReadReview(path string) ([]Decision, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := parseTOML(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	decisions := []Decision{}
	if !slices.ContainsFunc(doc.sections, func(table *section) bool { return slices.Equal(table.path, []string{ReviewTable}) }) {
		return nil, fmt.Errorf("%s is not a review file saved from the review page (it has no [%s] table)", path, ReviewTable)
	}
	for _, table := range doc.sections {
		switch {
		case len(table.path) == 1 && table.path[0] == ReviewTable:
		case len(table.path) == 2 && table.path[0] == figuresTable:
			decision, err := readDecision(table)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			decisions = append(decisions, decision)
		default:
			return nil, fmt.Errorf("%s: %w", path, lineError(table.start, "unknown table ["+displayPath(table.path)+"] in a review file"))
		}
	}
	return decisions, nil
}

func readDecision(table *section) (Decision, error) {
	decision := Decision{Key: table.path[1]}
	targets := map[string]*string{"alt": &decision.Alt, "long": &decision.Long, "details": &decision.Details, "shown": &decision.Shown,
		"shown_long": &decision.ShownLong, "shown_details": &decision.ShownDetails, "sha256": &decision.SHA256}
	for _, key := range table.order {
		target, known := targets[key]
		text, isString := table.fields[key].value.(string)
		if !known || !isString {
			return decision, lineError(table.fields[key].start, "a reviewed figure takes the strings "+strings.Join(ReviewKeys, ", "))
		}
		*target = text
	}
	return decision, nil
}

// Outcome is what an import did to each figure.
type Outcome struct {
	Accepted, Edited, Unchanged []string
	Refused                     map[string]string // figure → why its decision was not applied
}

// Import applies the author's decisions on date: a draft accepted as shown becomes reviewed, a text
// the author edited (or wrote for a figure without one) becomes the author's, an accepted text that
// is already the author's or reviewed stays as it is. A decision about a text that is no longer the
// one the page showed, about another version of the figure, or that leaves no description, is refused.
func (s *Sidecar) Import(decisions []Decision, date string) (Outcome, error) {
	outcome := Outcome{Refused: map[string]string{}}
	for _, decision := range decisions {
		if reason := s.stale(decision); reason != "" {
			outcome.Refused[decision.Key] = reason
			continue
		}
		var err error
		switch entry := s.Entries[decision.Key]; {
		case decision.Edited():
			err = s.Adopt(Entry{Key: decision.Key, Alt: decision.Alt, Long: decision.Long, Details: decision.Details, SHA256: decision.SHA256}, date)
			outcome.Edited = append(outcome.Edited, decision.Key)
		case entry != nil && entry.Provenance == Model:
			_, err = s.Accept(decision.Key, date)
			outcome.Accepted = append(outcome.Accepted, decision.Key)
		default:
			outcome.Unchanged = append(outcome.Unchanged, decision.Key)
		}
		if err != nil {
			return outcome, err
		}
	}
	return outcome, nil
}

// stale is why a decision cannot be applied, "" when it can.
func (s *Sidecar) stale(decision Decision) string {
	entry := s.Entries[decision.Key]
	current := Entry{}
	if entry != nil {
		current = *entry
	}
	switch {
	case Normalize(decision.Alt) == "":
		return "no description: an empty text is not accepted (mark a decorative figure in " + FileName + ")"
	case Normalize(decision.Details) != "" && Normalize(decision.Long) == "":
		return "details without a short description: details extend it (decision 125)"
	case Normalize(current.Alt) != Normalize(decision.Shown) || Normalize(current.Long) != Normalize(decision.ShownLong) ||
		Normalize(current.Details) != Normalize(decision.ShownDetails):
		return "its text in " + FileName + " changed after the review page was written; review it again"
	case entry != nil && entry.SHA256 != "" && decision.SHA256 != "" && entry.SHA256 != decision.SHA256:
		return "its entry describes another version of the figure; review it again"
	case entry == nil && !decision.Edited():
		return "no description was accepted"
	}
	return ""
}

// Adopt writes the author's own text for entry.Key on date: its alt and long description, the
// figure's bytes when the entry records none, provenance "author" and the review date. A draft's
// model and drafting date stay as its history.
func (s *Sidecar) Adopt(entry Entry, date string) error {
	if s.table(figuresTable, entry.Key) == nil {
		entry.Provenance, entry.Reviewed = Author, date
		entry.Alt, entry.Long, entry.Details = Normalize(entry.Alt), Normalize(entry.Long), Normalize(entry.Details)
		s.appendLines(renderEntry(entry))
		return s.reparse()
	}
	if err := s.SetTexts(entry.Key, map[string]string{"alt": entry.Alt, "long": entry.Long, "details": entry.Details}); err != nil {
		return err
	}
	if existing := s.Entries[entry.Key]; existing.SHA256 == "" && entry.SHA256 != "" {
		table := s.table(figuresTable, entry.Key)
		end := fieldsEnd(table)
		s.replace(end, end, []string{"sha256 = " + quoteString(entry.SHA256)})
		if err := s.reparse(); err != nil {
			return err
		}
	}
	return s.mark(entry.Key, Author, date)
}
