package pdfpaint

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fontbinding"
	"github.com/sarielhp/relatex/internal/paintmodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// ToolSeries is the release series of the helper of PDFTablePaint and PDFRichTablePaint (decision
// 129, D7 amendment: any 1.28.x on every platform; a patch release keeps the structured text and
// SVG forms, a new series needs verification).
const ToolSeries = "mutool version 1.28."

// SupportedVersion reports whether line, the first line of `mutool -v`, names a release of
// ToolSeries ("mutool version 1.28.5").
func SupportedVersion(line string) bool {
	patch, found := strings.CutPrefix(line, ToolSeries)
	if !found || patch == "" {
		return false
	}
	_, err := strconv.ParseUint(patch, 10, 16)
	return err == nil
}

// tool is the helper itself: RELATEX_MUTOOL, else mutool on PATH (/usr/bin/mutool on Linux, the
// mupdf.com build on Windows); "" when there is none.
func tool() string { return command.Resolve("mutool") }

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func executable(path string) bool { return path != "" && command.IsExecutable(path) }

// paintTables is PDFTablePaint.new: the table grids of the document in reading order, a
// classic grid with the one page its block ships on.
func paintTables(reading, document rec.Map) []rec.Map {
	sources := map[any]rec.Map{}
	blocks := rec.FetchList(reading, "blocks")
	for _, note := range rec.FetchList(rec.FetchMap(reading, "notes"), "notes") {
		blocks = append(slices.Clone(blocks), rec.FetchList(rec.AsMap(note), "blocks")...)
	}
	for _, block := range blocks {
		sources[rec.Fetch(rec.AsMap(block), "id")] = rec.AsMap(block)
	}
	nodes := rec.FetchMap(document, "nodes")
	result := []rec.Map{}
	for _, id := range rec.FetchList(document, "block_order") {
		node := rec.FetchMap(nodes, rec.AsString(id))
		for _, item := range rec.FetchList(node, "content") {
			span := rec.AsMap(item)
			if span["type"] != "table_grid" {
				continue
			}
			if paintmodel.RichGrid(span["grid"]) {
				result = append(result, rec.Map{"id": rec.Fetch(span, "id"), "grid": rec.Fetch(span, "grid")})
				continue
			}
			key := rec.Fetch(rec.FetchMap(node, "provenance"), "inspection_key")
			source, ok := sources[key]
			if !ok {
				panic(rec.Failure{Message: fmt.Sprintf("key not found: %q", paintmodel.ToS(key)), Lookup: true})
			}
			pages := rec.FetchList(source, "pages")
			if len(pages) != 1 {
				rec.Fail("PDF paint requires a table on one page")
			}
			result = append(result, rec.Map{"id": rec.Fetch(span, "id"), "grid": rec.Fetch(span, "grid"), "page": pages[0]})
		}
	}
	return result
}

// CaptureTables is PDFTablePaint.new(pdf, reading, document).capture and its directory: the
// proof that each table's paint is the ordinary PDF's, or nil without tables.
func CaptureTables(pdf string, reading, document rec.Map) (paint rec.Map, directory string) {
	pdf, _ = filepath.Abs(pdf)
	list := paintTables(reading, document)
	if len(list) == 0 {
		return nil, ""
	}
	for _, table := range list {
		if paintmodel.RichGrid(table["grid"]) {
			return captureRich(pdf, list)
		}
	}
	pages := []any{}
	for _, table := range list {
		if !slices.ContainsFunc(pages, func(page any) bool { return rec.Equal(page, table["page"]) }) {
			pages = append(pages, table["page"])
		}
	}
	if len(pages) != len(list) {
		rec.Fail("Ambiguous PDF ownership: multiple tables on one page")
	}
	if !executable(tool()) {
		rec.Fail("PDF paint helper unavailable")
	}
	directory = filepath.Join(command.Scratch("table-paint-"), "evidence")
	if exists(directory) {
		rec.Fail("PDF paint output must be new")
	}
	mkdir(directory)
	pdfHash, helperHash := command.FileDigest(pdf), command.FileDigest(tool())
	verifyTool(directory)
	captured := []any{}
	for index, table := range list {
		captured = append(captured, captureClassic(pdf, directory, table, index))
	}
	if command.FileDigest(pdf) != pdfHash || command.FileDigest(tool()) != helperHash {
		rec.Fail("PDF paint source or helper changed")
	}
	proof := rec.Map{"status": "native_paint_verified", "qualified": false, "pdf_sha256": pdfHash, "helper_sha256": helperHash,
		"tables": captured}
	fontbinding.WriteJSON(filepath.Join(directory, "paint.json"), proof)
	return proof, directory
}

