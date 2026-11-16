package a11ylint

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/gaplocate"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablelayout"
)

// The export's open structural items (decision 184). An export's accessibility.json lists the tables
// without header cells and the captions that name colours it found, by node id; the lint reads them
// from the export in --output, places each one in the source and merges it with the item the source
// scan has for the same place, so an author who does what the lint lists knows whether the export
// earns the claim, or why not.

// ExportGap is one open structural item of an export, located in the source when the conversion's
// source map knows it.
type ExportGap struct {
	Check string // table_headers or use_of_color
	// File and Line: the caption of the float holding the item (the first figure of a table when the
	// caption is not found); File is "" when the item has no place.
	File string
	Line int
	// Float is figure_float or table_float; Caption, Number and Label name the float.
	Float, Caption, Number, Label string
	// Table is the table's position among the float's tables and Of their number; Cells its cells.
	Table, Of, Cells int
	// Colours are the colour words of a caption; Panel says why a grid of figures is read as a table.
	Colours []string
	Panel   string
	// SHA is the sha256 of the float caption's text a use_of_color item carries (the value of an
	// attestation, decision 188) and Voided says an attestation of the float no longer matches it.
	SHA    string
	Voided bool
}

// Export is what the lint takes from an export's accessibility.json.
type Export struct {
	Gaps []ExportGap
	// Claimed: the export holds the claim already.
	Claimed bool
	// Unlisted counts open table_headers and use_of_color items the report holds beyond the ones it lists.
	Unlisted int
	// Other lists the open items no source edit of the lint closes, "id (count)" (axe's rules, other checks).
	Other []string
}

// WithExportFile reads the export's accessibility.json, when path holds one, and places its open
// items with the conversion's document (WithConversion's) and source map.
func (paper Paper) WithExportFile(path string, document rec.Map) Paper {
	value, err := ojson.ReadFile(path)
	if err != nil {
		return paper
	}
	report, ok := ojson.Plain(value).(rec.Map)
	if !ok {
		return paper
	}
	return paper.WithExport(report, document)
}

// WithExport reads an accessibility report: its structural gaps, placed from the item's own file and
// line when the product wrote them, else by looking its node up in the document and the source map.
func (paper Paper) WithExport(report, document rec.Map) Paper {
	export := &Export{Claimed: report["claim"] != nil}
	places := gaplocate.New(document, paper.SourceMap)
	for _, value := range rec.SliceOf(report["open_items"]) {
		item := rec.MapOf(value)
		if id := rec.StringOf(item["id"]); id != "table_headers" && id != "use_of_color" {
			export.Other = append(export.Other, fmt.Sprintf("%s (%d)", id, rec.IntOf(item["count"])))
		}
	}
	for _, value := range rec.SliceOf(report["structural"]) {
		row := rec.MapOf(value)
		check := rec.StringOf(row["check"])
		if check != "table_headers" && check != "use_of_color" || row["status"] != "gap" {
			continue
		}
		listed := rec.SliceOf(row["items"])
		export.Unlisted += max(int(rec.IntOf(row["count"]))-len(listed), 0)
		for _, entry := range listed {
			export.Gaps = append(export.Gaps, exportGap(check, rec.MapOf(entry), places))
		}
	}
	paper.Export = export
	return paper
}

// exportGap is one structural item of the report, with the place it gives or the document finds.
func exportGap(check string, item rec.Map, places *gaplocate.Locator) ExportGap {
	gap := ExportGap{Check: check, Cells: int(rec.IntOf(item["cells"]))}
	var place gaplocate.Place
	switch check {
	case "table_headers":
		id := rec.StringOf(item["id"])
		place = places.Table(id)
		if grid := places.Grid(id); grid != nil {
			gap.Panel = tablelayout.PanelReason(grid)
		}
	default:
		place = places.Caption(rec.StringOf(item["block"]))
		for _, colour := range rec.SliceOf(item["colours"]) {
			gap.Colours = append(gap.Colours, rec.StringOf(colour))
		}
		if len(gap.Colours) == 0 {
			gap.Colours = nil
		}
		gap.SHA, gap.Voided = rec.StringOf(item["sha256"]), rec.StringOf(item["attestation"]) != ""
	}
	if file := rec.StringOf(item["file"]); file != "" {
		place.File, place.Line = file, int(rec.IntOf(item["line"]))
	}
	gap.File, gap.Line, gap.Float, gap.Caption, gap.Number, gap.Label = place.File, place.Line, place.Float, place.Caption, place.Number, place.Label
	gap.Table, gap.Of = place.Index, place.Of
	if gap.Caption == "" {
		gap.Caption = rec.StringOf(item["caption"])
		if gap.Caption == "" {
			gap.Caption = rec.StringOf(item["text"])
		}
	}
	return gap
}

// exportMark ends the message of a source item the export's check also lists.
const exportMark = " [the export's %s check lists it]"

