package reading

import "github.com/sarielhp/relatex/internal/rec"

// SlotKey is a flow item as the glyph map records it on each glyph (decision 105): its owner, its
// slot and the page that shipped it.
type SlotKey struct{ Owner, Slot, Page int64 }

// recordSlot remembers the block a flow item went into: the tagged PDF binds each glyph to its
// block through the item it was typeset from. An item shipped twice on one page keeps its first
// block.
func (e *Evidence) recordSlot(owner rec.Map, page any) {
	block := e.lastBlocks[e.readingPartition(owner)]
	number, ok := page.(int64)
	if block == nil || !ok {
		return
	}
	key := SlotKey{rec.AsInt(rec.Fetch(owner, "owner")), e.activeSlot, number}
	if e.slotBlocks == nil {
		e.slotBlocks = map[SlotKey]rec.Map{}
	}
	if _, seen := e.slotBlocks[key]; !seen {
		e.slotBlocks[key] = block
	}
}

// SlotBlocks are the reading blocks' ids by the flow items they hold, once the report is made.
func (e *Evidence) SlotBlocks() map[SlotKey]string {
	result := make(map[SlotKey]string, len(e.slotBlocks))
	for key, block := range e.slotBlocks {
		id, ok := block["id"].(string)
		switch {
		case ok && e.titleSlots[key] && titleSpans(block) > 0:
			result[key] = id + TitleSuffix
		case ok:
			result[key] = id
		}
	}
	return result
}
