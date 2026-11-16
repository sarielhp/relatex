package tables

import (
	"regexp"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// fillColor is a fill's colour as the collector records it.
var fillColor = regexp.MustCompile(`\A#[0-9a-f]{6}\z`)

// cellFills are the colortbl fills of a table's cells (\cellcolor, \rowcolor, \columncolor,
// \rowcolors): the colour of each cell's last area fill in paint order, by cell id. colortbl
// paints a column's, then a row's, then a cell's own colour over each other; the PDF shows the
// last one. The collector gives such a leader its colour and its cell; a fill of an empty cell
// under a \multirow names no cell and colours nothing. Only a leader of running height and depth
// (colortbl's \vrule, as tall as the row) is one: soul's \hl leader behind a word is not.
func (e *rich) cellFills(table any) map[string]string {
	fills := map[string]string{}
	for _, event := range e.events {
		color, colored := event["color"].(string)
		cell, owned := event["cell"]
		if event["code"] != "rich_table_rule" || !colored || !owned || !fillColor.MatchString(color) ||
			!rec.Equal(event["table"], table) || !tablemodel.AreaFill(event) || !runningLeader(event) {
			continue
		}
		fills[key(cell)] = color
	}
	return fills
}

// runningLeader: a leader rule of running height and depth, as tall as its row.
func runningLeader(event rec.Map) bool {
	return truthy(event["native_leader"]) && rec.Equal(event["height"], int64(runningSize)) &&
		rec.Equal(event["depth"], int64(runningSize))
}

// withFill gives a grid cell the fill painted behind it, if any.
func withFill(cell rec.Map, fills map[string]string) rec.Map {
	if fill, ok := fills[key(cell["source_cell"])]; ok {
		cell["fill"] = fill
	}
	return cell
}
