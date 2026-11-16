package authorhints

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// TablesSection keys a table's header rows and columns by its \label or caption number (decision
// 131). A comment gives them as one string, `%% relatex tables: tab:a = "header_rows=1 header_columns=1"`;
// relatex/hints.json as an object, {"tables": {"tab:a": {"header_rows": 1, "header_columns": 1}}}.
// Both are kept in the comment form's canonical text, so a conflict between them is found as for
// any other hint.
const TablesSection = "tables"

// Table header fields: how many leading rows are column headers and how many leading columns are
// row headers.
const (
	HeaderRowsField    = "header_rows"
	HeaderColumnsField = "header_columns"
)

// TableHint is the author's word on a table's header cells; a field the hint leaves out is nil,
// and the table's typesetting decides it.
type TableHint struct {
	Rows, Columns *int
}

// Table is the hint for a table key (a label or a caption number), if the author gave a valid one.
func (h *Hints) Table(key string) (TableHint, bool) {
	hint, ok := h.Get(TablesSection, key)
	if !ok {
		return TableHint{}, false
	}
	parsed, err := ParseTableHint(hint.Value)
	return parsed, err == nil
}

// ParseTableHint reads "header_rows=N header_columns=N" (either may be left out; fields are
// separated by spaces or commas); N is a count from 0.
func ParseTableHint(value string) (TableHint, error) {
	hint := TableHint{}
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' })
	if len(fields) == 0 {
		return hint, errors.New("a table hint names header_rows or header_columns")
	}
	for _, field := range fields {
		name, text, found := strings.Cut(field, "=")
		count, err := strconv.Atoi(text)
		if !found || err != nil || count < 0 {
			return hint, fmt.Errorf("%q is not name=count", field)
		}
		switch name {
		case HeaderRowsField:
			hint.Rows = &count
		case HeaderColumnsField:
			hint.Columns = &count
		default:
			return hint, fmt.Errorf("unknown table hint field %q", name)
		}
	}
	return hint, nil
}

// tableValue is a hints.json table entry ({"header_rows": 1}) in the comment form's canonical text.
func tableValue(key string, value any) (string, error) {
	entry, ok := value.(rec.Map)
	if !ok {
		return "", errors.New("relatex/hints.json table hint " + key + " is not an object")
	}
	names := make([]string, 0, len(entry))
	for name := range entry {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		count, ok := entry[name].(int64)
		if !ok {
			return "", errors.New("relatex/hints.json table hint " + key + " field " + name + " is not a count")
		}
		parts = append(parts, name+"="+strconv.FormatInt(count, 10))
	}
	text := strings.Join(parts, " ")
	if _, err := ParseTableHint(text); err != nil {
		return "", errors.New("relatex/hints.json table hint " + key + ": " + err.Error())
	}
	return text, nil
}
