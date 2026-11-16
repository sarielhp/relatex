package contrast

import (
	"math"

	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/rec"
)

// Paper is the PDF's page behind text outside a table: white.
const Paper = "#ffffff"

// HiddenRatio is the contrast below which an ink is invisible on the paper in the PDF (1.1:1).
const HiddenRatio = 1.1

// Hidden are the inks the author hid (accessibility plan §8.1, phase 1b decision 1): an ink
// invisible on the PDF's paper and of the paper's colour (paperLike, decision 113), used only outside
// tables, beamer blocks and headings, where nothing
// paints a background behind it (a table cell or a block may: white on a dark cell reads; a heading
// in white is a beamer frame title on its painted bar, decision 095). Text on an author's fill
// (decision 099) is painted too: its uses there are never hidden. The light theme
// keeps such text invisible, as the PDF shows it, adjusts no hidden ink and marks its text
// aria-hidden; the dark theme hides it too.
func Hidden(document rec.Map) map[string]bool {
	paper, _ := Parse(Paper)
	inTables := map[string]bool{}
	for _, value := range rec.FetchMap(document, "nodes") {
		content, _ := rec.AsMap(value)["content"].([]any)
		for _, span := range content {
			if rec.AsMap(span)["type"] != "table_grid" {
				continue
			}
			for _, item := range inline.Deep([]any{span}) {
				if color, ok := item["color"].(string); ok {
					inTables[color] = true
				}
			}
		}
	}
	inBlocks := blockInks(document)
	for color := range headingInks(document) {
		inBlocks[color] = true
	}
	hidden := map[string]bool{}
	for color, kinds := range Inks(document) {
		if len(PageKinds(kinds)) == 0 {
			continue // set only on fills (decision 099): painted, so it reads
		}
		if ink, ok := Parse(color); ok && !inTables[color] && !inBlocks[color] && Ratio(ink, paper) < HiddenRatio && paperLike(ink) {
			hidden[color] = true
		}
	}
	return hidden
}

// HiddenDistance is the OKLab distance from white within which an ink is the paper's colour: white,
// a near white or a faint tint. A saturated ink as light as the paper (pure yellow, 1.07:1) is no
// hiding: the PDF shows it, as a colour (slides 0013), and the page adjusts it to read.
const HiddenDistance = 0.1

// paperLike: the ink is within HiddenDistance of white.
func paperLike(ink RGB) bool {
	lightness, chroma, _ := ink.oklch()
	return math.Hypot(1-lightness, chroma) < HiddenDistance
}

// headingInks are the inks of headings' text.
func headingInks(document rec.Map) map[string]bool {
	found := map[string]bool{}
	for _, value := range rec.FetchMap(document, "nodes") {
		node := rec.AsMap(value)
		if node["type"] != "heading" {
			continue
		}
		content, _ := node["content"].([]any)
		for _, span := range inline.Deep(content) {
			if color, ok := span["color"].(string); ok {
				found[color] = true
			}
		}
	}
	return found
}
