package contrast

import (
	"math"
	"slices"
	"sort"

	"github.com/sarielhp/relatex/internal/rec"
)

// Field is the document's record of the light theme's adjusted inks.
const Field = "ink_adjustments"

// Version is the record's schema version.
const Version = "ink-adjustments-1"

// Theme is one palette's backgrounds behind text: the page, theorem-like structures and proofs
// (the custom properties --bg, --theorem-bg and --proof-bg of the HTML stylesheet).
type Theme struct {
	Name        string
	Lighten     bool
	Backgrounds map[string]string
}

// Light is the light palette, the PDF's paint, which the browser checks measure.
var Light = Theme{Name: "light", Backgrounds: map[string]string{"page": "#faf9f6", "theorem": "#eef3fb", "proof": "#eef7ea"}}

// Dark is the reading page's default palette.
var Dark = Theme{Name: "dark", Lighten: true, Backgrounds: map[string]string{"page": "#1b1d22", "theorem": "#202836", "proof": "#1f2a21"}}

// Inks are the document's text colours (#rrggbb, black excluded), each with the backgrounds it
// is shown on: "fill:#rrggbb" on an author's fill (FillOf, Behind), else "theorem" or "proof"
// inside such a structure (the innermost one), "page" otherwise.
func Inks(document rec.Map) map[string][]string {
	nodes := rec.FetchMap(document, "nodes")
	parents := map[string]string{}
	for id, value := range nodes {
		children, _ := rec.AsMap(value)["children"].([]any)
		for _, child := range children {
			parents[rec.ToS(child)] = id
		}
	}
	inks, canvas, backdrops := map[string][]string{}, Canvas(document), DocumentBackdrops(document)
	for id, value := range nodes {
		node := rec.AsMap(value)
		content, _ := node["content"].([]any)
		if len(content) == 0 {
			continue
		}
		structure := backgroundOf(id, nodes, parents)
		if canvas != "" {
			structure = FillKind(canvas)
		}
		// a fill rule of the list (099) is innermost, then the PDF's backdrop (113), a cell's own
		// fill, then the canvas (107)
		eachList(content, func(list []any, base string) { addInks(inks, list, base, structure, backdrops) }, 0, "")
	}
	return inks
}

// addInks adds the text colours of one content list set on base (a table cell's fill, "" for none),
// each on its fill or else on structure.
func addInks(inks map[string][]string, list []any, base, structure string, backdrops Backdrops) {
	fills := BehindOn(list, base, backdrops)
	for index, value := range list {
		span := rec.AsMap(value)
		color, ok := span["color"].(string)
		if !ok || span["type"] == "rule" || color == "#000000" {
			continue
		}
		kind := structure
		if fills[index] != "" {
			kind = FillKind(fills[index])
		}
		if _, valid := Parse(color); valid && !slices.Contains(inks[color], kind) {
			inks[color] = append(inks[color], kind)
		}
	}
}

// backgroundOf is the background behind a node's text: its innermost theorem or proof, else the page.
func backgroundOf(id string, nodes rec.Map, parents map[string]string) string {
	for current, seen := id, 0; current != "" && seen <= len(nodes); current, seen = parents[current], seen+1 {
		switch rec.AsMap(nodes[current])["type"] {
		case "theorem":
			return "theorem"
		case "proof":
			return "proof"
		}
	}
	return "page"
}

// backgrounds are the palette's backgrounds of kinds, the page when there are none.
func (t Theme) backgrounds(kinds []string) []RGB {
	if len(kinds) == 0 {
		kinds = []string{"page"}
	}
	result := []RGB{}
	for _, kind := range kinds {
		hex, isFill := fillOfKind(kind)
		if !isFill {
			hex = t.Backgrounds[kind]
		}
		background, _ := Parse(hex)
		result = append(result, background)
	}
	return result
}

// Background is the one of kinds' backgrounds on which a colour reads worst.
func (t Theme) Background(color RGB, kinds []string) RGB {
	var worst RGB
	ratio := math.Inf(1)
	for _, background := range t.backgrounds(kinds) {
		if value := Ratio(color, background); value < ratio {
			worst, ratio = background, value
		}
	}
	return worst
}

