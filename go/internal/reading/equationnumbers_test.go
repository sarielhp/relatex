package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func textBlock(id, kind string, entity any, texts ...string) rec.Map {
	content := []any{}
	for _, text := range texts {
		content = append(content, rec.Map{"type": "text", "text": text})
	}
	return rec.Map{"id": id, "type": kind, "entity": entity, "pages": []any{int64(1)}, "content": content}
}

// withMath appends an inline formula and then the closing text to a block.
func withMath(block rec.Map, closing string) rec.Map {
	block["content"] = append(rec.FetchList(block, "content"), rec.Map{"type": "math", "formula": int64(7)},
		rec.Map{"type": "text", "text": closing})
	return block
}

func blockIDs(blocks []rec.Map) []string {
	ids := []string{}
	for _, block := range blocks {
		ids = append(ids, rec.FetchString(block, "id"))
	}
	return ids
}

func TestAttachEquationNumbers(t *testing.T) {
	cases := map[string]struct {
		blocks []rec.Map
		want   []string
		first  string
	}{
		"trailing number": {[]rec.Map{textBlock("p", "paragraph", int64(9), "Text"), textBlock("d", "display_math", int64(1), "x"),
			textBlock("n", "paragraph", int64(1), "(1.2)")}, []string{"p", "d"}, "x"},
		"leading number": {[]rec.Map{textBlock("n", "paragraph", int64(1), "(", "3", ")"), textBlock("d", "display_math", int64(1), "x")},
			[]string{"d"}, "("},
		"another entity": {[]rec.Map{textBlock("d", "display_math", int64(1), "x"), textBlock("n", "paragraph", int64(2), "(1)")},
			[]string{"d", "n"}, "x"},
		"prose in parentheses": {[]rec.Map{textBlock("d", "display_math", int64(1), "x"), textBlock("n", "paragraph", int64(1), "(see above)")},
			[]string{"d", "n"}, "x"},
		// \tag{$A$} and \tag{NLS$_a$}: the tag reads as text and inline formulas (papers2/0431, papers4/1172).
		"math tag": {[]rec.Map{withMath(textBlock("n", "paragraph", int64(1), "("), ")"), textBlock("d", "display_math", int64(1), "x")},
			[]string{"d"}, "("},
		"text and math tag": {[]rec.Map{textBlock("d", "display_math", int64(1), "x"), withMath(textBlock("n", "paragraph", int64(1), "(NLS"), ")")},
			[]string{"d"}, "x"},
		"prose with a formula": {[]rec.Map{textBlock("d", "display_math", int64(1), "x"), withMath(textBlock("n", "paragraph", int64(1), "(for "), ")")},
			[]string{"d", "n"}, "x"},
		// The flow's placeholder for an unmodelled rule (an algorithm box's \hline) is no number.
		"a rule placeholder": {[]rec.Map{textBlock("d", "display_math", int64(1), "x"), textBlock("n", "paragraph", int64(1), " [rule]")},
			[]string{"d", "n"}, "x"},
	}
	for name, test := range cases {
		got := attachEquationNumbers(test.blocks, isEquationNumber)
		if ids := blockIDs(got); len(ids) != len(test.want) || (len(ids) > 0 && ids[len(ids)-1] != test.want[len(test.want)-1]) {
			t.Errorf("%s: blocks %v, want %v", name, ids, test.want)
			continue
		}
		display := got[len(got)-1]
		if first := rec.FetchString(rec.AsMap(rec.FetchList(display, "content")[0]), "text"); rec.FetchString(display, "id") == "d" &&
			first != test.first {
			t.Errorf("%s: first span %q, want %q", name, first, test.first)
		}
	}
}
