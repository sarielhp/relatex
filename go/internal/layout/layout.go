// Package layout is the Go port of Relatex::PDFLayout#capture, the 04-layout stage: mutool's
// structured text (metric and ink), paint trace and page SVGs of the ordinary PDF, and from
// them layout.json, each page's glyphs bound to the paint that drew them and to their SVG
// outlines, and its words. Pages are bound concurrently; the files are those of the Ruby
// reference.
package layout

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/svgoutline"
)

const (
	// clip=no: the page SVG keeps every glyph use, clipped or not, and MuPDF judges some
	// visible glyphs entirely clipped (a raised footnote mark in an ocgx2 colour link's clip).
	metricOptions = "preserve-ligatures,preserve-whitespace,accurate-bboxes=no,clip=no"
	// MuPDF 1.28.2's side-bearing option corrupts vertical coordinates. Its ordinary
	// outline-height quad is retained with its advance-width semantics.
	inkOptions = "preserve-ligatures,preserve-whitespace,accurate-bboxes=yes,accurate-side-bearings=no,clip=no"
	// inkExtent is PDFGlyphOutlines::EXTENT.
	inkExtent = "Unclipped glyph fill outlines from SVG curves; excludes stroke expansion, clipping, hinting and compositing"
	// maxBytes is DocumentBundle::MAX_BYTES.
	maxBytes = 512 * 1024 * 1024
)

// capture is one PDFLayout#capture.
type capture struct {
	pdf, tool, directory string
	mutex                sync.Mutex
	recovered            []string // the content errors MuPDF drew the pages past (command.Recovered)
}

// Capture is PDFLayout.new(pdf).capture(output: output): it writes the capture into output
// and returns layout.json's data.
func Capture(pdf, output string) rec.Map {
	var pages []any
	data := CaptureTo(pdf, output, func(int) (func(rec.Map) any, func(any)) {
		return func(page rec.Map) any { return page }, func(page any) { pages = append(pages, page) }
	})
	data["pages"] = pages
	if err := canonjson.WriteFile(filepath.Join(output, "layout.json"), data); err != nil {
		rec.Fail("%v", err)
	}
	return data
}

// CaptureTo is Capture without holding the document's pages: the pages are bound in order, a few at a
// time. begin is called with the page count first and returns two functions: prepare, called for pages
// concurrently, and deliver, called in page order with what prepare returned. The returned data is layout.json's without its pages, and nothing is
// written as layout.json (a book's layout is gigabytes).
func CaptureTo(pdf, output string, begin func(count int) (func(rec.Map) any, func(any))) rec.Map {
	resolved, err := fspath.Real(pdf)
	if err == nil {
		resolved, err = filepath.Abs(resolved)
	}
	if err != nil {
		rec.Fail("%v", err)
	}
	c := &capture{pdf: resolved, tool: command.Resolve("mutool")}
	if c.tool == "" {
		rec.Fail("Layout capture requires mutool")
	}
	return c.run(output, begin)
}

// traces are the three XML traces of a capture, read page by page (a large book) or whole.
type traces struct {
	metric, ink, paint pageSource
}

// pageSource hands out the pages of one trace, in any order.
type pageSource interface {
	count() int
	page(index int) *nativetrace.Node
}

// wholePages are the pages of a trace parsed at once.
type wholePages []*nativetrace.Node

func (w wholePages) count() int { return len(w) }
func (w wholePages) page(index int) *nativetrace.Node {
	if index >= len(w) {
		panic(rec.Failure{Message: fmt.Sprintf("index %d outside of array bounds: %d...%d", index, -len(w), len(w)), Lookup: true})
	}
	return w[index]
}

// splitPages are the pages of a large trace, each parsed when it is bound and released after.
type splitPages struct{ pages *nativetrace.Pages }

func (s splitPages) count() int                       { return s.pages.Len() }
func (s splitPages) page(index int) *nativetrace.Node { return s.pages.Page(index) }

