package formulatex

import (
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/xmltree"
)

// alignmentEnvironments are the display environments luamml names in its table's class, as the
// starred environments the formula TeX uses (a numbered row gets its number as \tag).
var alignmentEnvironments = map[string]string{"align": "align*", "gather": "gather*", "flalign": "flalign*",
	"alignat": "alignat*", "multline": "multline*", "eqnarray": "eqnarray*"}

// equationNumber is a printed equation number "(3.1)", whose inside \tag gives back.
var equationNumber = regexp.MustCompile(`\A\(([^()]*)\)\z`)

// alignment is a display that is one amsmath alignment: its environment and body.
func alignment(root *xmltree.Node) (string, string, bool) {
	elements := root.Elements()
	if len(elements) != 1 || elements[0].Name != "mtable" {
		return "", "", false
	}
	table := elements[0]
	environment, ok := alignmentEnvironments[table.Attrs["class"]]
	if !ok {
		return "", "", false
	}
	if environment == "alignat*" {
		fail("alignat")
	}
	rows := []string{}
	for _, tableRow := range table.Elements() {
		rows = append(rows, alignedRow(tableRow, environment))
	}
	return environment, strings.Join(rows, " \\\\\n"), true
}

// alignedRow is one row of an alignment: its cells joined by &, then its number.
func alignedRow(tableRow *xmltree.Node, environment string) string {
	if tableRow.Name != "mtr" {
		fail("alignment row %s", tableRow.Name)
	}
	cells := tableRow.Elements()
	tag := ""
	if len(cells) > 0 && cells[0].Attrs["intent"] == ":equation-label" {
		tag = rowTag(cells[0].Content())
		if tag != "" && environment == "eqnarray*" {
			fail("eqnarray number")
		}
		cells = cells[1:]
	}
	texts := make([]string, len(cells))
	for index, cell := range cells {
		switch environment {
		case "multline*":
			texts[index] = multlineCell(cell)
			continue
		case "eqnarray*":
			// eqnarray braces each cell: no empty group stands before a left-aligned one.
			texts[index] = sequence(cellElements(cell))
			continue
		}
		texts[index] = alignedCell(cell)
	}
	return strings.Join(texts, " & ") + tag
}

func rowTag(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	if match := equationNumber.FindStringSubmatch(label); match != nil {
		return " \\tag" + braced(textEscape(match[1]))
	}
	return " \\tag*" + braced(textEscape(label))
}

// alignedCell is a cell of an alignment, without the display style amsmath sets in every cell.
func alignedCell(cell *xmltree.Node) string {
	return sequenceAfter(cellElements(cell), cell.Attrs["columnalign"] == "left")
}

// multlineGap is \multlinegap (10 pt) in big points, the indent of multline's first and last rows.
const multlineGap = "9.963pt"

// multlineCell is a row of multline without the gap multline puts before the first row (aligned
// left) and after the last (aligned right) itself.
func multlineCell(cell *xmltree.Node) string {
	elements := cellElements(cell)
	last := len(elements) - 1
	switch align := cell.Attrs["columnalign"]; {
	case align == "left" && last >= 0 && isGap(elements[0]):
		elements = elements[1:]
	case align == "right" && last >= 0 && isGap(elements[last]):
		elements = elements[:last]
	}
	return sequenceAfter(elements, cell.Attrs["columnalign"] == "left")
}

func isGap(node *xmltree.Node) bool {
	return node.Name == "mspace" && len(node.Attrs) == 1 && node.Attrs["width"] == multlineGap
}

// cellElements are what a cell holds inside the rows and display style amsmath wraps it in.
func cellElements(cell *xmltree.Node) []*xmltree.Node {
	elements := cell.Elements()
	if len(elements) == 1 && elements[0].Name == "mrow" {
		elements = elements[0].Elements()
	}
	if len(elements) == 1 && elements[0].Name == "mstyle" && elements[0].Attrs["displaystyle"] == "true" {
		elements = elements[0].Elements()
	}
	if len(elements) == 1 && elements[0].Name == "mrow" && !isFenced(elements[0]) {
		elements = elements[0].Elements()
	}
	return elements
}

