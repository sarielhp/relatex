package reading

import (
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// Rows of a box in a table cell: tabbing's rows, a box's rows set one under the other. Each row is
// a flow owner of its own (or the items of one owner's run), and the words of two rows would read
// as one line ("forI=1 tom doif violate"). The flow records, as a flow_row, the item that opens
// each row after the first with the row's leading width (its tab indent). The reading ends the
// line before that item with a line break span and keeps the indent in the span's table origin
// (tablemodel.LineIndentKey). The table's grid turns it into a line of the cell, or drops it when
// the cell wraps its lines (a p, m or b column).

// breakRow ends the line the item at slot continues, when the item opens a row. The line is the last
// span read: it must belong to the same table cell, or to no table at all (a row with no text of its
// own leaves the previous block to another flow), and no formula may be open. Outside a table the
// break is a line of the paragraph: the rows of a tabbing in an \fbox'd minipage stay lines.
func (e *Evidence) breakRow(owner rec.Map, slot int64) {
	if e.pending != nil {
		return
	}
	indent, ok := e.rowIndents[ruleKey{rec.Fetch(owner, "owner"), slot}]
	if !ok {
		return
	}
	last := e.lastBlocks[e.readingPartition(owner)]
	if last == nil || rec.Fetch(last, "type") == "display_math" {
		return
	}
	previous := lastSpan(last)
	if previous == nil || previous["line_break"] == true {
		return
	}
	origin := e.origins[rec.Identity(previous)]
	if !rec.Equal(origin["table"], e.activeTable) || !rec.Equal(origin["cell"], e.activeCell) {
		return
	}
	span := lineBreak()
	e.origins[rec.Identity(span)] = rec.Merge(e.tableOrigin(), rec.Map{tablemodel.LineIndentKey: indent})
	e.registered = append(e.registered, span)
	push(last, span)
}
