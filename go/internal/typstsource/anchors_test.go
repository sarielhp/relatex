package typstsource

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// Tests of the Typst acceptance's writer gaps (decision 154, item C-TY): every label of the model
// is placed once, as its element's label or as a link target beside it.

const blockX = `<math xmlns="http://www.w3.org/1998/Math/MathML" display="block"><mi>x</mi></math>`

func text(value string) rec.Map { return rec.Map{"type": "text", "text": value} }

func boldText(value string) rec.Map {
	return rec.Map{"type": "text", "text": value, "style": rec.Map{"series": "bx"}}
}

// testDocument is a document of the given top-level nodes (each with an "id") and labels
// (name => target id, number).
func testDocument(top []rec.Map, extra []rec.Map, labels map[string][2]string) rec.Map {
	nodes := rec.Map{}
	ids := []any{}
	for _, node := range top {
		nodes[toS(node["id"])] = node
		ids = append(ids, node["id"])
	}
	for _, node := range extra {
		nodes[toS(node["id"])] = node
	}
	// The nodes are a section's: blocks before the first section are front matter.
	nodes["root"] = rec.Map{"id": "root", "type": "document", "children": []any{"section"}}
	nodes["section"] = rec.Map{"id": "section", "type": "section", "children": ids}
	for _, node := range nodes {
		if _, ok := node.(rec.Map)["children"]; !ok {
			node.(rec.Map)["children"] = []any{}
		}
	}
	recorded := rec.Map{}
	for name, label := range labels {
		recorded[name] = rec.Map{"target": label[0], "number": label[1]}
	}
	return rec.Map{"root": "root", "nodes": nodes, "labels": recorded}
}

// renderTest writes the document's body (no preamble) and returns it with the labels it placed.
func renderTest(t *testing.T, document rec.Map) (string, map[string]string) {
	t.Helper()
	e := &exporter{document: document, nodes: rec.FetchMap(document, "nodes"), directory: t.TempDir(), math: &mathWriter{}}
	e.index()
	return e.render(), e.placed
}

func display(id, math string, spans ...rec.Map) rec.Map {
	content := []any{rec.Map{"type": "math", "node": math}}
	for _, span := range spans {
		content = append(content, span)
	}
	return rec.Map{"id": id, "type": "display_math", "content": content}
}

func mathNode(id string) rec.Map { return rec.Map{"id": id, "type": "math", "mathml": blockX} }

func wantPlaced(t *testing.T, placed map[string]string, want map[string]string) {
	t.Helper()
	for name, placement := range want {
		if placed[name] != placement {
			t.Errorf("label %s placed as %q, want %q", name, placed[name], placement)
		}
	}
}

// A heading with two \labels (papers/0060, 0073): the first labels the heading, the second
// follows it as a link target.
func TestHeadingKeepsEverySecondLabel(t *testing.T) {
	heading := rec.Map{"id": "h", "type": "heading", "content": []any{text("1 Introduction")}}
	document := testDocument([]rec.Map{heading}, nil, map[string][2]string{"sec:a": {"h", "1"}, "sec:b": {"h", "1"}})
	output, placed := renderTest(t, document)
	if !strings.Contains(output, "= Introduction <sec:a>\n#metadata(none) <sec:b>") {
		t.Errorf("heading:\n%s", output)
	}
	wantPlaced(t, placed, map[string]string{"sec:a": element, "sec:b": anchor})
}

// A display's labels: a second one, one on its tag paragraph "(16)" (papers/0191) and one on an
// untagged display (books/0002) are all placed.
func TestDisplayPlacesTagParagraphAndSecondLabels(t *testing.T) {
	tagged := display("d1", "m1")
	tagged["semantics"] = rec.Map{"entity": int64(1)}
	tag := rec.Map{"id": "t1", "type": "paragraph", "content": []any{text("(16)")}, "semantics": rec.Map{"entity": int64(1)}}
	untagged := display("d2", "m2")
	document := testDocument([]rec.Map{tagged, tag, untagged}, []rec.Map{mathNode("m1"), mathNode("m2")},
		map[string][2]string{"eq:own": {"d1", "16"}, "eq:tag": {"t1", "16"}, "eq:untagged": {"d2", "17"}})
	output, placed := renderTest(t, document)
	if !strings.Contains(output, "$ x $ <eq:own>\n#metadata(none) <eq:tag>") {
		t.Errorf("tagged display:\n%s", output)
	}
	if !strings.Contains(output, "$ x $ <ltd-nonumber>\n#metadata(none) <eq:untagged>") {
		t.Errorf("untagged display:\n%s", output)
	}
	wantPlaced(t, placed, map[string]string{"eq:own": element, "eq:tag": anchor, "eq:untagged": anchor})
}

// A subequation row (9a) labels its display rather than the group's (9) (papers2/0245); the
// group's label follows as a link target.
func TestDisplayPrefersTheLabelOfItsPrintedTag(t *testing.T) {
	row := rec.Map{"id": "r1", "type": "math_row", "tag": "(9a)"}
	math := mathNode("m1")
	math["children"] = []any{"r1"}
	document := testDocument([]rec.Map{display("d1", "m1")}, []rec.Map{math, row},
		map[string][2]string{"eq:group": {"d1", "9"}, "eq:row": {"r1", "9a"}})
	output, placed := renderTest(t, document)
	if !strings.Contains(output, `$x$) <eq:row>`+"\n#metadata(none) <eq:group>") {
		t.Errorf("subequation:\n%s", output)
	}
	wantPlaced(t, placed, map[string]string{"eq:row": element, "eq:group": anchor})
}

// Labels on a theorem's head paragraph whose number LaTeX wrote with braces (36.4.{5},
// books/0003) and a second label on the theorem are link targets at its body's start.
func TestTheoremHeadParagraphLabelsAreLinkTargets(t *testing.T) {
	head := rec.Map{"id": "p1", "type": "paragraph", "content": []any{boldText("Claim 8.2.6."), text(" Every x is y.")}}
	theorem := rec.Map{"id": "th", "type": "theorem", "children": []any{"p1"},
		"semantics": rec.Map{"environment": "claim", "kind": "claim"}}
	document := testDocument([]rec.Map{theorem}, []rec.Map{head},
		map[string][2]string{"claim:a": {"p1", "8.2.{6}"}, "claim:b": {"th", "8.2.6"}})
	output, placed := renderTest(t, document)
	if !strings.Contains(output, "[#metadata(none) <claim:a> Every x is y.] <claim:b>") {
		t.Errorf("theorem:\n%s", output)
	}
	wantPlaced(t, placed, map[string]string{"claim:a": anchor, "claim:b": element})
}

// A \label in a bibliography entry (\label{LastBibItem}, papers/0210) is a link target there.
func TestBibliographyEntryLabelIsALinkTarget(t *testing.T) {
	entry := rec.Map{"id": "b1", "type": "paragraph", "content": []any{text("[1] A. Author, A title.")}}
	document := testDocument([]rec.Map{entry}, nil, map[string][2]string{"LastBibItem": {"b1", "1"}})
	document["citations"] = rec.Map{"entries": []any{rec.Map{"id": "e1", "key": "a", "number": "1", "blocks": []any{"b1"}}},
		"calls": []any{}}
	output, placed := renderTest(t, document)
	if !strings.Contains(output, `#bibitem("1")[A. Author, A title. #metadata(none) <LastBibItem>] <a>`) {
		t.Errorf("bibliography:\n%s", output)
	}
	wantPlaced(t, placed, map[string]string{"LastBibItem": anchor})
}
