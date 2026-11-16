package reading

import "github.com/sarielhp/relatex/internal/rec"

// A run-in heading's title (decision 088, amendment 1): the collector marks the glyphs of the
// title it sets at the start of the next paragraph (runin_title); the block records how many of
// its first spans are the title (title_spans), and no span joins title and body.

// observeTitled observes an item and, when it belongs to a run-in title, extends the title of the
// block it went into, as long as every span before it is the title's.
func (e *Evidence) observeTitled(item, owner rec.Map, page any) {
	if !truthy(item["runin_title"]) {
		e.observeItem(item, owner, page)
		return
	}
	part := e.readingPartition(owner)
	before, start := e.lastBlocks[part], 0
	if before != nil {
		start = len(rec.FetchList(before, "content"))
	}
	e.observeItem(item, owner, page)
	block := e.lastBlocks[part]
	if block == nil {
		return
	}
	if before == nil || !rec.Equal(before["id"], block["id"]) {
		start = 0
	}
	if titleSpans(block) == start {
		if count := len(rec.FetchList(block, "content")); count > 0 {
			block["title_spans"] = int64(count)
			e.titleSlot(owner, page)
		}
	}
}

// titleSlot remembers that the active flow item is its block's run-in title, so its glyphs belong
// to the title (SlotBlocks).
func (e *Evidence) titleSlot(owner rec.Map, page any) {
	number, ok := page.(int64)
	if !ok {
		return
	}
	if e.titleSlots == nil {
		e.titleSlots = map[SlotKey]bool{}
	}
	e.titleSlots[SlotKey{rec.AsInt(rec.Fetch(owner, "owner")), e.activeSlot, number}] = true
}

// TitleSuffix marks a slot block that is a run-in title (SlotBlocks): the glyph owners give its
// glyphs the title, the tagged PDF its H.
const TitleSuffix = "#title"

// titleSpans is how many of a block's first spans are its run-in title.
func titleSpans(block rec.Map) int {
	count, _ := block["title_spans"].(int64)
	return int(count)
}

// titleEdge: the item would start the body right after the block's run-in title, so it must not
// join the title's last span.
func titleEdge(block, item rec.Map) bool {
	count := titleSpans(block)
	return count > 0 && count == len(rec.FetchList(block, "content")) && !truthy(item["runin_title"])
}
