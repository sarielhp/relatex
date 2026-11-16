package semantic

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/mathexport"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/rubynum"
	"github.com/sarielhp/relatex/internal/tablelayout"
)

var typstGroups = []string{"document", "section", "theorem", "proof", "ordered_list", "unordered_list", "list_item", "code",
	"figure_float", "table_float", "float_caption"}

var (
	layoutsOnce sync.Once
	mathLayout  string
	tableLayout string
)

func readLayouts() {
	layoutsOnce.Do(func() {
		directory := filepath.Join(Root, "lib", "relatex")
		mathBytes, err := os.ReadFile(filepath.Join(directory, "math_layout.typ"))
		if err != nil {
			rec.Fail("%v", err)
		}
		tableBytes, err := os.ReadFile(filepath.Join(directory, "table_layout.typ"))
		if err != nil {
			rec.Fail("%v", err)
		}
		mathLayout, tableLayout = string(mathBytes), string(tableBytes)
	})
}

// typst is TypstExport.
type typst struct {
	*export
	translator *mathexport.Typst
}

func (t *typst) header() string {
	readLayouts()
	return "// Generated from structured IR; printed source numbers are retained.\n" +
		"#set heading(numbering: none)\n#set math.equation(numbering: none)\n\n" + mathLayout + "\n" + tableLayout + "\n"
}

func label(id any) string { return "<" + toS(id) + ">" }

// points is Geometry.sp_to_bp(value).to_s + "pt".
func points(value any) string {
	number, ok := rec.Number(value)
	if !ok {
		rec.Fail("expected a number, got %T", value)
	}
	return mathexport.RubyFloat(geometry.SPToBP(number)) + "pt"
}

func (t *typst) renderGroup(node rec.Map, children []string) string {
	kind := rec.FetchString(node, "type")
	id := rec.Fetch(node, "id")
	if kind == "ordered_list" || kind == "unordered_list" {
		items := []string{}
		for _, child := range children {
			items = append(items, "["+child+"]")
		}
		function := "list(marker: []"
		if t.drawnMarkers(node) {
			function = "list(tight: true"
		}
		if kind == "ordered_list" {
			function = "enum(numbering: n => []"
		}
		return "#" + function + ", " + strings.Join(items, ", ") + ") " + label(id)
	}
	body := strings.Join(children, "\n\n")
	if kind != "note" && !contains(typstGroups, kind) {
		rec.Fail("Unsupported Typst structure: %s", kind)
	}
	return "#block[\n" + body + "] " + label(id)
}

func (t *typst) block(node rec.Map, body string) string {
	kind := rec.FetchString(node, "type")
	if kind == "heading" {
		return "#heading(level: 2)[" + body + "] " + label(rec.Fetch(node, "id"))
	}
	if kind != "paragraph" && kind != "display_math" {
		rec.Fail("Unsupported Typst block: %s", kind)
	}
	return "#block[" + body + "] " + label(rec.Fetch(node, "id"))
}

func (t *typst) styledText(span rec.Map) string {
	style, styled := span["style"].(rec.Map)
	size := ""
	if styled {
		size = "size: " + points(rec.Fetch(style, "size_sp")) + ", "
	}
	content := "#text(" + size + quote(spokenText(span)) + ")"
	if truthy(span["line_break"]) {
		// A heading set in several lines (decision 126): its space, then a visible break.
		content += "#linebreak()"
	}
	// Text-mode \underline and soul's \ul: underlined text; soul's \st: struck-out text.
	switch span["decoration"] {
	case "underline":
		content = "#underline[" + content + "]"
	case "strike":
		content = "#strike[" + content + "]"
	}
	if !styled {
		return content
	}
	if strings.HasPrefix(rec.FetchString(style, "series"), "b") {
		content = "#strong[" + content + "]"
	}
	if shape := rec.FetchString(style, "shape"); shape == "it" || shape == "sl" {
		content = "#emph[" + content + "]"
	}
	if rec.FetchString(style, "shape") == "sc" {
		content = "#smallcaps[" + content + "]"
	}
	return content
}

func (t *typst) reference(span rec.Map) string {
	return t.internalLink("#text("+quote(rec.FetchString(span, "text"))+")", rec.Fetch(span, "target"))
}

