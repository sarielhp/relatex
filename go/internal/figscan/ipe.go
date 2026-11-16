package figscan

import (
	"fmt"
	"strings"
)

// ipeReader turns the objects of one Ipe page, as one view shows them, into nodes.
type ipeReader struct {
	styles   styles
	toBox    matrix
	visible  map[string]bool
	layers   map[string]bool
	colorMap map[string]string
	symbols  map[string]bool
}

// readIpe reads the Ipe document source as its PDF page pdfPage (1-based) shows it inside box.
// pdfPages is the PDF's page count (0 when unknown); it tells a marked-views export from a full one.
func readIpe(source []byte, pdfPage, pdfPages int, box Box) (*Figure, error) {
	root, err := parseIpe(source)
	if err != nil {
		return nil, err
	}
	place, ok := findView(root, pdfPage, pdfPages)
	if !ok {
		return nil, fmt.Errorf("ipe: PDF page %d is not one of the document's %d views", pdfPage, place.totalPdfPages)
	}
	reader := &ipeReader{styles: readStyles(root), visible: visibleLayers(place), layers: pageLayers(place.page),
		colorMap: viewColorMap(place.view), symbols: readSymbols(root)}
	reader.toBox = box.normalizer()
	figure := &Figure{Source: SourceIpe, Page: pdfPage, box: box,
		Detail: fmt.Sprintf("Ipe %s (%s)", root.attr("version"), root.attr("creator")),
		View:   describeView(place), Layers: shownLayers(place)}
	figure.Nodes = reader.objects(place.page.Children, identity, firstLayer(place.page))
	return figure, nil
}