func (c *capture) run(output string, begin func(count int) (func(rec.Map) any, func(any))) rec.Map {
	directory, err := filepath.Abs(output)
	if err != nil {
		rec.Fail("%v", err)
	}
	if _, err := os.Lstat(directory); err == nil {
		rec.Fail("Layout output already exists")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		rec.Fail("%v", err)
	}
	c.directory = directory
	pdfHash, toolHash := command.FileDigest(c.pdf), command.FileDigest(c.tool)
	c.extract()
	command.Trace("layout: extracted")
	source := c.readTraces()
	command.Trace("layout: parsed")
	if source.paint.count() != source.ink.count() {
		rec.Fail("Layout paint/ink page coverage differs")
	}
	if source.metric.count() == 0 || source.metric.count() != source.ink.count() {
		rec.Fail("Layout has no pages or mismatched ink coverage")
	}
	prepare, deliver := begin(source.metric.count())
	c.bindPages(source, prepare, deliver)
	command.Trace("layout: pages bound")
	if command.FileDigest(c.pdf) != pdfHash || command.FileDigest(c.tool) != toolHash {
		rec.Fail("Layout source/helper changed during capture")
	}
	data := rec.Map{"schema_version": "layout-1.2", "pdf_sha256": pdfHash, "helper_sha256": toolHash,
		"measurement": rec.Map{"metric_options": metricOptions, "ink_options": inkOptions, "ink_extent": inkExtent,
			"outline_contract": "svg-glyph-outlines-1", "paint_sha256": command.FileDigest(filepath.Join(directory, "paint.xml"))},
		"coordinates":    rec.Map{"unit": "bp", "origin": "top-left", "source": "MuPDF page coordinates"},
		"reconstruction": "Vector page graphics with outlined text"}
	if len(c.recovered) > 0 {
		// MuPDF left out what it could not read (a shading, papers3/0786) from every extraction alike:
		// the glyphs bind as ever, and the page graphics lack it, which the conversion reports.
		slices.Sort(c.recovered)
		errors := make([]any, len(c.recovered))
		for index, message := range c.recovered {
			errors[index] = message
		}
		rec.FetchMap(data, "measurement")["recovered_errors"] = errors
	}
	return data
}

type extraction struct {
	name, format string
	options      []string
	destination  string
}

// extract runs mutool's four extractions concurrently (they read the same PDF and write
// their own files); a failure is reported in Ruby's order.
func (c *capture) extract() {
	// -KK: the structured text reads the text alone. MuPDF's text device otherwise weighs every stroked
	// path against the text near it (underlines), which takes minutes on a page of a quarter million
	// strokes; only the underline bit of a character's flags differs, and nothing here reads it.
	runs := []extraction{{"metric", "stext", []string{"-KK", "-O", metricOptions}, "metric.xml"},
		{"ink", "stext", []string{"-KK", "-O", inkOptions}, "ink.xml"}, {"paint", "trace", nil, "paint.xml"},
		{"vectors", "svg", []string{"-O", "text=path"}, "page-%d.svg"}}
	failures := make([]any, len(runs))
	var group sync.WaitGroup
	for index, run := range runs {
		group.Add(1)
		go func() {
			defer group.Done()
			defer keep(&failures[index])
			c.extraction(run)
		}()
	}
	group.Wait()
	for index, failure := range failures {
		if failure != nil {
			// Ruby stops at the first failing extraction: the later ones leave no files.
			for _, later := range runs[index+1:] {
				c.discard(later)
			}
			panic(failure)
		}
	}
}

// discard removes an extraction's files: its output, its log and its receipt.
func (c *capture) discard(run extraction) {
	pattern := strings.ReplaceAll(run.destination, "%d", "*")
	paths, _ := filepath.Glob(filepath.Join(c.directory, pattern))
	paths = append(paths, filepath.Join(c.directory, run.name+".log"), filepath.Join(c.directory, run.name+"-stage.json"))
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			rec.Fail("%v", err)
		}
	}
}

// keep stores a goroutine's panic for the caller to raise in order.
func keep(slot *any) {
	if recovered := recover(); recovered != nil {
		*slot = recovered
	}
}