// colourRewordText is the fix for a caption the export's use_of_color check lists: the check reads the
// caption's words only and has no way to see a cue beside the colour, so the item closes when the
// caption names no colour, or when the author attests it (decision 188).
const colourRewordText = "the export's use_of_color check lists every float caption that names a colour and cannot see another cue, " +
	"so it closes when the caption names none: say it with more than colour, e.g. shown as dashed red line, red circles, " +
	"or with a letter, a label or a line style " +
	`("the dashed edges", "the edges marked a") in the figure and the caption (WCAG 1.4.1)`

// colourReword is colourRewordText and, where the figure already says it with more than colour (line
// styles, markers, labels), the exact line that attests the caption as it stands: its key is the
// float's \label, else its caption number, and the sha binds it to the caption's text, so an edit
// voids it. The lint never writes the line itself: only the author knows the figure.
func colourReword(gap ExportGap) string {
	key := gap.Label
	if key == "" {
		key = gap.Number
	}
	if key == "" || gap.SHA == "" {
		return colourRewordText
	}
	line := fmt.Sprintf(`"colour_captions": {%q: %q} in relatex/hints.json, or the source comment %%%% relatex colour_captions: %s = "%s"`,
		key, gap.SHA, key, gap.SHA)
	if gap.Voided {
		return colourRewordText + "; the caption was attested and has changed since, so the attestation is void: if the figure still says " +
			"it with more than colour, replace the sha in " + line
	}
	return colourRewordText + "; or, if the figure already says it with more than colour, attest the caption as it stands: add " + line +
		" (an edit to the caption voids it)"
}

// mergeExport adds the export's gaps to the source items: a gap at the place of an item of the same
// kind marks it; any other becomes an item of its own.
func mergeExport(items []Item, paper Paper) []Item {
	if paper.Export == nil {
		return items
	}
	for _, gap := range paper.Export.Gaps {
		item := gapItem(gap, paper.Files)
		found := -1
		for index, other := range items {
			if other.Kind == item.Kind && other.File == item.File && other.Line == item.Line {
				found = index
				break
			}
		}
		if found < 0 {
			items = append(items, item)
			continue
		}
		mark := fmt.Sprintf(exportMark, gap.Check)
		if !strings.Contains(items[found].Message, mark) {
			items[found].Message += mark
		}
		switch gap.Check {
		case "use_of_color":
			items[found].Fix = colourReword(gap)
		case "table_headers":
			items[found].CropLine = max(items[found].CropLine, item.CropLine)
			items[found].Fix += fmt.Sprintf(" (for the export, %s: %s)", tablePosition(gap), tableGapFix(gap))
		}
	}
	return items
}

// gapItem is the lint item of a gap: where it is in the source and how to close it.
func gapItem(gap ExportGap, files []*File) Item {
	file := fileNamed(files, gap.File)
	if gap.Check == "use_of_color" {
		return colourGapItem(gap, file, files)
	}
	return tableGapItem(gap, file, files)
}

func fileNamed(files []*File, name string) *File {
	for _, file := range files {
		if file.Path == name {
			return file
		}
	}
	return nil
}

// colourGapItem: the \caption command whose argument holds the caption's first words.
func colourGapItem(gap ExportGap, file *File, files []*File) Item {
	item := Item{Kind: "colour", Fix: colourReword(gap),
		Message: fmt.Sprintf("the caption names colours (%s); colour may be the only thing that tells them apart"+exportMark,
			strings.Join(gap.Colours, ", "), gap.Check)}
	if file == nil {
		return unplaced(item, gap, files)
	}
	item.File, item.Line = file.Path, captionLine(file, gap.Line)
	return item
}

// unplaced is an item whose float the source map did not find: it names the float in the first file.
func unplaced(item Item, gap ExportGap, files []*File) Item {
	if len(files) > 0 {
		item.File = files[0].Path
	}
	item.Message += fmt.Sprintf(" (%s, not found in the source: search for its caption %q)", floatName(gap), clip(gap.Caption, 60))
	return item
}

// captionLine is the line of the \caption command whose argument covers line, else line itself.
func captionLine(file *File, line int) int {
	for _, match := range caption.FindAllStringIndex(file.Text, -1) {
		_, end, ok := argument(file.Text, match[1], '{', '}')
		if ok && file.Line(match[0]) <= line && line <= file.Line(end) {
			return file.Line(match[0])
		}
	}
	return line
}

func floatName(gap ExportGap) string {
	name := "the float"
	switch {
	case gap.Float == "table_float" && gap.Number != "":
		name = "Table " + gap.Number
	case gap.Float == "figure_float" && gap.Number != "":
		name = "Figure " + gap.Number
	case gap.Float == "table_float":
		name = "a table float"
	case gap.Float == "figure_float":
		name = "a figure float"
	}
	return name
}

