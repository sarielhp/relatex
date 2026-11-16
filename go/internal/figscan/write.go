package figscan

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// textPoints is the most points a part lists in the text form (the JSON keeps all).
const textPoints = 10

// textMembers is the most member positions a collapsed run lists in the text form.
const textMembers = 12

// JSON is the figure's JSON form, indented, ending in a newline.
func (f *Figure) JSON() []byte {
	data, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		panic(fmt.Sprintf("figscan: %v", err))
	}
	return append(data, '\n')
}

// Text is the figure's readable form: a header, every label with its nearest primitives, then
// the forest of shapes, indented by nesting.
func (f *Figure) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "geometry (%s): source %s", Version, f.Source)
	if f.Detail != "" {
		fmt.Fprintf(&b, ", %s", f.Detail)
	}
	// A picture read by OCR is measured in its own pixels and has no page (decision 123, amendment 4).
	place, unit := fmt.Sprintf("PDF page %d", f.Page), "pt"
	if f.Source == SourceOCR {
		place, unit = "picture", "px"
	}
	fmt.Fprintf(&b, "\n%s", place)
	if f.View != "" {
		fmt.Fprintf(&b, " = %s", f.View)
	}
	b.WriteString("\n")
	if f.Source == SourceNone {
		return b.String()
	}
	fmt.Fprintf(&b, "box %s x %s %s; coordinates x 0-%s, y 0-%s (x right, y up; 0-100 along the longer side; origin bottom left, same scale on both axes)\n",
		num(f.Size[0]), num(f.Size[1]), unit, num(f.Extent[0]), num(f.Extent[1]))
	b.WriteString(f.turnedLine())
	fmt.Fprintf(&b, "shapes: %d of %d shown; labels: %d\n", f.Shown, f.Total, len(f.Labels))
	b.WriteString("\nlabels (position; nearest primitives with distance, no attachment implied):\n")
	if len(f.Labels) == 0 {
		b.WriteString("  none\n")
	}
	for _, label := range f.Labels {
		fmt.Fprintf(&b, "  %s %s at %s;", label.ID, printedText(label.Text, label.Math, label.Printed, label.Note), point(label.Point))
		for i, near := range label.Nearest {
			sep := ","
			if i == 0 {
				sep = " nearest"
			}
			fmt.Fprintf(&b, "%s %s %s %s", sep, near.ID, near.Kind, num(near.Distance))
		}
		b.WriteString("\n")
	}
	if f.Source == SourceOCR {
		b.WriteString("\nshapes: none read (OCR reads only the text)\n")
		return b.String()
	}
	b.WriteString("\nshapes:\n")
	writeNodes(&b, f.Nodes, 1)
	writeCut(&b, f.Cut, 1)
	return b.String()
}

func labelText(text string, math bool) string {
	text = strings.Join(strings.Fields(text), " ")
	if math {
		text = "$" + text + "$"
	}
	return `"` + text + `"`
}

// printedText is a label as printed with its source, or its source alone marked "source" when
// nothing printed matched it; a trace label (no note) is printed text already.
func printedText(text string, math bool, printed, note string) string {
	switch {
	case printed != "" && note != "":
		return "printed " + labelText(printed, false) + " (" + note + "; source " + labelText(text, math) + ")"
	case printed != "":
		return "printed " + labelText(printed, false) + " (source " + labelText(text, math) + ")"
	case note != "":
		return note + " " + labelText(text, math)
	}
	return labelText(text, math)
}

func writeNodes(b *strings.Builder, nodes []*Node, depth int) {
	indent := strings.Repeat("  ", depth)
	for _, n := range nodes {
		line := nodeLine(n)
		if n.Repeats > 1 {
			line += fmt.Sprintf(" (drawn %d times over each other)", n.Repeats)
		}
		fmt.Fprintf(b, "%s%s %s\n", indent, n.ID, line)
		if n.Kind == KindGroup && n.Count == 0 {
			writeNodes(b, n.Children, depth+1)
			writeCut(b, n.Cut, depth+1)
		}
	}
}

func writeCut(b *strings.Builder, cut map[string]int, depth int) {
	if len(cut) == 0 {
		return
	}
	total := 0
	var parts []string
	for _, kind := range slices.Sorted(maps.Keys(cut)) {
		total += cut[kind]
		parts = append(parts, fmt.Sprintf("%d %s", cut[kind], kind))
	}
	fmt.Fprintf(b, "%splus %d more not listed (%s)\n", strings.Repeat("  ", depth), total, strings.Join(parts, ", "))
}

