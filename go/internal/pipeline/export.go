package pipeline

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/sarielhp/relatex/internal/accessibility"
	"github.com/sarielhp/relatex/internal/browser"
	"github.com/sarielhp/relatex/internal/bundle"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/epub"
	"github.com/sarielhp/relatex/internal/exportwriter"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// payload is GeneralConversion#payload: the document, the layout and source map (nil when
// left out), no losses, and the capabilities.
func (c *conversion) payload() rec.Map {
	return rec.Map{"document": c.document, "layout": nilMap(c.layout), "source_map": nilMap(c.sourceMap), "losses": []any{},
		"capabilities": ojson.Plain(c.options.Capabilities)}
}

// layoutDigest names the layout for the stage cache: the layout stored as part files is its reference
// (the page files' digests), not the directory they are read from.
func (c *conversion) layoutDigest() string {
	if c.layout == nil {
		return digest(nil)
	}
	return digest(bundle.WithoutDirectory(c.layout))
}

// partDigest is the digest of a payload part, the layout by its reference.
func (c *conversion) partDigest(key string, value any) string {
	if key == "layout" && c.layout != nil {
		return c.layoutDigest()
	}
	return digest(value)
}

func nilMap(m rec.Map) any {
	if m == nil {
		return nil
	}
	return m
}

func (c *conversion) exportStage() {
	c.pipeline.stage("export", ojson.Object{}, c.configuration(), false, func(root string) any {
		c.exportRoot = root
		c.applyAltText()
		c.figureNameDiagnostics()
		payload := c.payload()
		bundle.Validate(payload)
		// The bundle is marshalled and compressed beside the formats, which only read the payload;
		// its layout goes into part files cut at the chapters (decision 087).
		written := make(chan any, 1)
		go func() {
			defer func() { written <- recover() }()
			bundle.WriteBook(filepath.Join(root, "document.json.gz"), payload, c.chapterPages)
		}()
		writer := &exportwriter.Writer{Part: func(key string) any { return payload[key] }, AllowFallback: c.options.AllowFallback,
			CompileTypst: c.options.CompileTypst, EPUB: c.epubOptions(), Pages: c.pageList, Chapters: c.chapterPages,
			Bibliography: c.typstBibliography()}
		results := c.exportFormats(writer, payload, root)
		command.Trace("export: formats written")
		c.bibliographyDiagnostic(results)
		if failure := <-written; failure != nil {
			panic(failure)
		}
		c.publishGlyphOwners(root)
		command.Trace("export: bundle and glyph owners")
		if c.layout != nil && c.options.LayoutViewer {
			bundle.WriteLayoutHTML(bundle.WholeLayout(c.layout), c.sourceMap, root)
		}
		// The PDF keeps the name TeX gave it, that of the main file: paper.tex gives paper.pdf.
		copyFile(c.pdf, filepath.Join(root, filepath.Base(c.pdf)), 0o644)
		if c.options.Synctex != "none" {
			c.publishSynctex(root)
		}
		c.publishProducts(root)
		command.Trace("export: products published")
		if c.options.VerifyBrowser {
			c.verifyBrowser(root, results)
		}
		command.Trace("export: browser verified")
		c.splitBook(root)
		command.Trace("export: book split")
		results = c.exportPackage(writer, payload, root, results)
		command.Trace("export: packaged")
		results = c.exportTagged(writer, payload, root, results)
		command.Trace("export: tagged")
		c.writeAccessibilityPage(root, results)
		c.exports = results
		return results
	})
}

// productsStage is the export of a products-only run: the PDF, its SyncTeX file, and the copies
// beside the main file.
func (c *conversion) productsStage() {
	c.pipeline.stage("export", ojson.Object{}, c.configuration(), false, func(root string) any {
		c.exportRoot = root
		copyFile(c.pdf, filepath.Join(root, filepath.Base(c.pdf)), 0o644)
		if c.options.Synctex != "none" {
			c.publishSynctex(root)
		}
		if c.options.Products != nil {
			c.options.Products(Built{PDF: c.productFile(root, ".pdf"), Synctex: c.productFile(root, ".synctex.gz"), Sources: c.sourceDigests()})
		}
		c.publishProducts(root)
		return ojson.Object{{"products", "pdf"}}
	})
}

