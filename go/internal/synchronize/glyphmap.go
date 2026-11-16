// Package synchronize is the Go port of the 05-synchronize stage (decision 066): the source
// map built from TeX's own glyph map (Relatex::GlyphSourceMap, GlyphMap, SourceAlignment,
// MathPlacement). Every typeset word gets its page boxes from TeX, the source lines TeX
// recorded, and characters placed by aligning the word with the source text.
package synchronize

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/mathtokens"
	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

// Glyph map columns and flags (GlyphMap::COLUMNS, BOUNDARY, LINE_END, UNKNOWN).
var columns = []string{"text", "font", "char", "dx", "dy", "ex", "tag", "before", "after", "atag", "formula", "flags"}

const (
	boundary = 1
	lineEnd  = 4
	unknown  = 8
)

// spPerBP is GlyphMap::SP_PER_BP (SP_PER_TEX_PT / BP_PER_TEX_PT), divided at run time as Ruby
// does: geometry.SPToBP(65_536) is BP_PER_TEX_PT exactly.
var spPerBP = 65_536.0 / geometry.SPToBP(65_536)

// Coordinates is GlyphMap::COORDINATES.
var Coordinates = rec.Map{"unit": "bp", "origin": "top-left", "source": "TeX shipout glyph map (decision 066)"}

// glyph is one shipped glyph: its text, position and box in sp, and the source tags and lines
// TeX recorded around it.
type glyph struct {
	index                                  int
	text                                   string
	x, y, w, h, d                          int64
	tag, before, after, atag, formula, flg int64
	matrix                                 []float64
}

// box is the glyph's box on the page, [x0, y0, x1, y1] in bp from the top-left corner.
func (g *glyph) box() [4]float64 {
	a, b, c, e := 1.0, 0.0, 0.0, 1.0
	if g.matrix != nil {
		a, b, c, e = g.matrix[0], g.matrix[1], g.matrix[2], g.matrix[3]
	}
	x, y := float64(g.x), float64(g.y)
	// Ruby negates the Integer depth: no -0.0 enters the products.
	depth := float64(-g.d)
	offsets := [4][2]float64{{0, depth}, {float64(g.w), depth}, {0, float64(g.h)}, {float64(g.w), float64(g.h)}}
	xs, ys := make([]float64, 4), make([]float64, 4)
	for index, offset := range offsets {
		dx, dy := offset[0], offset[1]
		xs[index] = x + float64(a*dx) + float64(c*dy)
		ys[index] = y - (float64(b*dx) + float64(e*dy))
	}
	return [4]float64{scaled(minimum(xs)), scaled(minimum(ys)), scaled(maximum(xs)), scaled(maximum(ys))}
}

// scaled is (value / SP_PER_BP).round(4).
func scaled(value float64) float64 { return rec.Round(value/spPerBP, 4) }

// minimum and maximum are Array#min and #max: the first of equal values.
func minimum(values []float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func maximum(values []float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		if value > result {
			result = value
		}
	}
	return result
}

// word is a run of glyphs between word boundaries on one page. attached: for text cut from a
// run that also held a formula, that formula's id; 0 otherwise.
type word struct {
	id       string
	page     int64
	glyphs   []*glyph
	attached int64
}

func (w *word) text() string {
	var builder strings.Builder
	for _, g := range w.glyphs {
		builder.WriteString(g.text)
	}
	return builder.String()
}

func (w *word) tag() int64    { return w.glyphs[0].tag }
func (w *word) before() int64 { return w.glyphs[0].before }

func (w *word) after() int64 {
	result := w.glyphs[0].after
	for _, g := range w.glyphs[1:] {
		result = max(result, g.after)
	}
	return result
}

// afterTag is the file tag of the after line: that of the first glyph with the largest after.
func (w *word) afterTag() int64 {
	best := w.glyphs[0]
	for _, g := range w.glyphs[1:] {
		if g.after > best.after {
			best = g
		}
	}
	return best.atag
}

func (w *word) formula() int64 {
	for _, g := range w.glyphs {
		if g.formula > 0 {
			return g.formula
		}
	}
	return 0
}

func (w *word) unknown() bool {
	for _, g := range w.glyphs {
		if g.flg&unknown != 0 {
			return true
		}
	}
	return false
}

