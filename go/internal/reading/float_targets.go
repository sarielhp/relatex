package reading

import (
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/evidence"
	"github.com/sarielhp/relatex/internal/rec"
)

// subfloatMark reads a subfigure number (1b, 1(b), IIIc) for its printed mark.
var subfloatMark = regexp.MustCompile(`\A(?:[0-9]+|[IVXLCDM]+)\(?([a-z])\)?\z`)

// printedFloatBlocks: a label whose recorded caption does not print its number names another
// numbered part of its own float:
//   - a longtable's caption row, which carries the label's own anchor (its recorded caption is
//     the previous float's: first cohort 161);
//   - a line of an algorithm (algorithm2e's AlgoLine, algorithmicx's numbered \State), which
//     opens with the number ("2 compute ...", "3: for ...") while the recorded caption prints
//     "Algorithm 1" (first cohort 177, second 446, third 602);
//   - a subfigure whose \label precedes its own subcaption (subfig sets it after the picture),
//     which records the previous subcaption: the float block opening with its mark (fifth cohort 1303);
//   - the float's own caption, when the label follows the float's subcaptions and their last
//     one ("(c) Unigrams") is recorded (second cohort 446, third 781).
//
// The blocks it returns print the number; none is returned when the recorded blocks print it.
func (e *Evidence) printedFloatBlocks(label rec.Map, blocks []rec.Map) []rec.Map {
	number := evidence.LiteralNumber(toS(rec.Fetch(label, "number")))
	if number == nil || *number == "" || printsNumber(blocks, *number) {
		return nil
	}
	if rich := e.richCaptionBlocks(label); len(rich) == 1 {
		return rich
	}
	float := e.floatBlocks(rec.Fetch(label, "entity"))
	if mark := subfloatMark.FindStringSubmatch(*number); mark != nil {
		opening := "(" + mark[1] + ")"
		return one(blocksWhere(float, func(block rec.Map) bool {
			return strings.HasPrefix(strings.TrimLeft(leadingText(block), " \t\n\v\f\r"), opening)
		}))
	}
	if lines := one(blocksWhere(float, func(block rec.Map) bool { return printedNumber(leadingText(block), *number) })); len(lines) == 1 {
		return lines
	}
	opening := captionOpening(*number)
	return one(blocksWhere(float, func(block rec.Map) bool { return opening.MatchString(leadingText(block)) }))
}

// printsNumber: some block opens with the number, alone or after a caption name (Figure 3).
func printsNumber(blocks []rec.Map, number string) bool {
	opening := captionOpening(number)
	for _, block := range blocks {
		text := leadingText(block)
		if printedNumber(text, number) || opening.MatchString(text) {
			return true
		}
	}
	return false
}

// captionOpening matches a caption's name and number: "Table III:", "Figure 3.", "Algorithm 2 ".
func captionOpening(number string) *regexp.Regexp {
	return regexp.MustCompile(`\A` + rubySpace + `*\p{L}+\.?` + rubySpace + `+` + regexp.QuoteMeta(number) + `([^0-9A-Za-z.]|\.[^0-9]|\.?\z)`)
}

// floatBlocks are the blocks of a float context and of the captions set in it.
func (e *Evidence) floatBlocks(float any) []rec.Map {
	if !positive(float) {
		return nil
	}
	entities := []any{float}
	observed := false
	for _, row := range e.records {
		if row["kind"] != "diagnostic" {
			continue
		}
		observed = observed || (row["code"] == "float_context" && rec.Equal(row["entity"], float))
		if row["code"] == "caption_context" && rec.Equal(row["float"], float) {
			entities = append(entities, rec.Fetch(row, "entity"))
		}
	}
	if !observed {
		return nil
	}
	return e.blocksWhere(func(block rec.Map) bool {
		entity := rec.Fetch(block, "entity")
		for _, owner := range entities {
			if rec.Equal(entity, owner) {
				return true
			}
		}
		return false
	})
}

func blocksWhere(blocks []rec.Map, test func(rec.Map) bool) []rec.Map {
	result := []rec.Map{}
	for _, block := range blocks {
		if test(block) {
			result = append(result, block)
		}
	}
	return result
}
