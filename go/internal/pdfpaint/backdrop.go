package pdfpaint

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/wcag"
)

// The backdrops of text (decision 113): what the PDF paints behind a faint ink's glyphs. A tcolorbox
// title, a title page's coloured band or a frame filled in its own colour (metropolis' standout
// frames, a deck's blue title page) is a path the PDF fills before the text set on it; the
// document's spans do not say so, since the picture is drawn in literal PDF code. The text paint
// proof reads, for each glyph of an ink that reads below 4.5:1 on white paper, the last opaque fill
// painted before it whose box (clipped by the clips around it) holds the glyph's middle, and
// records each ink's text on each such fill: the HTML page paints that fill behind the spans whose
// text it holds, and their ink is read against it.

// paperWhite is the paper's colour, behind text no fill covers.
const paperWhite = "#ffffff"

// backdropHidden is the contrast below which an ink is invisible on a fill too (contrast.HiddenRatio).
const backdropHidden = 1.1

// glyphRise is how far above its baseline a glyph's middle is, in ems.
const glyphRise = 0.3

// paintedFill is an opaque fill of the page: its colour ("" for a paint of unknown colour, an
// image or a shading) and its box in device space.
type paintedFill struct {
	color string
	box   [4]float64
}

// backdropReader follows a trace's pages, the fills each paints and the faint glyphs set on them.
type backdropReader struct {
	fills []paintedFill
	clips [][4]float64
	path  *pathReader
	texts map[[2]string]*strings.Builder
	media [4]float64
}

// pathReader reads the corners of a fill_path or clip_path element.
type pathReader struct {
	clip   bool
	color  string
	depth  int
	matrix []float64
	points [][2]float64
}

// visit reads one trace element; open are its enclosing elements.
func (r *backdropReader) visit(name string, attrs map[string]string, open []*node) {
	if r.path != nil && len(open) >= r.path.depth {
		r.path.add(attrs)
		return
	}
	r.closePath()
	switch name {
	case "page":
		r.fills, r.clips = nil, nil
		r.media = pageBox(attrs)
	case "fill_path", "clip_path":
		color, _ := canvasColor(attrs)
		r.path = &pathReader{clip: name == "clip_path", color: color, depth: len(open) + 1, matrix: numbers(attrs["transform"])}
	case "fill_image", "fill_image_mask":
		r.fills = append(r.fills, paintedFill{box: r.clipped(unitBox(numbers(attrs["transform"])))})
	case "clip_image_mask":
		r.clips = append(r.clips, unitBox(numbers(attrs["transform"])))
	case "clip_text", "clip_stroke_path", "clip_stroke_text":
		r.clips = append(r.clips, r.media)
	case "pop_clip":
		if len(r.clips) > 0 {
			r.clips = r.clips[:len(r.clips)-1]
		}
	case "fill_shade":
		r.fills = append(r.fills, paintedFill{box: r.clipped(r.media)})
	case "g":
		r.glyph(attrs, open)
	}
}

// closePath ends the path being read: a fill joins the page's fills, a clip the clips.
func (r *backdropReader) closePath() {
	path := r.path
	r.path = nil
	if path == nil || len(path.points) == 0 || len(path.matrix) != 6 {
		return
	}
	box := placedBox(path.matrix, path.points)
	if path.clip {
		r.clips = append(r.clips, box)
		return
	}
	r.fills = append(r.fills, paintedFill{color: path.color, box: r.clipped(box)})
}

// clipped is box within every open clip.
func (r *backdropReader) clipped(box [4]float64) [4]float64 {
	for _, clip := range r.clips {
		box = [4]float64{math.Max(box[0], clip[0]), math.Max(box[1], clip[1]), math.Min(box[2], clip[2]), math.Min(box[3], clip[3])}
	}
	return box
}

// glyph records a glyph of a faint ink on the fill behind its middle.
func (r *backdropReader) glyph(attrs map[string]string, open []*node) {
	if len(open) < 2 || open[len(open)-1].Name != "span" || open[len(open)-2].Name != "fill_text" {
		return
	}
	text := open[len(open)-2].Attrs
	ink, ok := InkHex(text["colorspace"], strings.Fields(text["color"]))
	matrix, trm := numbers(text["transform"]), numbers(open[len(open)-1].Attrs["trm"])
	character := strings.Join(strings.Fields(attrs["unicode"]), "")
	if !ok || ink == "" || !faint(ink) || len(matrix) != 6 || len(trm) != 4 || character == "" {
		return
	}
	advance := number(attrs["adv"]) / 2
	x := number(attrs["x"]) + float64(trm[0]*advance) + float64(trm[2]*glyphRise)
	y := number(attrs["y"]) + float64(trm[1]*advance) + float64(trm[3]*glyphRise)
	point := placed(matrix, [2]float64{x, y})
	fill := ""
	for index := len(r.fills) - 1; index >= 0; index-- {
		box := r.fills[index].box
		if box[0] <= point[0] && point[0] <= box[2] && box[1] <= point[1] && point[1] <= box[3] {
			fill = r.fills[index].color
			if fill == "" {
				return // an image or a shading: its colour is unknown
			}
			break
		}
	}
	if fill == "" {
		return
	}
	if r.texts == nil {
		r.texts = map[[2]string]*strings.Builder{}
	}
	key := [2]string{ink, fill}
	if r.texts[key] == nil {
		r.texts[key] = &strings.Builder{}
	}
	r.texts[key].WriteString(character)
}

