package synchronize

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/artifacts"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/mathsource"
	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/sourcemap"
	"github.com/sarielhp/relatex/internal/sourcetext"
)

// Schema is GlyphSourceMap::SCHEMA.
const Schema = "source-map-2.0"

// generated is GlyphSourceMap::GENERATED: files TeX writes and reads back in, with the calls
// that read them: their text is that call's (a generated bibliography is \bibliography's).
var generated = map[string][]string{".bbl": {"bibliography", "printbibliography"}, ".toc": {"tableofcontents"},
	".lof": {"listoffigures"}, ".lot": {"listoftables"}, ".ind": {"printindex"}}

// textSource is SourceMap::TEXT_SOURCE.
var textSource = regexp.MustCompile(`(?i)\.(?:tex|bib|sty|cls|ltx|cfg|def|lua)\z`)

// source is a paper file TeX typeset text from: its text, math spans and visible text.
type source struct {
	text    string
	spans   []*mathsource.Span
	visible *sourcetext.Visible
}

// Options are GlyphSourceMap.new's arguments.
type Options struct {
	GlyphMap, PDF, Synctex, Stream, SourceDirectory string
	SourceFiles                                     []any
	Prefixes                                        map[string]string
}

// context is a word's source file and the lines TeX recorded around it.
type context struct {
	file          string
	before, after int64
}

type capture struct {
	glyphs     *glyphMap
	pdf        string
	synctex    string
	stream     string
	root       string
	files      map[string]string
	resolver   *artifacts.Resolver
	inputs     *omap.Map[int64, string]
	texts      *omap.Map[string, *source]
	generated  map[int64][]string
	contexts   map[string]*context
	math       *mathPlacement
	generatedO map[int64]rec.Map
}

// Capture is GlyphSourceMap.new(...).capture.
func Capture(options Options) rec.Map {
	c := &capture{glyphs: readGlyphMap(options.GlyphMap), pdf: options.PDF, synctex: options.Synctex, stream: options.Stream,
		files: map[string]string{}, generatedO: map[int64]rec.Map{}}
	root, err := fspath.Real(options.SourceDirectory)
	if err != nil {
		rec.Fail("%v", err)
	}
	c.root = root
	for _, value := range options.SourceFiles {
		row := rec.AsMap(value)
		if truthy(rec.Fetch(row, "directory")) {
			continue
		}
		c.files[rec.FetchString(row, "path")] = rec.FetchString(row, "sha256")
	}
	c.resolver = artifacts.New(c.root, c.files, options.Prefixes)
	return c.run()
}

func truthy(value any) bool { return value != nil && value != false }

func (c *capture) run() rec.Map {
	raw := synctexInputs(c.synctex)
	c.inputs = omap.New[int64, string]()
	raw.Each(func(tag int64, path string) { c.inputs.Set(tag, c.sourceName(path)) })
	c.texts = c.readableSources()
	c.generated = map[int64][]string{}
	raw.Each(func(tag int64, path string) {
		if name, _ := c.inputs.Get(tag); name == "" {
			if calls, ok := generated[rec.Extname(path)]; ok {
				c.generated[tag] = calls
			}
		}
	})
	c.contexts = c.wordContexts()
	c.math = newMathPlacement(c.texts, c.shippedSources(), c.files)
	c.math.prepare(c.formulaSights())
	c.math.alignWords(c.glyphs.words)
	visible := omap.New[string, *sourcetext.Visible]()
	c.texts.Each(func(name string, entry *source) { visible.Set(name, entry.visible) })
	aligner := newAlignment(visible)
	placements := aligner.align(c.items())
	words := make([]any, len(c.glyphs.words))
	byID := map[string]rec.Map{}
	for index, w := range c.glyphs.words {
		row := c.mapWord(w, placements[w.id])
		words[index], byID[w.id] = row, row
	}
	for _, chain := range aligner.chains {
		for at := 0; at+1 < len(chain); at++ {
			byID[chain[at]]["continues"] = chain[at+1]
		}
	}
	result := rec.Map{"schema_version": Schema, "pdf_sha256": command.FileDigest(c.pdf),
		"synctex_sha256": command.FileDigest(c.synctex), "glyph_map_sha256": c.glyphs.sha256,
		"coordinates": Coordinates, "pages": c.glyphs.pageSizes(), "source_files": c.sourceFiles(),
		"words": words, "logical_words": c.logicalWords(byID, aligner.chains, placements), "sources": c.catalog(),
		"precision_policy": "TeX glyph positions and recorded lines; columns by aligning typeset and source text"}
	result["index"] = sourcemap.Index(result)
	return result
}

