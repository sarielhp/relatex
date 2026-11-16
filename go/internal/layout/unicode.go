package layout

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/rec"
)

// tolerance is PDFUnicodeGlyphs::TOLERANCE; maxComponents is MAX_COMPONENTS.
const (
	tolerance     = 0.001
	maxComponents = 32
)

// subsetPrefix is what MuPDF's structured text leaves out of a paint trace's font name, once
// (svgoutline.fontPrefix); a structured-text font name is compared as it is.
var subsetPrefix = regexp.MustCompile(`\A[^+]*\+`)

// unicodeGlyphs is PDFUnicodeGlyphs: ToUnicode can give one painted glyph several Unicode
// characters. Structured text emits extra zero-advance records; only the paint trace proves
// ownership.
type unicodeGlyphs struct {
	painted  []rec.Map
	groups   map[string][]rec.Map
	formulas []formula
}

// paintFont is a paint record's font name as the structured text writes it.
func paintFont(name any) string { return subsetPrefix.ReplaceAllString(rec.ToS(name), "") }

func firstCharacter(text string) string {
	_, size := utf8.DecodeRuneInString(text)
	return text[:size]
}

func newUnicodeGlyphs(page *nativetrace.Node) *unicodeGlyphs {
	u := &unicodeGlyphs{groups: map[string][]rec.Map{}}
	recordOf := map[*nativetrace.Node]rec.Map{}
	for _, span := range page.Descendants("span") {
		records, nodes := paintedGlyphs(span)
		u.painted = append(u.painted, records...)
		for index, node := range nodes {
			recordOf[node] = records[index]
		}
	}
	for _, content := range page.Descendants("metatext") {
		if found, ok := wholeFormula(content, recordOf); ok {
			u.formulas = append(u.formulas, found)
		}
	}
	for index, glyph := range u.painted {
		glyph["index"] = int64(index)
		text := rec.FetchString(glyph, "text")
		if utf8.RuneCountInString(text) > 1 {
			key := paintFont(glyph["font"]) + "\x00" + firstCharacter(text)
			u.groups[key] = append(u.groups[key], glyph)
		}
	}
	return u
}

// required is an attribute the trace must carry.
func required(element *nativetrace.Node, name string) string {
	value, ok := element.Attr(name)
	if !ok {
		rec.Fail("PDF trace element lacks %s", name)
	}
	return value
}

func numbers(element *nativetrace.Node, name string, count int) []float64 {
	return geometry.Numbers(geometry.Floats(nativetrace.Floats(required(element, name))), count)
}

func paintedGlyphs(span *nativetrace.Node) ([]rec.Map, []*nativetrace.Node) {
	transform := numbers(span.Parent, "transform", 6)
	matrix := numbers(span, "trm", 4)
	direction := matrix[:2]
	if mode, _ := span.Attr("wmode"); mode == "1" {
		direction = matrix[2:]
	}
	records, nodes := []rec.Map{}, []*nativetrace.Node{}
	for _, glyph := range span.Children {
		if glyph.Name != "g" {
			continue
		}
		text := required(glyph, "unicode")
		actual := actualText(span.Parent)
		id, anchored := glyph.Attr("glyph")
		if !anchored {
			if len(records) == 0 {
				rec.Fail("Unanchored PDF Unicode continuation")
			}
			last := records[len(records)-1]
			last["text"] = rec.FetchString(last, "text") + text
			continue
		}
		advance := nativetrace.Float(required(glyph, "adv"))
		font := required(span, "font")
		x, y := nativetrace.Float(required(glyph, "x")), nativetrace.Float(required(glyph, "y"))
		baselineX, baselineY := geometry.Point(transform, x, y)
		advanceX, advanceY := geometry.Point([]float64{transform[0], transform[1], transform[2], transform[3], 0, 0},
			direction[0]*advance, direction[1]*advance)
		placed := geometry.Compose(transform, []float64{matrix[0], matrix[1], matrix[2], matrix[3], x, y})
		record := rec.Map{"glyph": id, "text": text, "font": font, "operation": span.Parent.Name,
			"matrix": geometry.Floats(placed), "baseline": []any{baselineX, baselineY}, "advance": []any{advanceX, advanceY}}
		if actual != "" && actual != text {
			record["text"], record["drawn_text"] = actual, text
		}
		if fill := paintColor(span.Parent); fill != "" {
			record["fill"] = fill
		}
		lineWidths(record, span.Parent, transform)
		records, nodes = append(records, record), append(nodes, glyph)
	}
	return records, nodes
}

