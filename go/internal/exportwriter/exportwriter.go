// Package exportwriter is BundleExport: one export format of a payload written into a new
// directory with its export.json report, as the relatex command line writes it. Reports keep
// their key order (they are printed); export.json is canonical.
package exportwriter

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/sarielhp/relatex/internal/browser"
	"github.com/sarielhp/relatex/internal/bundle"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/epub"
	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/html"
	"github.com/sarielhp/relatex/internal/mathexport"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/semantic"
	"github.com/sarielhp/relatex/internal/sourcefonts"
	"github.com/sarielhp/relatex/internal/tablemodel"
	"github.com/sarielhp/relatex/internal/typstsource"
)

// Formats is BundleExport::FORMATS.
var Formats = []string{"json", "json.gz", "html", "markdown", "typst", "typst-source", "pdf", "epub", "pdf-ua"}

// SetRoots names the repository root every writer reads its resources from.
func SetRoots(root string) {
	tablemodel.Root, sourcefonts.Root, fontmodel.Root, paperdoc.Root, bundle.Root = root, root, root, root, root
	mathexport.Root, semantic.Root, typstsource.Root, html.Root = root, root, root, root
}

// Writer is BundleExport.new(payload, allow_fallback:, compile_typst:). Part returns a payload
// member (document, layout, source_map, losses, capabilities), read when first used.
type Writer struct {
	Part          func(key string) any
	AllowFallback string
	CompileTypst  bool
	// EPUB names and pins the EPUB package: its file stem, source digest, date and claim.
	EPUB epub.Options
	// Pages are where the print PDF's pages begin (papersource.PageList): the web page marks each
	// (epub.MarkWebPages), as the EPUB package does; none for a bundle read back.
	Pages []epub.PageMark
	// Chapters are the pages a book's parts and chapters begin on, where the json formats cut the
	// layout into part files (decision 087); none for a paper or a bundle read back.
	Chapters []int64
	// Tagged writes the tagged PDF (pdf-ua, decision 097) into a directory and returns its report: a
	// LuaLaTeX build of the sources, which only the conversion has; nil where there are none.
	Tagged func(directory string) ojson.Object
	// Bibliography is the typst-source writer's bibliography inputs (decision 172); nil writes literal
	// entries (a bundle read back).
	Bibliography *typstsource.Bibliography
}

func (w *Writer) payload() rec.Map {
	return rec.Map{"document": w.Part("document"), "layout": w.Part("layout"), "source_map": w.Part("source_map"),
		"losses": w.Part("losses"), "capabilities": w.Part("capabilities")}
}

// Write is BundleExport#write: one format into a new directory, with its export.json, or an
// export-failure.json and the failure.
func (w *Writer) Write(format, directory string) ojson.Object {
	defer exportFailure(format, directory)
	if !slices.Contains(Formats, format) {
		rec.Fail("Unknown export format")
	}
	if _, err := os.Lstat(directory); err == nil {
		rec.Fail("Export directory already exists")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		rec.Fail("%v", err)
	}
	report := w.render(format, directory)
	report = append(report, ojson.Pair{"format", format}, ojson.Pair{"qualified", false},
		ojson.Pair{"validation", "Generation complete; independent per-input acceptance is separate"})
	writeJSONFile(filepath.Join(directory, "export.json"), report.Plain())
	return report
}

// exportFailure records a failed format's message beside its partial output, as Ruby does.
func exportFailure(format, directory string) {
	recovered := recover()
	if recovered == nil {
		return
	}
	message := fmt.Sprint(recovered)
	var failure rec.Failure
	if err, ok := recovered.(error); ok && errors.As(err, &failure) {
		message = failure.Message
	}
	if info, err := os.Stat(directory); err == nil && info.IsDir() {
		writeJSONFile(filepath.Join(directory, "export-failure.json"), rec.Map{"format": format, "error": message})
	}
	panic(recovered)
}

func writeJSONFile(path string, value any) {
	if err := canonjson.WriteFile(path, value); err != nil {
		rec.Fail("%v", err)
	}
}

func writeText(path, text string) {
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		rec.Fail("%v", err)
	}
}