// tableGapItem: the \begin of the table among the tabulars of the float's environment.
func tableGapItem(gap ExportGap, file *File, files []*File) Item {
	item := Item{Kind: "table"}
	item.Message = fmt.Sprintf("a table of the export has no header cells (table_headers; %d cells, %s)"+exportMark,
		gap.Cells, tablePosition(gap), gap.Check)
	item.Fix = tableGapFix(gap)
	if file == nil {
		return unplaced(item, gap, files)
	}
	item.File, item.Line, item.CropLine = file.Path, tableLine(file, gap), gap.Line
	return item
}

// tablePosition says which table of which float it is.
func tablePosition(gap ExportGap) string {
	position := floatName(gap)
	if gap.Of > 1 {
		position = fmt.Sprintf("table %d of %d in %s", gap.Table, gap.Of, position)
	} else {
		position = "in " + position
	}
	return position
}

// tableGapFix is the exact way to give the table a header the export reads (decision 131), or to
// make a grid of figures read as a layout (decision 167 point 6).
func tableGapFix(gap ExportGap) string {
	rule := `\midrule (booktabs) or \hline under the header row, or a bold first row`
	if gap.Float == "table_float" {
		key := gap.Label
		if key == "" {
			key = gap.Number
		}
		hint := `a hint in the source: %% relatex tables: ` + key + ` = "header_rows=1" (or "header_columns=1" when the first column names the rows)`
		if key == "" {
			hint = `a tables hint (relatex/hints.json), which needs the table's \label or caption number`
		}
		return rule + "; or " + hint
	}
	fix := rule + figureTableHint(gap)
	if gap.Panel != "" {
		fix = "the table holds figures and is read as a table because " + gap.Panel +
			"; make it an arrangement of figures (a row of \\includegraphics, each label in a row directly under its picture, " +
			"or inside the picture) so it reads as a layout; if it is a data table instead, " + fix
	}
	return fix
}

// figureTableHint is the hint line for a table inside a figure float (decision 188): the float's
// \label, else its caption number, and the table's position in it. The hint says only what the author
// writes: the table's first row is a header row, so the author must mean it.
func figureTableHint(gap ExportGap) string {
	key := gap.Label
	if key == "" {
		key = gap.Number
	}
	if key == "" || gap.Table < 1 {
		return "; a tables hint (relatex/hints.json) names a table inside a figure by the figure's \\label or caption number and the table's position, which this one lacks"
	}
	return fmt.Sprintf(`; or a hint in the source, if its first row is a header row: %%%% relatex tables: %s#%d = "header_rows=1" `+
		`(or "header_columns=1" when the first column names the rows)`, key, gap.Table)
}

// floatEnvironment is a float's \begin.
var floatEnvironment = regexp.MustCompile(`\\begin\s*\{(figure\*?|table\*?|sidewaysfigure|sidewaystable|wrapfigure|wraptable)\}`)

// tableLine is the line of the \begin of the gap's table: the tabulars of the smallest float environment
// holding the gap's line, the one at the table's position when their number is the float's, else the
// first; the gap's own line when there is none.
func tableLine(file *File, gap ExportGap) int {
	if gap.Line < 1 || gap.Line > len(file.lines) {
		return gap.Line
	}
	at := file.lines[gap.Line-1]
	start, end := -1, -1
	for _, match := range floatEnvironment.FindAllStringSubmatchIndex(file.Text, -1) {
		length := matchingEnd(file.Text[match[1]:], file.Text[match[2]:match[3]])
		if length >= 0 && match[0] <= at && at <= match[1]+length && (start < 0 || match[0] > start) {
			start, end = match[0], match[1]+length
		}
	}
	if start < 0 {
		return gap.Line
	}
	starts := []int{}
	for _, match := range tabular.FindAllStringIndex(file.Text[start:end], -1) {
		if !insideDefinition(file.Text, start+match[0]) {
			starts = append(starts, start+match[0])
		}
	}
	switch {
	case len(starts) == 0:
		return gap.Line
	case gap.Table > 0 && gap.Of == len(starts):
		return file.Line(starts[gap.Table-1])
	}
	return file.Line(starts[0])
}

// ExportSummary says, after the items, whether the export would earn the WCAG claim or what is left;
// "" when the lint read no export.
func ExportSummary(paper Paper) string {
	export := paper.Export
	if export == nil {
		return ""
	}
	if export.Claimed {
		return "Export: it holds the WCAG claim already."
	}
	left := append([]string{}, export.Other...)
	if export.Unlisted > 0 {
		left = append(left, fmt.Sprintf("%d more table_headers or use_of_color items the export's report does not list", export.Unlisted))
	}
	for _, gap := range export.Gaps {
		if gap.File == "" {
			left = append(left, "an item the source map could not place ("+gap.Check+": "+clip(gap.Caption, 40)+")")
		}
	}
	if len(left) > 0 {
		return "Export: the claim stays withdrawn after these items; also open, with no source item here: " + strings.Join(left, ", ") + "."
	}
	return fmt.Sprintf("Export: its open items are the %d table_headers and use_of_color items listed here; "+
		"when each is fixed as its fix says, the export earns the WCAG claim.", len(export.Gaps))
}
