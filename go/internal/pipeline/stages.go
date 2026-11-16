package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/a11ylint"
	"github.com/sarielhp/relatex/internal/authorhints"
	"github.com/sarielhp/relatex/internal/bundle"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/formulatex"
	"github.com/sarielhp/relatex/internal/layout"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/papersource"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/sourcemap"
	"github.com/sarielhp/relatex/internal/split"
	"github.com/sarielhp/relatex/internal/synchronize"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

// sourceMapSchema is GlyphSourceMap::SCHEMA.
const sourceMapSchema = "source-map-2.0"

var passNumber = regexp.MustCompile(`pass-(\d+)`)

func (c *conversion) helper(name string) ojson.Object {
	identity := command.Identity(name)
	return exportOrdered(identity, "available", "path", "sha256")
}

// exportOrdered is a map with the keys Ruby writes first in its order, any others sorted after.
func exportOrdered(m rec.Map, keys ...string) ojson.Object {
	result := ojson.Object{}
	for _, key := range keys {
		if value, ok := m[key]; ok {
			result = append(result, ojson.Pair{key, value})
		}
	}
	for _, key := range sortedKeys(m) {
		if !result.Has(key) {
			result = append(result, ojson.Pair{key, m[key]})
		}
	}
	return result
}

// hintSources reads the paper's sources for author hints (decision 122), following the files the
// main file inputs as the lint does.
func hintSources(directory, main string) ([]authorhints.Source, error) {
	files, err := a11ylint.Load(directory, main)
	if err != nil {
		return nil, err
	}
	sources := make([]authorhints.Source, 0, len(files))
	for _, file := range files {
		sources = append(sources, authorhints.Source{Path: file.Path, Text: file.Source})
	}
	return sources, nil
}

func (c *conversion) reconcileStage() {
	manifest := filepath.Join(c.probe, "artifact-manifest.json")
	configuration := ojson.Object{{"source_fonts", c.options.SourceFonts}, {"diagnostics", diagnostics.Version},
		{"formula_tex", formulatex.Version}}
	if c.authorMath {
		configuration = append(configuration, ojson.Pair{"author_math", true})
	}
	configuration = append(configuration, ojson.Pair{"helpers", ojson.Object{{"mutool", c.helper("mutool")}, {"qpdf", c.helper("qpdf")}}})
	options := papersource.Options{SourceFonts: c.options.SourceFonts, AuthorMath: c.authorMath, Engine: c.options.Engine,
		Hints: hintSources}
	c.pipeline.stage("reconcile", ojson.Object{{"artifacts", manifest}}, configuration, true, func(root string) any {
		return exportOrdered(papersource.Reconcile(c.options.Root, c.probe, root, options), "nodes", "qualified")
	})
	c.document = c.readStageJSON("reconcile", "document.json")
	paperdoc.Validate(c.document)
	diagnosticsData := c.readStageJSON("reconcile", "diagnostics.json")
	papersource.ValidateContracts(c.options.Root, diagnosticsData, c.readStageJSON("reconcile", "provenance.json"))
	// What reconciling the paper could not keep, compiler-style (profiles/diagnostics.json).
	profile, err := diagnostics.LoadProfile(c.options.Root)
	if err != nil {
		rec.Fail("%v", err)
	}
	c.diagnostics = diagnostics.New(profile, nil, false)
	if !texsandbox.Sandboxed {
		// Decision 129 (D1): every run on a platform without the TeX sandbox says how it was made.
		c.diagnostics.Add("sandbox.none", "TeX ran without a sandbox, with its own file limits only", nil)
	}
	c.addSnapshotSkips()
	c.chapterPages = c.readChapterPages()
	c.pageList = c.readPageList()
	for _, value := range rec.FetchList(diagnosticsData, "entries") {
		entry := rec.AsMap(value)
		location, _ := entry["location"].(rec.Map)
		places := []diagnostics.Place{}
		for _, key := range sortedKeys(location) {
			places = append(places, diagnostics.Place{Key: key, Value: location[key]})
		}
		c.diagnostics.Add(rec.FetchString(entry, "code"), rec.FetchString(entry, "message"), nil, places...)
	}
}

// degradedCodes are the diagnostics of the layers degrade leaves out.
var degradedCodes = map[string]string{"layout": "layout.absent", "synchronize": "synchronization.absent"}

// degrade is a left-out layer: its failure becomes an alert, the conversion goes on.
func (c *conversion) degrade(layer, effect string, run func()) (left bool) {
	defer func() {
		value := recover()
		if value == nil {
			return
		}
		failure, ok := isFailure(value)
		if !ok {
			panic(value)
		}
		c.degradations = append(c.degradations, ojson.Object{{"layer", layer}, {"level", "alert"}, {"reason", failure.Message},
			{"effect", effect}})
		left = true
		// A recorded diagnostic (profiles/diagnostics.json) reaches the report, its counts and the
		// compiler-style lines; a products-only run has no diagnostics and says it on stderr alone.
		if c.diagnostics != nil {
			c.diagnostics.Add(degradedCodes[layer], failure.Message, nil)
			return
		}
		label := map[string]string{"layout": "Layout", "synchronize": "Source synchronization"}[layer]
		fmt.Fprintf(os.Stderr, "%s left out: %s\n", label, failure.Message)
	}()
	run()
	return false
}

