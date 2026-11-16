package figscan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/nativetrace"
)

// traceTimeout bounds one mutool trace run.
const traceTimeout = 2 * time.Minute

// tracePage runs mutool trace on one page of pdf and returns the page element.
func tracePage(pdf []byte, page int) (*nativetrace.Node, error) {
	tool := command.Resolve("mutool")
	if tool == "" {
		return nil, errors.New("mutool not found")
	}
	dir := command.Scratch("figscan-")
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "figure.pdf")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		return nil, err
	}
	count, err := runMutool(tool, "show", path, "trailer/Root/Pages/Count")
	if err != nil {
		return nil, err
	}
	// mutool clamps a page number past the end to the last page: check it first.
	if pages, convErr := strconv.Atoi(string(bytes.TrimSpace(count))); convErr != nil || page > pages {
		return nil, fmt.Errorf("page %d: not in the PDF (%s pages)", page, bytes.TrimSpace(count))
	}
	trace, err := runMutool(tool, "trace", path, strconv.Itoa(page))
	if err != nil {
		return nil, err
	}
	if nativetrace.Declared(trace) {
		return nil, errors.New("mutool trace: declarations in the trace")
	}
	document, err := nativetrace.ParseTree(trace)
	if err != nil {
		return nil, fmt.Errorf("mutool trace: %w", err)
	}
	pages := document.Descendants("page")
	if len(pages) == 0 {
		return nil, fmt.Errorf("page %d: mutool traced no page", page)
	}
	return pages[0], nil
}

// runMutool runs one mutool command and returns its output.
func runMutool(tool string, arguments ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), traceTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, arguments...)
	cmd.Env = command.Environment(nil)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("mutool %s: %v: %s", arguments[0], err, firstLine(stderr.String()))
	}
	return out.Bytes(), nil
}

func firstLine(text string) string {
	line, _, _ := bytes.Cut(bytes.TrimSpace([]byte(text)), []byte("\n"))
	return string(line)
}

// traceBounds is the page's box in mutool's device space (y down), from the trace's mediabox.
func traceBounds(page *nativetrace.Node) (Box, error) {
	values := numbers(page.Attrs["mediabox"])
	if len(values) != 4 {
		return Box{}, errors.New("mutool trace: page without mediabox")
	}
	box := Box{X0: values[0], Y0: values[1], X1: values[2], Y1: values[3]}
	if box.Width() <= 0 || box.Height() <= 0 {
		return Box{}, errors.New("mutool trace: empty page")
	}
	return box, nil
}

// readTrace reads a traced page as box shows it. crop is the page's box in PDF points (where mutool's
// device origin sits), bounds the same box in device space.
func readTrace(page *nativetrace.Node, number int, crop, bounds, box Box) *Figure {
	toPDF := matrix{1, 0, 0, -1, crop.X0 - bounds.X0, crop.Y1 + bounds.Y0}
	reader := &traceReader{toBox: toPDF.then(box.normalizer()), scale: box.scale(),
		extent: [4]float64{0, 0, box.Width() * box.scale(), box.Height() * box.scale()}}
	nodes := reader.elements(page.Children)
	figure := &Figure{Source: SourceTrace, Page: number, box: box, Detail: "mutool trace"}
	if image := reader.bitmap(); image != nil {
		figure.Source, figure.Detail = SourceBitmap, "one image covers the box"
		nodes = []*Node{image}
	}
	figure.Nodes = groupSegments(finishLabels(unwrap(nodes)))
	if count := unmappedLabels(figure.Nodes); count > 0 {
		figure.Detail += fmt.Sprintf("; %d labels with glyphs unmapped", count)
	}
	return figure
}

func unmappedLabels(nodes []*Node) int {
	count := 0
	for _, n := range flatten(nodes) {
		if n.TextType == unmappedText {
			count++
		}
	}
	return count
}

// unwrap lifts the children of a lone top-level group (a page drawn as one transparency group).
func unwrap(nodes []*Node) []*Node {
	for len(nodes) == 1 && nodes[0].Kind == KindGroup && nodes[0].Count == 0 {
		nodes = nodes[0].Children
	}
	return nodes
}

// traceReader turns the paint elements of a trace into nodes.
type traceReader struct {
	toBox  matrix
	scale  float64
	extent [4]float64
	clips  [][4]float64
	label  *openLabel
	// images and paints count the images and every other painted element, for the bitmap rule.
	images []*Node
	paints int
}