// productFile is the product of the engine's PDF with this extension in root, "" when absent.
func (c *conversion) productFile(root, extension string) string {
	path := filepath.Join(root, strings.TrimSuffix(filepath.Base(c.pdf), ".pdf")+extension)
	if !isFile(path) {
		return ""
	}
	return path
}

// sourceDigests are the input's files with their SHA-256, by path relative to the input.
func (c *conversion) sourceDigests() map[string]string {
	digests := map[string]string{}
	for _, value := range rec.FetchList(c.archive.Manifest, "files") {
		file := rec.AsMap(value)
		if file["directory"] != true {
			digests[rec.AsString(rec.Fetch(file, "path"))] = rec.AsString(file["sha256"])
		}
	}
	return digests
}

// publishProducts copies the PDF, its SyncTeX file and the bibliography the engine wrote to the
// directory of the main file, as latexmk leaves them, when the input was a directory. --no-publish
// keeps them in the output directory alone.
func (c *conversion) publishProducts(root string) {
	if c.options.Publish != nil && !*c.options.Publish {
		return
	}
	directory := c.inputDirectory()
	main, _ := c.archive.Manifest["main"].(string)
	if directory == "" || main == "" {
		return
	}
	target := filepath.Join(directory, filepath.Dir(filepath.FromSlash(main)))
	job := strings.TrimSuffix(filepath.Base(c.pdf), ".pdf")
	published := []string{}
	for _, product := range []string{filepath.Join(root, job+".pdf"), filepath.Join(root, job+".synctex.gz"),
		filepath.Join(filepath.Dir(c.pdf), job+".bbl")} {
		if info, err := os.Stat(product); err == nil && info.Mode().IsRegular() {
			destination := filepath.Join(target, filepath.Base(product))
			copyFile(product, destination, 0o644)
			published = append(published, destination)
		}
	}
	if c.options.Published != nil {
		c.options.Published(published)
	}
	if c.options.ProductsOnly {
		c.recordProducts(published)
	}
}

// inputDirectory is the absolute directory the author's sources were read from, or "" for an archive.
func (c *conversion) inputDirectory() string {
	resolved, err := fspath.Real(c.input)
	if err != nil {
		return ""
	}
	if info, statErr := os.Stat(resolved); statErr != nil || !info.IsDir() {
		return ""
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return ""
	}
	return absolute
}

func (c *conversion) verifyBrowser(root string, results ojson.Object) {
	if !results.Has("html") {
		rec.Fail("Browser verification requires HTML among selected formats")
	}
	browser.Root = c.options.Root
	report, err := browser.CheckPaper(filepath.Join(root, "html"), c.document)
	if err != nil {
		// A browser that timed out or could not be reached on every attempt leaves the export
		// unverified, not failed: a diagnostic says so.
		c.degradations = append(c.degradations, ojson.Object{{"layer", "browser"}, {"level", "alert"}, {"reason", err.Error()},
			{"effect", "browser verification left out"}})
		c.diagnostics.Add("browser.unverified", err.Error(), nil)
		fmt.Fprintf(os.Stderr, "Browser verification left out: %v\n", err)
		report = rec.Map{"status": "unverified", "reason": err.Error()}
	}
	measured, _ := report["accessibility"].(rec.Map)
	delete(report, "accessibility")
	writeOrdered(filepath.Join(root, "browser-verification.json"), report)
	c.writeAccessibility(root, measured, err)
}