// objects reads a page's objects (layer set) or a group's children (layer empty: they share the
// group's). Ipe writes an object's layer only when it differs from the previous object's; a name
// that is not one of the page's layers leaves the layer as it was (ImlParser::parseObject).
func (r *ipeReader) objects(elements []element, outer matrix, layer string) []*Node {
	var nodes []*Node
	for i := range elements {
		e := &elements[i]
		if layer != "" && r.layers[e.attr("layer")] {
			layer = e.attr("layer")
		}
		if layer != "" && !r.visible[layer] {
			continue
		}
		node := r.object(e, outer)
		if node == nil {
			continue
		}
		if layer != "" {
			node.Layer = layer
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// object reads one object. Paths, groups and images draw with their matrix restricted by their
// transformations attribute (Painter::untransform); marks and texts only place their position
// with the full matrix.
func (r *ipeReader) object(e *element, outer matrix) *Node {
	own := parseMatrix(e.attr("matrix")).then(outer)
	switch e.XMLName.Local {
	case "path":
		return r.path(e, own.restricted(e.attr("transformations")))
	case "use":
		return r.mark(e, own)
	case "text":
		return r.text(e, own)
	case "image":
		return r.image(e, own.restricted(e.attr("transformations")))
	case "group":
		return r.group(e, own.restricted(e.attr("transformations")))
	}
	return nil
}

// group reads a group and its clipping path (the clip attribute, in the group's coordinates).
func (r *ipeReader) group(e *element, own matrix) *Node {
	group := &Node{Kind: KindGroup, Children: r.objects(e.Children, own, "")}
	if len(group.Children) == 0 {
		return nil
	}
	if clip := e.attr("clip"); clip != "" {
		group.Clip = parsePath(clip, own.then(r.toBox))
	}
	return group
}

// color is a colour attribute after the view's colour mapping.
func (r *ipeReader) color(value string) string {
	if to, ok := r.colorMap[value]; ok {
		return to
	}
	return value
}

// path reads a path object as ipelib's Path(XmlAttributes) does: neither stroke nor fill means
// stroked black, and stroke-opacity defaults to opacity.
func (r *ipeReader) path(e *element, own matrix) *Node {
	parts := parsePath(e.Text, own.then(r.toBox))
	if len(parts) == 0 {
		return nil
	}
	node := &Node{Kind: KindPath, Parts: parts}
	stroke, fill := r.color(e.attr("stroke")), r.color(e.attr("fill"))
	if stroke == "" && fill == "" {
		stroke = "black"
	}
	opacity := r.styles.opacity(e.attr("opacity"))
	strokeOpacity := opacity
	if e.has("stroke-opacity") {
		strokeOpacity = r.styles.opacity(e.attr("stroke-opacity"))
	}
	node.Stroke, node.Fill = r.styles.colorName(stroke), r.styles.colorName(fill)
	if stroke != "" {
		node.Pen = round1(r.styles.pen(e.attr("pen"))*100) / 100
		node.contrast = r.styles.contrast(stroke) * strokeOpacity
	}
	if fill != "" {
		node.fillShade = r.styles.contrast(fill) * opacity
	}
	if fill != "" && opacity < 1 {
		node.Opacity = opacity
	}
	node.Dash = e.attr("dash")
	if len(parts) == 1 && !parts[0].Closed {
		node.Arrows = arrows(e.attr("arrow"), e.attr("rarrow"), e.has("arrow"), e.has("rarrow"))
	}
	return node
}

// arrows describes a path's arrows; ipelib draws them only on a path of one open subpath
// (Path::makeArrowData), and a mid-arrow shape ("mid-normal/normal") sits halfway along.
func arrows(forward, backward string, hasForward, hasBackward bool) string {
	var ends []string
	if hasBackward {
		ends = append(ends, arrowPlace(backward, "start"))
	}
	if hasForward {
		ends = append(ends, arrowPlace(forward, "end"))
	}
	switch {
	case len(ends) == 2 && ends[0] == "at start" && ends[1] == "at end":
		return "both ends"
	case len(ends) == 2 && ends[0] == ends[1]:
		return ends[0]
	}
	return strings.Join(ends, " and ")
}

func arrowPlace(value, end string) string {
	if strings.HasPrefix(value, "mid-") {
		return "at middle"
	}
	return "at " + end
}

// mark is a <use> of a symbol: Ipe's marks (mark/disk(sx) …) or another named symbol. ipelib
// draws nothing for a symbol the stylesheets do not define (Reference::draw); a document without
// any symbol definitions (no stylesheet saved) keeps every mark.
func (r *ipeReader) mark(e *element, own matrix) *Node {
	name := e.attr("name")
	if len(r.symbols) > 0 && !r.symbols[name] {
		return nil
	}
	shape := strings.TrimPrefix(name, "mark/")
	if open := strings.IndexByte(shape, '('); open > 0 {
		shape = shape[:open]
	}
	pos := numbers(e.attr("pos"))
	if len(pos) != 2 {
		pos = []float64{0, 0}
	}
	point := roundPoint(own.then(r.toBox).apply(pos[0], pos[1]))
	stroke, fill := r.color(e.attr("stroke")), r.color(e.attr("fill"))
	node := &Node{Kind: KindMark, Shape: shape, Point: &point, Size: e.attr("size")}
	node.Stroke, node.Fill = r.styles.colorName(stroke), r.styles.colorName(fill)
	node.contrast = 1
	if node.Stroke != "" || node.Fill != "" {
		node.contrast = 0
		for _, color := range []string{stroke, fill} {
			if color != "" {
				node.contrast = max(node.contrast, r.styles.contrast(color))
			}
		}
	}
	return node
}

// text reads a text object; without a type attribute, a width makes it a minipage (Text's
// XML constructor).
func (r *ipeReader) text(e *element, own matrix) *Node {
	pos := numbers(e.attr("pos"))
	if len(pos) != 2 {
		pos = []float64{0, 0}
	}
	point := roundPoint(own.then(r.toBox).apply(pos[0], pos[1]))
	textType := e.attr("type")
	switch {
	case textType == "" && e.has("width"):
		textType = "minipage"
	case textType == "":
		textType = "label"
	}
	node := &Node{Kind: KindText, Point: &point, Text: strings.TrimSpace(e.Text), TextType: textType,
		Math: e.attr("style") == "math", Size: e.attr("size")}
	node.Stroke = r.styles.colorName(r.color(e.attr("stroke")))
	return node
}

func (r *ipeReader) image(e *element, own matrix) *Node {
	rect := numbers(e.attr("rect"))
	if len(rect) != 4 {
		return nil
	}
	full := own.then(r.toBox)
	box := extent([]Point{full.apply(rect[0], rect[1]), full.apply(rect[2], rect[1]),
		full.apply(rect[0], rect[3]), full.apply(rect[2], rect[3])})
	return &Node{Kind: KindImage, Box: &box}
}

// extent is the rounded bounding box of points.
func extent(points []Point) [4]float64 {
	box := [4]float64{points[0][0], points[0][1], points[0][0], points[0][1]}
	for _, p := range points[1:] {
		box[0], box[1] = min(box[0], p[0]), min(box[1], p[1])
		box[2], box[3] = max(box[2], p[0]), max(box[3], p[1])
	}
	return [4]float64{round1(box[0]), round1(box[1]), round1(box[2]), round1(box[3])}
}