func (c *capture) sourceFiles() rec.Map {
	result := rec.Map{}
	for name, sha := range c.files {
		result[name] = sha
	}
	return result
}

// sourceName is the snapshot name of a file TeX read, "" when it is not one of the paper's.
func (c *capture) sourceName(path string) string {
	absolute := c.resolver.Translate(path, c.root)
	name, inside := fspath.Cut(c.root, absolute)
	if !inside {
		return ""
	}
	for strings.HasPrefix(name, "./") {
		name = strings.TrimPrefix(name, "./")
	}
	name = strings.ReplaceAll(name, "/./", "/")
	if _, ok := c.files[name]; ok {
		return name
	}
	return ""
}

// readableSources are the paper files TeX typeset text from, with their visible text, when
// their text is UTF-8.
func (c *capture) readableSources() *omap.Map[string, *source] {
	texts := omap.New[string, *source]()
	for _, name := range c.inputNames() {
		if texts.Has(name) {
			continue
		}
		data, err := os.ReadFile(c.resolver.Resolve(name))
		if err != nil {
			rec.Fail("%v", err)
		}
		if !utf8.Valid(data) {
			continue
		}
		text := string(data)
		spans := mathsource.NewSourceText(text, nil).Spans
		masks := make([]sourcetext.Span, len(spans))
		for index, span := range spans {
			masks[index] = sourcetext.Span{Start: span.Start, Stop: span.Stop}
		}
		texts.Set(name, &source{text: text, spans: spans, visible: sourcetext.New(text, masks)})
	}
	return texts
}

// inputNames are the paper files among the inputs, in order, once each.
func (c *capture) inputNames() []string {
	names := []string{}
	c.inputs.Each(func(_ int64, name string) {
		if name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	})
	return names
}

// shippedSources are the recorded locations of the shipped formulas, in reading order.
func (c *capture) shippedSources() []formulaSource {
	shipped := []int64{}
	for _, w := range c.glyphs.words {
		if formula := w.formula(); formula > 0 && !slices.Contains(shipped, formula) {
			shipped = append(shipped, formula)
		}
	}
	recorded := c.formulaSources()
	result := []formulaSource{}
	for _, formula := range shipped {
		if location, ok := recorded[formula]; ok {
			result = append(result, formulaSource{formula, location})
		}
	}
	return result
}

func (c *capture) formulaSources() map[int64]rec.Map {
	sources := map[int64]rec.Map{}
	if c.stream == "" {
		return sources
	}
	data, err := os.ReadFile(c.stream)
	if err != nil {
		return sources
	}
	reader := bufio.NewReader(bytes.NewReader(data))
	for {
		line, readErr := reader.ReadBytes('\n')
		if bytes.Contains(line, []byte(`"kind":"math"`)) && bytes.Contains(line, []byte(`"source"`)) {
			c.formulaSource(line, sources)
		}
		if errors.Is(readErr, io.EOF) {
			return sources
		}
	}
}

func (c *capture) formulaSource(line []byte, sources map[int64]rec.Map) {
	value, err := canonjson.Decode(bytes.NewReader(line))
	if err != nil {
		rec.Fail("%v", err)
	}
	record := rec.AsMap(value)
	location, ok := record["source"].(rec.Map)
	if record["kind"] == "math" && truthy(record["source"]) {
		if !ok {
			rec.Fail("expected an object, got %T", record["source"])
		}
		sources[rec.FetchInt(record, "formula")] = location
	}
}

// wordContexts are each word's source file and lines: its own when TeX's tag names a paper
// file, else those of the nearest words around it that have one (glue made inside a package,
// as by listings or amsthm, carries the package's tag).
func (c *capture) wordContexts() map[string]*context {
	words := c.glyphs.words
	own := make([]*context, len(words))
	for index, w := range words {
		own[index] = c.ownContext(w)
	}
	previous := make([]*context, len(words))
	var seen *context
	for index := range words {
		if own[index] != nil {
			seen = own[index]
		}
		previous[index] = seen
	}
	following := make([]*context, len(words))
	seen = nil
	for index := len(words) - 1; index >= 0; index-- {
		if own[index] != nil {
			seen = own[index]
		}
		following[index] = seen
	}
	result := map[string]*context{}
	for index, w := range words {
		result[w.id] = own[index]
		if own[index] == nil {
			result[w.id] = inherit(previous[index], following[index])
		}
	}
	return result
}

