package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func codeNodes() rec.Map {
	line := func(id string, spans ...any) rec.Map {
		return rec.Map{"id": id, "type": "paragraph", "children": []any{}, "content": spans}
	}
	bold := rec.Map{"type": "text", "text": "int", "color": "#0000ff"}
	return rec.Map{
		"c":  rec.Map{"id": "c", "type": "code", "children": []any{"l1", "l2"}},
		"l1": line("l1", rec.Map{"type": "text", "text": "1 struct customer {"}),
		"l2": line("l2", rec.Map{"type": "text", "text": "2 "}, bold, rec.Map{"type": "text", "text": " acct;"}),
	}
}

// A listing is one pre whose lines are code elements, one per line, in order, with the printed
// line numbers as text and the runs' own styles (decision 120).
func TestCodeIsPreWithOneCodeElementPerLine(t *testing.T) {
	paper := &Paper{document: rec.Map{"main_block_order": []any{"l1", "l2"}}, nodes: codeNodes()}
	got := paper.RenderNode("c")
	if !strings.HasPrefix(got, `<pre id="c" data-structure="code" class="code"><code id="l1" data-block="paragraph" class="code-line">`) ||
		!strings.HasSuffix(got, "</code></pre>") {
		t.Fatalf("code = %s", got)
	}
	if strings.Count(got, "<code ") != 2 || strings.Count(got, "</code>\n<code id=\"l2\"") != 1 {
		t.Errorf("lines = %s", got)
	}
	if !strings.Contains(got, ">1 struct customer {</span>") || !strings.Contains(got, ">int</span>") {
		t.Errorf("text or colour lost: %s", got)
	}
}

// Only lines (paragraph blocks) may sit in a code block.
func TestCodeRefusesOtherChildren(t *testing.T) {
	nodes := codeNodes()
	nodes["l2"] = rec.Map{"id": "l2", "type": "display_math", "children": []any{}, "content": []any{}}
	paper := &Paper{document: rec.Map{}, nodes: nodes}
	defer func() {
		if recover() == nil {
			t.Error("a display in a code block was written")
		}
	}()
	paper.RenderNode("c")
}

// The page keeps the listing's own fonts: pre and code inherit the font, and long lines wrap.
func TestCodeStyleInheritsTheFont(t *testing.T) {
	if !strings.Contains(pageStyle, "pre.code { font: inherit;") || !strings.Contains(pageStyle, "white-space: pre-wrap") {
		t.Error("code style missing")
	}
}
