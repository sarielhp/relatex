package reading

import (
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/evidence"
	"github.com/sarielhp/relatex/internal/notes"
	"github.com/sarielhp/relatex/internal/rec"
)

// printedText is what a block of text and math prints, read as plain characters: its text spans
// and its formulas' MathML text in order, compatibility-folded (a script 𝓠 reads Q) and without
// spaces. It is false for a block with no formula or with another kind of span.
func (e *Evidence) printedText(block rec.Map) (string, bool) {
	var text strings.Builder
	math := false
	for _, span := range content(block) {
		switch span["type"] {
		case "text":
			text.WriteString(toS(span["text"]))
		case "math":
			math = true
			text.WriteString(notes.MathMLText(e.mathMLOf(rec.Fetch(span, "formula"))))
		default:
			return "", false
		}
	}
	return notes.RemoveSpace(norm.NFKC.String(text.String())), math
}

// printsMathTag: the block prints "(number)" with math in it, as a tag typeset as math does
// ((𝓠₁) for $\bm{\mathcal{Q}}_1$).
func (e *Evidence) printsMathTag(block rec.Map, number string) bool {
	text, math := e.printedText(block)
	return math && text == "("+number+")"
}

// printsMathDisplayTag: a display whose tag is typeset with math (\tag{$A$}) carries it as text and
// inline formulas before or after its display formula (attachEquationNumbers); those spans print
// "(number)". A tag of text only is taggedDisplays'.
func (e *Evidence) printsMathDisplayTag(block rec.Map, number string) bool {
	if rec.Fetch(block, "type") != "display_math" || e.formulas == nil {
		return false
	}
	spans := content(block)
	display := slices.IndexFunc(spans, func(span rec.Map) bool {
		if span["type"] != "math" {
			return false
		}
		definition, ok := e.formulas.Get(span["formula"])
		return ok && truthy(definition["display"])
	})
	if display < 0 {
		return false
	}
	want := "(" + notes.RemoveSpace(number) + ")"
	for _, part := range [][]rec.Map{spans[:display], spans[display+1:]} {
		text, math := e.printedText(rec.Map{"content": toAnySlice(part)})
		// Spans the collector marked as the tag's are the tag, math or not; unmarked ones need math
		// (a text-only tag is taggedDisplays').
		if marked, decided := e.markedTag(toAnySlice(part)); decided {
			math = marked
		}
		if math && text == want {
			return true
		}
	}
	return false
}

func toAnySlice(spans []rec.Map) []any {
	values := make([]any, len(spans))
	for index, span := range spans {
		values[index] = span
	}
	return values
}

// printedTagBlocks: a label whose counter no context owns, and whose number is typeset as math
// (prolog.sty's \plabel steps querynum, prints (\thequerynum) as a line of its own and labels it,
// papers3/0593), names the block that prints that tag; of several, the last one whose formula
// comes before the label.
func (e *Evidence) printedTagBlocks(label rec.Map) []rec.Map {
	number := evidence.MathNumber(rec.FetchString(label, "number"))
	if number == nil || *number == "" {
		return []rec.Map{}
	}
	printing := e.blocksWhere(func(block rec.Map) bool { return e.printsMathTag(block, *number) })
	if len(printing) <= 1 {
		return printing
	}
	if e.formulas == nil {
		return []rec.Map{}
	}
	seqs := map[any]any{}
	for _, id := range e.formulas.Keys() {
		row, _ := e.formulas.Get(id)
		seqs[rec.Fetch(row, "formula")] = rec.Fetch(row, "seq")
	}
	var nearest rec.Map
	var nearestSeq any
	for _, block := range printing {
		for _, span := range content(block) {
			seq, ok := seqs[span["formula"]]
			if span["type"] != "math" || !ok || !less(seq, rec.Fetch(label, "seq")) {
				continue
			}
			if nearest == nil || less(nearestSeq, seq) {
				nearest, nearestSeq = block, seq
			}
		}
	}
	if nearest == nil {
		return []rec.Map{}
	}
	return []rec.Map{nearest}
}

// entityPrintsTag: a block of the context prints the label's number as an equation tag, "(‡)"
// as text, math or both (a \tag in a starred display).
func (e *Evidence) entityPrintsTag(entity any, number string) bool {
	tag := evidence.LiteralNumber(number)
	if tag == nil {
		tag = evidence.MathNumber(number)
	}
	if tag == nil || *tag == "" {
		return false
	}
	return len(e.blocksWhere(func(block rec.Map) bool {
		if !rec.Equal(rec.Fetch(block, "entity"), entity) {
			return false
		}
		text, _ := e.printedText(block)
		return text == "("+notes.RemoveSpace(*tag)+")" || e.printsTag(block, *tag)
	})) > 0
}

// printedLabelNumber is what a label's number prints ({\color{blue}8} prints 8, {$\dagger $} †):
// its literal or math reading, else the number as recorded.
func printedLabelNumber(label rec.Map) string {
	number := rec.FetchString(label, "number")
	if literal := evidence.LiteralNumber(number); literal != nil {
		return *literal
	}
	if math := evidence.MathNumber(number); math != nil {
		return *math
	}
	return number
}