// Shown is ink as this palette shows it on kinds' backgrounds: adjusted to TextRatio on every one
// of them when it reads too faintly, otherwise itself.
func (t Theme) Shown(color string, kinds []string) string {
	ink, ok := Parse(color)
	if !ok {
		return color
	}
	adjusted := Adjust(ink, t.backgrounds(kinds), TextRatio, t.Lighten)
	if adjusted == ink {
		return color
	}
	return adjusted.Hex()
}

// Adjustments is the record of the light palette's adjusted inks (§7.4 decision 1): each ink
// that fails TextRatio on one of its page backgrounds, with the colour shown instead, the
// background on which that colour reads worst, and the worst ratios of both; then, for each fill
// the ink is set on and fails on (decision 099), one item naming that fill, adjusted toward black
// or white (LightensOn). A hidden ink (Hidden) has no page item. Nil when no ink needs one.
func Adjustments(document rec.Map) rec.Map {
	inks := Inks(document)
	colors := make([]string, 0, len(inks))
	for color := range inks {
		colors = append(colors, color)
	}
	sort.Strings(colors)
	taken := takenColors(document, colors)
	hidden := Hidden(document)
	items := []any{}
	for _, color := range colors {
		page := PageKinds(inks[color])
		if len(page) > 0 && !hidden[color] { // a hidden ink stays invisible on the page
			items = appendAdjustment(items, color, page, "", taken)
		}
		for _, fill := range FillsOf(inks[color]) {
			items = appendAdjustment(items, color, []string{FillKind(fill)}, fill, taken)
		}
	}
	if len(items) == 0 {
		return nil
	}
	return rec.Map{"schema_version": Version, "theme": Light.Name, "method": "oklch-lightness-bisection", "items": items}
}

// appendAdjustment appends the item of color on kinds when it fails there; fill names the fill
// of a fill's item, "" for the page's.
func appendAdjustment(items []any, color string, kinds []string, fill string, taken map[string]bool) []any {
	ink, _ := Parse(color)
	backgrounds := Light.backgrounds(kinds)
	lighten := false
	if fill != "" {
		lighten = LightensOn(backgrounds[0])
	}
	adjusted := Adjust(ink, backgrounds, TextRatio, lighten)
	if adjusted == ink {
		return items
	}
	adjusted = Distinct(adjusted, ink, lighten, func(hex string) bool { return taken[hex] })
	taken[adjusted.Hex()] = true
	item := rec.Map{
		"ink": color, "adjusted": adjusted.Hex(), "background": Light.Background(adjusted, kinds).Hex(),
		"minimum_ratio": TextRatio, "ratio": rec.Round(Worst(ink, backgrounds), 2),
		"adjusted_ratio": rec.Round(Worst(adjusted, backgrounds), 2),
	}
	if fill != "" {
		item["fill"] = fill
	}
	return append(items, item)
}

// takenColors are the colours an adjustment may not become: the document's text colours and the
// text paint's inks, which the page shows as themselves.
func takenColors(document rec.Map, colors []string) map[string]bool {
	taken := map[string]bool{}
	for _, color := range colors {
		taken[color] = true
	}
	if paint, ok := document["text_paint"].(rec.Map); ok {
		list, _ := paint["inks"].([]any)
		for _, value := range list {
			taken[rec.ToS(value)] = true
		}
	}
	return taken
}

// Recorded maps each recorded ink of the document, by its Context, to the colour the light
// palette shows instead.
func Recorded(document rec.Map) map[string]string {
	record, ok := document[Field].(rec.Map)
	if !ok {
		return nil
	}
	result := map[string]string{}
	for _, value := range rec.FetchList(record, "items") {
		item := rec.AsMap(value)
		fill, _ := item["fill"].(string)
		result[Context(rec.FetchString(item, "ink"), fill)] = rec.FetchString(item, "adjusted")
	}
	return result
}

// Context names an ink on a fill ("#rrggbb on #rrggbb"), or on the page (the ink itself) when
// fill is "".
func Context(ink, fill string) string {
	if fill == "" {
		return ink
	}
	return ink + " on " + fill
}
