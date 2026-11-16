package paperdoc

import "github.com/sarielhp/relatex/internal/rec"

// withItemMarker adds a list item's drawn marker (decision 127: a beamer theme's square or ball)
// to its fields: the item_marker the reading gave the item's first block, its first paragraph.
func (b *builder) withItemMarker(fields, node rec.Map) rec.Map {
	content := rec.FetchList(node, "content")
	if len(content) == 0 {
		return fields
	}
	id, ok := rec.AsMap(content[0])["block"]
	if !ok {
		return fields
	}
	if b.itemMarkers == nil {
		b.itemMarkers = map[string]rec.Map{}
		for _, block := range b.allBlocks() {
			if marker, marked := block["item_marker"].(rec.Map); marked {
				b.itemMarkers[toS(block["id"])] = marker
			}
		}
	}
	if marker, marked := b.itemMarkers[toS(id)]; marked {
		fields["marker"] = marker
	}
	return fields
}

// itemMarkerShapes are the shapes a drawn list marker takes: CSS's list-style-type names.
var itemMarkerShapes = map[string]bool{"square": true, "disc": true, "circle": true}

// validateItemMarker: only a list item has a marker, a known shape in an optional fill colour.
func validateItemMarker(kind string, value any) {
	marker, ok := value.(rec.Map)
	if kind != "list_item" || !ok {
		rec.Fail("Only a list item has a marker")
	}
	shape, _ := marker["shape"].(string)
	color, colored := marker["color"]
	text, isText := color.(string)
	if !itemMarkerShapes[shape] || len(marker) > 2 || (colored && (!isText || !ruleColor.MatchString(text))) ||
		(!colored && len(marker) != 1) {
		rec.Fail("Invalid list item marker")
	}
}
