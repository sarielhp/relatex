package synctexout

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/rec"
)

// spPerBP is WordSynctex::SP_PER_BP, divided at run time as Ruby does.
var spPerBP = spPerTeXPoint / (bigPoints / texPoints)

var spPerTeXPoint, bigPoints, texPoints = 65_536.0, 72.0, 72.27

// placed is WordSynctex::PLACED: source map 2 precisions whose first column is placed.
var placed = []any{"exact", "macro", "math"}

// wordSynctex is WordSynctex: measured PDF word boxes with honest native source hints.
// Inferred character spans remain in JSON; they are never relabelled as native columns.
type wordSynctex struct {
	pages                      []any
	words                      []rec.Map
	tags                       []string
	tagOf                      map[string]int
	mapped, unmapped, boxCount int64
	// inputName is the name an Input record gives a source file.
	inputName func(name string) string
}

func newWordSynctex(layout, sourceMap rec.Map) *wordSynctex {
	pages, ok := sourceMap["pages"].([]any)
	if !ok || pages == nil {
		if layout != nil {
			pages = rec.FetchList(layout, "pages")
		}
	}
	if pages == nil {
		rec.Fail("Word SyncTeX has no pages")
	}
	if layout != nil && !rec.Equal(rec.Fetch(layout, "pdf_sha256"), rec.Fetch(sourceMap, "pdf_sha256")) {
		rec.Fail("Word SyncTeX PDF bindings differ")
	}
	w := &wordSynctex{pages: pages, tagOf: map[string]int{}, inputName: func(name string) string { return "sources/" + name }}
	referenced := map[string]bool{}
	for _, value := range rec.FetchList(sourceMap, "words") {
		word := rec.AsMap(value)
		w.words = append(w.words, word)
		for _, origin := range rec.FetchList(word, "origins") {
			referenced[rec.FetchString(rec.AsMap(origin), "file")] = true
		}
	}
	for name := range rec.FetchMap(sourceMap, "source_files") {
		if referenced[name] {
			w.tags = append(w.tags, name)
		}
	}
	slices.Sort(w.tags)
	for index, name := range w.tags {
		w.tagOf[name] = index + 1
	}
	return w
}

func (w *wordSynctex) generate() *SynctexFile {
	records := [][]byte{[]byte("SyncTeX Version:1\n")}
	for _, name := range w.tags {
		records = append(records, fmt.Appendf(nil, "Input:%d:%s\n", w.tagOf[name], w.inputName(name)))
	}
	for _, line := range []string{"Output:pdf\n", "Magnification:1000\n", "Unit:1\n", "X Offset:0\n", "Y Offset:0\n", "Content:\n"} {
		records = append(records, []byte(line))
	}
	count := 0
	for _, page := range w.pages {
		pageRecords := w.pageRecords(rec.AsMap(page))
		records = append(records, pageRecords...)
		for _, line := range pageRecords {
			if !strings.HasPrefix(string(line), "x") && !strings.HasPrefix(string(line), "Input:") {
				count++
			}
		}
	}
	if w.mapped == 0 {
		rec.Fail("Word SyncTeX has no source-linked boxes")
	}
	for _, line := range []string{"!0\n", "Postamble:\n", fmt.Sprintf("Count:%d\n", count+1), "!0\n", "Post scriptum:\n"} {
		records = append(records, []byte(line))
	}
	return NewSynctexFile(synctexAnchors(records))
}

// coordinate is (Float(value) * SP_PER_BP).round.
func coordinate(value any) int64 {
	number, ok := rec.Number(value)
	if !ok {
		rec.Fail("can't convert %T into Float", value)
	}
	return int64(math.Round(number * spPerBP))
}

func (w *wordSynctex) pageRecords(page rec.Map) [][]byte {
	if len(w.tags) == 0 {
		rec.Fail("Word SyncTeX has no source file catalog")
	}
	tag := w.tagOf[w.tags[0]]
	height := coordinate(rec.Fetch(page, "height"))
	number := toS(rec.Fetch(page, "number"))
	records := [][]byte{[]byte("!0\n"), []byte("{" + number + "\n"),
		fmt.Appendf(nil, "[%d,1:0,%d:%d,%d,0\n", tag, height, coordinate(rec.Fetch(page, "width")), height)}
	for _, word := range w.words {
		if rec.Equal(rec.Fetch(word, "page"), rec.Fetch(page, "number")) {
			records = append(records, w.wordRecords(word)...)
		}
	}
	return append(records, []byte("]\n"), []byte("!0\n"), []byte("}"+number+"\n"))
}

