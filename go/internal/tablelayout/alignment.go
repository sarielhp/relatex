package tablelayout

import "github.com/sarielhp/relatex/internal/rec"

var horizontalAlignments = map[string]string{"l": "left", "c": "center", "r": "right"}

// HorizontalAlignment is the CSS alignment of a cell's l/c/r (or already CSS) alignment.
func HorizontalAlignment(cell rec.Map) string {
	alignment := rec.ToS(rec.FetchOr(cell, "alignment", "l"))
	if value, ok := horizontalAlignments[alignment]; ok {
		return value
	}
	return rec.ToS(rec.FetchOr(cell, "alignment", "left"))
}
