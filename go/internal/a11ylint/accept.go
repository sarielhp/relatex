package a11ylint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/authordir"
	"github.com/sarielhp/relatex/internal/figdesc"
)

// The review page's decisions drive the patch, as a figure's do its sidecar: the author ticks the
// suggestions they accept and the page saves them as DecisionsFile; relatex lint --patch --accept
// carries exactly those. An item is named by its file, the line its suggestion goes after and its
// kind, not its number in the lint, which moves when the lint changes; the sha of its suggestion
// and that line makes a decision about a line that has changed since the page was built refused.

// DecisionsFile is what the review page saves the author's decisions under (a download). Its
// [review] table makes a sidecar reader refuse it, so a decisions file saved over relatex/alt.toml
// describes nothing.
const DecisionsFile = authordir.Lint

const (
	decisionsTable = "review"
	acceptTable    = "accept"
)

// Decision is one item the author ticked: its stable id and the sha of what the page showed.
type Decision struct {
	ID, SHA string
}

// Refusal is a decision not applied: the item's id and why.
type Refusal struct {
	ID, Reason string
}

// Acceptance is what a decisions file selects: the items the patch carries, in the order decided,
// and the decisions refused.
type Acceptance struct {
	Items   []Item
	Refused []Refusal
}

// ReadDecisions reads a review page's saved decisions, in file order. The file must carry the
// [review] table and no other table than [[accept]]: a figure sidecar's means the wrong file.
func ReadDecisions(path string) ([]Decision, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reader := decisionReader{path: path}
	for number, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		if err := reader.line(number+1, line); err != nil {
			return nil, err
		}
	}
	if !reader.review {
		return nil, fmt.Errorf("%s is not a decisions file saved from the review page (it has no [%s] table)", path, decisionsTable)
	}
	for _, decision := range reader.decisions {
		if decision.ID == "" || decision.SHA == "" {
			return nil, fmt.Errorf("%s: an [[%s]] entry needs id and sha", path, acceptTable)
		}
	}
	return reader.decisions, nil
}

// decisionReader is ReadDecisions' state over the file's lines.
type decisionReader struct {
	path      string
	review    bool
	inAccept  bool
	decisions []Decision
}

func (reader *decisionReader) errorf(number int, format string, values ...any) error {
	return fmt.Errorf("%s:%d: %s", reader.path, number, fmt.Sprintf(format, values...))
}

// line takes one trimmed line.
func (reader *decisionReader) line(number int, line string) error {
	switch {
	case line == "["+decisionsTable+"]":
		if reader.review {
			return reader.errorf(number, "the [%s] table appears twice", decisionsTable)
		}
		reader.review, reader.inAccept = true, false
		return nil
	case line == "[["+acceptTable+"]]":
		reader.decisions = append(reader.decisions, Decision{})
		reader.inAccept = true
		return nil
	case line[0] == '[':
		return reader.tableError(number, strings.Trim(line, "[]"))
	}
	return reader.key(number, line)
}

// tableError refuses a table a decisions file does not have; a figure sidecar's table means the
// author saved the wrong file.
func (reader *decisionReader) tableError(number int, name string) error {
	if name == "figs" || strings.HasPrefix(name, "figs.") {
		return reader.errorf(number, "[%s] is a figure sidecar table: this is %s, not %s", name, DecisionsFile, figdesc.FileName)
	}
	return reader.errorf(number, "unknown table [%s] in a decisions file", name)
}

// key takes a key = "value" line: [review]'s settings (the save date) are not decisions; an
// [[accept]] entry has an id and a sha.
func (reader *decisionReader) key(number int, line string) error {
	name, value, found := strings.Cut(line, "=")
	name = strings.TrimSpace(name)
	switch {
	case !found:
		return reader.errorf(number, "expected key = \"value\"")
	case !reader.review && !reader.inAccept:
		return reader.errorf(number, "a key before any table; a decisions file starts with [%s]", decisionsTable)
	case !reader.inAccept:
		return nil
	}
	text, err := decisionString(strings.TrimSpace(value))
	if err != nil {
		return reader.errorf(number, "%s %v", name, err)
	}
	decision := &reader.decisions[len(reader.decisions)-1]
	switch name {
	case "id":
		decision.ID = text
	case "sha":
		decision.SHA = text
	default:
		return reader.errorf(number, "an accepted item takes the strings id, sha")
	}
	return nil
}

// decisionString is a TOML basic string; the page writes JSON strings, whose escapes TOML shares.
func decisionString(value string) (string, error) {
	if len(value) < 2 || value[0] != '"' {
		return "", fmt.Errorf("takes a quoted string")
	}
	text, err := strconv.Unquote(value)
	if err != nil {
		return "", fmt.Errorf("takes a quoted string")
	}
	return text, nil
}

// decisionID is an item's stable name on the review page: its file, the line its suggestion goes
// after (or replaces) and its kind.
func decisionID(item Item) string {
	return fmt.Sprintf("%s:%d:%s", item.File, item.NoteLine, item.Kind)
}

// decisionSHA is the page's data-sha over the item's suggestion and the source line it goes after
// (or replaces), so a decision about a line that has changed since is refused.
func decisionSHA(item Item, file *File) string {
	line := ""
	if file != nil && item.NoteLine > 0 {
		if lines, _ := sourceLines(file.Source); item.NoteLine <= len(lines) {
			line = strings.TrimSuffix(lines[item.NoteLine-1], "\r")
		}
	}
	sum := sha256.Sum256([]byte(item.Suggestion + line))
	return hex.EncodeToString(sum[:])
}

// Accept selects the items a decisions file accepts, in the order decided: an id that is not in the
// current lint, an item the patch cannot carry, and a sha that no longer matches are refused with
// the reason, never silently dropped.
func Accept(paper Paper, items []Item, decisions []Decision) Acceptance {
	files := make(map[string]*File, len(paper.Files))
	for _, file := range paper.Files {
		files[file.Path] = file
	}
	taken := make([]bool, len(items))
	acceptance := Acceptance{}
	for _, decision := range decisions {
		at := decisionIndex(items, taken, decision.ID)
		switch {
		case at < 0:
			acceptance.refuse(decision, "no item with this id is in the current lint (the source changed since the page was built)")
		case !patchable(items[at]):
			acceptance.refuse(decision, "the patch cannot carry this item: it has no suggestion with a place")
		case decisionSHA(items[at], files[items[at].File]) != decision.SHA:
			acceptance.refuse(decision, "its suggestion or the source line it goes after changed since the page was built; review it again")
		default:
			taken[at] = true
			acceptance.Items = append(acceptance.Items, items[at])
		}
	}
	return acceptance
}

// decisionIndex is the first item with the id that no earlier decision took, -1 when none has it.
func decisionIndex(items []Item, taken []bool, id string) int {
	for at, item := range items {
		if !taken[at] && decisionID(item) == id {
			return at
		}
	}
	return -1
}

func (acceptance *Acceptance) refuse(decision Decision, reason string) {
	acceptance.Refused = append(acceptance.Refused, Refusal{ID: decision.ID, Reason: reason})
}
