package a11ylint

import (
	"fmt"
	"regexp"
	"strings"
)

// Tables: relatex reads a table's header rows from its source as the conversion does (decision
// 088, go/internal/tablelayout/headers.go): longtable's \endhead, or the rows above a full-width
// rule under row 1 or 2 that sets them apart from the body (fewer than half of the body's row
// boundaries are ruled, or the head's rule is stronger: double or thicker). The LaTeX tagging
// project's \tagpdfsetup{table/header-rows={…}} (latex-lab-testphase-table, TeX Live 2026) declares
// them for the tagged PDF; the lint takes it as the author's declaration.

var (
	tabular       = regexp.MustCompile(`\\begin\s*\{(tabular\*?|tabularx|tabulary|longtable|tblr|longtblr)\}`)
	tableRule     = regexp.MustCompile(`\A\s*\\(hline|midrule|cmidrule|toprule|bottomrule|hhline|Xhline|specialrule|cline|thickhline|addlinespace|noalign)\b\*?`)
	headerSetting = regexp.MustCompile(`\\tagpdfsetup\s*\{[^{}]*\btable[/-]header-rows\s*=`)
	boldCell      = regexp.MustCompile(`\A\s*(?:\\rowcolor\s*(?:\[[^\]]*\])?\s*\{[^}]*\}\s*)?(?:\\multicolumn\s*\{[^}]*\}\s*\{[^}]*\}\s*\{\s*)?(?:\\textbf\b|\{\s*\\bf(?:series)?\b|\\bf(?:series)?\b)`)
)

// ruleWeights are the rules' thicknesses in points at 10 pt (\arrayrulewidth, booktabs' defaults);
// a rule absent here (cline, cmidrule, addlinespace) is partial or no rule.
var ruleWeights = map[string]float64{"hline": 0.4, "hhline": 0.4, "midrule": 0.5, "toprule": 0.8, "bottomrule": 0.8,
	"Xhline": 1, "specialrule": 1, "thickhline": 1}

// strongerRule is how much thicker a head rule must be to set its rows apart (tablelayout's).
const strongerRule = 1.2

// rowRule is the full-width rules at a row boundary: how many lines and the thickest.
type rowRule struct {
	lines     int
	thickness float64
	partial   bool // a \cline or \cmidrule: a rule under some of the columns
}

func (r rowRule) stronger(other rowRule) bool {
	return r.lines > other.lines || r.thickness > other.thickness*strongerRule
}

// tableRow is one row of a table's body: its text, where it ends (after its \\) and the rules
// under it.
type tableRow struct {
	text  string
	end   int
	after rowRule
}

// tableContext is what the notes need from the whole paper.
type tableContext struct {
	booktabs, metadata, declared bool             // booktabs loaded; \DocumentMetadata; a global header-rows setting
	macros                       map[string]macro // the commands the files define (layout.go)
}

func newTableContext(files []*File) tableContext {
	context := tableContext{macros: macros(files)}
	for index, file := range files {
		preamble := file.Text
		if match := beginDocument.FindStringIndex(preamble); match != nil {
			preamble = preamble[:match[0]]
		}
		context.metadata = context.metadata || index == 0 && documentMetadata.MatchString(file.Text)
		context.declared = context.declared || index == 0 && headerSetting.MatchString(preamble)
		for _, match := range packageUse.FindAllStringSubmatch(file.Text, -1) {
			context.booktabs = context.booktabs || strings.Contains(","+strings.ReplaceAll(match[1], " ", "")+",", ",booktabs,")
		}
	}
	return context
}

// tableItems: a table marks its header rows as the conversion reads them, or declares them with
// the tagging project's setting; otherwise the note names the row that looks like the header and
// the exact rule (and setting) to add.
func tableItems(file *File, context tableContext) []Item {
	items := []Item{}
	previous := 0
	for _, match := range tabular.FindAllStringSubmatchIndex(file.Text, -1) {
		name := file.Text[match[2]:match[3]]
		end := matchingEnd(file.Text[match[1]:], name)
		if end < 0 {
			continue
		}
		body := file.Text[match[1] : match[1]+end]
		if insideDefinition(file.Text, match[0]) || insideLegend(file.Text, match[0]) {
			continue // a definition, not a table; a legend in a caption or a drawing, not a table
		}
		// A table nested in the previous one has only that table's setting before it.
		declared := context.declared || previous <= match[0] && headerSetting.MatchString(file.Text[previous:match[0]])
		previous = max(previous, match[1]+end)
		if declared || strings.Contains(body, `\endhead`) || strings.Contains(body, `\endfirsthead`) {
			continue
		}
		item, ok := tableItem(file, name, match, body, context)
		if ok {
			items = append(items, item)
		}
	}
	return items
}

