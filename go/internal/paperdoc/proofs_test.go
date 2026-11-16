package paperdoc

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func face(series, shape string) rec.Map { return rec.Map{"series": series, "shape": shape} }

func text(value string, style rec.Map) rec.Map {
	return rec.Map{"type": "text", "text": value, "style": style}
}

func paragraph(content ...any) rec.Map {
	return rec.Map{"type": "paragraph", "content": content, "children": []any{}}
}

// proofDocument is a document of the given children (paragraph or list nodes) in one section.
func proofDocument(children ...rec.Map) *builder {
	b := &builder{nodes: rec.Map{}, identity: rec.Map{"input_sha256": "test"}}
	ids := []any{}
	for index, child := range children {
		id := "p" + string(rune('a'+index))
		child["id"] = id
		b.nodes[id] = child
		ids = append(ids, id)
	}
	b.nodes["s"] = rec.Map{"id": "s", "type": "section", "children": ids}
	b.nodes["root"] = rec.Map{"id": "root", "type": "document", "children": []any{"s"}}
	return b
}

func sectionChildren(b *builder) []string {
	result := []string{}
	for _, id := range rec.FetchList(rec.AsMap(b.nodes["s"]), "children") {
		kind := rec.FetchString(rec.AsMap(b.nodes[toS(id)]), "type")
		result = append(result, kind)
	}
	return result
}

func TestInferProofsWrapsAStyledHeadingToItsMark(t *testing.T) {
	body, bold := face("m", "n"), face("bx", "n")
	b := proofDocument(
		paragraph(text("Before.", body)),
		paragraph(text("Proof.", bold), text(" It holds.", body)),
		paragraph(text("Therefore it ends. ∎", body)),
		paragraph(text("After.", body)))
	if found := b.inferProofs("root"); found != 1 {
		t.Fatalf("found %d proofs", found)
	}
	if got := sectionChildren(b); len(got) != 3 || got[1] != "proof" {
		t.Fatalf("section children %v", got)
	}
}

func TestInferProofsNeedsTheMarkAndTheStyle(t *testing.T) {
	body, bold := face("m", "n"), face("bx", "n")
	cases := map[string]*builder{
		// "Proof of concept." in bold opens nothing without a mark.
		"no mark": proofDocument(paragraph(text("Proof of concept.", bold), text(" We built it.", body)),
			paragraph(text("It works.", body))),
		// A plain "Proof." in the body face is no heading.
		"unstyled": proofDocument(paragraph(text("Proof. It holds. ∎", body))),
		// A heading between the two ends stops the search.
		"interrupted": proofDocument(paragraph(text("Proof.", bold), text(" It holds.", body)),
			rec.Map{"type": "heading", "content": []any{}, "children": []any{}},
			paragraph(text("Done. ∎", body))),
		// A proof does not run across a run-in sectioning heading.
		"across run-in": proofDocument(paragraph(text("Proof.", bold), text(" It holds.", body)),
			runIn(paragraph(text("Remark.", bold), text(" Done. ∎", body)))),
	}
	for name, b := range cases {
		if found := b.inferProofs("root"); found != 0 {
			t.Errorf("%s: found %d proofs", name, found)
		}
	}
}

func TestInferProofsReadsAFormulaOrRuleMark(t *testing.T) {
	body, italic := face("m", "n"), face("m", "it")
	b := proofDocument(
		paragraph(text("Proof of Lemma 2:", italic), text(" Clear.", body), rec.Map{"type": "math", "node": "m1"}),
		paragraph(text("Proof.", italic), text(" Also clear.", body),
			rec.Map{"type": "rule", "width_sp": int64(4 * 65536), "height_sp": int64(4 * 65536), "depth_sp": int64(0)}))
	b.nodes["m1"] = rec.Map{"id": "m1", "type": "math", "mathml": `<math><mi mathvariant="normal">□</mi></math>`}
	if found := b.inferProofs("root"); found != 2 {
		t.Fatalf("found %d proofs", found)
	}
}

func TestInferProofsNestsAClaimProofAndReadsLooseHeadings(t *testing.T) {
	body, italic := face("m", "n"), face("m", "it")
	b := proofDocument(
		paragraph(text("Proof:", italic), text(" Outer.", body)),
		paragraph(text("Proof of Claim 1:", italic), text(" Inner. □", body)),
		rec.Map{"type": "figure_float", "content": []any{}, "children": []any{}},
		paragraph(text("This proves the lemma. □", body)),
		// A heading alone in its paragraph, a display after it.
		paragraph(text("Proof:", italic)),
		rec.Map{"type": "display_math", "content": []any{}, "children": []any{}},
		paragraph(text("Done. □", body)),
		// A cross-reference in the heading, its colon after the styled run.
		paragraph(text("Proof of Lemma ", italic), rec.Map{"type": "reference", "text": "3", "style": body},
			text(": Clear. □", body)))
	if found := b.inferProofs("root"); found != 4 {
		t.Fatalf("found %d proofs", found)
	}
	if got := sectionChildren(b); len(got) != 3 {
		t.Fatalf("section children %v", got)
	}
	outer := rec.AsMap(b.nodes[toS(rec.FetchList(rec.AsMap(b.nodes["s"]), "children")[0])])
	inner := rec.AsMap(b.nodes[toS(rec.FetchList(outer, "children")[1])])
	if rec.FetchString(inner, "type") != "proof" {
		t.Fatalf("inner proof not nested: %v", inner)
	}
}

// runIn marks a paragraph as opening with a run-in sectioning heading, as the reading's semantics do.
func runIn(node rec.Map) rec.Map {
	node["semantics"] = rec.Map{"kind": "heading", "runin": true, "level": int64(4), "title_spans": int64(1)}
	return node
}

func TestInferProofsReadsAParagraphUnderARunInHeading(t *testing.T) {
	body, italic := face("m", "n"), face("m", "it")
	under := paragraph(text("Proof:", italic), text(" It holds. ∎", body))
	under["semantics"] = rec.Map{"kind": "heading", "runin": true, "level": int64(2)}
	b := proofDocument(runIn(paragraph(text("1. Lemmas.", body))), under)
	if found := b.inferProofs("root"); found != 1 {
		t.Fatalf("found %d proofs", found)
	}
}

// A run-in heading that declares a proof (\paragraph{\bf Proof.}, papers4/1041) opens one.
func TestInferProofsOpensAtARunInProofHeading(t *testing.T) {
	body, bold := face("m", "n"), face("bx", "n")
	b := proofDocument(runIn(paragraph(text("Proof.", bold), text(" It holds.", body))), paragraph(text("Done. ∎", body)))
	if found := b.inferProofs("root"); found != 1 {
		t.Fatalf("found %d proofs", found)
	}
}
