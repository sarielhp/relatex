package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// The body's first item never joins the run-in title's last span, and the title's slots name the
// title in the slot blocks.
func TestRuninTitleKeepsItsSpans(t *testing.T) {
	block := rec.Map{"id": "b1", "content": []any{rec.Map{"type": "text", "text": "Setup."}}, "title_spans": int64(1)}
	if !titleEdge(block, rec.Map{"text": " We"}) || titleEdge(block, rec.Map{"text": "!", "runin_title": true}) {
		t.Error("only a body item stops at the title's end")
	}
	push(block, rec.Map{"type": "text", "text": " We"})
	if titleEdge(block, rec.Map{"text": " start"}) {
		t.Error("the body goes on after its first span")
	}
	e := &Evidence{activeSlot: 2, slotBlocks: map[SlotKey]rec.Map{}}
	owner := rec.Map{"owner": int64(5)}
	e.titleSlot(owner, int64(1))
	e.slotBlocks[SlotKey{5, 2, 1}], e.slotBlocks[SlotKey{5, 3, 1}] = block, block
	slots := e.SlotBlocks()
	if slots[SlotKey{5, 2, 1}] != "b1"+TitleSuffix || slots[SlotKey{5, 3, 1}] != "b1" {
		t.Errorf("slot blocks %v", slots)
	}
}

func overlayRule(width int64) rec.Map {
	return rec.Map{"type": "rule", "width_sp": width, "height_sp": int64(10), "depth_sp": int64(0),
		"margin_left_sp": int64(0), "margin_right_sp": -width}
}

// A frame title's fill rules, shipped under the title text, go with the title's heading.
func TestOverlayRulesBeforeAHeadingMoveWithIt(t *testing.T) {
	text := rec.Map{"type": "text", "text": "Frames"}
	previous := rec.Map{"id": "b1", "type": "paragraph", "content": []any{text, overlayRule(500)}}
	heading := rec.Map{"id": "b2", "type": "section", "content": []any{rec.Map{"type": "text", "text": "Title"}}}
	adoptOverlayRules(previous, heading)
	if got := rec.FetchList(previous, "content"); len(got) != 1 {
		t.Errorf("paragraph keeps %v, want its text only", got)
	}
	got := rec.FetchList(heading, "content")
	if len(got) != 2 || rec.AsMap(got[0])["type"] != "rule" || rec.AsMap(got[1])["text"] != "Title" {
		t.Errorf("heading content %v, want the rule then the title", got)
	}
}

// A black rule kerned back over that is taller than wide is a drawing's edge (chessboard's frame at
// the end of a beamer column, decision 144): it stays, so the list item after it opens with its
// drawn marker. A coloured fill of the same shape is a background and moves.
func TestOverlayStrokeStaysAndColouredFillMoves(t *testing.T) {
	text := rec.Map{"type": "text", "text": "1 2 3 4 5"}
	stroke := rec.Map{"type": "rule", "width_sp": int64(52420), "height_sp": int64(6606020), "depth_sp": int64(52420),
		"margin_left_sp": int64(0), "margin_right_sp": int64(-52420)}
	previous := rec.Map{"content": []any{text, stroke}}
	item := rec.Map{"content": []any{}}
	adoptOverlayRules(previous, item)
	if len(rec.FetchList(previous, "content")) != 2 || len(rec.FetchList(item, "content")) != 0 {
		t.Error("a drawing's edge must stay with its paragraph")
	}
	if !itemMarkerTaken(item, rec.Map{"item_marker": "square"}, []any{int64(318946), int64(318946), int64(0)}) {
		t.Error("the item after a drawing's edge must take its drawn marker")
	}
	fill := rec.Map{"type": "rule", "width_sp": int64(52420), "height_sp": int64(6606020), "depth_sp": int64(0),
		"margin_left_sp": int64(0), "margin_right_sp": int64(-52420), "color": "#e6e6e6"}
	previous = rec.Map{"content": []any{text, fill}}
	block := rec.Map{"content": []any{}}
	adoptOverlayRules(previous, block)
	if len(rec.FetchList(previous, "content")) != 1 || len(rec.FetchList(block, "content")) != 1 {
		t.Error("a coloured fill must move to the block it sits under")
	}
}

// A fill rule followed by text in its own paragraph, or a rule that is no overlay, stays.
func TestOverlayRulesFollowedByTextOrPlainStay(t *testing.T) {
	text := rec.Map{"type": "text", "text": "after"}
	previous := rec.Map{"id": "b1", "content": []any{overlayRule(500), text}}
	adoptOverlayRules(previous, rec.Map{"content": []any{}})
	if len(rec.FetchList(previous, "content")) != 2 {
		t.Error("a fill rule followed by text must stay")
	}
	plain := rec.Map{"type": "rule", "width_sp": int64(500), "margin_left_sp": int64(0), "margin_right_sp": int64(0)}
	previous = rec.Map{"content": []any{text, plain}}
	heading := rec.Map{"content": []any{}}
	adoptOverlayRules(previous, heading)
	if len(rec.FetchList(previous, "content")) != 2 || len(rec.FetchList(heading, "content")) != 0 {
		t.Error("a rule that overlays nothing must stay")
	}
	only := rec.Map{"content": []any{overlayRule(500)}}
	adoptOverlayRules(only, heading)
	if len(rec.FetchList(only, "content")) != 1 {
		t.Error("a paragraph must not be emptied")
	}
}