var environmentEdge = regexp.MustCompile(`\\(begin|end)\s*\{([^}]*)\}`)

// matchingEnd is the offset in text of the \end that closes an environment name opened just
// before it (tables of the same kind nested in its cells counted), -1 when none does.
func matchingEnd(text, name string) int {
	depth := 0
	for _, edge := range environmentEdge.FindAllStringSubmatchIndex(text, -1) {
		if text[edge[4]:edge[5]] != name {
			continue
		}
		if text[edge[2]:edge[3]] == "begin" {
			depth++
			continue
		}
		if depth == 0 {
			return edge[0]
		}
		depth--
	}
	return -1
}

// tableItem is the item of one table whose header rows are not known, false when they are (or
// the table has one row or one column).
func tableItem(file *File, name string, match []int, body string, context tableContext) (Item, bool) {
	top, rows := tableRows(file.Text[match[1]:match[1]+len(body)], specificationEnd(file.Text, match[1], name)-match[1])
	if len(rows) < 2 || !anyColumns(rows) {
		return Item{}, false
	}
	if pictureGrid(rows, context.macros) || itemGrid(rows) {
		return Item{}, false // a grid of figures, not a table
	}
	head, apart := ruledHead(top, rows)
	if apart || head == 0 && boldHead(rows) || groupedHead(rows, head) {
		return Item{}, false
	}
	item := Item{Kind: "table", File: file.Path, Line: file.Line(match[0]),
		Message: "a " + name + " has no rule under its first row, so no header row is known",
		Fix: `\midrule (booktabs) or \hline under the header row; a table used only for layout needs none; ` +
			`for the tagged PDF also \tagpdfsetup{table/header-rows={1}}`}
	if head > 0 {
		item.Message = fmt.Sprintf("a %s rules every row alike, so the rule under row %d does not set a header row apart", name, head)
	}
	return tableNote(item, file, match[1], rows, head, context), true
}

// insideLegend: the tabular at offset sits in a caption's argument or in a drawing (a TikZ \node's
// legend): a few cells that name marks, not a data table (F7).
func insideLegend(text string, at int) bool {
	for _, match := range captionCommand.FindAllStringSubmatchIndex(text[:at], -1) {
		if group, end, ok := argument(text, match[1], '{', '}'); ok && end > at && group != "" {
			return true
		}
	}
	for _, match := range drawing.FindAllStringSubmatchIndex(text[:at], -1) {
		if length := matchingEnd(text[match[1]:], text[match[2]:match[3]]); length >= 0 && match[1]+length > at {
			return true
		}
	}
	return false
}

// groupedHead: a \cline or \cmidrule under a row above the full rule that ends the header: the header
// has two rows, the first grouping the columns of the second (\multicolumn, \cline{2-7}, the
// sub-headings, \hline), so the author has set it apart as far as a rule can (F6).
func groupedHead(rows []tableRow, head int) bool {
	for _, row := range rows[:max(head-1, 0)] {
		if row.after.partial {
			return true
		}
	}
	return false
}