func stageOK(stage rec.Map) bool {
	exit, ok := stage["exit"].(int64)
	return ok && exit == 0 && stage["timeout"] != true
}

func verifyTool(directory string) {
	log := filepath.Join(directory, "helper-version.log")
	stage := command.Run([]string{tool(), "-v"}, directory, log, 5*time.Second, nil)
	fontbinding.WriteJSON(filepath.Join(directory, "helper-version-stage.json"), stage)
	data, _ := os.ReadFile(log)
	if !stageOK(stage) || !SupportedVersion(rec.Strip(string(data))) {
		rec.Fail("Unsupported PDF paint helper version")
	}
}

func captureClassic(pdf, directory string, table rec.Map, index int) rec.Map {
	page := rec.FetchInt(table, "page")
	path := filepath.Join(directory, fmt.Sprintf("table-%d.xml", index+1))
	stage := command.Run(command.MutoolDrawCommand(tool(), "-F", "trace", "-o", path, filepath.Base(pdf), strconv.FormatInt(page, 10)),
		filepath.Dir(pdf), filepath.Join(directory, fmt.Sprintf("table-%d.log", index+1)), 30*time.Second, nil)
	fontbinding.WriteJSON(filepath.Join(directory, fmt.Sprintf("table-%d-stage.json", index+1)), stage)
	if !stageOK(stage) {
		rec.Fail("PDF paint extraction failed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	found := newClassicGeometry(rec.FetchMap(table, "grid"), page).verify(data)
	fontbinding.WriteJSON(filepath.Join(directory, fmt.Sprintf("table-%d-geometry.json", index+1)), found)
	return rec.Map{"id": table["id"], "page": table["page"], "rules": found["rules"], "glyphs": found["glyphs"],
		"color": paintmodel.Color, "grid_sha256": paintmodel.DigestOf(rec.Fetch(table, "grid")), "trace_sha256": command.FileDigest(path)}
}

// captureRich is PDFRichTablePaint#capture: a versioned multipage paint proof.
func captureRich(pdf string, list []rec.Map) (rec.Map, string) {
	if !executable(tool()) {
		rec.Fail("PDF table paint helper unavailable")
	}
	directory := filepath.Join(command.Scratch("rich-table-paint-"), "evidence")
	if exists(directory) {
		rec.Fail("PDF table paint output must be new")
	}
	mkdir(directory)
	sourceHash, helperHash := command.FileDigest(pdf), command.FileDigest(tool())
	runRich([]string{tool(), "-v"}, directory, "version", directory, 5*time.Second)
	if data, _ := os.ReadFile(filepath.Join(directory, "version.log")); !SupportedVersion(rec.Strip(string(data))) {
		rec.Fail("Unsupported PDF paint helper version")
	}
	products := map[any][]any{}
	fonts := captureFonts(pdf, nativeGlyphs(list), filepath.Join(directory, "font-encoding"))
	encodings, names := paintmodel.FontEncodings(fonts), paintmodel.FontNames(fonts)
	for _, page := range paintPages(list) {
		path := filepath.Join(directory, fmt.Sprintf("page-%d.xml", page))
		runRich(command.MutoolDrawCommand(tool(), "-F", "trace", "-o", path, filepath.Base(pdf), strconv.FormatInt(page, 10)),
			directory, fmt.Sprintf("page-%d", page), filepath.Dir(pdf), 60*time.Second)
		data, err := os.ReadFile(path)
		if err != nil {
			rec.Fail("%v", err)
		}
		trace := command.FileDigest(path)
		if fragments := richFragments(list, page); len(fragments) > 0 {
			found := newRichGeometry(fragments, newInventory(data, page), encodings, names).verify()
			for _, entry := range fragments {
				id := rec.Fetch(entry, "id")
				geometry := rec.AsMap(found[rec.AsString(id)])
				geometry["page"], geometry["trace_sha256"] = page, trace
				products[id] = append(products[id], geometry)
			}
		}
		for _, table := range list {
			if paintmodel.RichGrid(table["grid"]) || !rec.Equal(table["page"], page) {
				continue
			}
			found := newClassicGeometry(rec.FetchMap(table, "grid"), page).verify(data)
			found["page"], found["trace_sha256"] = page, trace
			products[table["id"]] = append(products[table["id"]], found)
		}
	}
	if sourceHash != command.FileDigest(pdf) || helperHash != command.FileDigest(tool()) {
		rec.Fail("PDF table paint source/helper changed")
	}
	captured := []any{}
	for _, table := range list {
		fragments := products[table["id"]]
		if fragments == nil {
			fragments = []any{}
		}
		captured = append(captured, rec.Map{"id": table["id"], "color": paintmodel.Color, "grid_sha256": paintmodel.DigestOf(rec.Fetch(table, "grid")),
			"fragments": fragments})
	}
	proof := rec.Map{"schema_version": paintmodel.RichVersion, "status": "native_paint_verified", "qualified": false,
		"pdf_sha256": sourceHash, "helper_sha256": helperHash, "font_encodings": encodings, "font_bindings": fonts, "tables": captured}
	fontbinding.WriteJSON(filepath.Join(directory, "paint.json"), proof)
	return proof, directory
}

func runRich(arguments []string, directory, name, workdir string, timeout time.Duration) {
	runTool(arguments, directory, name, workdir, timeout, "Rich table paint extraction failed")
}

// runTool runs one extraction helper with its log and stage receipt in directory; a failed
// run is the failure message.
func runTool(arguments []string, directory, name, workdir string, timeout time.Duration, failure string) {
	log := filepath.Join(directory, name+".log")
	stage := command.Run(arguments, workdir, log, timeout, nil)
	fontbinding.WriteJSON(filepath.Join(directory, name+"-stage.json"), stage)
	if !stageOK(stage) && !recoveredStage(stage, log) {
		rec.Fail("%s", failure)
	}
}

// recoveredStage: mutool exited 1 only because MuPDF drew the pages past content it could not read
// (command.Recovered; papers3/0786's shading), which paints no text or table glyph.
func recoveredStage(stage rec.Map, log string) bool {
	if exit, ok := stage["exit"].(int64); !ok || exit != 1 || stage["timeout"] == true {
		return false
	}
	data, err := os.ReadFile(log)
	if err != nil {
		return false
	}
	_, recovered := command.Recovered(data)
	return recovered
}

func nativeFragments(table rec.Map) []any {
	fragments, _ := rec.FetchMap(table, "grid")["native_fragments"].([]any)
	return fragments
}

func nativeGlyphs(list []rec.Map) []rec.Map {
	result := []rec.Map{}
	for _, table := range list {
		for _, fragment := range nativeFragments(table) {
			for _, glyph := range rec.FetchList(rec.AsMap(fragment), "glyphs") {
				result = append(result, rec.AsMap(glyph))
			}
		}
	}
	return result
}

func paintPages(list []rec.Map) []int64 {
	pages := []int64{}
	for _, table := range list {
		if paintmodel.RichGrid(table["grid"]) {
			for _, fragment := range rec.FetchList(rec.FetchMap(table, "grid"), "native_fragments") {
				pages = append(pages, rec.FetchInt(rec.AsMap(fragment), "page"))
			}
		} else {
			pages = append(pages, rec.FetchInt(table, "page"))
		}
	}
	slices.Sort(pages)
	return slices.Compact(pages)
}

func richFragments(list []rec.Map, page int64) []rec.Map {
	result := []rec.Map{}
	for _, table := range list {
		if !paintmodel.RichGrid(table["grid"]) {
			continue
		}
		for _, fragment := range rec.FetchList(rec.FetchMap(table, "grid"), "native_fragments") {
			if rec.Equal(rec.Fetch(rec.AsMap(fragment), "page"), page) {
				result = append(result, rec.Map{"id": rec.Fetch(table, "id"), "fragment": fragment})
				break
			}
		}
	}
	return result
}
