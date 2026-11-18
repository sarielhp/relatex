package html

import (
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// A rule frame is a box TeX draws with four rules around some paragraphs: Springer's svjour3
// \makeheadbox ("Noname manuscript No. (will be inserted by the editor)") sets a top rule and the
// left rule at the end of the first line of the box, the lines of text, and the right rule and the
// bottom rule after its last line. As inline rules in three paragraphs they paint broken strokes;
// as one bordered block around those paragraphs they paint the frame.
//
// The frame is recognised by its rules alone: a paragraph ending in a rule as wide as the line and
// a vertical rule kerned back over its own width (the left side), and, within the next
// maxFrameBlocks siblings, one ending in the vertical rule that stands one line width to the right
// (margin_left = width - stroke, margin_right = -width) and a rule of the same width.
const maxFrameBlocks = 8

// frameTail is the two rules a paragraph's tail holds: their indexes and the frame they state.
type frameTail struct {
	horizontal, vertical int
	width, stroke        float64
}

// ruleFrame is a frame found among a node's children: the first and last child it surrounds.
type ruleFrame struct {
	first, last int
	shadow      bool
}

func spanNumber(span rec.Map, key string) float64 {
	value, _ := rec.Number(span[key])
	return value
}

func blankSpan(span rec.Map) bool {
	text, _ := span["text"].(string)
	return span["type"] == "text" && strings.TrimSpace(text) == ""
}

// tailRules reads the last two rules of a content list, ignoring blank spans after them; the
// first returned rule is the earlier one.
func tailRules(content []any) (first, second int, ok bool) {
	found := []int{}
	for index := len(content) - 1; index >= 0 && len(found) < 2; index-- {
		span := rec.AsMap(content[index])
		switch {
		case span["type"] == "rule":
			found = append(found, index)
		case !blankSpan(span):
			return 0, 0, false
		}
	}
	if len(found) < 2 {
		return 0, 0, false
	}
	return found[1], found[0], true
}

func isHorizontal(span rec.Map) bool {
	width, height := spanNumber(span, "width_sp"), spanNumber(span, "height_sp")+spanNumber(span, "depth_sp")
	return span["type"] == "rule" && width > 20*height && height > 0
}

func isVertical(span rec.Map) bool {
	width, height := spanNumber(span, "width_sp"), spanNumber(span, "height_sp")+spanNumber(span, "depth_sp")
	return span["type"] == "rule" && height > 10*width && width > 0
}

// openingTail: a horizontal rule, then a vertical one kerned back over itself.
func openingTail(content []any) (frameTail, bool) {
	first, second, ok := tailRules(content)
	if !ok {
		return frameTail{}, false
	}
	horizontal, vertical := rec.AsMap(content[first]), rec.AsMap(content[second])
	stroke := spanNumber(vertical, "width_sp")
	if !isHorizontal(horizontal) || !isVertical(vertical) || spanNumber(vertical, "margin_left_sp") != 0 ||
		spanNumber(vertical, "margin_right_sp") != -stroke {
		return frameTail{}, false
	}
	return frameTail{horizontal: first, vertical: second, width: spanNumber(horizontal, "width_sp"), stroke: stroke}, true
}

// closingTail: a vertical rule one line width to the right of the left side, then a horizontal one.
func closingTail(content []any, opening frameTail) (frameTail, bool) {
	first, second, ok := tailRules(content)
	if !ok {
		return frameTail{}, false
	}
	vertical, horizontal := rec.AsMap(content[first]), rec.AsMap(content[second])
	stroke := spanNumber(vertical, "width_sp")
	if !isVertical(vertical) || !isHorizontal(horizontal) || stroke != opening.stroke ||
		spanNumber(horizontal, "width_sp") != opening.width ||
		spanNumber(vertical, "margin_left_sp") != opening.width-stroke ||
		spanNumber(vertical, "margin_right_sp") != -opening.width {
		return frameTail{}, false
	}
	return frameTail{horizontal: second, vertical: first, width: opening.width, stroke: stroke}, true
}

func sameRule(a, b rec.Map) bool {
	for _, key := range []string{"width_sp", "height_sp", "depth_sp"} {
		if spanNumber(a, key) != spanNumber(b, key) {
			return false
		}
	}
	return true
}

// boxRules finds the four rules of a boxed paragraph among its spans that are not blank: top and
// left, right and bottom, a horizontal and a vertical rule, a vertical and a horizontal one, of
// the same width and the same stroke. The paragraph holds them either around its text (the first
// two and the last two) or after it, the four together at its end (the box's text is read before
// the box's own rules: phd, walk). shadow.sty's \shabox adds a vertical and a horizontal rule
// after the four, the shadow at the right and the bottom: shadow is true then, and the six
// indexes come back.
func boxRules(content []any) (indexes []int, shadow, ok bool) {
	var spans []int
	for index, value := range content {
		if !blankSpan(rec.AsMap(value)) {
			spans = append(spans, index)
		}
	}
	for _, tail := range []int{0, 2} {
		count := len(spans) - tail
		if count < 5 {
			return nil, false, false
		}
		for _, quartet := range [][]int{{0, 1, count - 2, count - 1}, {count - 4, count - 3, count - 2, count - 1}} {
			found := []int{spans[quartet[0]], spans[quartet[1]], spans[quartet[2]], spans[quartet[3]]}
			if !isBox(content, found) {
				continue
			}
			if tail == 0 {
				return found, false, true
			}
			dropRight, dropBottom := rec.AsMap(content[spans[count]]), rec.AsMap(content[spans[count+1]])
			top := rec.AsMap(content[found[0]])
			if isVertical(dropRight) && isHorizontal(dropBottom) && spanNumber(dropBottom, "width_sp") == spanNumber(top, "width_sp") {
				return append(found, spans[count], spans[count+1]), true, true
			}
		}
	}
	return nil, false, false
}

func isBox(content []any, indexes []int) bool {
	top, left, right, bottom := rec.AsMap(content[indexes[0]]), rec.AsMap(content[indexes[1]]),
		rec.AsMap(content[indexes[2]]), rec.AsMap(content[indexes[3]])
	return isHorizontal(top) && isVertical(left) && isVertical(right) && isHorizontal(bottom) &&
		sameRule(top, bottom) && sameRule(left, right)
}

func (p *Paper) paragraphContent(id any) []any {
	node := p.node(id)
	if node["type"] != "paragraph" {
		return nil
	}
	content, _ := node["content"].([]any)
	return content
}

// ruleFrames finds the frames among a node's children, keyed by their first child's index, and
// records the rule spans to leave out (the border replaces them) in p.frameCuts.
func (p *Paper) ruleFrames(children []any) map[int]ruleFrame {
	frames := map[int]ruleFrame{}
	for index := 0; index < len(children); index++ {
		if rules, shadow, boxed := boxRules(p.paragraphContent(children[index])); boxed {
			frames[index] = ruleFrame{first: index, last: index, shadow: shadow}
			p.cutRules(children[index], rules...)
			continue
		}
		opening, ok := openingTail(p.paragraphContent(children[index]))
		if !ok {
			continue
		}
		for next := index + 1; next < len(children) && next <= index+maxFrameBlocks; next++ {
			content := p.paragraphContent(children[next])
			if _, again := openingTail(content); again {
				break
			}
			closing, closed := closingTail(content, opening)
			if !closed {
				continue
			}
			frames[index] = ruleFrame{first: index, last: next}
			p.cutRules(children[index], opening.horizontal, opening.vertical)
			p.cutRules(children[next], closing.horizontal, closing.vertical)
			index = next
			break
		}
	}
	return frames
}

func (p *Paper) cutRules(id any, indexes ...int) {
	if p.frameCuts == nil {
		p.frameCuts = map[string]map[int]bool{}
	}
	key := rec.AsString(id)
	if p.frameCuts[key] == nil {
		p.frameCuts[key] = map[int]bool{}
	}
	for _, index := range indexes {
		p.frameCuts[key][index] = true
	}
}

// frameRules is a paragraph's content with the rules a frame's border replaces marked hidden: the
// paper's blocks keep every rule (the browser check reads them back), the border paints the frame.
func (p *Paper) frameRules(identifier string, content []any) []any {
	cuts := p.frameCuts[identifier]
	if len(cuts) == 0 {
		return content
	}
	marked := make([]any, len(content))
	for index, span := range content {
		marked[index] = span
		if cuts[index] {
			marked[index] = rec.Merge(rec.AsMap(span), rec.Map{"frame_hidden": true})
		}
	}
	return marked
}

// frameSides names each frame paragraph's side of the box, "first", "middle" or "last", by index.
// The frame is drawn by borders on the paragraphs themselves, so the page has no wrapper element
// between a node and its children (the browser check reads the structure back).
func frameSides(frames map[int]ruleFrame) map[int]string {
	sides := map[int]string{}
	for _, frame := range frames {
		for index := frame.first; index <= frame.last; index++ {
			sides[index] = "middle"
		}
		sides[frame.first], sides[frame.last] = "first", "last"
		if frame.first == frame.last {
			sides[frame.first] = "box"
			if frame.shadow {
				sides[frame.first] = "shadow"
			}
		}
	}
	return sides
}

// markFrameSide gives the element a rendered block opens with its side of a frame.
func markFrameSide(html, side string) string {
	end := strings.IndexAny(html, " >")
	if side == "" || !strings.HasPrefix(html, "<") || end < 0 {
		return html
	}
	return html[:end] + ` data-frame="` + side + `"` + html[end:]
}