// tableNote names the row that looks like the header (the first bold one of the first two, else
// the first) and the rule to add under it, at the line where that row ends.
func tableNote(item Item, file *File, bodyStart int, rows []tableRow, head int, context tableContext) Item {
	candidate, reason := 1, "the first row"
	if !bold(rows[0].text) && bold(rows[1].text) && len(rows) > 2 {
		candidate, reason = 2, "rows 1–2, the second bold"
	} else if bold(rows[0].text) {
		reason = "the first row, bold"
	}
	rule := `\hline`
	if context.booktabs {
		rule = `\midrule`
	}
	action := "add " + rule + " after it"
	if head > 0 {
		candidate, rule, reason = head, `\hline`, "above the first rule"
		action = `make the rule under it double (add \hline after it) so it sets the header apart`
	}
	rows = rows[:candidate]
	shown := []string{}
	for _, row := range rows {
		shown = append(shown, clip(oneLine(row.text), 60))
	}
	end := bodyStart + rows[candidate-1].end
	item.Suggestion = rule
	item.NoteLine = file.Line(end - 1)
	rest, _, _ := strings.Cut(file.Text[end:], "\n")
	if strings.TrimSpace(tableRule.ReplaceAllString(rest, "")) == "" && !strings.Contains(rest, "&") {
		item.Place = PlaceAfter
	}
	setting := fmt.Sprintf(`\tagpdfsetup{table/header-rows={%s}}`, rowList(candidate))
	tagging := "; for the tagged PDF also " + setting + " before \\begin on line " + fmt.Sprint(item.Line)
	if !context.metadata {
		tagging += " (it needs \\DocumentMetadata{tagging=on})"
	}
	item.Note = fmt.Sprintf("“%s” looks like the header (%s): %s, at the end of this line%s", strings.Join(shown, ` \\ `), reason, action, tagging)
	return item
}

// specificationEnd is where the table's column specification ends: after the width of tabular*,
// tabularx and tabulary, the optional position and the specification itself.
func specificationEnd(text string, offset int, name string) int {
	if name == "tabular*" || name == "tabularx" || name == "tabulary" {
		_, offset, _ = argument(text, offset, '{', '}')
	}
	_, offset, _ = argument(text, offset, '[', ']')
	_, offset, _ = argument(text, offset, '{', '}')
	return offset
}

// anyColumns: some row has two cells or more.
func anyColumns(rows []tableRow) bool {
	for _, row := range rows {
		if len(splitTop(row.text, '&')) > 1 {
			return true
		}
	}
	return false
}

func rowList(rows int) string {
	numbers := []string{}
	for row := 1; row <= rows; row++ {
		numbers = append(numbers, fmt.Sprint(row))
	}
	return strings.Join(numbers, ",")
}

func clip(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}

// bold: every non-empty cell of the row starts bold (\textbf, \bfseries, {\bf …}).
func bold(row string) bool {
	cells := 0
	for _, cell := range splitTop(row, '&') {
		if strings.TrimSpace(cell) == "" {
			continue
		}
		if !boldCell.MatchString(cell) {
			return false
		}
		cells++
	}
	return cells > 0
}

// boldHead: the first row is bold and fewer than half of the body's cells are, which the
// conversion reads as the header row (decision 111).
func boldHead(rows []tableRow) bool {
	if !bold(rows[0].text) {
		return false
	}
	cells, bolds := 0, 0
	for _, row := range rows[1:] {
		for _, cell := range splitTop(row.text, '&') {
			if strings.TrimSpace(cell) != "" {
				cells++
				bolds += boolInt(boldCell.MatchString(cell))
			}
		}
	}
	return cells > 0 && bolds*2 < cells
}

// ruledHead is the header the conversion reads: the first of rows 1 and 2 with a full rule under
// it (0 for none), and whether that rule sets the rows above apart.
func ruledHead(top rowRule, rows []tableRow) (int, bool) {
	for _, head := range []int{1, 2} {
		if head >= len(rows) || rows[head-1].after.lines == 0 {
			continue
		}
		return head, setApart(top, rows, head)
	}
	return 0, false
}

// setApart is tablelayout's rule: a single body row needs a head rule thinner than the top rule
// (\midrule under \toprule); more need fewer than half of their boundaries ruled, or the head's
// rule stronger than each of theirs.
func setApart(top rowRule, rows []tableRow, head int) bool {
	headRule := rows[head-1].after
	if head+1 >= len(rows) {
		return top.lines > 0 && headRule.thickness < top.thickness
	}
	others, ruled, weaker := 0, 0, 0
	for boundary := head + 1; boundary < len(rows); boundary++ {
		others++
		if rows[boundary-1].after.lines == 0 {
			continue
		}
		ruled++
		if headRule.stronger(rows[boundary-1].after) {
			weaker++
		}
	}
	return ruled*2 < others || weaker == ruled
}