func (w *Writer) render(format, directory string) ojson.Object {
	document := rec.AsMap(w.Part("document"))
	switch format {
	case "html":
		writeText(filepath.Join(directory, "index.html"), epub.MarkWebPages(html.Render(document), w.Pages))
		// The page links its figures as files beside it (figs/, the EPUB 3 layout).
		html.WriteImageFiles(document, directory)
		fontmodel.WriteFiles(document, directory)
		mathexport.WriteAccentFont(document, directory)
		html.WritePreviewScript(directory)
		return ojson.Object{{"status", "success"}, {"file", "index.html"}, {"losses", []any{}}}
	case "json", "json.gz":
		name := "document." + format
		// The layout's part files go beside the bundle, in layout/ (decision 087).
		storage := bundle.WriteBook(filepath.Join(directory, name), w.payload(), w.Chapters)
		return ojson.Object{{"status", "success"}, {"file", name}, {"losses", []any{}},
			{"storage", Ordered(storage, "sha256", "bytes", "expanded_bytes")}}
	case "markdown", "typst":
		return w.semantic(format, document, directory)
	case "epub":
		options := w.EPUB
		options.Page = readingPage
		return epub.Write(document, directory, options)
	case "pdf-ua":
		if w.Tagged == nil {
			rec.Fail("The tagged PDF needs the sources: convert them, not a bundle")
		}
		return w.Tagged(directory)
	case "typst-source":
		writer, losses := typstsource.WriteWith(document, directory, w.Bibliography)
		bibliography, styled := writer["bibliography"]
		delete(writer, "bibliography")
		result := ojson.Object{{"status", status(losses)}, {"file", "main.typ"}, {"losses", orderedLosses(losses)},
			{"writer", Ordered(writer, "labels", "frozen_references", "unexpressed_citations", "losses", "collapsed_displays")}}
		if styled {
			result = append(result, ojson.Pair{"bibliography", Ordered(rec.AsMap(bibliography), "status", "reason", "processor", "names", "path", "style", "databases", "entries", "differing")})
		}
		if w.CompileTypst {
			result = append(result, ojson.Pair{"compilation", compileTypst(directory, "main.typ", "main.pdf", "--ignore-system-fonts")})
		}
		return result
	}
	layout, _ := w.Part("layout").(rec.Map)
	report := Ordered(bundle.Reconstruct(layout, directory), "file", "pages", "helper_sha256", "source_pdf_sha256", "fidelity", "losses")
	for index, pair := range report {
		if pair.Key() == "losses" {
			report[index] = ojson.Pair{"losses", orderedLosses(rec.AsList(pair.Value()))}
		}
	}
	return report.Set("status", "degraded")
}

// Ordered is a report's keys in Ruby's order: those named first, any others after, sorted.
func Ordered(report rec.Map, keys ...string) ojson.Object {
	result := ojson.Object{}
	for _, key := range keys {
		if value, ok := report[key]; ok {
			result = append(result, ojson.Pair{key, value})
		}
	}
	rest := []string{}
	for key := range report {
		if !slices.Contains(keys, key) {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	for _, key := range rest {
		result = append(result, ojson.Pair{key, report[key]})
	}
	return result
}

// orderedLosses are the losses' entries in the writers' key order.
func orderedLosses(losses []any) []any {
	result := make([]any, len(losses))
	for index, loss := range losses {
		entry, ok := loss.(rec.Map)
		if !ok {
			result[index] = loss
			continue
		}
		result[index] = Ordered(entry, "node", "original", "replacement", "reason", "properties", "evidence_asset")
	}
	return result
}

func status(losses []any) string {
	if len(losses) == 0 {
		return "success"
	}
	return "degraded"
}

func (w *Writer) semantic(format string, document rec.Map, directory string) ojson.Object {
	text, losses, representations := semantic.Render(format, document, directory, w.AllowFallback, borrowed)
	name := "document.md"
	if format == "typst" {
		name = "document.typ"
	}
	writeText(filepath.Join(directory, name), text)
	if format == "markdown" {
		// The Markdown's stylesheet links the same font files as the page, beside it in fonts/.
		fontmodel.WriteFiles(document, directory)
		mathexport.WriteAccentFont(document, directory)
	}
	report := ojson.Object{{"status", status(losses)}, {"file", name}, {"losses", orderedLosses(losses)},
		{"math_representations", representations}}
	if format == "typst" && w.CompileTypst {
		report = append(report, ojson.Pair{"compilation", compileTypst(directory, "", name, "document.pdf")})
	}
	return report
}

// borrowed are the renderings the Markdown and Typst writers take from the other outputs: the
// browser's formula images (allow_fallback image) and the page's HTML tables.
var borrowed = semantic.Borrowed{
	MathImages: func(document rec.Map, directory string) func(string) rec.Map {
		return browser.NewFallback(document, directory, html.Render).Capture
	},
	TableHTML: func(document rec.Map) func(rec.Map) string { return html.New(document).TableGridSpan },
}

// compileTypst is BundleExport#compile_typst: the generated Typst compiled by the typst
// helper, which must not change while it runs.
func compileTypst(directory, name, output string, flags ...string) ojson.Object {
	tool := command.Resolve("typst")
	if tool == "" {
		rec.Fail("Typst compiler unavailable; source export remains in the failed attempt")
	}
	hash := command.FileDigest(tool)
	absolute, err := filepath.Abs(directory)
	if err != nil {
		rec.Fail("%v", err)
	}
	arguments := append(append([]string{tool, "compile"}, flags...), "--root", absolute, name, output)
	stage := command.Run(arguments, directory, filepath.Join(directory, "typst.log"), 120*time.Second, nil)
	writeJSONFile(filepath.Join(directory, "typst-stage.json"), stage)
	exit, _ := stage["exit"].(int64)
	if stage["exit"] == nil || exit != 0 || stage["timeout"] == true || command.FileDigest(tool) != hash {
		rec.Fail("Generated Typst failed compilation")
	}
	return ojson.Object{{"status", "compiled"}, {"helper_sha256", hash}, {"pdf_sha256", command.FileDigest(filepath.Join(directory, output))}}
}

// readingPage writes the reading page's figures, fonts and accent font into directory and
// returns the page (html.Render): the page the EPUB packages (epub.Options.Page).
func readingPage(document rec.Map, directory string) string {
	html.WriteImageFiles(document, directory)
	fontmodel.WriteFiles(document, directory)
	mathexport.WriteAccentFont(document, directory)
	return html.Render(document)
}