func (w *wordSynctex) wordRecords(word rec.Map) [][]byte {
	letters := lettersOf(word)
	if len(letters) == 0 {
		return w.boxRecords(word, word)
	}
	records := [][]byte{}
	for _, value := range letters {
		letter := rec.AsMap(value)
		records = append(records, w.boxRecords(word, letter)...)
	}
	if len(records) == 0 {
		w.unmapped++
	} else {
		w.mapped++
	}
	return records
}

// boxRecords are the records of one word, or of one letter of it: the letter's own boxes and
// origins on the word's baseline.
func (w *wordSynctex) boxRecords(word, shape rec.Map) [][]byte {
	origins := []rec.Map{}
	for _, value := range rec.FetchList(shape, "origins") {
		origin := rec.AsMap(value)
		if _, known := w.tagOf[rec.FetchString(origin, "file")]; known && positiveNumber(rec.Fetch(origin, "line")) {
			origins = append(origins, origin)
		}
	}
	if len(origins) == 0 {
		if len(lettersOf(word)) == 0 {
			w.unmapped++
		}
		return nil
	}
	if len(lettersOf(word)) == 0 {
		w.mapped++
	}
	records := [][]byte{}
	for _, origin := range origins {
		link := w.link(origin)
		for _, bounds := range rec.FetchList(shape, "boxes") {
			box := geometry.Box(rec.AsList(bounds))
			x0, y0, x1, y1 := box[0], box[1], box[2], box[3]
			baseline := clamp(rec.FetchList(word, "baseline"), y0, y1)
			w.boxCount++
			// Conventional inverse lookup indexes full horizontal boxes. A void h record alone
			// can resolve to the enclosing page's source instead.
			records = append(records, fmt.Appendf(nil, "(%s:%d,%d:%d,%d,%d\n", link, coordinate(x0), coordinate(baseline),
				coordinate(x1-x0), coordinate(baseline-y0), coordinate(y1-baseline)))
			if strings.Count(link, ",") == 2 {
				records = append(records, columnMarkers(link, coordinate(x0), coordinate(x0)+coordinate(x1-x0), coordinate(baseline))...)
			}
			records = append(records, []byte(")\n"))
		}
	}
	return records
}

// columnMarkers are the math records inside a word box that carry its source column. A reader's
// inverse lookup answers with the node nearest the point, and a box boundary has no column but a
// math record has; markers one unit inside each edge are nearer than a boundary to every point of the
// box, so any click in the word finds the column.
func columnMarkers(link string, left, right, baseline int64) [][]byte {
	if right-left <= 4 {
		return nil
	}
	return [][]byte{fmt.Appendf(nil, "$%s:%d,%d\n", link, left+1, baseline), fmt.Appendf(nil, "$%s:%d,%d\n", link, right-1, baseline)}
}

func positiveNumber(value any) bool {
	number, ok := rec.Number(value)
	if !ok {
		rec.Fail("undefined method 'positive?' for %T", value)
	}
	return number > 0
}

// clamp is baseline.last.clamp(y0, y1), as a float.
func clamp(baseline []any, low, high float64) float64 {
	value, ok := rec.Number(baseline[len(baseline)-1])
	if !ok {
		rec.Fail("comparison failed")
	}
	return math.Min(math.Max(value, low), high)
}

func (w *wordSynctex) link(origin rec.Map) string {
	tag := w.tagOf[rec.FetchString(origin, "file")]
	hint, _ := origin["native_hint"].(rec.Map)
	column := hint["column"]
	if column == nil {
		column = origin["native_column"]
		if column == nil {
			column = int64(0)
		}
	}
	columnNumber, _ := rec.Number(column)
	if slices.Contains(placed, origin["precision"]) && columnNumber == 0 {
		first, _ := rec.Number(rec.Index(rec.FetchList(origin, "characters"), 0))
		column = int64(first) + 1
		columnNumber = first + 1
	}
	line := hint["line"]
	if line == nil {
		line = rec.Fetch(origin, "line")
	}
	link := fmt.Sprintf("%d,%s", tag, toS(line))
	if columnNumber > 0 {
		link += "," + toS(column)
	}
	return link
}

// lettersOf are a word's per-glyph rows; only math words matched token by token have them.
func lettersOf(word rec.Map) []any {
	letters, _ := word["letters"].([]any)
	return letters
}