func (c *conversion) layoutStage() {
	if c.degrade("layout", "no layout view", func() {
		// The layout is stored as part files cut at the chapters, never held whole (decision 185).
		chapters := make([]any, len(c.chapterPages))
		for index, page := range c.chapterPages {
			chapters[index] = page
		}
		configuration := ojson.Object{{"helper", c.helper("mutool")}, {"chapters", chapters}}
		c.pipeline.stage("layout", ojson.Object{{"pdf", c.pdf}}, configuration, true, func(root string) any {
			return c.captureLayout(filepath.Join(root, "capture"))
		})
		directory := filepath.Dir(c.stageArtifact("layout", "capture/layout.json"))
		c.layout = bundle.AttachLayout(c.readStageJSON("layout", "capture/layout.json"), directory)
		bundle.ValidateLayout(c.layout)
		c.reportRecoveredLayout()
	}) {
		c.layout = nil
	}
}

// captureLayout captures the PDF's layout into directory: its pages go into part files as they are bound,
// and layout.json is the reference to them.
func (c *conversion) captureLayout(directory string) any {
	var writer *bundle.LayoutWriter
	data := layout.CaptureTo(c.pdf, directory, func(count int) (func(rec.Map) any, func(any)) {
		writer = bundle.NewLayoutWriter(directory, count, c.chapterPages)
		return writer.Prepare, writer.Add
	})
	reference := writer.Finish(data)
	if err := canonjson.WriteFile(filepath.Join(directory, "layout.json"), reference); err != nil {
		rec.Fail("%v", err)
	}
	return ojson.Object{{"pages", rec.FetchInt(reference, "page_count")}, {"pdf_sha256", rec.Fetch(reference, "pdf_sha256")}}
}

// reportRecoveredLayout reports the content errors MuPDF drew the layout's pages past (layout.json's
// recovered_errors): what failed is missing from the page graphics.
func (c *conversion) reportRecoveredLayout() {
	errors, _ := rec.FetchMap(c.layout, "measurement")["recovered_errors"].([]any)
	if len(errors) == 0 {
		return
	}
	messages := make([]string, len(errors))
	for index, message := range errors {
		messages[index] = rec.ToS(message)
	}
	message := fmt.Sprintf("MuPDF left out what it could not read in the PDF (%s); the layout's page graphics lack it",
		strings.Join(messages, "; "))
	if c.diagnostics == nil {
		fmt.Fprintf(os.Stderr, "Layout incomplete: %s\n", message)
		return
	}
	c.diagnostics.Add("layout.incomplete", message, int64(len(errors)))
}

func (c *conversion) glyphMap() string {
	matches, _ := filepath.Glob(filepath.Join(c.probe, "external/output", "pass-*.jsonl.glyphs.jsonl"))
	sort.SliceStable(matches, func(i, j int) bool { return passOf(matches[i]) < passOf(matches[j]) })
	if len(matches) == 0 {
		rec.Fail("The engine wrote no glyph map")
	}
	return matches[len(matches)-1]
}

func passOf(path string) int {
	match := passNumber.FindStringSubmatch(path)
	if match == nil {
		return 0
	}
	number, _ := strconv.Atoi(match[1])
	return number
}

func (c *conversion) synchronizationStage() {
	left := c.degradeUnlessWord(func() {
		glyphs := c.glyphMap()
		stream := strings.TrimSuffix(glyphs, ".glyphs.jsonl")
		synctex := strings.TrimSuffix(c.pdf, ".pdf") + ".synctex.gz"
		inputs := ojson.Object{{"pdf", c.pdf}, {"glyphs", glyphs}, {"synctex", synctex}}
		if !c.options.ProductsOnly {
			inputs = append(ojson.Object{{"pdf", c.pdf}, {"document", c.stageArtifact("reconcile", "document.json")}}, inputs[1:]...)
		}
		options := synchronize.Options{GlyphMap: glyphs, PDF: c.pdf, Synctex: synctex,
			SourceDirectory: filepath.Join(c.probe, "external/working"), SourceFiles: c.builtSourceFiles(),
			Prefixes: c.prefixMap()}
		if isFile(stream) {
			inputs = append(inputs, ojson.Pair{"stream", stream})
			options.Stream = stream
		}
		var captured rec.Map
		c.pipeline.stage("synchronize", inputs, ojson.Object{{"schema", sourceMapSchema}}, true, func(root string) any {
			sourceMap := synchronize.Capture(options)
			captured = sourceMap
			if c.document != nil {
				sourceMap["document_sha256"] = digest(c.document)
			}
			if err := canonjson.WriteFile(filepath.Join(root, "source-map.json"), sourceMap); err != nil {
				rec.Fail("%v", err)
			}
			return ojson.Object{{"words", int64(len(rec.FetchList(sourceMap, "words")))}, {"pdf_sha256", sourceMap["pdf_sha256"]}}
		})
		if c.options.ProductsOnly && captured != nil {
			// Only the SyncTeX is written from it, in this process: no reading back, no checking of a file.
			c.sourceMap = captured
			return
		}
		c.sourceMap = c.readStageJSON("synchronize", "source-map.json")
		sourcemap.Validate(c.sourceMap)
	})
	if left {
		c.sourceMap = nil
	}
}

