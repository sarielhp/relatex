package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A second paragraph of a heading's title box continues the heading (decision 126: slides/0007,
// slides/0020, an IEEEtran appendix's mark and title); a paragraph of another entity, a body
// paragraph or an empty heading block opens its own block.
func TestHeadingTitleParagraphsContinueTheHeading(t *testing.T) {
	heading := rec.Map{"id": "b4", "type": "section", "entity": int64(23),
		"content": []any{rec.Map{"type": "text", "text": "Early-exercise boundary"}}}
	line := rec.Map{"owner": int64(9), "role": "section", "entity": int64(23)}
	if !headingContinues(heading, line) {
		t.Fatal("a title box's next paragraph continues its heading")
	}
	for _, owner := range []rec.Map{
		{"owner": int64(9), "role": "section", "entity": int64(24)},
		{"owner": int64(9), "role": "paragraph", "entity": int64(23)},
	} {
		if headingContinues(heading, owner) {
			t.Errorf("%v must open a block of its own", owner)
		}
	}
	if headingContinues(rec.Map{"type": "section", "entity": int64(23), "content": []any{}}, line) ||
		headingContinues(rec.Map{"type": "paragraph", "entity": int64(23), "content": []any{1}}, line) {
		t.Error("only a heading block with text is continued")
	}
}

// The heading's lines meet at a line-break span of one space, which later text never joins: a
// beamer frame title broken with \\, a frame title and its subtitle, and an IEEEtran appendix's
// mark and title each read as one heading with a space between the lines.
func TestHeadingLinesMeetAtALineBreak(t *testing.T) {
	for _, lines := range [][2]string{
		{"Early-exercise boundary", "– American call"},
		{"Black–Scholes", "The pricing formula"},
		{"Appendix A", "Proof"},
	} {
		e := &Evidence{}
		block := rec.Map{"content": []any{rec.Map{"type": "text", "text": lines[0]}, lineBreak()}}
		e.appendText(block, rec.Map{"type": "text", "text": lines[1], "space": ""}, spanPaint{}, nil)
		content := rec.FetchList(block, "content")
		if len(content) != 3 {
			t.Fatalf("the next line opens a span of its own, got %v", content)
		}
		if brk := rec.AsMap(content[1]); brk["text"] != " " || brk["line_break"] != true {
			t.Errorf("the break is one space marked line_break, got %v", brk)
		}
		var text string
		for _, span := range content {
			text += rec.FetchString(rec.AsMap(span), "text")
		}
		if text != lines[0]+" "+lines[1] {
			t.Errorf("the heading reads %q", text)
		}
	}
	// A one-line heading's words join as before.
	e := &Evidence{}
	block := rec.Map{"content": []any{rec.Map{"type": "text", "text": "Intro"}}}
	e.appendText(block, rec.Map{"type": "text", "text": "duction", "space": ""}, spanPaint{}, nil)
	if content := rec.FetchList(block, "content"); len(content) != 1 || rec.AsMap(content[0])["text"] != "Introduction" {
		t.Errorf("a one-line heading is one span, got %v", content)
	}
}
