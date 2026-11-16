package a11ylint

import (
	"regexp"
	"strings"
)

// Sub-floats (decision 167, amendment of the 2026-11-16 night run): a panel of a figure that has its own
// \subfigure, \subfloat or subfigure environment is named by the figure's caption when the panel's
// own caption is empty, and by its own caption when it has one.

var (
	// subFloatCommand is the commands of the subfig and subcaption packages whose braced argument is a panel.
	subFloatCommand = regexp.MustCompile(`\\(?:subfigure|subfloat|subtable)\s*\*?\s*(?:\[[^\]]*\])?`)
	// subFloatBegin is the environments of the subcaption and subfig packages that hold one panel.
	subFloatBegin = regexp.MustCompile(`\\begin\s*\{(subfigure|subtable)\*?\}`)
)

// subFloat is a panel: the span of its body in the file's text.
type subFloat struct {
	start, end int
}

// subFloats are the panels of text, in order.
func subFloats(text string) []subFloat {
	spans := []subFloat{}
	for _, match := range subFloatCommand.FindAllStringIndex(text, -1) {
		if _, end, ok := argument(text, match[1], '{', '}'); ok {
			spans = append(spans, subFloat{match[0], end})
		}
	}
	for _, match := range subFloatBegin.FindAllStringSubmatchIndex(text, -1) {
		if length := matchingEnd(text[match[1]:], text[match[2]:match[3]]); length >= 0 {
			spans = append(spans, subFloat{match[0], match[1] + length})
		}
	}
	return spans
}

// panelAt is the smallest panel holding offset.
func panelAt(spans []subFloat, offset int) (subFloat, bool) {
	best, found := subFloat{}, false
	for _, span := range spans {
		if span.start <= offset && offset < span.end && (!found || span.end-span.start < best.end-best.start) {
			best, found = span, true
		}
	}
	return best, found
}

// parentCaption is the text of the caption of the float that holds the panel, outside the panel:
// "" when the panel is in no float or the float has no caption of its own.
func parentCaption(text string, panel subFloat) string {
	start, end := -1, -1
	for _, match := range floatEnvironment.FindAllStringSubmatchIndex(text, -1) {
		length := matchingEnd(text[match[1]:], text[match[2]:match[3]])
		if length >= 0 && match[0] <= panel.start && panel.end <= match[1]+length && (start < 0 || match[0] > start) {
			start, end = match[0], match[1]+length
		}
	}
	if start < 0 {
		return ""
	}
	for _, match := range captionCommand.FindAllStringSubmatchIndex(text[start:end], -1) {
		at := start + match[0]
		if at >= panel.start && at < panel.end || text[start+match[2]:start+match[3]] == "subcaption" {
			continue
		}
		shifted := []int{at, start + match[1], start + match[2], start + match[3]}
		if group, ok := captionArgument(text, shifted); ok && strings.TrimSpace(captionText(group)) != "" {
			return captionText(group)
		}
	}
	return ""
}

// namedByParent: the caption at offset is empty, sits in a panel, and the float's own caption is
// text that is more than the kind of float: it names what the panels show.
func namedByParent(text string, offset int) bool {
	panel, ok := panelAt(subFloats(text), offset)
	if !ok {
		return false
	}
	parent := parentCaption(text, panel)
	return parent != "" && !genericCaption(parent)
}
