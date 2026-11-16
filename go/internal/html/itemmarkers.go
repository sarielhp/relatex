package html

import (
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// listStyle is a list's or list item's style attribute. A list prints its markers as its items'
// text, so it has no list style; an item whose marker a beamer theme draws (decision 127: a square,
// a ball) has none in its text and shows it as its own list style, in the theme's colour
// (data-marker-color, painted by itemMarkerCSS).
func listStyle(node rec.Map) string {
	marker, ok := node["marker"].(rec.Map)
	if !ok {
		return ` style="list-style:none"`
	}
	attributes := ` style="list-style-type:` + rec.FetchString(marker, "shape") + `"`
	if color, colored := marker["color"].(string); colored {
		attributes += ` data-marker-color="` + color + `"`
	}
	return attributes
}

// itemMarkerCSS paints each drawn list marker's colour: one ::marker rule per colour.
func itemMarkerCSS(document rec.Map) string {
	nodes, _ := document["nodes"].(rec.Map)
	colors := []string{}
	for _, value := range nodes {
		marker, _ := rec.AsMap(value)["marker"].(rec.Map)
		if color, ok := marker["color"].(string); ok && !slices.Contains(colors, color) {
			colors = append(colors, color)
		}
	}
	slices.Sort(colors)
	var css strings.Builder
	for _, color := range colors {
		css.WriteString(`li[data-marker-color="` + color + `"]::marker{color:` + color + `}`)
	}
	return css.String()
}