// matrixFences are the fences of amsmath's matrix environments.
var matrixFences = map[[2]string]string{{"(", ")"}: "pmatrix", {"[", "]"}: "bmatrix", {"{", "}"}: "Bmatrix",
	{"|", "|"}: "vmatrix", {"‖", "‖"}: "Vmatrix"}

// fencedMatrix is a matrix environment starting at index (fence, kern, table, kern, fence) and
// the number of elements it takes; cases (a brace, the table, a kern) too.
func fencedMatrix(elements []*xmltree.Node, index int) (string, int) {
	if index+3 < len(elements) && isFence(elements[index]) && elements[index].Content() == "{" &&
		elements[index+1].Name == "mtable" && elements[index+2].Name == "mspace" {
		return matrix(elements[index+1], "cases"), 3
	}
	if index+4 >= len(elements) || !isFence(elements[index]) || elements[index+1].Name != "mspace" ||
		elements[index+2].Name != "mtable" || elements[index+3].Name != "mspace" || !isFence(elements[index+4]) {
		return "", 0
	}
	name, ok := matrixFences[[2]string{elements[index].Content(), elements[index+4].Content()}]
	if !ok {
		return "", 0
	}
	return matrix(elements[index+2], name), 5
}

// bareMatrix: amsmath's matrix at index, a table between the two kerns that take back its outer
// column separation (-\arraycolsep).
func bareMatrix(elements []*xmltree.Node, index int) bool {
	if index+2 >= len(elements) || elements[index+1].Name != "mtable" || !isGlue(elements[index]) ||
		!isGlue(elements[index+2]) {
		return false
	}
	width := elements[index].Attrs["width"]
	return width == elements[index+2].Attrs["width"] && length(width) < 0
}

// arrayTable writes a table without matrix kerns as an array, its columns aligned as the table's
// are, or as \substack: one column of script-style rows.
func arrayTable(table *xmltree.Node) string {
	if stack, ok := substack(table); ok {
		return stack
	}
	columns := 0
	for _, tableRow := range table.Elements() {
		columns = max(columns, len(tableRow.Elements()))
	}
	aligns := strings.Fields(table.Attrs["columnalign"])
	if len(aligns) == 0 {
		aligns = []string{"center"}
	}
	var spec strings.Builder
	for column := range columns {
		letter, ok := columnLetters[aligns[min(column, len(aligns)-1)]]
		if !ok {
			fail("column alignment %s", aligns[min(column, len(aligns)-1)])
		}
		spec.WriteString(letter)
	}
	return strings.Replace(matrix(table, "array"), "\\begin{array}", "\\begin{array}{"+spec.String()+"}", 1)
}

var columnLetters = map[string]string{"left": "l", "center": "c", "right": "r"}

// matrix writes a table as an amsmath matrix environment.
func matrix(table *xmltree.Node, environment string) string {
	rows := []string{}
	for _, tableRow := range table.Elements() {
		if tableRow.Name != "mtr" {
			fail("table row %s", tableRow.Name)
		}
		cells := []string{}
		for _, cell := range tableRow.Elements() {
			cells = append(cells, sequence(cell.Elements()))
		}
		rows = append(rows, strings.Join(cells, " & "))
	}
	return "\\begin{" + environment + "}" + strings.Join(rows, " \\\\ ") + "\\end{" + environment + "}"
}

func substack(table *xmltree.Node) (string, bool) {
	if table.Attrs["columnalign"] != "center" {
		return "", false
	}
	rows := []string{}
	for _, tableRow := range table.Elements() {
		cells := tableRow.Elements()
		if len(cells) != 1 {
			return "", false
		}
		content := cells[0].Elements()
		if len(content) == 1 && content[0].Name == "mrow" {
			content = content[0].Elements()
		}
		if len(content) != 1 || content[0].Name != "mstyle" || content[0].Attrs["scriptlevel"] != "1" ||
			content[0].Attrs["displaystyle"] != "false" {
			return "", false
		}
		rows = append(rows, sequence(content[0].Elements()))
	}
	return "\\substack" + braced(strings.Join(rows, " \\\\ ")), true
}