// paintColor is a filled glyph's color as the page SVG writes it (#rrggbb), from the trace's DeviceRGB or
// DeviceGray color; empty for another operation or color space, which tells no fill apart.
func paintColor(operation *nativetrace.Node) string {
	if operation.Name != "fill_text" {
		return ""
	}
	space, _ := operation.Attr("colorspace")
	color, _ := operation.Attr("color")
	fields := strings.Fields(color)
	if (space != "DeviceRGB" || len(fields) != 3) && (space != "DeviceGray" || len(fields) != 1) {
		return ""
	}
	digits := ""
	for _, field := range fields {
		value, err := strconv.ParseFloat(field, 64)
		if err != nil || value < 0 || value > 1 {
			return ""
		}
		digits += fmt.Sprintf("%02x", int(math.Round(value*255)))
	}
	if len(fields) == 1 {
		digits = strings.Repeat(digits, 3)
	}
	return "#" + digits
}

// lineWidths gives a stroked glyph's paint its line width as the trace writes it, in the user space
// of the text, and through the CTM (transform) in points: MuPDF 1.28.2's page SVG draws the first
// (svgoutline.strokeInk), the PDF the second.
func lineWidths(record rec.Map, operation *nativetrace.Node, transform []float64) {
	text, ok := operation.Attr("linewidth")
	if operation.Name != "stroke_text" || !ok {
		return
	}
	width := nativetrace.Float(text)
	if !(width > 0) || math.IsInf(width, 0) {
		return
	}
	expansion := math.Sqrt(math.Abs(float64(transform[0]*transform[3]) - float64(transform[1]*transform[2])))
	record["line_width"], record["device_line_width"] = width, width*expansion
}

// actualText is the replacement text (/ActualText) of the marked content a paint operation lies in
// when that content paints this one glyph alone, or "". MuPDF's structured text writes the
// replacement for the glyph, as one character per paint does: its first character at the glyph's
// origin with its advance, the others at its end (a ligature glyph without Unicode under ActualText
// "fi", papers5/1289). The paint trace and the page SVG keep the glyph's own Unicode (drawn_text).
func actualText(operation *nativetrace.Node) string {
	content := operation.Parent
	if content == nil || content.Name != "metatext" || len(content.Descendants("g")) != 1 {
		return ""
	}
	if kind, _ := content.Attr("type"); kind != "actualtext" {
		return ""
	}
	text, _ := content.Attr("txt")
	return text
}

func within(left, right []any) bool {
	for index := range left {
		a, _ := rec.Number(left[index])
		b, _ := rec.Number(right[index])
		if !(abs(a-b) <= tolerance) {
			return false
		}
	}
	return true
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

// bind is PDFUnicodeGlyphs#bind: a run of structured-text records that one painted glyph's
// Unicode string spells becomes that glyph.
func (u *unicodeGlyphs) bind(records []rec.Map) []rec.Map {
	result := []rec.Map{}
	for index := 0; index < len(records); {
		first := records[index]
		key := rec.ToS(first["font"]) + "\x00" + firstCharacter(rec.FetchString(first, "text"))
		var match rec.Map
		var parts []rec.Map
		count := 0
		for _, paint := range u.groups[key] {
			if !within(rec.FetchList(paint, "baseline"), rec.FetchList(first, "baseline")) {
				continue
			}
			if components := componentsFor(records, index, paint); components != nil {
				match, parts = paint, components
				count++
			}
		}
		if count != 1 {
			result = append(result, first)
			index++
			continue
		}
		merged := rec.Map{}
		for name, value := range first {
			merged[name] = value
		}
		list := make([]any, len(parts))
		for position, part := range parts {
			list[position] = part
		}
		merged["text"], merged["unicode_binding"], merged["unicode_components"] = rec.Fetch(match, "text"), match, list
		result = append(result, merged)
		index += len(parts)
	}
	return result
}

func componentsFor(records []rec.Map, index int, paint rec.Map) []rec.Map {
	first := records[index]
	target := rec.FetchString(paint, "text")
	components := []rec.Map{}
	var text strings.Builder
	for _, record := range records[index:min(index+maxComponents, len(records))] {
		for _, key := range []string{"font", "line", "size"} {
			if !rec.Equal(record[key], first[key]) {
				return nil
			}
		}
		if len(components) > 0 && !continuation(record, paint) {
			return nil
		}
		text.WriteString(rec.FetchString(record, "text"))
		if !strings.HasPrefix(target, text.String()) {
			return nil
		}
		components = append(components, record)
		if text.String() == target {
			return components
		}
	}
	return nil
}

func continuation(record, paint rec.Map) bool {
	quad := rec.FetchList(record, "metric_quad")
	if !within(quad[0:2], quad[2:4]) || !within(quad[4:6], quad[6:8]) {
		return false
	}
	baseline, advance := rec.FetchList(paint, "baseline"), rec.FetchList(paint, "advance")
	endpoint := make([]any, len(baseline))
	for position := range baseline {
		a, _ := rec.Number(baseline[position])
		b, _ := rec.Number(advance[position])
		endpoint[position] = a + b
	}
	at := rec.FetchList(record, "baseline")
	return within(at, endpoint) || within(at, baseline)
}