// ownContext is a word's own file and lines. When the glue after it was made in another
// paper file than the glue before it (a heading that \begin{theorem} typesets after an
// \input file ended), the word belongs to the file after it, at that line.
func (c *capture) ownContext(w *word) *context {
	beforeFile, _ := c.inputs.Get(w.tag())
	afterFile, _ := c.inputs.Get(w.afterTag())
	if afterFile != "" && afterFile != beforeFile {
		return &context{afterFile, w.after(), w.after()}
	}
	if beforeFile == "" {
		return nil
	}
	return &context{beforeFile, w.before(), w.after()}
}

// inherit: between words of two different files, the word goes with the following one:
// package text such as a theorem or proof heading leads into what comes after it.
func inherit(before, after *context) *context {
	if before == nil {
		return after
	}
	if after == nil {
		return before
	}
	if after.file != before.file {
		return after
	}
	return &context{before.file, before.before, max(after.before, after.after)}
}

func (c *capture) items() []*item {
	result := []*item{}
	for _, w := range c.glyphs.words {
		where := c.contexts[w.id]
		if where == nil || !c.texts.Has(where.file) || w.formula() != 0 {
			continue
		}
		if _, isGenerated := c.generated[w.tag()]; isGenerated {
			continue
		}
		chars := []string{}
		for _, g := range w.glyphs {
			for _, char := range strings.NewReplacer(" ", "", "\t", "").Replace(sourcetext.Fold(g.text)) {
				chars = append(chars, string(char))
			}
		}
		last := w.glyphs[len(w.glyphs)-1].text
		result = append(result, &item{key: w.id, file: where.file, before: where.before, after: where.after, chars: chars,
			lineEnd: w.lineEnd(), hyphen: slices.Contains(hyphens, last)})
	}
	return result
}

func (c *capture) mapWord(w *word, found *placement) rec.Map {
	where := c.contexts[w.id]
	if where == nil {
		where = &context{}
	}
	origin := c.originFor(w, found, where)
	precision := "none"
	origins := []any{}
	if origin != nil {
		precision = rec.FetchString(origin, "precision")
		origins = append(origins, origin)
	}
	text := w.text()
	row := rec.Map{"id": w.id, "page": w.page, "text": text, "normalized_text": norm.NFKC.String(text),
		"boxes": []any{w.box()}, "baseline": w.baseline(), "lines": []any{w.before(), w.after()},
		"source_precision": precision, "origins": origins}
	if w.unknown() {
		row["transform_unknown"] = true
	}
	if letters := c.letters(w); letters != nil {
		row["letters"] = letters
	}
	return row
}

// letters are the glyphs of a math word, each with its own box and the source range of the
// tokens that typeset it, when the word's origin is its tokens and they spell it glyph by glyph.
func (c *capture) letters(w *word) []any {
	origins := c.math.letterOrigins(w.id)
	if origins == nil {
		return nil
	}
	letters := make([]any, len(origins))
	for index, origin := range origins {
		g := w.glyphs[index]
		box := g.box()
		letters[index] = rec.Map{"text": g.text, "boxes": []any{[]any{box[0], box[1], box[2], box[3]}}, "origins": []any{origin}}
	}
	return letters
}

// originFor is, in order: the word's own formula; a generated file's call; an exact
// placement; the formula that text cut from it belongs to; any other placement; the word's
// lines.
func (c *capture) originFor(w *word, found *placement, where *context) rec.Map {
	if formula := w.formula(); formula > 0 {
		if tokens := c.math.wordOrigin(w.id); tokens != nil {
			return tokens
		}
		if math := c.math.origin(formula, where.file, where.before, where.after); math != nil {
			return math
		}
	}
	if _, isGenerated := c.generated[w.tag()]; isGenerated {
		return c.generatedOrigin(w.tag())
	}
	if found != nil && found.precision == "exact" {
		return c.originOf(found)
	}
	if w.attached > 0 {
		if attached := c.math.origin(w.attached, where.file, where.before, where.after); attached != nil {
			return attached
		}
	}
	if found != nil {
		return c.originOf(found)
	}
	if _, known := c.files[where.file]; where.file != "" && known {
		return lineOrigin(c.files, where.file, where.before, where.after)
	}
	return nil
}

