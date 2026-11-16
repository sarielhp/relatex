package reading

import (
	stdmaps "maps"

	"github.com/sarielhp/relatex/internal/rec"
)

// itemMarkerTaken: a rule the collector marked as a drawn beamer item marker (decision 127: the
// Madrid square, the Frankfurt ball) is no span of the item's text. Its shape and fill colour
// become the item_marker of the block it opens, the item's first paragraph, which the document
// gives its list item as a list marker. A marked rule inside a block already begun opens no item:
// it stays a rule.
func itemMarkerTaken(block, item rec.Map, size []any) bool {
	shape, ok := item["item_marker"].(string)
	if !ok {
		return false
	}
	if _, has := block["item_marker"]; has || len(rec.FetchList(block, "content")) > 0 {
		return false
	}
	marker := rec.Map{"shape": shape}
	if len(size) > 3 && truthy(size[3]) {
		marker["color"] = size[3]
	}
	block["item_marker"] = marker
	return true
}

// dropMarkerSpace: the first words of an item whose marker is drawn open its paragraph; the space
// TeX set between the marker and them prints nothing once the marker is the list's.
func dropMarkerSpace(block, item rec.Map) rec.Map {
	if _, has := block["item_marker"]; !has || len(rec.FetchList(block, "content")) > 0 || rec.FetchString(item, "space") == "" {
		return item
	}
	trimmed := stdmaps.Clone(item)
	trimmed["space"] = ""
	return trimmed
}
