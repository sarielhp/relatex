package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/textstyle"
)

// Plain \underline{debunked word} and \underline{Procedure $P$ here}: the space before an underlined
// run is set outside the underline box and joins the plain text; the spaces inside the run, and one
// after a formula inside it, keep the decoration.
func TestSpaceBeforeAnUnderlinedRunIsPlainText(t *testing.T) {
	roman := rec.Map{"font_id": int64(1), "family": "lmr", "series": "m", "shape": "n", "generic": "serif"}
	e := &Evidence{textStyles: textstyle.Catalog{1: roman}}
	block := rec.Map{"content": []any{}}
	add := func(text, space string, decorated bool) {
		item := rec.Map{"text": text, "space": space, "style": int64(1)}
		if decorated {
			item["decoration"] = "underline"
		}
		e.appendText(block, item, spanPaint{}, nil)
	}
	add("Plain", "", false)
	add("debunked", " ", true)
	add("word", " ", true)
	push(block, rec.Map{"type": "math", "formula": int64(1)})
	add("here", " ", true)
	add("end", " ", false)
	want := []struct {
		text      string
		decorated bool
	}{{"Plain ", false}, {"debunked word", true}, {"", false}, {" here", true}, {" end", false}}
	spans := rec.FetchList(block, "content")
	if len(spans) != len(want) {
		t.Fatalf("spans = %v", spans)
	}
	for index, expected := range want {
		span := rec.AsMap(spans[index])
		if span["type"] == "math" {
			continue
		}
		if span["text"] != expected.text || (span["decoration"] == "underline") != expected.decorated {
			t.Errorf("span %d = %v, want %q decorated %v", index, span, expected.text, expected.decorated)
		}
	}
}

// soul's "\ul{one} \st{two words} done": an underlined run and a struck-out run stay apart, the
// space between them is plain text, and the space inside the struck-out run keeps its strike.
func TestUnderlinedAndStruckRunsStayApart(t *testing.T) {
	roman := rec.Map{"font_id": int64(1), "family": "lmr", "series": "m", "shape": "n", "generic": "serif"}
	e := &Evidence{textStyles: textstyle.Catalog{1: roman}}
	block := rec.Map{"content": []any{}}
	for _, item := range []rec.Map{
		{"text": "one", "space": "", "decoration": "underline"},
		{"text": "two", "space": " ", "decoration": "strike"},
		{"text": "words", "space": " ", "decoration": "strike"},
		{"text": "done", "space": " "},
	} {
		item["style"] = int64(1)
		e.appendText(block, item, spanPaint{}, nil)
	}
	want := [][2]any{{"one", "underline"}, {" ", nil}, {"two words", "strike"}, {" done", nil}}
	spans := rec.FetchList(block, "content")
	if len(spans) != len(want) {
		t.Fatalf("spans = %v", spans)
	}
	for index, expected := range want {
		span := rec.AsMap(spans[index])
		if span["text"] != expected[0] || span["decoration"] != expected[1] {
			t.Errorf("span %d = %v, want %v", index, span, expected)
		}
	}
}

// slides/0016: "was \href{doi}{\underline{debunked}}" keeps the space before the link outside it (one
// link, its text the underlined word); inside a link ("\href{u}{a \underline{b}}") the space stays in
// the link, undecorated.
func TestSpaceBeforeAnUnderlinedLinkStaysOutsideTheLink(t *testing.T) {
	roman := rec.Map{"font_id": int64(1), "family": "lmr", "series": "m", "shape": "n", "generic": "serif"}
	for name, link := range map[string]any{"link starts at the underline": nil, "underline inside the link": int64(4)} {
		e := &Evidence{textStyles: textstyle.Catalog{1: roman}}
		block := rec.Map{"content": []any{}}
		first := rec.Map{"text": "was", "space": "", "style": int64(1)}
		if link != nil {
			first["external_link"] = link
		}
		e.appendText(block, first, spanPaint{}, nil)
		e.appendText(block, rec.Map{"text": "debunked", "space": " ", "style": int64(1), "decoration": "underline",
			"external_link": int64(4)}, spanPaint{}, nil)
		spans := rec.FetchList(block, "content")
		if len(spans) != 2 {
			t.Fatalf("%s: spans = %v", name, spans)
		}
		plain, linked := rec.AsMap(spans[0]), rec.AsMap(spans[1])
		if plain["text"] != "was " || !rec.Equal(plain["external_link"], link) || linked["text"] != "debunked" ||
			linked["external_link"] != int64(4) {
			t.Errorf("%s: spans %v", name, spans)
		}
	}
}
