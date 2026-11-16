package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

// A subsection heading (entity 26) followed by a starred display (entity 28) whose \tag prints (‡).
func taggedDisplayEvidence() *Evidence {
	heading := textBlock("heading", "heading", int64(26), "4.2 Induction step")
	display := textBlock("display", "display_math", int64(28))
	tag := textBlock("tag", "paragraph", int64(28), "(‡)")
	records := []rec.Map{{"kind": "diagnostic", "code": "equation_context", "source": "latex-equation-hooks",
		"environment": "equation*", "entity": int64(28), "seq": int64(10)}}
	return &Evidence{blocks: []rec.Map{heading, display, tag}, records: records}
}

func resolvedBlock(e *Evidence, label rec.Map) string {
	e.targets = nil
	e.resolveLabel(label["name"], label)
	if len(e.targets) != 1 {
		return ""
	}
	return toS(e.targets[0]["block"])
}

// A \tag on an unnumbered display (papers5/1350 eq:indhyp, papers4/1172 61): the label records
// the subsection's counter, but names the display that prints its tag.
func TestTaggedStarredDisplayLabelNamesTheDisplay(t *testing.T) {
	label := rec.Map{"kind": "label", "name": "eq:indhyp", "counter": "subsection", "number": `{$\ddagger $}`,
		"entity": int64(28), "target_entity": int64(26), "anchor": "AMS.3", "seq": int64(12)}
	if got := resolvedBlock(taggedDisplayEvidence(), label); got != "display" {
		t.Fatalf("tagged display label targets %q, want the display", got)
	}
}

// A section label, and a label of an untagged starred display (whose number is the section's,
// even after an earlier tag left amsmath's anchor), stay on the section heading.
func TestSectionLabelBesideAStarredDisplayKeepsTheSection(t *testing.T) {
	cases := map[string]rec.Map{
		"section label": {"kind": "label", "name": "sec:ind", "counter": "subsection", "number": "4.2",
			"entity": int64(26), "target_entity": int64(26), "anchor": "subsection.4.2", "seq": int64(5)},
		"untagged display label": {"kind": "label", "name": "eq:plain", "counter": "subsection", "number": "4.2",
			"entity": int64(28), "target_entity": int64(26), "anchor": "AMS.3", "seq": int64(12)},
		"no tag anchor": {"kind": "label", "name": "eq:other", "counter": "subsection", "number": "4.2",
			"entity": int64(28), "target_entity": int64(26), "anchor": "subsection.4.2", "seq": int64(12)},
	}
	for name, label := range cases {
		if got := resolvedBlock(taggedDisplayEvidence(), label); got != "heading" {
			t.Errorf("%s targets %q, want the heading", name, got)
		}
	}
}

// Without hyperref a \tag leaves no AMS anchor: the label still names the starred display when a
// block of the display prints its tag, and keeps its section when none does.
func TestTaggedStarredDisplayLabelWithoutHyperref(t *testing.T) {
	tagged := rec.Map{"kind": "label", "name": "eq:indhyp", "counter": "subsection", "number": `{$\ddagger $}`,
		"entity": int64(28), "target_entity": int64(26), "anchor": "", "seq": int64(12)}
	if got := resolvedBlock(taggedDisplayEvidence(), tagged); got != "display" {
		t.Fatalf("tagged display label without an anchor targets %q, want the display", got)
	}
	untagged := rec.Merge(tagged, rec.Map{"name": "eq:plain", "number": `{$\dagger $}`})
	if got := resolvedBlock(taggedDisplayEvidence(), untagged); got != "heading" {
		t.Fatalf("label of a display printing another tag targets %q, want the heading", got)
	}
}

// A tag typeset as math (\tag{$A$}, papers2/0431) joins its display as text and an inline formula
// before the display formula: the display prints (A), and no other number.
func TestDisplayPrintsItsMathTag(t *testing.T) {
	formulas := omap.New[any, rec.Map]()
	formulas.Set(int64(3), rec.Map{"formula": int64(3), "display": true})
	formulas.Set(int64(4), rec.Map{"formula": int64(4), "display": false})
	display := rec.Map{"id": "display", "type": "display_math", "entity": int64(12), "content": []any{
		rec.Map{"type": "text", "text": "("}, rec.Map{"type": "math", "formula": int64(4)}, rec.Map{"type": "text", "text": ")"},
		rec.Map{"type": "math", "formula": int64(3)}}}
	records := []rec.Map{{"kind": "math", "formula": int64(4), "mathml": `<math><mi>𝐴</mi></math>`},
		{"kind": "math", "formula": int64(3), "mathml": `<math display="block"><mi>x</mi></math>`}}
	e := &Evidence{blocks: []rec.Map{display}, records: records, formulas: formulas}
	if !e.printsTag(display, "A") {
		t.Fatal("display does not print its math tag (A)")
	}
	for _, number := range []string{"B", "x", "Ax"} {
		if e.printsTag(display, number) {
			t.Errorf("display prints (%s)", number)
		}
	}
}

// prolog.sty's \plabel (papers3/0593) steps querynum, which no context owns, and prints its number
// as math, $\bm{\mathcal{Q}}_1$, in a line of its own: the label names that line.
func TestMathNumberLabelNamesItsPrintedTag(t *testing.T) {
	math := func(formula int64) rec.Map { return rec.Map{"type": "math", "formula": formula} }
	query := textBlock("query", "paragraph", int64(29), "en:Gundam (:sameAs*/:isLocatedIn)+")
	tag := rec.Map{"id": "tag", "type": "paragraph", "entity": int64(29), "content": []any{
		rec.Map{"type": "text", "text": "("}, math(4), rec.Map{"type": "text", "text": ")"}}}
	other := rec.Map{"id": "other", "type": "paragraph", "entity": int64(29), "content": []any{
		rec.Map{"type": "text", "text": "("}, math(5), rec.Map{"type": "text", "text": ")"}}}
	mathml := func(formula int64, letter string) rec.Map {
		return rec.Map{"kind": "math", "formula": formula,
			"mathml": `<math><msub><mrow><mi>` + letter + `</mi></mrow><mn>1</mn></msub></math>`}
	}
	e := &Evidence{blocks: []rec.Map{query, tag, other}, records: []rec.Map{mathml(4, "𝓠"), mathml(5, "𝓔")}}
	label := rec.Map{"kind": "label", "name": "Q gundam", "counter": "querynum", "number": `$\bm  {\mathcal  {Q}}_1$`,
		"entity": int64(29), "target_entity": int64(0), "anchor": "querynum.1", "seq": int64(1944)}
	if got := resolvedBlock(e, label); got != "tag" {
		t.Fatalf("\\plabel label targets %q, want the printed tag", got)
	}
}