func (t *typst) internalLink(content string, id any) string {
	return "#link(" + label(id) + ")[" + content + "]"
}

func (t *typst) externalLink(content string, href any) string {
	return "#link(" + quote(rec.AsString(href)) + ")[" + content + "]"
}

func (t *typst) noteReference(span rec.Map) string {
	return "#link(" + label(rec.Fetch(span, "target")) + ")[#super[#text(" + quote(rec.FetchString(span, "text")) + ")]] " +
		label(rec.Fetch(span, "callsite"))
}

func (t *typst) convert(xml string) string {
	if t.translator == nil {
		t.translator = mathexport.NewTypst()
	}
	return t.translator.Convert(xml)
}

func (t *typst) mathFormat() string                                { return "typst" }
func (t *typst) nativeMath(rec.Map, string, string) (string, bool) { return "", false }
func (t *typst) mathSpan(span, owner rec.Map) string               { return t.semanticMath(span, owner) }

func (t *typst) math(value string, display bool, id string) string {
	var rows strings.Builder
	for _, child := range rec.FetchList(t.node(id), "children") {
		rows.WriteString(t.rowAnchor(t.node(child)))
	}
	if display {
		return rows.String() + "$ " + value + " $"
	}
	return rows.String() + "$" + value + "$"
}

func (t *typst) image(span rec.Map) string {
	image := "#image(" + quote(t.assetFile(span)) + ", width: " + points(rec.Fetch(span, "width_sp")) + ", " +
		"height: " + points(rec.Fetch(span, "height_sp")) + typstAlt(span) + ")"
	depth, has := span["depth_sp"]
	if number, _ := rec.Number(depth); !has || number == 0 {
		return image
	}
	return "#box(baseline: " + points(depth) + ")[" + image + "]"
}

// typstAlt is an image's text alternative as Typst's alt argument, "" without one.
func typstAlt(span rec.Map) string {
	if alt, ok := span["alt"].(string); ok && strings.TrimSpace(alt) != "" {
		return ", alt: " + quote(alt)
	}
	return ""
}

// longDescription is a figure's long description (decision 110): a block after the figure's.
func (t *typst) longDescription(text string) string {
	return "#block[#emph[Description of the figure:] #text(" + quote(text) + ")]"
}

func (t *typst) fallbackImage(path, _ string) string { return "#image(" + quote(path) + ")" }