// generatedOrigin is the call that read a generated file in, found in the paper's readable
// sources.
func (c *capture) generatedOrigin(tag int64) rec.Map {
	if origin, ok := c.generatedO[tag]; ok {
		return origin
	}
	names := c.generated[tag]
	var origin rec.Map
	for _, name := range c.texts.Keys() {
		entry, _ := c.texts.Get(name)
		index := slices.IndexFunc(entry.visible.Calls, func(call sourcetext.Call) bool { return slices.Contains(names, call.Name) })
		if index >= 0 {
			call := entry.visible.Calls[index]
			origin = c.spanOrigin(name, call.Start, call.Stop, "macro")
			break
		}
	}
	c.generatedO[tag] = origin
	return origin
}

func (c *capture) originOf(found *placement) rec.Map {
	if found.precision == "line" {
		return lineOrigin(c.files, found.file, found.lines[0], found.lines[1])
	}
	return c.spanOrigin(found.file, found.start, found.stop, found.precision)
}

func lineOrigin(files map[string]string, file string, before, after int64) rec.Map {
	sha, ok := files[file]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + rec.InspectString(file), Lookup: true})
	}
	line := max(before, 1)
	origin := rec.Map{"file": file, "line": line, "precision": "line", "source_sha256": sha}
	if after > line {
		origin["end_line"] = after
	}
	return origin
}

func (c *capture) spanOrigin(file string, start, stop int, precision string) rec.Map {
	entry, _ := c.texts.Get(file)
	return spanOrigin(entry, c.files, file, entry.visible.LineOf(start), entry.visible.LineOf(stop-1), start, stop, precision)
}

// spanOrigin is a span's origin: its lines, line-relative characters and their byte offsets.
func spanOrigin(entry *source, files map[string]string, file string, line, endLine, start, stop int, precision string) rec.Map {
	visible := entry.visible
	first, ok := visible.LineStart(line)
	last, endOK := visible.LineStart(endLine)
	if !ok || !endOK {
		rec.Fail("index %d outside of array bounds", max(line, endLine)-1)
	}
	characters := []any{int64(start - first), int64(stop - last)}
	origin := rec.Map{"file": file, "line": int64(line), "precision": precision, "source_sha256": files[file],
		"characters": characters}
	if endLine != line {
		origin["end_line"] = int64(endLine)
	}
	firstBytes, lastBytes, _ := visible.ByteOffsets(line, endLine, start-first, stop-last)
	origin["byte_offsets"] = []any{int64(firstBytes), int64(lastBytes)}
	return origin
}

// logicalWords: a word hyphenated across line or page breaks, selectable as one word: its
// fragments and one exact origin spanning the whole source word.
func (c *capture) logicalWords(byID map[string]rec.Map, chains [][]string, placements map[string]*placement) []any {
	result := []any{}
	for _, keys := range chains {
		spans := make([]*placement, len(keys))
		valid := true
		for index, key := range keys {
			spans[index] = placements[key]
			valid = valid && spans[index] != nil && spans[index].precision == "exact" && spans[index].file == spans[0].file
		}
		if !valid {
			continue
		}
		chain := make([]rec.Map, len(keys))
		for index, key := range keys {
			chain[index] = byID[key]
		}
		origin := c.spanOrigin(spans[0].file, spans[0].start, spans[len(spans)-1].stop, "exact")
		result = append(result, sourcemap.Record(chain, []any{origin}))
	}
	return result
}

// catalog is SourceMap.catalog: the source files the map refers to, with their text when it
// is valid UTF-8.
func (c *capture) catalog() rec.Map {
	names := c.texts.Keys()
	for name := range c.files {
		if textSource.MatchString(name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	names = slices.Compact(names)
	result := rec.Map{}
	for _, name := range names {
		data, err := os.ReadFile(c.resolver.Resolve(name))
		if err != nil {
			rec.Fail("%v", err)
		}
		if len(data) > 16*1024*1024 {
			rec.Fail("Source catalog text exceeds size limit")
		}
		record := c.resolver.Source(name)
		record["role"], record["encoding"] = "original", "unavailable"
		if utf8.Valid(data) {
			record["encoding"], record["text"] = "UTF-8", string(data)
		}
		result[name] = record
	}
	return result
}

// formulaSights are the formulas the words belong to, in word order, with their lines.
func (c *capture) formulaSights() []formulaSight {
	sights := []formulaSight{}
	for _, w := range c.glyphs.words {
		where := c.contexts[w.id]
		if where == nil {
			where = &context{}
		}
		for _, formula := range []int64{w.formula(), w.attached} {
			if formula > 0 {
				sights = append(sights, formulaSight{formula, where.file, where.before, where.after})
			}
		}
	}
	return sights
}
