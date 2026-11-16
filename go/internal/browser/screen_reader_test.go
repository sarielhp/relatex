package browser

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// Text in an ink the author hid is marked aria-hidden as the HTML writer marks it: not a rule,
// not text on an author's fill, not text in a shown ink.
func TestHiddenMarksFollowTheWriter(t *testing.T) {
	fill := rec.Map{"type": "rule", "color": "#ffffff", "width_sp": int64(6_553_600), "height_sp": int64(655_360),
		"depth_sp": int64(0), "margin_left_sp": int64(0), "margin_right_sp": int64(-6_553_600)}
	content := []any{
		rec.Map{"type": "text", "text": "hidden", "color": "#ffffff"},
		rec.Map{"type": "text", "text": "shown", "color": "#ff0000"},
		rec.Map{"type": "rule", "color": "#ffffff", "width_sp": int64(65_536), "height_sp": int64(0), "depth_sp": int64(0)},
		fill,
		rec.Map{"type": "text", "text": "on fill", "color": "#ffffff"},
	}
	marked := hiddenMarks(content, map[string]bool{"#ffffff": true}, nil)
	for index, want := range []bool{true, false, false, false, false} {
		if got := rec.AsMap(marked[index])["aria_hidden"] == "true"; got != want {
			t.Errorf("span %d: aria_hidden %v, want %v", index, got, want)
		}
	}
	if _, changed := rec.AsMap(content[0])["aria_hidden"]; changed {
		t.Error("the document's span was changed")
	}
}

// The page's language and hidden marks are part of the blocks it must keep: a page without the
// span's lang or with another one, reading hidden text aloud, or hiding shown text, is a changed
// block.
func TestBlocksCompareLanguageAndHiddenText(t *testing.T) {
	span := rec.Map{"type": "text", "text": "English", "lang": "en"}
	hidden := rec.Map{"type": "text", "text": "hidden", "color": "#ffffff"}
	blocks := []rec.Map{{"id": "n-0", "type": "paragraph", "content": hiddenMarks([]any{span, hidden}, map[string]bool{"#ffffff": true}, nil)}}
	page := func(spans ...rec.Map) rec.Map {
		content := []any{}
		for _, item := range spans {
			content = append(content, item)
		}
		return rec.Map{"blocks": []any{rec.Map{"id": "n-0", "type": "paragraph", "content": content}},
			"structures": []any{}, "math": []any{}}
	}
	marked := rec.Merge(hidden, rec.Map{"aria_hidden": "true"})
	verifyBlocksAndMath(page(span, marked), nil, blocks, nil, rec.Map{}, nil, 0)
	for name, inventory := range map[string]rec.Map{
		"no lang":      page(rec.Map{"type": "text", "text": "English"}, marked),
		"other lang":   page(rec.Merge(span, rec.Map{"lang": "de"}), marked),
		"read aloud":   page(span, hidden),
		"hidden shown": page(rec.Merge(span, rec.Map{"aria_hidden": "true"}), marked),
	} {
		if message := failure(func() { verifyBlocksAndMath(inventory, nil, blocks, nil, rec.Map{}, nil, 0) }); message != "Browser changed paper blocks" {
			t.Errorf("%s: %q", name, message)
		}
	}
}

// failure is the message a check stops with, "" when it passes.
func failure(run func()) (message string) {
	var err error
	func() {
		defer rec.Recover(&err)
		run()
	}()
	if err != nil {
		message = err.Error()
	}
	return message
}
