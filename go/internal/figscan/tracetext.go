package figscan

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/nativetrace"
)

const (
	// baselineSlack is how far (in text sizes) a glyph may sit off a label's baseline and still join
	// it; a glyph lowered or raised further is a sub- or superscript and starts its own label.
	baselineSlack = 0.15
	// sizeSlack is the smallest size ratio of two glyphs that join one label (a smaller one is a script).
	sizeSlack = 0.85
	// gapSlack is the widest gap (in text sizes) between a label's end and a glyph that joins it.
	gapSlack = 0.5
	// spaceGap is the gap (in text sizes) read as a space between words.
	spaceGap = 0.15
	// unmappedText marks a label with glyphs that carry no Unicode.
	unmappedText = "glyphs unmapped"
)

// openLabel is the text node glyphs may still join: where its first glyph sits (the baseline)
// and its last ended, its direction and size (normalized), its colour and the script level its
// text is at ("", "_" or "^").
type openLabel struct {
	node  *Node
	base  Point
	end   Point
	dir   Point
	size  float64
	color string
	level string
}

// glyph is one traced glyph, normalized.
type glyph struct {
	text  string
	at    Point
	end   Point
	dir   Point
	size  float64
	color string
	// smallCaps marks a glyph from a small-caps font: its lowercase Unicode prints as capitals.
	smallCaps bool
}

// text reads a fill_text or stroke_text: its glyphs join the open label or start new ones,
// which it returns.
func (r *traceReader) text(e *nativetrace.Node) []*Node {
	r.paints++
	ctm := parseMatrix(e.Attrs["transform"])
	color, _ := traceColor(e.Attrs["color"])
	var started []*Node
	for _, span := range e.Named("span") {
		trm := numbers(span.Attrs["trm"])
		if len(trm) != 4 {
			continue
		}
		linear := matrix{trm[0], trm[1], trm[2], trm[3], 0, 0}.then(ctm).then(r.toBox)
		size := math.Sqrt(math.Abs(linear.determinant()))
		smallCaps := smallCapsFont(span.Attrs["font"])
		for _, g := range span.Named("g") {
			read := r.readGlyph(g, ctm, linear, size, color)
			read.smallCaps = smallCaps
			if node := r.glyph(read); node != nil {
				started = append(started, node)
			}
		}
	}
	return started
}

func (r *traceReader) readGlyph(g *nativetrace.Node, ctm, linear matrix, size float64, color string) glyph {
	at := ctm.then(r.toBox).apply(traceFloat(g.Attrs["x"]), traceFloat(g.Attrs["y"]))
	origin := linear.apply(0, 0)
	step := linear.apply(traceFloat(g.Attrs["adv"]), 0)
	unit := linear.apply(1, 0)
	dir := Point{unit[0] - origin[0], unit[1] - origin[1]}
	if length := math.Hypot(dir[0], dir[1]); length > 0 {
		dir = Point{dir[0] / length, dir[1] / length}
	}
	text, ok := g.Attrs["unicode"]
	if !ok || text == "" || strings.ContainsRune(text, utf8.RuneError) {
		text = string(utf8.RuneError)
	}
	return glyph{text: text, at: at, end: Point{at[0] + step[0] - origin[0], at[1] + step[1] - origin[1]},
		dir: dir, size: size, color: color}
}

// glyph adds g to the open label when it continues it, else starts a label (returned).
func (r *traceReader) glyph(g glyph) *Node {
	if open := r.label; open != nil && open.continues(g) {
		gap := float64((g.at[0]-open.end[0])*open.dir[0]) + float64((g.at[1]-open.end[1])*open.dir[1])
		if gap > spaceGap*open.size && !strings.HasSuffix(open.node.Text, " ") && g.text != " " {
			open.node.Text += " "
			open.node.traced += " "
		}
		if level := open.levelOf(g); level != open.level {
			if level != "" {
				open.node.Text += level
				open.node.traced += level
			}
			open.level = level
		}
		r.addText(open.node, g)
		open.end = g.end
		open.node.traceEnd = &g.end
		return nil
	}
	point := roundPoint(g.at)
	node := &Node{Kind: KindText, Point: &point, TextType: "label",
		Size: fmt.Sprintf("%gpt", round1(g.size/r.scale))}
	if g.color != "black" {
		node.Stroke = g.color
	}
	r.addText(node, g)
	node.traceEnd = &g.end
	r.label = &openLabel{node: node, base: g.at, end: g.end, dir: g.dir, size: g.size, color: g.color}
	if !r.inside(g.at) {
		return nil // a label starting outside the box (a cut page's neighbouring text) is left out
	}
	return node
}

// inside reports whether p lies in the box.
func (r *traceReader) inside(p Point) bool {
	return p[0] >= r.extent[0] && p[1] >= r.extent[1] && p[0] <= r.extent[2] && p[1] <= r.extent[3]
}

func (r *traceReader) addText(node *Node, g glyph) {
	if strings.ContainsRune(g.text, utf8.RuneError) {
		node.TextType = unmappedText
	}
	node.traced += g.text
	if g.smallCaps && strings.ToUpper(g.text) != g.text {
		node.Text += strings.ToUpper(g.text)
		node.smallCaps = true
		return
	}
	node.Text += g.text
}