// writeAccessibility writes accessibility.json beside browser-verification.json (decision 080):
// axe-core's verdict from the browser check and the structural rows. Phase 0 measures, so the
// diagnostics are at level whatever and change no grade.
func (c *conversion) writeAccessibility(root string, measured rec.Map, unavailable error) {
	structural, failure := accessibility.Structural(c.document, c.sourceMap, filepath.Join(root, "html"))
	if failure != nil {
		structural = []any{}
		c.diagnostics.Add("accessibility.unmeasured", "structural checks: "+failure.Error(), nil)
	}
	gaps := int64(0)
	for _, value := range structural {
		if row := rec.AsMap(value); row["status"] == "gap" {
			gaps++
		}
	}
	var report rec.Map
	switch {
	case unavailable != nil:
		report = accessibility.Report("unverified", unavailable.Error(), nil, structural)
	case measured["status"] == "unmeasured":
		reason, _ := measured["reason"].(string)
		report = accessibility.Report("unmeasured", reason, nil, structural)
		c.diagnostics.Add("accessibility.unmeasured", reason, nil)
	default:
		report = accessibility.Report("measured", "", measured, structural)
		nodes := accessibility.Nodes(measured)
		rules := len(rec.FetchList(report, "open_items")) - int(gaps)
		if nodes > 0 || gaps > 0 {
			c.diagnostics.Add("accessibility.violations", fmt.Sprintf("axe-core %s: %d violating elements of %d rules over the light and dark themes; %d structural gaps",
				browser.AxeVersion, nodes, rules, gaps), nodes)
		}
	}
	writeOrdered(filepath.Join(root, "accessibility.json"), report)
}

// exportFormats writes the selected formats, each distinct one once, side by side: each has its own
// directory and its own stage cache entry, and reads the payload only. The results keep the order of
// the formats, and the first failing format in that order is the one raised.
func (c *conversion) exportFormats(writer *exportwriter.Writer, payload rec.Map, root string) ojson.Object {
	formats := []string{}
	for _, format := range c.options.Formats {
		// The EPUB package is written after the browser verification (exportPackage), the tagged
		// PDF last (exportTagged).
		if !slices.Contains(formats, format) && format != "epub" && format != "pdf-ua" {
			formats = append(formats, format)
		}
	}
	written := make([]ojson.Object, len(formats))
	failures := make([]any, len(formats))
	var group sync.WaitGroup
	for index, format := range formats {
		group.Add(1)
		go func() {
			defer group.Done()
			defer func() { failures[index] = recover() }()
			written[index] = c.exportFormat(writer, payload, format, root)
		}()
	}
	group.Wait()
	for _, failure := range failures {
		if failure != nil {
			panic(failure)
		}
	}
	results := ojson.Object{}
	for index, format := range formats {
		results = append(results, ojson.Pair{format, written[index]})
	}
	return results
}

// exportPackage writes the EPUB package when it is selected, after the other formats and the
// browser verification, so it carries the conformance claim of the accessibility report when the
// report makes one (decision 080: a claim only when mechanically true). The results keep the
// order of the selected formats.
func (c *conversion) exportPackage(writer *exportwriter.Writer, payload rec.Map, root string, results ojson.Object) ojson.Object {
	if !slices.Contains(c.options.Formats, "epub") {
		return results
	}
	writer.EPUB.Claim = epub.ReadClaim(filepath.Join(root, "accessibility.json"))
	packaged := c.exportFormat(writer, payload, "epub", root)
	ordered := ojson.Object{}
	for _, format := range c.options.Formats {
		switch {
		case ordered.Has(format):
		case format == "epub":
			ordered = append(ordered, ojson.Pair{format, packaged})
		default:
			ordered = append(ordered, ojson.Pair{format, results.Get(format)})
		}
	}
	return ordered
}

