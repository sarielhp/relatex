package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
)

func TestCaptionTagsKeepOneEdgeFigcaption(t *testing.T) {
	nodes := rec.Map{"a": rec.Map{"type": "paragraph"}, "b": rec.Map{"type": "paragraph"},
		"first": rec.Map{"type": "float_caption"}, "middle": rec.Map{"type": "float_caption"}, "last": rec.Map{"type": "float_caption"}}
	cases := []struct {
		children []any
		want     map[string]string
	}{
		{[]any{"a", "last"}, map[string]string{"last": "figcaption"}},
		{[]any{"first", "a"}, map[string]string{"first": "figcaption"}},
		{[]any{"a", "middle", "b"}, map[string]string{"middle": "div"}},
		{[]any{"first", "a", "last"}, map[string]string{"first": "figcaption", "last": "div"}},
		{[]any{"a", "middle", "last"}, map[string]string{"middle": "div", "last": "figcaption"}},
	}
	for _, c := range cases {
		got := htmlkit.CaptionTags(rec.Map{"children": c.children}, nodes)
		if len(got) != len(c.want) {
			t.Errorf("%v: %v", c.children, got)
		}
		for id, tag := range c.want {
			if got[id] != tag {
				t.Errorf("%v: %s is %q, want %q", c.children, id, got[id], tag)
			}
		}
	}
}

func TestInlineCaptionIsADivisionReadOnceWhereItStands(t *testing.T) {
	p := &Paper{nodes: rec.Map{
		"f":  rec.Map{"id": "f", "type": "figure_float", "children": []any{"p1", "c", "p2"}},
		"c":  rec.Map{"id": "c", "type": "float_caption", "children": []any{}},
		"p1": rec.Map{"id": "p1", "type": "figure_float", "children": []any{}},
		"p2": rec.Map{"id": "p2", "type": "figure_float", "children": []any{}},
	}}
	html := p.floatNode(rec.FetchMap(p.nodes, "f"))
	for _, want := range []string{`<figure id="f" data-structure="figure_float" class="figure_float" style="margin:0">`,
		`<div id="c" data-structure="float_caption" class="figcaption"></div>`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %s", want, html)
		}
	}
}
