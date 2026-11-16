package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/textstyle"
)

// \texttt{repeat} \texttt{repeat} (papers4/0919): the roman space between two typewriter words is a
// run of its own in the roman font, so the two typewriter runs stay apart (decided 2026-11-09). A
// space of the words' own font keeps the words in one run.
func TestForeignSpaceIsARunOfItsOwn(t *testing.T) {
	roman := rec.Map{"font_id": int64(1), "family": "lmr", "series": "m", "shape": "n", "generic": "serif"}
	mono := rec.Map{"font_id": int64(2), "family": "lmtt", "series": "m", "shape": "n", "generic": "monospace"}
	e := &Evidence{textStyles: textstyle.Catalog{1: roman, 2: mono}}
	block := rec.Map{"content": []any{}}
	e.appendText(block, rec.Map{"text": "repeat", "space": "", "style": int64(2)}, spanPaint{}, nil)
	e.appendText(block, rec.Map{"text": "repeat", "space": " ", "style": int64(2), "space_style": int64(1)}, spanPaint{}, nil)
	e.appendText(block, rec.Map{"text": "again", "space": " ", "style": int64(2)}, spanPaint{}, nil)
	spans := rec.FetchList(block, "content")
	want := []struct {
		text  string
		style rec.Map
	}{{"repeat", mono}, {" ", roman}, {"repeat again", mono}}
	if len(spans) != len(want) {
		t.Fatalf("spans = %v", spans)
	}
	for index, expected := range want {
		span := rec.AsMap(spans[index])
		if span["text"] != expected.text || !rec.Equal(span["style"], expected.style) || span["space_style"] != nil {
			t.Errorf("span %d = %v, want %q in %v", index, span, expected.text, expected.style)
		}
	}
}