// elements reads a list of sibling trace elements; containers become groups or are read through.
func (r *traceReader) elements(list []*nativetrace.Node) []*Node {
	var nodes []*Node
	for _, e := range list {
		nodes = r.element(e, nodes)
	}
	return nodes
}

func (r *traceReader) element(e *nativetrace.Node, nodes []*Node) []*Node {
	switch e.Name {
	case "fill_path", "stroke_path":
		r.label = nil
		return r.path(e, nodes)
	case "fill_text", "stroke_text":
		return append(nodes, r.text(e)...)
	case "fill_image", "fill_image_mask":
		r.label = nil
		return append(nodes, r.image(e))
	case "fill_shade":
		r.label = nil
		return append(nodes, r.shade())
	case "group":
		return append(nodes, r.group(e)...)
	case "layer":
		inner := r.elements(e.Children)
		for _, n := range inner {
			n.Layer = e.Attrs["name"]
		}
		return append(nodes, inner...)
	case "pop_clip":
		if len(r.clips) > 0 {
			r.clips = r.clips[:len(r.clips)-1]
		}
		return nodes
	case "ignore_text", "set_default_colorspaces", "end_mask":
		return nodes
	}
	if isClip(e.Name) {
		r.clips = append(r.clips, r.clipBox(e))
		return nodes
	}
	return append(nodes, r.elements(e.Children)...)
}

func isClip(name string) bool {
	return len(name) > 5 && name[:5] == "clip_"
}

// group is a transparency group: one node when it holds several, its content when one.
func (r *traceReader) group(e *nativetrace.Node) []*Node {
	inner := r.elements(e.Children)
	if len(inner) <= 1 {
		return inner
	}
	return []*Node{{Kind: KindGroup, Children: inner}}
}

// clipBox is a clip's extent (normalized), within the clip around it.
func (r *traceReader) clipBox(e *nativetrace.Node) [4]float64 {
	outer := r.extent
	if len(r.clips) > 0 {
		outer = r.clips[len(r.clips)-1]
	}
	var points []Point
	if e.Name == "clip_path" {
		toBox := parseMatrix(e.Attrs["transform"]).then(r.toBox)
		for _, step := range e.Children {
			points = append(points, stepPoints(step, toBox)...)
		}
	} else if bbox := numbers(e.Attrs["bbox"]); len(bbox) == 4 {
		points = []Point{r.toBox.apply(bbox[0], bbox[1]), r.toBox.apply(bbox[2], bbox[3])}
	}
	if len(points) == 0 {
		return outer
	}
	box := extent(points)
	return [4]float64{max(box[0], outer[0]), max(box[1], outer[1]), min(box[2], outer[2]), min(box[3], outer[3])}
}

// image is a raster image: the unit square through its transform.
func (r *traceReader) image(e *nativetrace.Node) *Node {
	full := parseMatrix(e.Attrs["transform"]).then(r.toBox)
	box := extent([]Point{full.apply(0, 0), full.apply(1, 0), full.apply(0, 1), full.apply(1, 1)})
	node := &Node{Kind: KindImage, Box: &box}
	if e.Name == "fill_image" {
		r.images = append(r.images, node)
	} else {
		r.paints++
	}
	return node
}

// shade is a smooth shading: it fills the current clip.
func (r *traceReader) shade() *Node {
	r.paints++
	box := r.extent
	if len(r.clips) > 0 {
		box = r.clips[len(r.clips)-1]
	}
	box = [4]float64{round1(box[0]), round1(box[1]), round1(box[2]), round1(box[3])}
	return &Node{Kind: KindImage, Shape: "shading", Box: &box}
}

// bitmapCover is the share of the box one image must cover for a bitmap-only figure.
const bitmapCover = 0.8

// bitmap is the one image of a figure that paints nothing else and covers most of the box.
func (r *traceReader) bitmap() *Node {
	if len(r.images) != 1 || r.paints > 0 {
		return nil
	}
	b, e := r.images[0].Box, r.extent
	width := min(b[2], e[2]) - max(b[0], e[0])
	height := min(b[3], e[3]) - max(b[1], e[1])
	if width <= 0 || height <= 0 || width*height < bitmapCover*e[2]*e[3] {
		return nil
	}
	return r.images[0]
}
