package a11ylint

import (
	"regexp"
	"strings"
)

// Layout tabulars (decision 111): a table of one row or one column is presumed a layout by the
// conversion, and the lint gives the same judgement to a tabular whose every content cell holds
// a picture: a grid of figures needs no header row. A tabular in the body of a command the files
// define is a definition, not a table (it is expanded where the command is used).

var (
	pictureCommand = regexp.MustCompile(`\\(?:includegraphics|includesvg)\b`)
	// cellSpacing is what a cell may hold besides its content and still say nothing: spacing, row
	// ends and rules, with their arguments.
	cellSpacing = regexp.MustCompile(`\\(?:quad|qquad|hfill|hss|vfill)\b|\\[hv]space\*?\{[^{}]*\}` +
		`|\\\\(?:\[[^\]]*\])?|~(?:\[[^\]]*\])?` +
		`|\\(?:hline|midrule|toprule|bottomrule|addlinespace)(?:\[[^\]]*\])?` +
		`|\\(?:cline|cmidrule)(?:\([^)]*\))?\{[^{}]*\}`)
	// cellWrapper is the environment and box around a cell's content (a label in a minipage),
	// with the environment's arguments.
	cellWrapper = regexp.MustCompile(`\\(?:begin|end)\s*\{[^{}]*\}(?:\s*(?:\[[^\]]*\]|\{[^{}]*\}))*` +
		`|\\[hv]space\*?\{[^{}]*\}`)
	// cellLabel is a short label such as (A), (i) or (iv) that names the picture beside it.
	cellLabel = regexp.MustCompile(`\A\([A-Za-z]{1,4}\)\z`)
)

// pictureDepth bounds how many command definitions a cell is followed through for a picture.
const pictureDepth = 4

// The kinds of a table row for pictureGrid.
const (
	rowLayout   = iota // spacing, labels and connecting arrows only
	rowPictures        // every content cell a picture or a panel
	rowMixed           // pictures or panels beside text
	rowText            // no picture
)

// pictureGrid: the tabular only arranges figures. Every row's content cells hold a picture, or a
// panel (a nested table), with the short labels under them ((A), (i)), spacing, rules and arrows
// between pictures saying nothing (decision 111). Decision 167 adds the panel layouts of figures: a
// row of text directly under a row of pictures is their captions, and a row that sets pictures or
// panels beside text is a row of panels when a row of labels names them beneath. A row of text
// anywhere else keeps the table.
func pictureGrid(rows []tableRow, defined map[string]macro) bool {
	shown := false
	previous := rowText
	for index, row := range rows {
		kind := rowKindOf(row.text, defined)
		switch {
		case kind == rowPictures:
			shown = true
		case kind == rowLayout:
		case kind == rowText && previous == rowPictures:
		case kind == rowMixed && index+1 < len(rows) && labelRow(rows[index+1].text):
			shown = true
		default:
			return false
		}
		previous = kind
	}
	return shown
}

// itemLabel is a list item's label at the start of a cell: a), (b), iv).
var itemLabel = regexp.MustCompile(`\A\(?(?:[a-z]|[ivx]{1,4})\)`)

// itemGrid: every content cell starts with a list item's label (a), b), …), so the tabular sets a
// list of exercises or cases in columns, a layout with no header (decision 167); at least two.
func itemGrid(rows []tableRow) bool {
	items := 0
	for _, row := range rows {
		for _, cell := range splitTop(row.text, '&') {
			if layoutCell(cell) {
				continue
			}
			if !itemLabel.MatchString(strings.TrimSpace(cell)) {
				return false
			}
			items++
		}
	}
	return items > 1
}

// rowKindOf is a row's kind: what its content cells hold.
func rowKindOf(text string, defined map[string]macro) int {
	pictures, others := 0, 0
	for _, cell := range splitTop(text, '&') {
		switch {
		case layoutCell(cell):
		case cellPicture(cell, defined, pictureDepth) || nestedTable.MatchString(cell):
			pictures++
		default:
			others++
		}
	}
	switch {
	case pictures == 0 && others == 0:
		return rowLayout
	case others == 0:
		return rowPictures
	case pictures > 0:
		return rowMixed
	}
	return rowText
}

// labelRow: the row's cells are the short labels of panels ((i), (ii), (iii)), at least one of them.
func labelRow(text string) bool {
	labels := 0
	for _, cell := range splitTop(text, '&') {
		if !layoutCell(cell) {
			return false
		}
		if cellLabel.MatchString(strings.Join(strings.Fields(strings.NewReplacer("{", "", "}", "").Replace(cell)), "")) {
			labels++
		}
	}
	return labels > 0
}

var (
	// nestedTable is a whole table in a cell: a panel of a figure, linted on its own.
	nestedTable = regexp.MustCompile(`\\begin\s*\{(?:tabular\*?|tabularx|tabulary|tblr)\}`)
	// connector is a formula that only joins pictures: arrows, struts and spacing ($\Rightarrow$, $\bigr.$).
	connector = regexp.MustCompile(`\A\$(?:\\[A-Za-z]*(?:arrow|Arrow|mapsto)\b|\\(?:big|Big|bigg|Bigg)[lrm]?\.|\\[,;:! ]|\\quad|\\qquad|\s)+\$\z`)
)

// layoutCell: the cell holds only spacing, row ends, rules, the box around a label, the label
// itself, or an arrow joining pictures; it carries no content of its own.
func layoutCell(cell string) bool {
	rest := cellSpacing.ReplaceAllString(cell, " ")
	rest = cellWrapper.ReplaceAllString(rest, " ")
	rest = strings.NewReplacer("{", "", "}", "").Replace(rest)
	rest = strings.Join(strings.Fields(rest), "")
	return rest == "" || cellLabel.MatchString(rest) || connector.MatchString(rest)
}

// cellPicture: the cell holds \includegraphics or \includesvg, directly or through a command the
// files define, up to depth levels.
func cellPicture(cell string, defined map[string]macro, depth int) bool {
	if pictureCommand.MatchString(cell) {
		return true
	}
	if depth <= 0 {
		return false
	}
	for _, name := range innerCommands(cell, defined, "") {
		for _, use := range calls(cell, name, defined[name]) {
			if cellPicture(substitute(defined[name].body, use.arguments), defined, depth-1) {
				return true
			}
		}
	}
	return false
}