// epubOptions name the EPUB package after the main file, as the PDF is, pin its identifier
// and date to the sources and the build, and give it the PDF's pages (decision 094).
func (c *conversion) epubOptions() epub.Options {
	if !slices.Contains(c.options.Formats, "epub") {
		return epub.Options{}
	}
	source, _ := c.archive.Manifest["zip_sha256"].(string)
	return epub.Options{Name: strings.TrimSuffix(filepath.Base(c.pdf), ".pdf"), Source: source, Modified: epub.ModifiedDate(c.pdf),
		Split: c.options.Split, Pages: c.pageList, PrintSource: epub.PrintSource(c.pdf)}
}

// exportFormat is GeneralConversion#export_format: one format written, or restored from the
// stage cache when its inputs, configuration and writer are those of a cached entry.
func (c *conversion) exportFormat(writer *exportwriter.Writer, payload rec.Map, format, root string) ojson.Object {
	destination := filepath.Join(root, strings.ReplaceAll(format, ".", "-"))
	if c.cache == nil {
		return writer.Write(format, destination)
	}
	inputs := ojson.Object{{"document", digest(c.document)}}
	switch format {
	case "json", "json.gz":
		inputs = ojson.Object{}
		for _, key := range []string{"document", "layout", "source_map", "losses", "capabilities"} {
			inputs = append(inputs, ojson.Pair{key, c.partDigest(key, payload[key])})
		}
	case "pdf":
		inputs = ojson.Object{{"layout", c.layoutDigest()}}
	}
	configuration := ojson.Object{{"allow_fallback", nilString(c.options.AllowFallback)}, {"compile_typst", c.options.CompileTypst}}
	if format == "typst-source" {
		if bibliography := c.bibliographyInputs(); bibliography != nil {
			inputs = append(inputs, ojson.Pair{"bibliography", bibliography})
		}
	}
	if strings.HasPrefix(format, "typst") && c.options.CompileTypst {
		configuration = append(configuration, ojson.Pair{"compiler", c.helper("typst")})
	}
	if format == "epub" {
		configuration = append(configuration, ojson.Pair{"epub", ojson.Object{{"name", writer.EPUB.Name},
			{"source", writer.EPUB.Source}, {"modified", writer.EPUB.Modified}, {"claim", writer.EPUB.Claim}, {"split", writer.EPUB.Split},
			{"pages", digest(epub.PageMarksValue(writer.EPUB.Pages))}, {"print_source", writer.EPUB.PrintSource}}})
	}
	if format == "html" && len(writer.Pages) > 0 {
		// the page-break markers (plan accessibility A8)
		configuration = append(configuration, ojson.Pair{"pages", digest(epub.PageMarksValue(writer.Pages))})
	}
	if format == "pdf-ua" {
		inputs = append(inputs, c.taggedInputs()...)
		configuration = append(configuration, taggedStageConfiguration(c.taggedConfiguration(), c.webClaim)...)
	}
	identity := projection(c.options.Root, format)
	key := stageKey("export."+format, inputs, configuration, identity)
	prior := ""
	if c.resume != "" {
		prior = filepath.Join(c.resume, "layers/06-export", strings.ReplaceAll(format, ".", "-"))
	}
	force := slices.Contains(c.forced, "export") || slices.Contains(c.forced, "export."+format)
	restored := c.cache.stage(key, destination, "export."+format, force, prior, func(directory string) any {
		generated := writer.Write(format, directory)
		if !rec.Equal(ojson.Plain(projection(c.options.Root, format)), ojson.Plain(identity)) {
			rec.Fail("Exporter implementation changed during execution")
		}
		return generated
	})
	writeOrdered(filepath.Join(destination, "stage.json"), ojson.Object{{"layer", "export." + format}, {"identity", key},
		{"status", "complete"}, {"result", restored.result}, {"cache_metadata", restored.metadata}, {"cache", restored.cache},
		{"implementation", identity}})
	report := asObject(restored.result).Set("cache", restored.cache).Set("implementation", identity)
	writeOrdered(filepath.Join(root, strings.ReplaceAll(format, ".", "-")+"-reuse.json"), report)
	return report
}

