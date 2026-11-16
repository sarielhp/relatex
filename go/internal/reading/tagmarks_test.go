package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/textstyle"
)

func markedParagraph(e *Evidence, items ...rec.Map) rec.Map {
	block := rec.Map{"type": "paragraph", "entity": int64(7), "content": []any{}}
	for _, item := range items {
		e.appendText(block, item, spanPaint{}, nil)
	}
	return block
}

// A paragraph of tag material is the equation's number by the collector's mark, whatever its shape
// (\tag{by computation}); one that mixes tag material and other text is not; an unmarked paragraph is
// judged by its shape, as before the mark.
func TestMarkedTagIsAnEquationNumber(t *testing.T) {
	e := &Evidence{textStyles: textstyle.Catalog{}}
	long := markedParagraph(e, rec.Map{"text": "(by computation)", "space": "", "tag": true})
	if isEquationNumber(long) || !e.isMarkedEquationNumber(long) {
		t.Error("a marked long tag is not read as the number")
	}
	mixed := markedParagraph(e, rec.Map{"text": "(1)", "space": "", "tag": true}, rec.Map{"text": "and", "space": " "})
	if e.isMarkedEquationNumber(mixed) {
		t.Error("tag material mixed with text is read as a number")
	}
	shaped := markedParagraph(e, rec.Map{"text": "(2)", "space": ""})
	if !e.isMarkedEquationNumber(shaped) {
		t.Error("an unmarked (2) lost the shape fallback")
	}
}

// A display whose marked tag holds no math (\tag*{x}, attached before the formula) prints "x" by the
// mark; the same spans unmarked do not (a text-only tag is taggedDisplays').
func TestMarkedTextTagPrintsOnItsDisplay(t *testing.T) {
	formulas := omap.New[any, rec.Map]()
	formulas.Set(int64(3), rec.Map{"formula": int64(3), "display": true})
	for _, marked := range []bool{true, false} {
		e := &Evidence{textStyles: textstyle.Catalog{}, formulas: formulas}
		tag := markedParagraph(e, rec.Map{"text": "(B)", "space": "", "tag": marked})
		display := rec.Map{"type": "display_math", "entity": int64(7),
			"content": append(rec.FetchList(tag, "content"), rec.Map{"type": "math", "formula": int64(3)})}
		if got := e.printsMathDisplayTag(display, "B"); got != marked {
			t.Errorf("marked=%v: display prints (B) = %v", marked, got)
		}
	}
}