func (t *typst) table(grid, span rec.Map) string {
	model := tablelayout.NewTable(grid, t.nodes)
	layout := tablelayout.NewLayout(model)
	sections := tablelayout.NewSections(layout)
	id := rec.FetchString(span, "id")
	first, last := "table-first-"+id, "table-last-"+id
	cells := []string{}
	for _, entry := range sections.Cells {
		cells = append(cells, t.tableCell(entry, layout, first, last))
	}
	widths := []string{}
	for _, value := range layout.ColumnWidths {
		widths = append(widths, points(value))
	}
	heights := []string{}
	for _, value := range sections.Heights {
		heights = append(heights, points(value))
	}
	captions := []string{}
	for _, cell := range sections.Captions {
		captions = append(captions, "#block(width: "+points(rubynum.Sum(layout.ColumnWidths))+")[#align(center)["+t.tableCellContent(cell)+"]]")
	}
	return strings.Join(captions, "\n") + "\n#l2d-table((" + strings.Join(widths, ", ") + ",), (" + strings.Join(heights, ", ") + ",), (" +
		strings.Join(cells, ", ") + ",), " + quote(first) + ", " + quote(last) + ", repeat-head: " + boolText(sections.RepeatHead) +
		", repeat-foot: " + boolText(sections.RepeatFoot) + ") " + label(id)
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func (t *typst) tableCell(entry tablelayout.SectionCell, layout *tablelayout.Layout, first, last string) string {
	before, after := "", ""
	if entry.First {
		before = "#metadata(none) " + label(first)
	}
	if entry.Last {
		after = "#metadata(none) " + label(last)
	}
	main := t.tableVariant(entry.Cell, layout, before, after)
	alternate := "none"
	if entry.Alternate != nil {
		alternate = "(" + t.tableVariant(entry.Alternate, layout, "", "") + ")"
	}
	cell := entry.Cell
	return "(x: " + toS(rec.Fetch(cell, "column")) + ", y: " + toS(int64(entry.Row)) + ", colspan: " + toS(rec.Fetch(cell, "colspan")) +
		", rowspan: " + toS(rec.Fetch(cell, "rowspan")) + ", section: " + quote(entry.Section) + ", alternate: " + alternate + ", " + main + ")"
}

var typstVertical = map[string]string{"top": "top", "middle": "horizon", "bottom": "bottom"}

func (t *typst) tableVariant(cell rec.Map, layout *tablelayout.Layout, before, after string) string {
	horizontal := tablelayout.HorizontalAlignment(cell)
	verticalName := "top"
	if value, ok := cell["vertical_align"]; ok {
		verticalName = toS(value)
	}
	vertical, ok := typstVertical[verticalName]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + quote(verticalName), Lookup: true})
	}
	rules := []string{}
	for _, rule := range layout.RulesFor(cell) {
		rules = append(rules, "(axis: "+quote(rec.FetchString(rule, "axis"))+", side: "+quote(rec.FetchString(rule, "side"))+
			", start: "+toS(rec.Fetch(rule, "start"))+", end: "+toS(rec.Fetch(rule, "end"))+
			", thickness: "+points(rec.Fetch(rule, "thickness_sp"))+", offset: "+points(rec.Fetch(rule, "offset_sp"))+")")
	}
	tuple := "()"
	if len(rules) > 0 {
		tuple = "(" + strings.Join(rules, ", ") + ",)"
	}
	body := t.tableCellContent(cell)
	if wrap, ok := cell["wrap"]; ok && !truthy(wrap) {
		body = "#box[" + body + "]"
	}
	padding := any(int64(0))
	if value, ok := cell["padding_sp"]; ok {
		padding = value
	}
	return "align: " + horizontal + " + " + vertical + ", padding: " + points(padding) + ", " +
		"body: [" + before + body + after + "], rules: " + tuple
}

// symbol: the open square, or an author's QED mark as drawn (decision 176): its glyphs or its image.
func (t *typst) symbol(span rec.Map) string {
	switch {
	case span["name"] == "qed_open_square":
		return `#text("□")`
	case span["name"] != "qed":
		rec.Fail("Unsupported Typst symbol")
	}
	if image, ok := span["image"].(rec.Map); ok {
		return t.image(image)
	}
	if text, ok := span["text"].(string); ok {
		return "#text(" + quote(text) + ")"
	}
	return `#text("□")`
}

func (t *typst) rule(span rec.Map) string {
	fill := "black"
	if color, ok := span["color"]; ok {
		fill = "rgb(" + quote(rec.AsString(color)) + ")"
	}
	height := points(rubynum.Add(rec.Fetch(span, "height_sp"), rec.Fetch(span, "depth_sp")))
	box := "#box(width: " + points(rec.Fetch(span, "width_sp")) + ", height: " + height + ", baseline: " + points(rec.Fetch(span, "depth_sp")) +
		", fill: " + fill + ")"
	if _, ok := span["margin_left_sp"]; !ok {
		return box
	}
	return "#h(" + points(rec.Fetch(span, "margin_left_sp")) + ")" + box + "#h(" + points(rec.Fetch(span, "margin_right_sp")) + ")"
}

func (t *typst) pageTotal(node rec.Map) string {
	value := "#text(" + quote(toS(rec.Fetch(node, "count"))) + ")"
	if rendered, ok := renderedContent(node, t.renderSpan); ok {
		value = rendered
	}
	return "#text(" + quote("Source PDF: ") + ") " + value + " #text(" + quote(" pages.") + ") " + label(rec.Fetch(node, "id"))
}

func (t *typst) rowAnchor(node rec.Map) string { return "#box[] " + label(rec.Fetch(node, "id")) }

// drawnMarkers: every item of the list has a marker a beamer theme draws (decision 127), which its
// text does not print: Typst's own list marker shows it.
func (t *typst) drawnMarkers(node rec.Map) bool {
	children := rec.FetchList(node, "children")
	for _, child := range children {
		if _, marked := rec.AsMap(t.nodes[rec.AsString(child)])["marker"].(rec.Map); !marked {
			return false
		}
	}
	return len(children) > 0
}
