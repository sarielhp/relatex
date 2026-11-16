package reading

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/evidence"
	"github.com/sarielhp/relatex/internal/mathml"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/xmltree"
)

// mathRows is MathRows: row identities are occurrence-scoped paths into retained,
// unmodified MathML.
type mathRows struct {
	math           map[any]rec.Map
	classes        map[any]string
	gatherFormulas []any
	rows           []rec.Map
}

func newMathRows(records []rec.Map, occurrences []any) *mathRows {
	m := &mathRows{math: map[any]rec.Map{}, classes: map[any]string{}}
	for _, record := range records {
		if rec.Fetch(record, "kind") == "math" {
			m.math[rec.Fetch(record, "formula")] = record
		}
	}
	// Older streams lack gather row evidence and keep their unresolved result.
	for _, record := range records {
		environment := record["math_environment"]
		if record["kind"] == "label" && (environment == "gather" || environment == "gather*") && truthy(record["math_row"]) {
			m.gatherFormulas = append(m.gatherFormulas, rec.Fetch(record, "formula"))
		}
	}
	m.rows = []rec.Map{}
	for _, occurrence := range occurrences {
		m.rows = append(m.rows, m.rowsFor(rec.AsMap(occurrence))...)
	}
	return m
}

func (m *mathRows) rowList() []any {
	result := make([]any, len(m.rows))
	for index, row := range m.rows {
		result[index] = row
	}
	return result
}

// target is MathRows#target: the row a label names inside block ({} when it names the whole
// formula); ok is false where Ruby returns nil.
func (m *mathRows) target(label, block rec.Map) (rec.Map, bool) {
	environment, _ := label["math_environment"].(string)
	if !truthy(label["math_environment"]) || environment == "equation" || environment == "equation*" {
		return rec.Map{}, true
	}
	if !slices.Contains([]string{"align", "align*", "gather", "gather*", "eqnarray", "eqnarray*"}, environment) || !truthy(label["math_row"]) {
		return nil, false
	}
	class, known := m.classes[rec.Fetch(label, "formula")]
	if !known || class != strings.TrimSuffix(environment, "*") {
		return nil, false
	}
	var occurrence any
	found := false
	for _, span := range content(block) {
		if rec.Equal(span["formula"], rec.Fetch(label, "formula")) {
			occurrence, found = rec.Fetch(span, "occurrence"), true
			break
		}
	}
	if !found {
		rec.Fail("undefined method 'fetch' for nil")
	}
	var row rec.Map
	for _, item := range m.rows {
		if rec.Equal(rec.Fetch(item, "occurrence"), occurrence) && rec.Equal(rec.Fetch(item, "index"), rec.Fetch(label, "math_row")) {
			row = item
			break
		}
	}
	number := evidence.LiteralNumber(rec.FetchString(label, "number"))
	if row == nil || number == nil {
		return nil, false
	}
	// LuaMML drops a hyphen from a tag's text (\tag{I-RI1} reads (IRI1)): compare without hyphens.
	tag := plainTag(toS(rec.Fetch(row, "tag")))
	if plainTag(*number) != tag && plainTag("("+*number+")") != tag {
		return nil, false
	}
	return rec.Map{"row": rec.Fetch(row, "id")}, true
}

var hyphens = strings.NewReplacer("-", "", "−", "", "‐", "")

func plainTag(text string) string { return hyphens.Replace(text) }

func (m *mathRows) rowsFor(occurrence rec.Map) []rec.Map {
	formula := rec.Fetch(occurrence, "formula")
	record, ok := m.math[formula]
	if !ok {
		panic(rec.Failure{Message: fmt.Sprintf("key not found: %v", formula), Lookup: true})
	}
	root, err := xmltree.Parse(rec.FetchString(record, "mathml"))
	if err != nil {
		rec.Fail("%v", err)
	}
	elements := root.Elements()
	if len(elements) != 1 {
		return nil
	}
	table := elements[0]
	family := strings.TrimSuffix(table.Attrs["class"], "*")
	if table.Name != "mtable" || (family != "align" && family != "gather" && family != "eqnarray") {
		return nil
	}
	if family == "gather" && !slices.ContainsFunc(m.gatherFormulas, func(id any) bool { return rec.Equal(id, formula) }) {
		return nil
	}
	m.classes[formula] = family
	result := []rec.Map{}
	for index, row := range table.Named("mtr") {
		cells := row.Elements()
		if len(cells) == 0 {
			rec.Fail("undefined method 'attributes' for nil")
		}
		tag := cells[0]
		// The row's tag is its label cell's text, as printed and as the document validation reads it:
		// math in a tag keeps its minus signs (\tag{use $c_{1-s}$}, second cohort 511). A label
		// matches it without hyphens or minus signs (plainTag).
		var text any
		if tag.Attrs["intent"] == ":equation-label" {
			text = labelCellText(tag)
		}
		id := rec.Fetch(occurrence, "id")
		result = append(result, rec.Map{"id": fmt.Sprintf("%s.r%d", toS(id), index+1), "occurrence": id, "formula": formula,
			"index": int64(index + 1), "mathml_path": fmt.Sprintf("/math/mtable/mtr[%d]", index+1), "tag": text})
	}
	return result
}

// labelCellText is the text of a row's label cell: its mtext children and the glyphs of the
// formulas its tag typesets (mrow class="tag-formula", tex/relatex-external-equations.lua), joined
// and stripped; a no-break space at a run's edge (MathML trims token edges) reads as a space.
func labelCellText(cell *xmltree.Node) string {
	var builder strings.Builder
	for _, child := range cell.Elements() {
		if child.Name == "mtext" || (child.Name == "mrow" && child.Attrs["class"] == mathml.TagFormulaClass) {
			builder.WriteString(child.Content())
		}
	}
	return rec.Strip(strings.ReplaceAll(builder.String(), "\u00a0", " "))
}