// tableRows splits a table's body (from its \begin, the column specification ending at skip) into
// rows at the \\ outside braces: the rules above the first row, then each row with the rules under it.
func tableRows(body string, skip int) (rowRule, []tableRow) {
	position := min(skip, len(body))
	top, position := rulesAt(body, position)
	rows := []tableRow{}
	for position < len(body) {
		end, next := rowEnd(body, position)
		text := body[position:end]
		if strings.TrimSpace(text) == "" && next >= len(body) {
			break
		}
		row := tableRow{text: text, end: next}
		row.after, position = rulesAt(body, next)
		rows = append(rows, row)
	}
	return top, rows
}

// rowEnd is where the row starting at position ends (its \\ or \tabularnewline, or the body's
// end) and where the next part starts (after the row end and its optional [space]).
func rowEnd(body string, position int) (int, int) {
	depth := 0
	for index := position; index < len(body); index++ {
		switch body[index] {
		case '{':
			depth++
		case '}':
			depth--
		case '\\':
			if index+1 >= len(body) {
				continue
			}
			if body[index+1] == '\\' && depth <= 0 {
				return index, skipSpace(body, index+2)
			}
			if depth <= 0 && strings.HasPrefix(body[index:], `\tabularnewline`) {
				return index, skipSpace(body, index+len(`\tabularnewline`))
			}
			// A table or other environment in a cell holds its own rows.
			depth += boolInt(strings.HasPrefix(body[index:], `\begin{`)) - boolInt(strings.HasPrefix(body[index:], `\end{`))
			index++ // an escaped character or the first letter of a command
		}
	}
	return len(body), len(body)
}

// skipSpace skips an optional [length] after a row end.
func skipSpace(body string, position int) int {
	if _, after, has := argument(body, position, '[', ']'); has {
		return after
	}
	return position
}

// rulesAt reads the rule commands at position: the full rules' lines and thickest weight, and the
// position after them (their arguments included).
func rulesAt(body string, position int) (rowRule, int) {
	rule := rowRule{}
	for {
		match := tableRule.FindStringSubmatchIndex(body[position:])
		if match == nil {
			return rule, position
		}
		name := body[position+match[2] : position+match[3]]
		position += match[1]
		rule.partial = rule.partial || name == "cline" || name == "cmidrule"
		if weight, full := ruleWeights[name]; full {
			rule.lines += hhlineLines(name, body, position)
			rule.thickness = max(rule.thickness, weight)
		}
		position = skipRuleArguments(body, position)
	}
}

// hhlineLines: \hhline{==} draws two lines, \hhline{--} one; every other full rule one.
func hhlineLines(name, body string, position int) int {
	if name != "hhline" {
		return 1
	}
	if spec, _, ok := argument(body, position, '{', '}'); ok && strings.Contains(spec, "=") {
		return 2
	}
	return 1
}

// skipRuleArguments skips a rule's optional and required arguments: [..], (..) and {..}.
func skipRuleArguments(body string, position int) int {
	for {
		start := position
		for start < len(body) && strings.IndexByte(" \t\r\n", body[start]) >= 0 {
			start++
		}
		if start >= len(body) {
			return position
		}
		switch body[start] {
		case '[':
			_, position, _ = argument(body, start, '[', ']')
		case '{':
			_, position, _ = argument(body, start, '{', '}')
		case '(':
			end := strings.IndexByte(body[start:], ')')
			if end < 0 {
				return position
			}
			position = start + end + 1
		default:
			return position
		}
		if position <= start {
			return start
		}
	}
}

// splitTop splits text at separator outside braces and outside environments (a table nested in a
// cell keeps its own cells).
func splitTop(text string, separator byte) []string {
	parts := []string{}
	depth, start := 0, 0
	for index := 0; index < len(text); index++ {
		switch text[index] {
		case '{':
			depth++
		case '}':
			depth--
		case '\\':
			// \begin{x} opens one level more than its braces close; \end{x} closes it.
			depth += boolInt(strings.HasPrefix(text[index:], `\begin{`)) - boolInt(strings.HasPrefix(text[index:], `\end{`))
			index++
		case separator:
			if depth == 0 {
				parts = append(parts, text[start:index])
				start = index + 1
			}
		}
	}
	return append(parts, text[start:])
}