func nodeLine(n *Node) string {
	switch {
	case n.Count > 0:
		return runLine(n)
	case n.Kind == KindGroup:
		return fmt.Sprintf("group of %d, box %s%s", len(n.Children), boxText(n.Box), clipText(n.Clip))
	case n.Kind == KindText:
		return fmt.Sprintf("label %s (%s)", printedText(n.Text, n.Math, n.Printed, n.PrintedNote), joinNonEmpty(n.TextType, n.Case, n.Size, n.Stroke))
	case n.Kind == KindMark:
		return fmt.Sprintf("%s mark at %s%s", n.Shape, point(*n.Point), styleText(n.Style, n.Size))
	case n.Kind == KindImage && n.Shape != "":
		return n.Shape + ", box " + boxText(n.Box)
	case n.Kind == KindImage:
		return "image, box " + boxText(n.Box)
	}
	texts := make([]string, len(n.Parts))
	for i, part := range n.Parts {
		texts[i] = partText(part)
	}
	return strings.Join(texts, "; ") + styleText(n.Style, "")
}

func clipText(clip []Part) string {
	if len(clip) == 0 {
		return ""
	}
	texts := make([]string, len(clip))
	for i, part := range clip {
		texts[i] = partText(part)
	}
	return ", clipped to " + strings.Join(texts, "; ")
}

func runLine(n *Node) string {
	line := fmt.Sprintf("%d alike %s%s, box %s", n.Count, plural(n.Shape), styleText(n.Style, n.Size), boxText(n.Box))
	if len(n.Members) == 0 || len(n.Members) > textMembers {
		return line
	}
	points := make([]string, len(n.Members))
	for i, p := range n.Members {
		points[i] = point(p)
	}
	return line + ": " + strings.Join(points, " ")
}

func partText(p Part) string {
	switch p.Shape {
	case ShapeEllipse:
		if p.Radii[0] == p.Radii[1] {
			return fmt.Sprintf("circle center %s radius %s", point(*p.Center), num(p.Radii[0]))
		}
		return fmt.Sprintf("ellipse center %s radii %s x %s at %s deg", point(*p.Center), num(p.Radii[0]), num(p.Radii[1]), num(p.Angle))
	case ShapeArc:
		radius := num(p.Radii[0])
		if p.Radii[0] != p.Radii[1] {
			radius = num(p.Radii[0]) + " x " + num(p.Radii[1])
		}
		return fmt.Sprintf("arc from %s through %s to %s, center %s radius %s",
			point(p.Points[0]), point(*p.Through), point(p.Points[1]), point(*p.Center), radius)
	case ShapePolyline:
		if len(p.Points) == 2 && p.Original == 0 {
			return fmt.Sprintf("segment %s-%s", point(p.Points[0]), point(p.Points[1]))
		}
	}
	name := p.Shape
	if p.Closed && p.Shape != ShapePolygon {
		name = "closed " + name
	}
	label := " points"
	if p.Shape == ShapeSpline {
		label = " control points"
	}
	count := fmt.Sprintf("%d%s", len(p.Points), label)
	if p.Original > 0 {
		count = fmt.Sprintf("%d points, simplified to %d", p.Original, len(p.Points))
	}
	return fmt.Sprintf("%s, %s %s", name, count, pointList(p.Points))
}

func pointList(points []Point) string {
	shown := points[:min(len(points), textPoints)]
	texts := make([]string, len(shown))
	for i, p := range shown {
		texts[i] = point(p)
	}
	if len(points) > textPoints {
		texts = append(texts, "...")
	}
	return strings.Join(texts, " ")
}

func styleText(s Style, size string) string {
	var parts []string
	if s.Stroke != "" {
		parts = append(parts, strings.TrimSpace("stroke "+s.Stroke+" "+penText(s.Pen)))
	}
	if s.Fill != "" {
		parts = append(parts, "fill "+s.Fill)
	}
	for _, extra := range []string{s.Dash, size} {
		if extra != "" {
			parts = append(parts, extra)
		}
	}
	if s.Opacity > 0 {
		parts = append(parts, "fill opacity "+num(s.Opacity))
	}
	if s.Arrows != "" {
		parts = append(parts, "arrow "+s.Arrows)
	}
	if len(parts) == 0 {
		return ""
	}
	return "; " + strings.Join(parts, ", ")
}

func penText(pen float64) string {
	if pen == 0 {
		return ""
	}
	return "pen " + num(pen)
}

func joinNonEmpty(values ...string) string {
	var kept []string
	for _, v := range values {
		if v != "" {
			kept = append(kept, v)
		}
	}
	return strings.Join(kept, ", ")
}

func boxText(box *[4]float64) string {
	if box == nil {
		return "empty"
	}
	return fmt.Sprintf("x %s-%s y %s-%s", num(box[0]), num(box[2]), num(box[1]), num(box[3]))
}

func point(p Point) string { return "(" + num(p[0]) + ", " + num(p[1]) + ")" }

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
