// Package fontmodel is the document model's fonts (decision 102): the fonts.items document.json
// holds (SourceFonts.validate!, the schema and the linked-file names), the styled spans that need
// a font, the font of a style, and the linked font files written beside a page. The core
// (sourcefonts) captures them from the PDF; every reader of the model reads them here.
package fontmodel

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/frontmatter"
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/schema"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// Root is the repository root, where schemas/source-fonts.schema.json is read; the export writer
// sets it.
var Root string

var linkedName = regexp.MustCompile(`\A[0-9a-f]{16}-[A-Za-z0-9_.-]+\z`)

var (
	loaded   = map[string]rec.Map{}
	loadLock sync.Mutex
)

// contract reads a JSON file of the repository once per path.
func contract(parts ...string) rec.Map { return Load("fontmodel", Root, parts...) }

// Load reads a JSON file under root once per path; owner names the reader in a failure (the
// file is read as Ruby reads it at load time). sourcefonts reads its profile with it.
func Load(owner, root string, parts ...string) rec.Map {
	path := filepath.Join(append([]string{root}, parts...)...)
	loadLock.Lock()
	defer loadLock.Unlock()
	if value, ok := loaded[path]; ok {
		return value
	}
	file, err := os.Open(path)
	if err != nil {
		panic(fmt.Sprintf("%s: %v", owner, err))
	}
	defer file.Close()
	value, err := canonjson.Decode(file)
	if err != nil {
		panic(fmt.Sprintf("%s: %v", owner, err))
	}
	loaded[path] = rec.AsMap(value)
	return loaded[path]
}

// Spans is SourceFonts.spans(document): the styled spans of the reading, note labels and
// front-matter decorations.
func Spans(document rec.Map) []rec.Map { return spans(document, flat) }

// PresentationSpans is SourceFonts.spans(document, presentation: true): rich cells in the
// order the reading displays them.
func PresentationSpans(document rec.Map) []rec.Map {
	return spans(document, func(content []any) []rec.Map { return inline.Flat(content, true) })
}

func spans(document rec.Map, walk func([]any) []rec.Map) []rec.Map {
	candidates := []rec.Map{}
	nodes := rec.FetchMap(document, "nodes")
	for _, id := range rec.FetchList(document, "block_order") {
		candidates = append(candidates, NoteLabelSpans(document, id)...)
		candidates = append(candidates, walk(rec.FetchList(rec.FetchMap(nodes, rec.AsString(id)), "content"))...)
	}
	for _, row := range frontmatter.Decorations(document) {
		runs, _ := rec.AsMap(row)["runs"].([]any)
		for _, run := range runs {
			candidates = append(candidates, rec.AsMap(run))
		}
	}
	result := []rec.Map{}
	for _, span := range candidates {
		if _, ok := span["style"]; ok {
			result = append(result, span)
		}
	}
	return result
}

// NoteLabelSpans is SourceFonts.note_label_spans: the styled labels of the notes whose
// first label sits in block.
func NoteLabelSpans(document rec.Map, block any) []rec.Map {
	result := []rec.Map{}
	order, _ := document["note_order"].([]any)
	for _, id := range order {
		labels := rec.FetchList(rec.FetchMap(rec.FetchMap(document, "nodes"), rec.AsString(id)), "labels")
		if len(labels) == 0 || !rec.Equal(rec.Fetch(rec.AsMap(labels[0]), "block"), block) {
			continue
		}
		for _, value := range labels {
			label := rec.AsMap(value)
			if _, ok := label["style"]; !ok {
				continue
			}
			span := rec.Map{"style": label["style"]}
			if text, has := label["text"]; has {
				span["text"] = text
			}
			result = append(result, span)
		}
	}
	return result
}

// flat is PaperInline.flat(content): a table grid's cells in row and column order.
func flat(content []any) []rec.Map {
	result := []rec.Map{}
	for _, value := range content {
		span := rec.AsMap(value)
		if span["type"] != "table_grid" {
			result = append(result, span)
			continue
		}
		for _, cell := range tableCells(rec.FetchMap(span, "grid")) {
			inner, _ := cell["content"].([]any)
			result = append(result, flat(inner)...)
		}
	}
	return result
}

// tableCells is RichTable.new(grid).rows.flatten; a legacy grid's cells hold their runs.
func tableCells(grid rec.Map) []rec.Map {
	cells := []rec.Map{}
	if grid["schema_version"] != "table-2" {
		for _, row := range rec.FetchList(grid, "rows") {
			for _, value := range rec.AsList(row) {
				cell := rec.AsMap(value)
				content := tablemodel.LegacyCellContent(cell)
				cells = append(cells, rec.Map{"content": content})
			}
		}
		return cells
	}
	for _, value := range rec.FetchList(grid, "cells") {
		cells = append(cells, rec.AsMap(value))
	}
	slices.SortStableFunc(cells, func(a, b rec.Map) int {
		for _, key := range []string{"row", "column"} {
			x, _ := rec.Number(a[key])
			y, _ := rec.Number(b[key])
			if x != y {
				if x < y {
					return -1
				}
				return 1
			}
		}
		return 0
	})
	return cells
}

func items(document rec.Map) []any {
	fonts, _ := document["fonts"].(rec.Map)
	list, _ := fonts["items"].([]any)
	return list
}

// ForStyle is SourceFonts.for_style: the font item of a style, or nil.
func ForStyle(document, style rec.Map) rec.Map {
	name := rec.Fetch(style, "font_name")
	for _, value := range items(document) {
		font := rec.AsMap(value)
		if rec.Equal(rec.Fetch(font, "font_name"), name) {
			return font
		}
	}
	return nil
}

// Validate is SourceFonts.validate!(document).
func Validate(document rec.Map) {
	fonts, ok := document["fonts"]
	if !ok || fonts == nil || fonts == false {
		return
	}
	schema.Validate(fonts, contract("schemas", "source-fonts.schema.json"), "$")
	names := []any{}
	for _, value := range rec.FetchList(rec.AsMap(fonts), "items") {
		font := rec.AsMap(value)
		names = append(names, rec.Fetch(font, "font_name"))
	}
	for index, name := range names {
		if slices.ContainsFunc(names[:index], func(other any) bool { return rec.Equal(other, name) }) {
			rec.Fail("Duplicate source font binding")
		}
	}
	for _, value := range rec.FetchList(rec.AsMap(fonts), "items") {
		font := rec.AsMap(value)
		file, linked := font["file"].(rec.Map)
		if !linked {
			continue
		}
		if rec.Fetch(font, "css_family") != "ltd-"+rec.FetchString(file, "sha256")[:16] {
			rec.Fail("Unsafe linked font family")
		}
		if !linkedName.MatchString(rec.FetchString(file, "name")) {
			rec.Fail("Unsafe linked font file name")
		}
	}
	for _, span := range Spans(document) {
		if ForStyle(document, rec.FetchMap(span, "style")) == nil {
			rec.Fail("Missing source font for styled text")
		}
	}
}