func (c *capture) extraction(run extraction) {
	drawArgs := append([]string{"-F", run.format}, run.options...)
	drawArgs = append(drawArgs, "-o", filepath.Join(c.directory, run.destination), filepath.Base(c.pdf))
	arguments := command.MutoolDrawCommand(c.tool, drawArgs...)
	stage := command.Run(arguments, filepath.Dir(c.pdf), filepath.Join(c.directory, run.name+".log"), 120*time.Second, nil)
	command.WriteReceipt(filepath.Join(c.directory, run.name+"-stage.json"), stage)
	if rec.Equal(stage["exit"], int64(0)) && stage["timeout"] != true {
		return
	}
	log, _ := os.ReadFile(filepath.Join(c.directory, run.name+".log"))
	errors, recovered := command.Recovered(log)
	if !rec.Equal(stage["exit"], int64(1)) || stage["timeout"] == true || !recovered {
		rec.Fail("Layout %s extraction failed", run.name)
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, message := range errors {
		if !slices.Contains(c.recovered, message) {
			c.recovered = append(c.recovered, message)
		}
	}
}

// readFile is a trace's text, checked as a trace may be.
func (c *capture) readFile(name string) []byte {
	path := filepath.Join(c.directory, name)
	info, err := os.Stat(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	if info.Size() > maxBytes {
		rec.Fail("Layout XML exceeds size limit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	if nativetrace.Declared(data) {
		rec.Fail("Layout XML declarations/entities are forbidden")
	}
	return data
}

// readTraces reads the three traces: a large one stays text, cut at its pages, and a page is parsed
// when it is bound; the others are parsed side by side (the first failing one in order is raised).
func (c *capture) readTraces() traces {
	names := []string{"metric.xml", "ink.xml", "paint.xml"}
	texts := make([][]byte, len(names))
	for index, name := range names {
		texts[index] = c.readFile(name)
	}
	split := make([]*nativetrace.Pages, len(names))
	for index, text := range texts {
		if split[index] = nativetrace.SplitPages(text); split[index] == nil {
			return c.parseTraces(texts)
		}
	}
	return traces{splitPages{split[0]}, splitPages{split[1]}, splitPages{split[2]}}
}

func (c *capture) parseTraces(texts [][]byte) traces {
	parsed := make([]wholePages, len(texts))
	failures := make([]any, len(texts))
	var group sync.WaitGroup
	for index, text := range texts {
		group.Add(1)
		go func() {
			defer group.Done()
			defer keep(&failures[index])
			parsed[index] = nativetrace.Parse(text).Descendants("page")
		}()
	}
	group.Wait()
	for _, failure := range failures {
		if failure != nil {
			panic(failure)
		}
	}
	return traces{parsed[0], parsed[1], parsed[2]}
}

// bindPages binds every page concurrently, a bounded window ahead of the one being handed over, and
// hands the pages to deliver in order; the first failing page in page order fails the capture, as
// the sequential reference does.
func (c *capture) bindPages(source traces, prepare func(rec.Map) any, deliver func(any)) {
	count := source.metric.count()
	type bound struct {
		page    any
		failure any
	}
	window := make(chan struct{}, 2*runtime.GOMAXPROCS(0))
	results := make([]chan bound, count)
	for index := range results {
		results[index] = make(chan bound, 1)
	}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for index := range count {
			select {
			case window <- struct{}{}:
			case <-stop:
				return
			}
			go func() {
				var result bound
				defer func() { results[index] <- result }()
				defer keep(&result.failure)
				result.page = prepare(c.pageData(source.metric.page(index), source.ink.page(index), source.paint.page(index), int64(index+1)))
			}()
		}
	}()
	for index := range count {
		result := <-results[index]
		if result.failure != nil {
			panic(result.failure)
		}
		deliver(result.page)
		<-window
	}
}

func (c *capture) pageData(page, inkPage, paintPage *nativetrace.Node, number int64) rec.Map {
	glyphs := glyphsFor(page, number)
	ink := glyphsFor(inkPage, number)
	if len(glyphs) != len(ink) {
		rec.Fail("Metric/ink glyph coverage differs")
	}
	for index, glyph := range glyphs {
		for _, key := range []string{"text", "baseline", "font", "size"} {
			if !rec.Equal(glyph[key], ink[index][key]) {
				rec.Fail("Metric/ink glyph identities differ")
			}
		}
		glyph["ink_quad"] = ink[index]["metric_quad"]
	}
	paint := newUnicodeGlyphs(paintPage)
	glyphs, flags := paint.respell(glyphs, charFlags(page))
	recoverOverstruck(glyphs, flags, paint.painted)
	glyphs = recoverStackedMarks(glyphs, flags, paint.painted, number)
	glyphs = paint.bind(glyphs)
	svg, err := os.ReadFile(filepath.Join(c.directory, fmt.Sprintf("page-%d.svg", number)))
	if err != nil {
		rec.Fail("%v", err)
	}
	mediamodel.ValidateSVG(svg)
	glyphs = svgoutline.Bind(svg, paint.painted, glyphs)
	width := nativetrace.Float(attribute(page, "width"))
	height := nativetrace.Float(attribute(page, "height"))
	list := make([]any, len(glyphs))
	for index, glyph := range glyphs {
		list[index] = glyph
	}
	return rec.Map{"number": number, "width": width, "height": height, "glyphs": list, "words": wordsFor(glyphs, number),
		"svg": base64.StdEncoding.EncodeToString(svg), "svg_sha256": digest(svg)}
}

func attribute(element *nativetrace.Node, name string) string {
	value, ok := element.Attr(name)
	if !ok {
		rec.Fail("Layout XML element lacks %s", name)
	}
	return value
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