func (w *word) lineEnd() bool { return w.glyphs[len(w.glyphs)-1].flg&lineEnd != 0 }

func (w *word) baseline() []any {
	return []any{scaled(float64(w.glyphs[0].x)), scaled(float64(w.glyphs[0].y))}
}

func (w *word) box() []any {
	boxes := make([][4]float64, len(w.glyphs))
	for index, g := range w.glyphs {
		boxes[index] = g.box()
	}
	pick := func(at int, choose func([]float64) float64) float64 {
		values := make([]float64, len(boxes))
		for index, box := range boxes {
			values[index] = box[at]
		}
		return choose(values)
	}
	return []any{pick(0, minimum), pick(1, minimum), pick(2, maximum), pick(3, maximum)}
}

// glyphMap is GlyphMap: the glyph map's pages and its words.
type glyphMap struct {
	sha256 string
	pages  []glyphPage
	words  []*word
}

type glyphPage struct {
	number        int64
	glyphs        []*glyph
	width, height float64
}

func readGlyphMap(path string) *glyphMap {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		rec.Fail("Missing glyph map")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	m := &glyphMap{sha256: command.FileDigest(path)}
	reader := bufio.NewReader(bytes.NewReader(data))
	for number := int64(1); ; number++ {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			value, decodeErr := canonjson.Decode(bytes.NewReader(line))
			if decodeErr != nil {
				rec.Fail("Invalid glyph map: %v", decodeErr)
			}
			m.pages = append(m.pages, parsePage(value, number))
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	for _, page := range m.pages {
		m.words = append(m.words, page.words()...)
	}
	return m
}

// own is a failure the page reader raises itself (a Relatex::Failure), which pageFailure
// passes through unchanged.
type own struct{ rec.Failure }

// pageFailure is the rescue of KeyError, TypeError and ArgumentError around a page.
func pageFailure(number int64) {
	recovered := recover()
	if recovered == nil {
		return
	}
	if failure, ok := recovered.(own); ok {
		panic(failure.Failure)
	}
	if failure, ok := recovered.(rec.Failure); ok {
		panic(rec.Failure{Message: fmt.Sprintf("Invalid glyph map page %d: %s", number, failure.Message)})
	}
	panic(recovered)
}

func emptyObject(value any) rec.Map {
	if list, ok := value.([]any); ok && len(list) == 0 {
		return rec.Map{}
	}
	return rec.AsMap(value)
}

func parsePage(value any, number int64) glyphPage {
	data, ok := value.(rec.Map)
	if !ok || !rec.Equal(data["page"], number) {
		rec.Fail("Glyph map pages out of order")
	}
	defer pageFailure(number)
	table := rec.FetchMap(data, "glyphs")
	size := len(rec.FetchList(table, "text"))
	lists := map[string][]any{}
	for _, name := range columns {
		lists[name] = rec.FetchList(table, name)
		if len(lists[name]) != size {
			panic(own{rec.Failure{Message: "Glyph map columns differ in length"}})
		}
	}
	matrices := emptyObject(rec.Fetch(data, "matrices"))
	metrics := emptyObject(rec.Fetch(data, "metrics"))
	fonts := emptyObject(rec.Fetch(data, "fonts"))
	page := glyphPage{number: number, width: scaled(number64(rec.Fetch(data, "width"))),
		height: scaled(number64(rec.Fetch(data, "height")))}
	x, y := int64(0), int64(0)
	for index := range size {
		x += rec.AsInt(lists["dx"][index])
		y += rec.AsInt(lists["dy"][index])
		page.glyphs = append(page.glyphs, makeGlyph(lists, metrics, matrices, fonts, index, x, y))
	}
	return page
}

func number64(value any) float64 {
	number, ok := rec.Number(value)
	if !ok {
		rec.Fail("no implicit conversion of %T into Float", value)
	}
	return number
}

func makeGlyph(lists map[string][]any, metrics, matrices, fonts rec.Map, index int, x, y int64) *glyph {
	key := rec.ToS(lists["font"][index]) + ":" + rec.ToS(lists["char"][index])
	metric := rec.FetchList(metrics, key)
	w, h, d := rec.AsInt(rec.Index(metric, 0)), rec.AsInt(rec.Index(metric, 1)), rec.AsInt(rec.Index(metric, 2))
	if expansion := rec.AsInt(lists["ex"][index]); expansion != 0 {
		w = int64(math.Round(float64(w) * (1 + (float64(expansion) / 1_000_000.0))))
	}
	g := &glyph{index: index, text: rec.AsString(lists["text"][index]), x: x, y: y, w: w, h: h, d: d,
		tag: rec.AsInt(lists["tag"][index]), before: rec.AsInt(lists["before"][index]), after: rec.AsInt(lists["after"][index]),
		atag: rec.AsInt(lists["atag"][index]), formula: rec.AsInt(lists["formula"][index]), flg: rec.AsInt(lists["flags"][index])}
	if g.formula > 0 {
		// The classic TeX math fonts give wrong or empty text for Greek letters and big symbols.
		name, _ := rec.AsMap(fonts[rec.ToS(lists["font"][index])])["name"].(string)
		g.text = mathtokens.Decode(name, int(rec.AsInt(lists["char"][index])), g.text)
	}
	if value, ok := matrices[strconv.Itoa(index+1)]; ok {
		entries := rec.AsList(value)
		g.matrix = make([]float64, 4)
		for at := range g.matrix {
			g.matrix[at] = number64(rec.Index(entries, at))
		}
	}
	return g
}

// pageSizes is GlyphMap#page_sizes: page numbers and sizes in bp.
func (m *glyphMap) pageSizes() []any {
	result := make([]any, len(m.pages))
	for index, page := range m.pages {
		result[index] = rec.Map{"number": page.number, "width": page.width, "height": page.height}
	}
	return result
}

// words splits a page into words: a word ends after a boundary glyph, and also where a
// formula starts or ends ("Name$^{1}$" is text and a formula).
func (p glyphPage) words() []*word {
	result := []*word{}
	for _, run := range sliceWhen(p.glyphs, func(left, _ *glyph) bool { return left.flg&boundary != 0 }) {
		formula := int64(0)
		for _, g := range run {
			if g.formula > 0 {
				formula = g.formula
				break
			}
		}
		for _, piece := range sliceWhen(run, func(left, right *glyph) bool { return left.formula != right.formula }) {
			if blank(piece) {
				continue
			}
			attached := int64(0)
			if piece[0].formula == 0 {
				attached = formula
			}
			result = append(result, &word{id: fmt.Sprintf("g%d-%d", p.number, piece[0].index), page: p.number,
				glyphs: piece, attached: attached})
		}
	}
	return result
}

func blank(piece []*glyph) bool {
	for _, g := range piece {
		if rec.Strip(g.text) != "" {
			return false
		}
	}
	return true
}

// sliceWhen is Enumerable#slice_when.
func sliceWhen(glyphs []*glyph, split func(left, right *glyph) bool) [][]*glyph {
	if len(glyphs) == 0 {
		return nil
	}
	result := [][]*glyph{{glyphs[0]}}
	for index := 1; index < len(glyphs); index++ {
		if split(glyphs[index-1], glyphs[index]) {
			result = append(result, []*glyph{})
		}
		result[len(result)-1] = append(result[len(result)-1], glyphs[index])
	}
	return result
}

// synctexInputs is GlyphMap.inputs: SyncTeX input tags to file paths, in the order the
// tags first appear.
func synctexInputs(path string) *omap.Map[int64, string] {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		rec.Fail("Missing SyncTeX data")
	}
	file, err := os.Open(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	defer file.Close()
	var reader io.Reader = file
	if strings.HasSuffix(path, ".gz") {
		decompressed, gzipErr := gzip.NewReader(file)
		if gzipErr != nil {
			rec.Fail("%v", gzipErr)
		}
		defer decompressed.Close()
		reader = decompressed
	}
	inputs := omap.New[int64, string]()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1<<20), 1<<30)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		rest, ok := strings.CutPrefix(line, "Input:")
		if !ok {
			continue
		}
		tag, name, _ := strings.Cut(rest, ":")
		number, convErr := strconv.ParseInt(tag, 10, 64)
		if convErr != nil {
			rec.Fail("invalid value for Integer(): %s", rec.InspectString(tag))
		}
		inputs.Set(number, name)
	}
	if err := scanner.Err(); err != nil {
		rec.Fail("%v", err)
	}
	return inputs
}