// asObject is a result as an ordered object (a copy): written by a writer, or read back.
func asObject(value any) ojson.Object {
	switch typed := value.(type) {
	case ojson.Object:
		return append(ojson.Object{}, typed...)
	case rec.Map:
		return sortedObject(typed)
	}
	rec.Fail("Invalid export result")
	return nil
}

// configuration is @options.transform_keys(&:to_s): the options in DEFAULTS' order.
func (c *conversion) configuration() ojson.Object {
	o := c.options
	configuration := ojson.Object{{"formats", stringList(o.Formats)}, {"jobs", o.Jobs}, {"layout", o.Layout}, {"synchronize", o.Synchronize},
		{"source_fonts", o.SourceFonts}, {"allow_fallback", nilString(o.AllowFallback)}, {"compile_typst", o.CompileTypst},
		{"verify_browser", o.VerifyBrowser}, {"synctex", o.Synctex}, {"snapshot_exclusions", stringList(o.SnapshotExclusions)},
		{"cache", o.Cache}, {"resume", nilString(o.Resume)}, {"force", stringList(o.Force)}, {"verification_profile", nil},
		{"authored_proof", nil}, {"frozen_pdf", nil}, {"author_math", c.authorMath}, {"main_ignore", optionalList(o.MainIgnore)}}
	if o.Accessible {
		// Listed only when asked for, so a paper keeps its configuration.
		configuration = append(configuration, ojson.Pair{"accessible", true})
	}
	if o.WebReport != "" {
		// Listed only when given, so other conversions keep their stage cache keys.
		configuration = append(configuration, ojson.Pair{"web_report", o.WebReport})
	}
	if o.Split != "" && o.Split != "chapters" {
		// Listed only when it differs from the default, so a paper keeps its configuration.
		configuration = append(configuration, ojson.Pair{"split", o.Split})
	}
	// The caller's TEXINPUTS the build searched (decision 103), the home directory written "~": listed only
	// when there is one, so a hermetic run keeps its configuration.
	if host := c.host(); host != nil && host.Given != "" {
		configuration = append(configuration, ojson.Pair{"tex_search_path", host.Given})
	}
	return configuration
}

func optionalList(values []string) any {
	if values == nil {
		return nil
	}
	return stringList(values)
}

// summary is VerificationStatus.unverified_generation.
func (c *conversion) summary() ojson.Object {
	losses := []any{}
	rendering := ojson.Object{}
	for _, pair := range c.exports {
		result := asObject(pair.Value())
		if list, ok := ojson.Plain(result.Get("losses")).([]any); ok {
			losses = append(losses, list...)
		}
		status := "generated_unverified"
		if result.Has("compilation") {
			status = "compiled_unverified"
		}
		rendering = append(rendering, ojson.Pair{pair.Key(), ojson.Object{{"status", status}}})
	}
	synctex := c.options.Synctex
	syncStatus, capabilities := "generated_unverified", []any{"synctex." + synctex}
	if synctex == "none" {
		syncStatus, capabilities = "not_requested", []any{}
	}
	return ojson.Object{{"qualified", false},
		{"generation", ojson.Object{{"status", "complete"}, {"formats", stringList(c.options.Formats)}}},
		{"rendering", rendering}, {"acceptance", ojson.Object{{"status", "not_requested"}, {"qualified", false}}},
		{"synchronization", ojson.Object{{"mode", synctex}, {"status", syncStatus}}},
		{"unverified_formats", stringList(c.options.Formats)}, {"unverified_capabilities", capabilities}, {"losses", losses}}
}

func (c *conversion) summaryStatus() string {
	for _, pair := range c.exports {
		if losses, ok := ojson.Plain(asObject(pair.Value()).Get("losses")).([]any); ok && len(losses) > 0 {
			return "degraded"
		}
	}
	return "diagnostic"
}