// smallCapsFont reports whether a trace font name (subset prefix "ABCDEF+" aside) names a
// small-caps font: "SC", "Caps" or "smcp" in it (NimbusRomNo9L-ReguSC, …SmallCaps), CM's
// "csc" (cmcsc10) or a name ending in "sc" (txfonts' rtxsc, rtxbsc).
func smallCapsFont(name string) bool {
	if _, after, found := strings.Cut(name, "+"); found {
		name = after
	}
	lower := strings.TrimRight(strings.ToLower(name), "0123456789")
	return strings.Contains(name, "SC") || strings.Contains(name, "Caps") || strings.Contains(lower, "smcp") ||
		strings.Contains(lower, "csc") || strings.HasSuffix(lower, "sc")
}

// caseNote is the note of a label printed in small caps: the traced (lowercase) text.
func caseNote(traced string) string {
	return `case "small caps", traced ` + strconv.Quote(traced)
}

// continues reports whether g follows the label's end in its colour and direction, on its
// baseline or as a sub- or superscript of it.
func (l *openLabel) continues(g glyph) bool {
	if g.color != l.color || float64(l.dir[0]*g.dir[0])+float64(l.dir[1]*g.dir[1]) < 0.99 {
		return false
	}
	dx, dy := g.at[0]-l.end[0], g.at[1]-l.end[1]
	along := float64(dx*l.dir[0]) + float64(dy*l.dir[1])
	return math.Abs(along) <= gapSlack*l.size && l.levelOf(g) != noLevel
}

// noLevel marks a glyph that is neither on a label's baseline nor one of its scripts.
const noLevel = "none"

// levelOf is g's script level in the label: "" for a glyph of the label's size on its baseline
// (within baselineSlack), "_" for a smaller one (below sizeSlack) lowered, "^" raised (by more than
// scriptShift), noLevel for anything else (a larger glyph, a shifted one of the same size).
func (l *openLabel) levelOf(g glyph) string {
	dx, dy := g.at[0]-l.base[0], g.at[1]-l.base[1]
	shift := float64(dy*l.dir[0]) - float64(dx*l.dir[1])
	switch {
	case l.size < sizeSlack*g.size:
		return noLevel
	case g.size >= sizeSlack*l.size:
		if math.Abs(shift) <= baselineSlack*l.size {
			return ""
		}
		return noLevel
	case shift < -scriptShift*l.size:
		return "_"
	case shift > scriptShift*l.size:
		return "^"
	}
	return noLevel
}

// finishLabels trims the labels' text and drops those that hold only spaces.
func finishLabels(nodes []*Node) []*Node {
	out := nodes[:0]
	for _, n := range nodes {
		if n.Kind == KindGroup {
			n.Children = finishLabels(n.Children)
		}
		if n.Kind == KindText {
			n.Text = strings.TrimFunc(n.Text, unicode.IsSpace)
			if n.smallCaps {
				n.Case = caseNote(strings.TrimFunc(n.traced, unicode.IsSpace))
			}
			if n.Text == "" {
				continue
			}
		}
		out = append(out, n)
	}
	return out
}

// minSegments is the fewest sibling segments of one style that become one "N segments" node.
const minSegments = 8

// lengthStep is the ratio of segment lengths one run spans: ticks and frame lines stay apart.
const lengthStep = 1.5

// groupSegments replaces, at every level, the many open two-point segments of one style (axis
// ticks, grid lines, hatching) by one node standing for them, placed where the first was.
func groupSegments(nodes []*Node) []*Node {
	runs := map[string][]*Node{}
	for _, n := range nodes {
		if n.Kind == KindGroup {
			n.Children = groupSegments(n.Children)
		}
		if key := segmentKey(n); key != "" {
			runs[key] = append(runs[key], n)
		}
	}
	var out []*Node
	for _, n := range nodes {
		members := runs[segmentKey(n)]
		switch {
		case len(members) < minSegments:
			out = append(out, n)
		case members[0] == n:
			out = append(out, segmentRun(members))
		}
	}
	return out
}

func segmentKey(n *Node) string {
	if n.Kind != KindPath || len(n.Parts) != 1 || n.Fill != "" || n.Arrows != "" {
		return ""
	}
	part := n.Parts[0]
	if part.Shape != ShapePolyline || len(part.Points) != 2 {
		return ""
	}
	length := math.Hypot(part.Points[1][0]-part.Points[0][0], part.Points[1][1]-part.Points[0][1])
	return fmt.Sprintf("%s|%g|%s|%s|%.0f", n.Stroke, n.Pen, n.Dash, n.Layer, math.Round(math.Log(length+0.1)/math.Log(lengthStep)))
}

func segmentRun(members []*Node) *Node {
	first := members[0]
	node := &Node{Kind: KindGroup, Layer: first.Layer, Shape: "segment", Style: first.Style, Count: len(members)}
	var ends, middles []Point
	weight := 0.0
	for _, m := range members {
		a, b := m.Parts[0].Points[0], m.Parts[0].Points[1]
		ends = append(ends, a, b)
		middles = append(middles, roundPoint(Point{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2}))
		weight += float64(math.Hypot(b[0]-a[0], b[1]-a[1]) * m.contrast)
	}
	box := extent(ends)
	node.Box = &box
	node.contrast = weight / markWeight
	if len(members) <= maxMembers {
		node.Members = middles
	}
	return node
}
