package semantic

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func listingNodes(second rec.Map) rec.Map {
	line := func(id string, spans ...any) rec.Map {
		return rec.Map{"id": id, "type": "paragraph", "children": []any{}, "content": spans}
	}
	return rec.Map{
		"c":  rec.Map{"id": "c", "type": "code", "children": []any{"l1", "l2"}},
		"l1": line("l1", rec.Map{"type": "text", "text": "1 a = `b`;"}),
		"l2": line("l2", second),
	}
}

// A listing is one fenced block in Markdown and one raw block in Typst, its lines in order, with
// the printed text (line numbers too); a fence outgrows the backticks of the code (decision 120).
func TestCodeIsAFencedAndARawBlock(t *testing.T) {
	nodes := listingNodes(rec.Map{"type": "text", "text": "2 end", "style": rec.Map{"series": "bx"}})
	md := &markdown{}
	md.export = &export{w: md, nodes: nodes}
	if got, want := md.export.renderNode("c", 0), "<a id=\"c\"></a>\n\n```\n1 a = `b`;\n2 end\n```"; got != want {
		t.Errorf("markdown = %q, want %q", got, want)
	}
	ty := &typst{}
	ty.export = &export{w: ty, nodes: nodes}
	if got, want := ty.export.renderNode("c", 0), "#raw(block: true, \"1 a = `b`;\\n2 end\") <c>"; got != want {
		t.Errorf("typst = %q, want %q", got, want)
	}
	if fence("x ```` y") != "`````" {
		t.Error("fence does not outgrow the code's backticks")
	}
}

// A line holding a formula is not raw text: the listing is written as its lines.
func TestCodeWithAFormulaIsWrittenAsLines(t *testing.T) {
	nodes := listingNodes(rec.Map{"type": "math", "node": "m"})
	nodes["m"] = rec.Map{"id": "m", "type": "math", "children": []any{}}
	md := &markdown{}
	md.export = &export{w: md, nodes: nodes}
	if _, ok := md.export.codeText(nodes["c"].(rec.Map)); ok {
		t.Error("a formula line was taken as raw text")
	}
}