// report is GeneralConversion#report: the run's state, written as run.json.
func (c *conversion) report(status string, details ojson.Object) ojson.Object {
	stages := make([]any, len(c.pipeline.stages))
	for index, receipt := range c.pipeline.stages {
		row := ojson.Object{}
		for _, key := range []string{"layer", "status", "seconds", "cache", "identity"} {
			if receipt.Has(key) {
				row = append(row, ojson.Pair{key, receipt.Get(key)})
			}
		}
		stages[index] = row
	}
	main := nilString(c.options.Main)
	if c.archive != nil {
		if value, ok := c.archive.Manifest["main"]; ok {
			main = value
		}
	}
	result := ojson.Object{{"status", status}, {"qualified", false},
		{"scope", "General external-JSON conversion; no frozen-paper qualification inferred"},
		{"directory", c.directory}, {"output", nilString(c.output)}, {"outputs", stringList(c.delivered)}, {"product", c.product}, {"stages", stages}, {"input", c.input}, {"main", main},
		{"engine", c.options.Engine}, {"configuration", c.configuration()}, {"resumed_from", nilString(c.resume)},
		{"prefixes", c.prefixes}, {"degradations", c.degradations}}
	for _, pair := range details {
		result = result.Set(pair.Key(), pair.Value())
	}
	if c.diagnostics != nil {
		result = result.Set("diagnostics", c.diagnosticCounts()).Set("diagnostic_codes", c.diagnosticCodes()).
			Set("diagnostic_lines", stringList(c.diagnostics.Lines("warning")))
	}
	counts := c.generationCounts()
	for _, pair := range counts {
		result = result.Set(pair.Key(), pair.Value())
	}
	if generation, ok := result.Get("generation").(ojson.Object); ok {
		generation = append(ojson.Object{}, generation...)
		for _, pair := range counts {
			generation = generation.Set(pair.Key(), pair.Value())
		}
		result = result.Set("generation", generation)
	}
	writeOrdered(filepath.Join(c.directory, "run.json"), result)
	return result
}

// diagnosticCodes tallies the conversion's diagnostics by code, in code order: those of the
// reconciled paper and those the later stages added (a left-out layout, the browser, the split).
func (c *conversion) diagnosticCodes() ojson.Object {
	tally := map[string]int64{}
	for _, entry := range c.diagnostics.Entries() {
		tally[rec.FetchString(rec.AsMap(entry), "code")]++
	}
	result := ojson.Object{}
	for _, code := range slices.Sorted(maps.Keys(tally)) {
		result = append(result, ojson.Pair{code, tally[code]})
	}
	return result
}

func (c *conversion) diagnosticCounts() ojson.Object {
	counts := c.diagnostics.Counts()
	result := ojson.Object{}
	for _, level := range []string{"fatal", "error", "alert", "warning", "whatever"} {
		result = append(result, ojson.Pair{level, counts[level]})
	}
	return result
}

// generationCounts are the engine invocations of this run: none when its builds came from
// the cache.
func (c *conversion) generationCounts() ojson.Object {
	receipt := c.pipeline.receipt("engine")
	if receipt == nil || receipt.Get("status") != "complete" {
		return ojson.Object{}
	}
	cache := asObject(receipt.Get("cache"))
	status := cache.Get("status")
	count := int64(0)
	if status != "hit" && status != "resumed" {
		results, _ := ojson.Plain(receipt.Get("result")).(rec.Map)
		builds, _ := results["results"].(rec.Map)
		for _, build := range builds {
			passes, _ := rec.AsMap(build)["passes"].(int64)
			count += passes
		}
	}
	return ojson.Object{{"engine_invocations", count}, {"engine_cache", status}}
}

// kpsewhichFor is the kpsewhich beside an engine.
func kpsewhichFor(engine string) string {
	if path := command.Executable(engine); path != "" {
		return filepath.Join(filepath.Dir(path), "kpsewhich")
	}
	return ""
}