// add reads one corner element of the path: the end and control points of its segments.
func (p *pathReader) add(attrs map[string]string) {
	for _, suffix := range []string{"", "1", "2", "3"} {
		if x, ok := attrs["x"+suffix]; ok {
			p.points = append(p.points, [2]float64{number(x), number(attrs["y"+suffix])})
		}
	}
}

// faint: an ink that reads below 4.5:1 on white paper, whose backdrops the proof records.
func faint(ink string) bool {
	color, _ := wcag.Parse(ink)
	white, _ := wcag.Parse(paperWhite)
	return wcag.Ratio(color, white) < wcag.TextRatio
}

func placed(m []float64, point [2]float64) [2]float64 {
	return [2]float64{float64(m[0]*point[0]) + float64(m[2]*point[1]) + m[4],
		float64(m[1]*point[0]) + float64(m[3]*point[1]) + m[5]}
}

// placedBox is the box of points through the matrix.
func placedBox(matrix []float64, points [][2]float64) [4]float64 {
	box := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, point := range points {
		at := placed(matrix, point)
		box = [4]float64{math.Min(box[0], at[0]), math.Min(box[1], at[1]), math.Max(box[2], at[0]), math.Max(box[3], at[1])}
	}
	return box
}

// unitBox is an image's box: the unit square through its matrix.
func unitBox(matrix []float64) [4]float64 {
	if len(matrix) != 6 {
		return [4]float64{math.Inf(-1), math.Inf(-1), math.Inf(1), math.Inf(1)}
	}
	return placedBox(matrix, [][2]float64{{0, 0}, {1, 0}, {0, 1}, {1, 1}})
}

// pageBox is a page's media box, unbounded when unreadable.
func pageBox(attrs map[string]string) [4]float64 {
	media := numbers(attrs["mediabox"])
	if len(media) != 4 {
		return [4]float64{math.Inf(-1), math.Inf(-1), math.Inf(1), math.Inf(1)}
	}
	return [4]float64{math.Min(media[0], media[2]), math.Min(media[1], media[3]), math.Max(media[0], media[2]), math.Max(media[1], media[3])}
}

// merge adds a later run of pages' texts.
func (r *backdropReader) merge(other *backdropReader) {
	for key, text := range other.texts {
		if r.texts == nil {
			r.texts = map[[2]string]*strings.Builder{}
		}
		if r.texts[key] == nil {
			r.texts[key] = &strings.Builder{}
		}
		r.texts[key].WriteString(text.String())
	}
}

// backdrops are the proof's record: each faint ink's text on each fill it reads on, sorted by ink
// and fill. A fill that is the paper's white or the document's canvas (decision 107, read
// already), one on which the ink is invisible too or reads no better than on paper, is none.
func (r *backdropReader) backdrops(canvas string) []any {
	keys := make([][2]string, 0, len(r.texts))
	white, _ := wcag.Parse(paperWhite)
	for key := range r.texts {
		ink, _ := wcag.Parse(key[0])
		fill, _ := wcag.Parse(key[1])
		ratio := wcag.Ratio(ink, fill)
		if key[1] == paperWhite || key[1] == canvas || ratio < backdropHidden || ratio <= wcag.Ratio(ink, white) {
			continue
		}
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b [2]string) int { return strings.Compare(a[0]+a[1], b[0]+b[1]) })
	result := make([]any, len(keys))
	for index, key := range keys {
		result[index] = rec.Map{"ink": key[0], "fill": key[1], "text": r.texts[key].String()}
	}
	return result
}

// number is a trace attribute's number, 0 when it is none: the backdrops are read beside the
// proof's own checks and never fail it.
func number(text string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

// numbers are a trace attribute's numbers (number).
func numbers(text string) []float64 {
	fields := strings.Fields(text)
	result := make([]float64, len(fields))
	for index, field := range fields {
		result[index] = number(field)
	}
	return result
}
