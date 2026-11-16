package reading

import (
	"strings"

	"github.com/sarielhp/relatex/internal/evidence"
	"github.com/sarielhp/relatex/internal/rec"
)

// splitUse is one printed use of a label whose number is partly text and partly math: its
// reference spans in print order (the block's text pieces and the pieces inside its formulas)
// and the block's formulas set wholly inside it.
type splitUse struct {
	pieces   []rec.Map
	formulas []rec.Map
	text     bool
}

// resolveSplitUses: \eqref{61} of {NLS$_a$} prints "NLS" as text and $_a$ as a formula, \ref of
// {($R_1$)} prints "(", $R_1$ and ")": the reference reads as spans of one use in the block and in
// the formulas inside it. The use resolves as one unit when its pieces, in print order, spell the
// label's number; its pieces then link the target and its formulas take the link. It returns the
// references it resolved, which the span-by-span pass leaves alone.
func (e *Evidence) resolveSplitUses(targets map[any]rec.Map) map[any]bool {
	inner := map[any][]rec.Map{}
	for _, entry := range e.mathReferences {
		inner[entry["occurrence"]] = append(inner[entry["occurrence"]], content(entry)...)
	}
	handled := map[any]bool{}
	for _, block := range e.blocks {
		uses, order := e.blockSplitUses(block, inner)
		for _, reference := range order {
			use := uses[reference]
			if len(use.formulas) > 0 && use.text && e.resolveSplitUse(targets, use) {
				handled[reference] = true
			}
		}
	}
	return handled
}

// blockSplitUses gathers, by reference, the pieces of the uses a block prints.
func (e *Evidence) blockSplitUses(block rec.Map, inner map[any][]rec.Map) (map[any]*splitUse, []any) {
	uses := map[any]*splitUse{}
	order := []any{}
	use := func(reference any) *splitUse {
		if _, ok := uses[reference]; !ok {
			uses[reference] = &splitUse{}
			order = append(order, reference)
		}
		return uses[reference]
	}
	for _, span := range content(block) {
		switch span["type"] {
		case "reference":
			current := use(span["reference"])
			current.pieces = append(current.pieces, span)
			current.text = true
		case "math":
			link, ok := e.enclosingLinks[span["occurrence"]]
			if !ok {
				continue
			}
			current := use(link.reference)
			for _, piece := range inner[span["occurrence"]] {
				if rec.Equal(piece["reference"], link.reference) {
					current.pieces = append(current.pieces, piece)
				}
			}
			current.formulas = append(current.formulas, span)
		}
	}
	return uses, order
}

// resolveSplitUse binds a split use whose joined pieces print the label's number exactly.
func (e *Evidence) resolveSplitUse(targets map[any]rec.Map, use *splitUse) bool {
	target := targets[use.pieces[0]["target"]]
	if target == nil {
		return false
	}
	for _, piece := range use.pieces {
		if !rec.Equal(piece["target"], use.pieces[0]["target"]) {
			return false
		}
	}
	// Only a number typeset as math is printed in pieces; a literal number keeps the span-by-span
	// reading (a \hyperref text around a formula, "$c$-packed").
	number := rec.FetchString(target, "number")
	if evidence.LiteralNumber(number) != nil {
		return false
	}
	definition, _ := e.references.Get(use.pieces[0]["reference"])
	expected := evidence.MathNumber(number)
	printed := rec.Strip(squeeze(strings.ReplaceAll(joinedText(use.pieces), "−", "-"), " "))
	if expected == nil || !printedUse(rec.Fetch(definition, "selector"), printed, *expected) {
		return false
	}
	for _, piece := range use.pieces {
		piece["target_block"] = rec.Fetch(target, "block")
		if truthy(target["row"]) {
			piece["target_row"] = rec.Fetch(target, "row")
		}
	}
	for _, formula := range use.formulas {
		linkFormula(formula, use.pieces[0])
	}
	return true
}
