package reading

import (
	"strings"

	"github.com/sarielhp/relatex/internal/evidence"
	"github.com/sarielhp/relatex/internal/rec"
)

// printsTag: the display prints "(number)": as its trailing text span, or in a row's label cell
// of one of its formulas.
func (e *Evidence) printsTag(block rec.Map, number string) bool {
	tag := "(" + number + ")"
	if len(taggedDisplays([]rec.Map{block}, number)) == 1 || e.printsMathDisplayTag(block, number) {
		return true
	}
	for _, span := range content(block) {
		if span["type"] == "math" && e.formulaPrintsTag(rec.Fetch(span, "formula"), tag) {
			return true
		}
	}
	return false
}

// printingDisplays are the displays that print "(number)" (printsTag).
func (e *Evidence) printingDisplays(displays []rec.Map, number string) []rec.Map {
	if number == "" {
		return nil
	}
	printing := []rec.Map{}
	for _, display := range displays {
		if e.printsTag(display, number) {
			printing = append(printing, display)
		}
	}
	return printing
}

// mathMLOf is a formula's recorded MathML, empty when none was recorded.
func (e *Evidence) mathMLOf(formula any) string {
	if e.formulaMathML == nil {
		e.formulaMathML = map[any]string{}
		for _, record := range e.records {
			if rec.Fetch(record, "kind") == "math" {
				e.formulaMathML[rec.Fetch(record, "formula")] = rec.FetchString(record, "mathml")
			}
		}
	}
	return e.formulaMathML[formula]
}

// continuedFormulaBlocks: one display set as several formulas (a multline broken over two
// displays, fourth cohort 961) records its label on the first formula, while its number is
// printed by the last part. When the formula's own display prints no number, the label names
// the one display of the same equation that prints it. A row label (an align row's) names a row
// of its own formula, which no other display holds: it keeps its display (second cohort 268,
// whose recorded row prints no number while the row before it prints the label's).
func (e *Evidence) continuedFormulaBlocks(label rec.Map, blocks []rec.Map) []rec.Map {
	number := evidence.LiteralNumber(rec.FetchString(label, "number"))
	if len(blocks) != 1 || truthy(label["math_row"]) || number == nil || *number == "" || rec.Fetch(blocks[0], "type") != "display_math" ||
		e.printsTag(blocks[0], *number) {
		return blocks
	}
	entity := rec.Fetch(blocks[0], "entity")
	printing := e.blocksWhere(func(block rec.Map) bool {
		return rec.Fetch(block, "type") == "display_math" && rec.Equal(rec.Fetch(block, "entity"), entity) &&
			e.printsTag(block, *number)
	})
	if len(printing) == 1 {
		return printing
	}
	return blocks
}

// unprintedDisplayTagBlocks: an equation whose number no display prints, but a line of its own
// does (a stray \\ at the end of an equation sets an empty row that carries the label's number,
// fourth cohort 1065): the label names that line.
func (e *Evidence) unprintedDisplayTagBlocks(label rec.Map, displays, blocks []rec.Map) []rec.Map {
	number := evidence.LiteralNumber(rec.FetchString(label, "number"))
	if number == nil || *number == "" {
		return nil
	}
	for _, display := range displays {
		if e.printsTag(display, *number) {
			return nil
		}
	}
	return one(tagBlocks(blocks, label))
}

// displayTagLabel: a \tag in a starred display (equation*, or \[ under amsmath) prints the
// label's number without stepping the equation counter, so the label records the last counter
// stepped (its section) and that section's heading as its target. When the label sits in the
// display's own context, carries amsmath's tag anchor (AMS.n) or prints its tag in a block of
// the display (without hyperref), and the heading does not print its number, it names the
// display (papers5/1350 eq:indhyp, papers4/1172 61). A label of an untagged
// starred display keeps its section, whose heading prints the number it records.
func (e *Evidence) displayTagLabel(label rec.Map) rec.Map {
	entity := label["entity"]
	target := label["target_entity"]
	if truthy(label["formula"]) || label["counter"] == "equation" || !positive(entity) || rec.Equal(entity, target) ||
		!e.starredDisplayContext(entity) {
		return label
	}
	number := toS(label["number"])
	// Without hyperref no anchor marks the tag: a block of the display prints it instead.
	if !strings.HasPrefix(toS(label["anchor"]), "AMS.") && !e.entityPrintsTag(entity, number) {
		return label
	}
	printing := e.blocksWhere(func(block rec.Map) bool {
		return rec.Equal(block["entity"], target) && printedNumber(leadingText(block), number)
	})
	if number == "" || len(printing) > 0 {
		return label
	}
	return rec.Merge(label, rec.Map{"counter": "equation", "target_entity": entity})
}

// starredDisplayContext: the context is a starred display's (an equation_context of equation*,
// align*, ...), which steps no equation counter.
func (e *Evidence) starredDisplayContext(entity any) bool {
	for _, record := range e.records {
		if record["code"] == "equation_context" && record["source"] == "latex-equation-hooks" &&
			rec.Equal(record["entity"], entity) && strings.HasSuffix(toS(record["environment"]), "*") {
			return true
		}
	}
	return false
}
