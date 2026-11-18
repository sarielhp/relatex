package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

func referenceDefinitions() *omap.Map[any, rec.Map] {
	references := omap.New[any, rec.Map]()
	references.Set(int64(7), rec.Map{"target": "eq:a", "selector": `\@firstoffive`})
	return references
}

// A reference printed inside a formula is one span whose text is the glyphs' printed run,
// however many items cover it.
func TestRecordMathReferenceJoinsItsPrintedText(t *testing.T) {
	e := &Evidence{references: referenceDefinitions()}
	pending := &pendingMath{}
	for _, printed := range []string{"3", ".", "1", "3"} {
		e.recordMathReference(rec.Map{"reference": int64(7), "text": printed}, pending)
	}
	if len(pending.references) != 1 {
		t.Fatalf("references %v", pending.references)
	}
	span := pending.references[0]
	if span["type"] != "reference" || span["text"] != "3.13" || span["target"] != "eq:a" || span["id"] != "r1" {
		t.Errorf("span %v", span)
	}
}

// Without the glyph text the reader cannot prove which number the use printed: it must not
// invent one from the label. A piece that prints nothing (an \overline's bar in a linked formula)
// adds no span; the formula keeps the piece.
func TestRecordMathReferenceInventsNothingWithoutPrintedText(t *testing.T) {
	e := &Evidence{references: referenceDefinitions()}
	pending := &pendingMath{}
	e.recordMathReference(rec.Map{"reference": int64(7)}, pending)
	if len(pending.references) != 0 || e.referenceCnt != 0 {
		t.Errorf("a span was invented: %v", pending.references)
	}
	e.recordMathReference(rec.Map{"reference": int64(7), "text": "e"}, pending)
	if len(pending.references) != 1 || pending.references[0]["text"] != "e" {
		t.Errorf("references %v", pending.references)
	}
}

// A formula's reference span resolves like an ordinary one: the printed number must be the
// label's, and an altered number stays unlinked instead of naming a target.
func TestMathReferenceResolvesAgainstItsLabelNumber(t *testing.T) {
	e := &Evidence{references: referenceDefinitions(),
		targets: []rec.Map{{"label": "eq:a", "number": "3.13", "block": "b1"}}}
	span := rec.Map{"type": "reference", "id": "r1", "reference": int64(7), "text": "3.13", "target": "eq:a"}
	e.mathReferences = []rec.Map{{"occurrence": "m1", "content": []any{span}}}
	e.resolveReferences()
	if span["target_block"] != "b1" || len(e.diagnostics) != 0 {
		t.Errorf("resolved %v diagnostics %v", span, e.diagnostics)
	}

	altered := rec.Map{"type": "reference", "id": "r2", "reference": int64(7), "text": "3.14", "target": "eq:a"}
	e.diagnostics = nil
	e.mathReferences = []rec.Map{{"occurrence": "m2", "content": []any{altered}}}
	e.resolveReferences()
	if altered["target_block"] != nil || len(e.diagnostics) != 1 || e.diagnostics[0]["code"] != "unresolved_reference_use" {
		t.Errorf("altered resolved %v diagnostics %v", altered, e.diagnostics)
	}
}

// \hyperref[label]{text} prints the author's text: it is the use of the label when it ends
// with the label's number as a word, and other text stays unlinked.
func TestHyperrefReferenceResolvesWhenItsTextEndsWithTheNumber(t *testing.T) {
	references := omap.New[any, rec.Map]()
	references.Set(int64(7), rec.Map{"target": "lem:a", "selector": `\hyperref`})
	for text, resolved := range map[string]bool{"Lemma 3.1": true, "3.1": true, "Lemma 3.1b": false, "Lemma3.1": false} {
		e := &Evidence{references: references, targets: []rec.Map{{"label": "lem:a", "number": "3.1", "block": "b1"}}}
		span := rec.Map{"type": "reference", "id": "r1", "reference": int64(7), "text": text, "target": "lem:a"}
		e.mathReferences = []rec.Map{{"occurrence": "m1", "content": []any{span}}}
		e.resolveReferences()
		if (span["target_block"] == "b1") != resolved || (len(e.diagnostics) == 0) != resolved {
			t.Errorf("%q: resolved %v diagnostics %v", text, span, e.diagnostics)
		}
	}
}

// A label number's hyphen prints in a formula as the math minus sign (\ref{c} of "A-1.3" in
// \stackrel{\rm Claim~\ref{c}}\le): the use still names that number.
func TestMathReferenceReadsTheMathMinusAsTheNumbersHyphen(t *testing.T) {
	e := &Evidence{references: referenceDefinitions(),
		targets: []rec.Map{{"label": "eq:a", "number": "A-1.3", "block": "b1"}}}
	span := rec.Map{"type": "reference", "id": "r1", "reference": int64(7), "text": "A−1.3", "target": "eq:a"}
	e.mathReferences = []rec.Map{{"occurrence": "m1", "content": []any{span}}}
	e.resolveReferences()
	if span["target_block"] != "b1" || len(e.diagnostics) != 0 {
		t.Errorf("resolved %v diagnostics %v", span, e.diagnostics)
	}
}
