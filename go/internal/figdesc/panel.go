package figdesc

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// subcaptionLimit bounds the text kept as a panel's subcaption.
const subcaptionLimit = 300

// Panel is where a picture sits among the pictures of its figure environment (decision 133): the
// describer is told which panel of a multi-part caption it is looking at, or that this is unknown.
type Panel struct {
	// InFigure says the picture is in a figure environment; without one nothing below is known.
	InFigure bool
	// Index is the picture's place among the figure's pictures in reading order (1-based), Count
	// how many pictures the figure has.
	Index, Count int
	// Column and Row say where it sits in a paragraph holding RowSize pictures side by side
	// (1-based Column); RowSize is 1 when the paragraph holds it alone.
	Column, RowSize int
	// Subcaption is the text printed with the picture's group in the figure, outside the caption
	// ("(b) Scenario 2"), when the figure prints one; for a group of several pictures it is the
	// part that belongs to this one when the text has one mark ("(a)", "b)") per picture.
	Subcaption string
	// RowText is the text printed with a group of pictures when it cannot be split between them.
	RowText string
}

// panelMark is a subfigure mark opening a part of a subcaption: "(a)", "a)", "(iv)".
var panelMark = regexp.MustCompile(`(?:^|\s)\(?([a-z]|[ivx]{1,4})\)\s`)

// pictureGroup is the pictures of one block of a figure and the text printed with them.
type pictureGroup struct {
	spans []rec.Map
	text  string
}

// panel is where span sits among the pictures of the figure environment holding block.
func (r *contextReader) panel(block string, span rec.Map) Panel {
	float := r.enclosing(block, "figure_float")
	if float == "" {
		return Panel{}
	}
	groups := r.pictureGroups(float)
	result := Panel{InFigure: true}
	for _, group := range groups {
		result.Count += len(group.spans)
	}
	index := 0
	for _, group := range groups {
		for column, other := range group.spans {
			index++
			if !sameSpan(other, span) {
				continue
			}
			result.Index, result.Column, result.RowSize = index, column+1, len(group.spans)
			result.Subcaption = groupPart(group.text, column, len(group.spans))
			if result.Subcaption == "" {
				result.RowText = clipTo(group.text, subcaptionLimit)
			}
		}
	}
	return result
}

func sameSpan(first, second rec.Map) bool {
	return rec.ToS(first["id"]) == rec.ToS(second["id"])
}

// pictureGroups are the blocks of a figure's body that hold pictures, in reading order, each with
// the text printed with it: its own text beside the pictures, else the next block's when that is
// a paragraph without pictures (a subcaption set under its picture). The caption is left out.
func (r *contextReader) pictureGroups(float string) []pictureGroup {
	inside := r.descendants(float)
	captioned := map[string]bool{}
	if caption := r.firstOfType(float, "float_caption", 0); caption != "" {
		captioned = r.descendants(caption)
	}
	groups := []pictureGroup{}
	open := false // the last group still takes the next paragraph as its text
	for _, id := range r.order {
		if !inside[id] || captioned[id] {
			continue
		}
		spans, text := r.blockPictures(id)
		if len(spans) > 0 {
			groups = append(groups, pictureGroup{spans: spans, text: text})
			open = text == ""
			continue
		}
		if open && text != "" && rec.AsMap(r.nodes[id])["type"] == "paragraph" {
			groups[len(groups)-1].text = text
		}
		open = false
	}
	return groups
}

// blockPictures are the pictures of one block (link icons left out) and the block's own text
// beside them, white space collapsed.
func (r *contextReader) blockPictures(id string) ([]rec.Map, string) {
	spans := []rec.Map{}
	var text strings.Builder
	for _, value := range rec.FetchList(rec.Node(r.nodes, id), "content") {
		span := rec.AsMap(value)
		if span["type"] == "image" {
			if _, linked := span["external_link"]; !linked {
				spans = append(spans, span)
			}
			continue
		}
		text.WriteString(string(r.rawSpan(span)))
	}
	if len(spans) == 0 {
		return nil, r.text(id)
	}
	return spans, strings.TrimSpace(spaces.ReplaceAllString(text.String(), " "))
}

// groupPart is the part of a group's text that belongs to its picture at column: the whole text
// for a picture alone, the part from its mark to the next when the text has one mark per picture,
// else "" (the text cannot be split between the pictures).
func groupPart(text string, column, size int) string {
	if size == 1 {
		return clipTo(text, subcaptionLimit)
	}
	marks := panelMark.FindAllStringIndex(text, -1)
	if len(marks) != size {
		return ""
	}
	end := len(text)
	if column+1 < size {
		end = marks[column+1][0]
	}
	return clipTo(strings.TrimSpace(text[marks[column][0]:end]), subcaptionLimit)
}

// Describe is the panel as the describer's prompt states it: what is known of where this picture
// sits in its figure, and plainly that nothing is known when that is so.
func (p Panel) Describe() string {
	switch {
	case !p.InFigure:
		return "not in a figure environment; nothing is known about panels."
	case p.Count <= 1:
		return "the only picture of its figure (it may still hold several panels of its own)."
	}
	var out strings.Builder
	out.WriteString("picture " + strconv.Itoa(p.Index) + " of " + strconv.Itoa(p.Count) + " of its figure, in reading order")
	if p.RowSize > 1 {
		out.WriteString("; picture " + strconv.Itoa(p.Column) + " of " + strconv.Itoa(p.RowSize) + " set in one paragraph (normally side by side, left to right)")
	}
	if p.Subcaption != "" {
		out.WriteString("; the text printed with it in the figure: \"" + p.Subcaption + "\".")
		return out.String()
	}
	if p.RowText != "" {
		out.WriteString("; the text printed with its group of pictures: \"" + p.RowText + "\".")
		return out.String()
	}
	out.WriteString("; the figure prints no subcaption for it, so which of the caption's panels it is, is not known.")
	return out.String()
}