// degradeUnlessWord: like the layout, a source map that cannot be proven is left out with an
// alert, unless Word SyncTeX needs it.
func (c *conversion) degradeUnlessWord(run func()) bool {
	if c.options.Synctex == "word" {
		run()
		return false
	}
	return c.degrade("synchronize", "no source synchronization", run)
}

// workspaceDirectories are the service directories command.Workspace gives the tools a stage runs.
var workspaceDirectories = []string{"_tmp", "_cache", "_config"}

// pageResources are the files beside the HTML export's page that the page loads; the copy of
// the page at the root of the publication (and a book's chapter files) loads them beside it.
var pageResources = []string{"preview.js", "figs", "fonts"}

func (c *conversion) publishStage() {
	c.pipeline.stage("publish", ojson.Object{}, ojson.Object{{"destination", c.product}, {"output", nilString(c.output)}}, false, func(string) any {
		parent := filepath.Dir(c.product)
		mkdirAll(parent)
		staged := command.MakeTemp(parent, ".relatex-publish-")
		defer removeAll(staged)
		entries, err := os.ReadDir(c.exportRoot)
		if err != nil {
			rec.Fail("%v", err)
		}
		for _, entry := range entries {
			// stage.json is the layer receipt; _tmp, _cache and _config are the service directories
			// the export stage gave the tools it ran (command.Workspace).
			if entry.Name() == "stage.json" || strings.HasPrefix(entry.Name(), ".") || slices.Contains(workspaceDirectories, entry.Name()) {
				continue
			}
			copyTree(filepath.Join(c.exportRoot, entry.Name()), filepath.Join(staged, entry.Name()))
		}
		publishPages(staged)
		// The EPUB package sits at the root beside the PDF, named as the PDF is.
		if packaged, _ := filepath.Glob(filepath.Join(staged, "epub", "*.epub")); len(packaged) == 1 {
			copyFile(packaged[0], filepath.Join(staged, filepath.Base(packaged[0])), 0o644)
		}
		checkPublishedReferences(staged)
		writeOrdered(filepath.Join(staged, "provenance.json"), ojson.Object{{"input_sha256", c.archive.Manifest["zip_sha256"]},
			{"main", c.archive.Manifest["main"]}, {"qualified", false},
			{"scope", "General conversion; independent per-source acceptance remains required"}, {"configuration", c.configuration()}})
		writeOrdered(filepath.Join(staged, "manifest.json"), sortedObject(outputInventory(staged)))
		publishInto(staged, c.product)
		c.delivered = c.deliver()
		return ojson.Object{{"product", c.product}, {"output", nilString(c.output)}, {"outputs", stringList(c.delivered)}, {"qualified", false},
			{"manifest_sha256", command.FileDigest(filepath.Join(c.product, "manifest.json"))}}
	})
}

// publishPages puts the reading page at the root of the staged publication with the files it
// loads; a split book opens on its contents page, its chapter files beside it (decision 081).
func publishPages(staged string) {
	if html := filepath.Join(staged, "html/index.html"); isFile(html) {
		copyFile(html, filepath.Join(staged, "index.html"), 0o644)
		for _, name := range pageResources {
			if resource := filepath.Join(staged, "html", name); isFile(resource) || isDirectory(resource) {
				copyTree(resource, filepath.Join(staged, name))
			}
		}
	}
	if contents := filepath.Join(staged, SplitDirectory, split.ContentsName); isFile(contents) {
		publishSplit(filepath.Join(staged, SplitDirectory), staged)
	}
}

// copyTree is FileUtils.cp_r: files with their permissions, directories recursively.
func copyTree(from, to string) {
	info, err := os.Lstat(from)
	if err != nil {
		rec.Fail("%v", err)
	}
	if !info.IsDir() {
		copyFile(from, to, info.Mode().Perm())
		return
	}
	mkdirAll(to)
	entries, err := os.ReadDir(from)
	if err != nil {
		rec.Fail("%v", err)
	}
	for _, entry := range entries {
		copyTree(filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name()))
	}
}

// addSnapshotSkips reports the source directory entries the snapshot left out (links and special
// files, input.symlink_skipped and input.special_skipped).
func (c *conversion) addSnapshotSkips() {
	if c.archive == nil {
		return
	}
	skipped, _ := c.archive.Manifest["skipped"].([]any)
	for _, value := range skipped {
		entry := rec.AsMap(value)
		path := rec.FetchString(entry, "path")
		message := fmt.Sprintf("%s: %s, left out of the snapshot", path, rec.FetchString(entry, "reason"))
		c.diagnostics.Add(rec.FetchString(entry, "code"), message, nil, diagnostics.Place{Key: "path", Value: path})
	}
}
