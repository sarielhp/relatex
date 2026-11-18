package pipeline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/artifacts"
	"github.com/sarielhp/relatex/internal/authormath"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/engine"
	"github.com/sarielhp/relatex/internal/epub"
	"github.com/sarielhp/relatex/internal/exportwriter"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/input"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/texsandbox"
	"github.com/sarielhp/relatex/internal/typstsource"
)

// Options are GeneralConversion.new's arguments: the input, output, work directory, main file
// and engine, and the conversion options (GeneralConversion::DEFAULTS).
type Options struct {
	Root  string // the repository root: the collector, schemas, profiles and resources
	Input string
	// Output is the directory the requested formats are written to (decision 191); "" writes none.
	Output string
	// Records is the project cache directory that holds, per output root, the list of the entries relatex
	// wrote there and a failed run's leftovers, so the output root itself holds only the formats (the
	// amendment of 2026-11-16 to decision 191). The work directory's _records when empty.
	Records string
	// Product is the directory of the full product: the document, layout, records and every format
	// (the stage and reuse files the next conversion reads). The project cache by default.
	Product string
	Work    string
	Main    string // "" when not given
	Engine  string

	Formats       []string
	Jobs          int64
	Layout        bool
	LayoutViewer  bool // write layout.html, the page graphics with a button per word, and its scripts
	Synchronize   bool
	SourceFonts   bool
	AllowFallback string // "" is nil
	CompileTypst  bool
	VerifyBrowser bool
	// Accessible is the accessibility conversion (decision 112): it writes accessibility.html, the report page.
	Accessible bool
	// WebReport is an earlier conversion's accessibility.json: the web claim when this builds no html.
	WebReport          string
	Synctex            string
	ProductsOnly       bool  // build the PDF and its SyncTeX file only: no document, exports or output directory
	Publish            *bool // nil: copy the products beside the main file when the input is a directory
	SnapshotExclusions []string
	Cache              string // "auto", "off", or a directory
	Resume             string // "" is nil
	Force              []string
	AuthorMath         *bool // nil: when Markdown, HTML or EPUB is written (decisions 064, 109)
	MainIgnore         []string
	Split              string // chapters: a book as one file per chapter (decision 081); none: one page
	// Host, when given, is the author's own build the engine reproduces: TEXINPUTS and time zone (decision 101).
	Host *engine.Host
	// TexInputs is the caller's TEXINPUTS ("" for none, or a hermetic run). A directory input builds with
	// it, its relative entries read from the main file's directory as lualatex would there (decision 103);
	// an archive builds as shipped.
	TexInputs string
	// BstInputs is the caller's BSTINPUTS ("" for none, or a hermetic run): the Typst bibliography's style is
	// looked up along it as BibTeX finds a .bst (decision 172).
	BstInputs string
	// BibInputs is the caller's BIBINPUTS ("" for none, or a hermetic run): BibTeX finds the paper's databases there.
	BibInputs string

	// Capabilities is Capabilities.report(public: true), the bundle's capabilities.
	Capabilities any

	// Started, when given, hears the attempt directory before the run begins (ProductCLI's
	// "Conversion attempt" line).
	Started func(directory string)

	// Published, when given, hears the files copied beside the main file.
	Published func(paths []string)

	// Unchanged, when given, hears that the products on disk were already those of these sources.
	Unchanged func()

	// Products, when given, hears what a products-only run built, before anything is published.
	Products func(Built)
}

// Built is what a products-only run built in its attempt: the PDF, its SyncTeX file ("" when none)
// and the source files it read, each path (relative to the input) with its SHA-256.
type Built struct {
	PDF, Synctex string
	Sources      map[string]string
}

// Defaults are GeneralConversion::DEFAULTS.
func Defaults() Options {
	return Options{Formats: []string{"markdown", "html"}, Jobs: 2, Layout: true, Synchronize: true, SourceFonts: true, Synctex: "none",
		SnapshotExclusions: []string{}, Cache: "auto", Force: []string{}, Split: "chapters"}
}

// conversion is one GeneralConversion.
type conversion struct {
	options Options
	// webClaim is the web page's WCAG claim (accessibility.json), "" when it reaches none: the
	// tagged PDF keeps its PDF/UA-2 identification only beside it (decision 112).
	webClaim   string
	authorMath bool
	input      string
	output     string
	product    string
	delivered  []string // the paths written to the output directory
	directory  string
	workspace  string
	resume     string
	forced     []string
	cacheDir   string
	cache      *StageCache
	pipeline   *layers

	archive      *input.Archive
	probe        string
	pdf          string
	prefixes     ojson.Object
	degradations []any
	document     rec.Map
	layout       rec.Map
	sourceMap    rec.Map
	// chapterPages are the pages the parts and chapters begin on, where the layout is cut into files.
	chapterPages []int64
	// pageList is where each page of the PDF begins in the reading order, for the EPUB page list.
	pageList    []epub.PageMark
	exportRoot  string
	exports     ojson.Object
	diagnostics *diagnostics.Diagnostics
	// bibliography is the typst-source writer's bibliography inputs, read once (decision 172).
	bibliography *typstsource.Bibliography
}

// Convert is GeneralConversion.new(input, output:, work:, main:, engine:, **options).run: the
// run report (also run.json), or the failure, with the report written as failed.
func Convert(options Options) (report ojson.Object, err error) {
	defer recovered(&err)
	c := newConversion(options)
	if options.Started != nil {
		options.Started(c.directory)
	}
	previous := command.Workspace
	command.Workspace = c.workspace
	defer func() { command.Workspace = previous }()
	return c.execute(), nil
}

// recovered turns a failure into the error Convert returns.
func recovered(err *error) {
	value := recover()
	if value == nil {
		return
	}
	if failure, ok := value.(rec.Failure); ok {
		*err = failure
		return
	}
	if failure, ok := value.(error); ok {
		*err = failure
		return
	}
	panic(value)
}

// authorMathDefault is whether author math runs when no switch says: Markdown ships the author's
// TeX, HTML and EPUB carry the intent read from it (decisions 064 and 109).
func authorMathDefault(formats []string) bool {
	return slices.ContainsFunc(formats, func(format string) bool {
		return format == "markdown" || format == "html" || format == "epub"
	})
}

func newConversion(options Options) *conversion {
	c := &conversion{options: options, prefixes: ojson.Object{}, degradations: []any{}}
	c.validate()
	c.authorMath = authorMathDefault(options.Formats)
	if options.AuthorMath != nil {
		c.authorMath = *options.AuthorMath
	}
	setRoots(options.Root)
	c.input = realPath(options.Input)
	if options.Output != "" {
		if c.options.Records == "" {
			// A caller without a project cache keeps the records in the run's own work directory.
			c.options.Records = filepath.Join(absolutePath(options.Work), "_records")
		}
		c.output = absolutePath(options.Output)
		c.checkOutputEarly()
	}
	if options.Product != "" {
		c.product = absolutePath(options.Product)
		checkOutput(c.product)
	}
	work := absolutePath(options.Work)
	mkdirAll(work)
	c.directory = command.MakeTemp(work, "conversion-")
	holdAttempt(c.directory)
	c.workspace = newWorkspace(c.directory)
	if options.Resume != "" {
		c.resume = realPath(options.Resume)
		ExpandAtRest(c.resume)
	}
	c.forced = forced(options.Force)
	if options.Cache != "" && options.Cache != "off" {
		c.cacheDir = filepath.Join(work, "_stage-cache")
		if options.Cache != "auto" {
			c.cacheDir = absolutePath(options.Cache)
		}
		c.cache = NewStageCache(c.cacheDir, kpsewhichFor(options.Engine), filepath.Join(options.Root, "tex"))
	}
	resume := ""
	if c.resume != "" {
		resume = filepath.Join(c.resume, "layers")
	}
	c.pipeline = newLayers(filepath.Join(c.directory, "layers"), options.Root, c.cache, resume, c.forced)
	return c
}

// setRoots names the repository root every stage package reads its resources from.
func setRoots(root string) {
	exportwriter.SetRoots(root)
	engine.Root, authormath.Root = root, root
}

func realPath(path string) string {
	resolved, err := fspath.Real(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	return absolutePath(resolved)
}

func (c *conversion) validate() {
	options := c.options
	if !slices.Contains([]string{"native", "word", "none"}, options.Synctex) {
		rec.Fail("Unsupported synchronization output mode")
	}
	if options.Synctex == "word" && !options.Synchronize {
		rec.Fail("Word SyncTeX requires synchronization")
	}
	valid := len(options.Formats) > 0
	for _, format := range options.Formats {
		valid = valid && slices.Contains(exportwriter.Formats, format)
	}
	if !valid {
		rec.Fail("Invalid conversion formats")
	}
	if options.VerifyBrowser && !slices.Contains(options.Formats, "html") {
		rec.Fail("Browser verification requires HTML among selected formats")
	}
}

// execute is GeneralConversion#execute: the stages in order, then the generation summary.
func (c *conversion) execute() (report ojson.Object) {
	defer c.failed()
	c.report("running", nil)
	if c.options.ProductsOnly {
		return c.executeProducts()
	}
	c.inputStage()
	c.engineStage()
	c.reconcileStage()
	if c.options.Layout {
		c.layoutStage()
	}
	if c.options.Synchronize {
		c.synchronizationStage()
	}
	c.exportStage()
	c.publishStage()
	return c.report(c.summaryStatus(), c.summary())
}

// executeProducts is the run of a build that wants only the PDF and its SyncTeX file (rlx): the
// engine and, when synchronization is requested, the source map; no layout, document, export or
// output directory.
func (c *conversion) executeProducts() ojson.Object {
	c.inputStage()
	if c.productsUpToDate() {
		return c.report("complete", ojson.Object{})
	}
	c.engineStage()
	if c.options.Synchronize {
		c.synchronizationStage()
	}
	c.productsStage()
	return c.report("complete", c.summary())
}

// failed reports a failed conversion and raises its failure with the layer and the
// evidence directory.
func (c *conversion) failed() {
	value := recover()
	if value == nil {
		return
	}
	message := failureMessage(value)
	if _, isFailure := value.(rec.Failure); !isFailure {
		if _, isError := value.(error); !isError {
			panic(value)
		}
	}
	c.report("failed", ojson.Object{{"error", message}})
	if !c.options.ProductsOnly {
		if c.product != "" {
			setAsideProducts(c.product, message)
		}
		if c.output != "" {
			if owned := c.owned(); len(owned) > 0 && setAsideNames(c.output, c.outputRecords(), owned, message) {
				message += "\nEarlier results and the failure note: " + c.outputRecords()
			}
		}
	}
	var layer any
	if count := len(c.pipeline.stages); count > 0 {
		layer = c.pipeline.stages[count-1].Get("layer")
	}
	// The message reads first, however many lines it has; the evidence is always the last line.
	rec.Fail("Conversion failed in %s: %s\nEvidence: %s", rec.ToS(layer), message, c.directory)
}

func (c *conversion) inputStage() {
	inputs := ojson.Object{}
	if isFile(c.input) {
		inputs = ojson.Object{{"input", c.input}}
	}
	ignore := c.options.MainIgnore
	if ignore == nil {
		ignore = input.Companions
	}
	configuration := ojson.Object{{"main", nilString(c.options.Main)}, {"main_ignore", stringList(ignore)}}
	c.pipeline.stage("input", inputs, configuration, false, func(root string) any {
		exclusions := append(slices.Clone(c.options.SnapshotExclusions), c.directory)
		exclusions = append(exclusions, c.outputExclusions()...)
		if c.product != "" {
			exclusions = append(exclusions, c.product)
		}
		if sidecar := c.altTextPath(); sidecar != "" {
			exclusions = append(exclusions, sidecar)
		}
		if c.cacheDir != "" {
			exclusions = append(exclusions, c.cacheDir)
		}
		c.archive = input.Capture(c.input, root, c.options.Main, exclusions, ignore)
		return c.archive.Manifest
	})
}

// host is the author's build the engine reproduces: the one a command gave (relatex synctex), else the
// caller's TEXINPUTS read from the main file's directory when the input is a directory, as lualatex run
// there reads it (decision 103). An archive builds as shipped. Resolved once, after the input stage.
func (c *conversion) host() *engine.Host {
	o := &c.options
	if o.Host != nil || (o.TexInputs == "" && o.BibInputs == "") || c.archive == nil {
		return o.Host
	}
	texinputs, bibinputs := o.TexInputs, o.BibInputs
	o.TexInputs, o.BibInputs = "", ""
	if directory := c.inputDirectory(); directory != "" {
		main := filepath.FromSlash(rec.AsString(c.archive.Manifest["main"]))
		where := filepath.Join(directory, filepath.Dir(main))
		o.Host = engine.WithBibliography(engine.SearchHost(texinputs, where), bibinputs, o.BstInputs, where)
		if o.Host != nil && o.Host.Given != "" && !engine.Quiet {
			fmt.Fprintf(os.Stderr, "Engine: TEXINPUTS %s (the caller's; --hermetic builds without it)\n", o.Host.Given)
		}
	}
	return o.Host
}

// sourceDate is the build date the engine takes from the environment: SOURCE_DATE_EPOCH, which with
// FORCE_SOURCE_DATE sets \today and \time. A build with another date typesets other text, so it
// never shares a cache entry or an up-to-date stamp with this one (the cohort pins each paper to its
// reference date, while the release smoke converts the same papers on the run date).
func sourceDate() ojson.Object {
	date := ojson.Object{}
	for _, name := range []string{"SOURCE_DATE_EPOCH", "FORCE_SOURCE_DATE"} {
		if value, ok := os.LookupEnv(name); ok {
			date = append(date, ojson.Pair{name, value})
		}
	}
	return date
}

func nilString(text string) any {
	if text == "" {
		return nil
	}
	return text
}

func (c *conversion) engineStage() {
	zip := filepath.Join(c.archive.Directory, "source.zip")
	doctor := texsandbox.Current(c.options.Engine)
	configuration := ojson.Object{{"engine", doctor.Report}, {"jobs", c.options.Jobs}, {"identity", engine.SourceIdentity(c.archive.Manifest)}}
	if c.authorMath {
		configuration = append(configuration, ojson.Pair{"author_math", true})
	}
	options := engine.Options{Archive: c.archive.Directory, Engine: c.options.Engine, Jobs: c.options.Jobs, AuthorMath: c.authorMath,
		ExternalOnly: c.options.ProductsOnly, Host: c.host()}
	if c.options.ProductsOnly {
		configuration = append(configuration, ojson.Pair{"external_only", true})
	}
	configuration = append(configuration, ojson.Pair{"time_zone", engine.TimeZone()})
	if date := sourceDate(); len(date) > 0 {
		configuration = append(configuration, ojson.Pair{"source_date", date})
	}
	if host := c.host(); host != nil {
		configuration = append(configuration, ojson.Pair{"host", ojson.Object{{"texinputs", strings.Join(host.SearchPath, ":")},
			{"files", host.Fingerprint()}}})
	}
	c.pipeline.stage("engine", ojson.Object{{"zip", zip}}, configuration, true, func(root string) any {
		return engine.Run(options, root)
	})
	root := c.pipeline.path("engine")
	products := c.readStageJSON("engine", "engine-products.json")
	c.probe = filepath.Join(root, artifacts.Relative(rec.FetchString(products, "probe")))
	c.pdf = c.stageArtifact("engine", rec.FetchString(products, "pdf"))
	degradations, _ := products["degradations"].([]any)
	for _, value := range degradations {
		row := rec.AsMap(value)
		fmt.Fprintf(os.Stderr, "Engine: %s; %s\n", rec.ToS(rec.Fetch(row, "reason")), rec.ToS(rec.Fetch(row, "effect")))
		c.degradations = append(c.degradations, row)
	}
	cache, _ := c.pipeline.stages[len(c.pipeline.stages)-1].Get("cache").(ojson.Object)
	if producer, ok := cache.Get("producer_directory").(string); ok && producer != root {
		c.prefixes = c.prefixes.Set(producer, root)
	}
}

// stageArtifact is GeneralConversion#stage_artifact: a completed stage's inventoried output.
func (c *conversion) stageArtifact(name, relative string) string {
	receipt := c.pipeline.receipt(name)
	if receipt == nil {
		rec.Fail("Stage has no completed receipt")
	}
	outputs, ok := receipt.Get("outputs").(ojson.Object)
	if !ok {
		panic(rec.Failure{Message: `key not found: "outputs"`, Lookup: true})
	}
	files := map[string]string{}
	for _, pair := range outputs {
		files[pair.Key()] = rec.AsString(pair.Value())
	}
	return artifacts.New(c.pipeline.path(name), files, nil).Resolve(relative)
}

func (c *conversion) readStageJSON(name, relative string) rec.Map {
	return readMap(c.stageArtifact(name, relative))
}

func readMap(path string) rec.Map {
	value := rec.AsMap(ojson.Plain(readPlain(path)))
	return value
}

func (c *conversion) prefixMap() map[string]string {
	result := map[string]string{}
	for _, pair := range c.prefixes {
		result[pair.Key()] = rec.AsString(pair.Value())
	}
	return result
}

// errorsAs reports whether a recovered value is a Relatex::Failure (Ruby's rescue Failure).
func isFailure(value any) (rec.Failure, bool) {
	failure, ok := value.(rec.Failure)
	if ok {
		return failure, true
	}
	if err, isError := value.(error); isError {
		var wrapped rec.Failure
		if errors.As(err, &wrapped) {
			return wrapped, true
		}
	}
	return rec.Failure{}, false
}

// ownedEntry is whether a name in the output directory is a product of an earlier conversion:
// relatex's own directories (_work, _cache, ...), hidden files and the run-* directories of the
// older layout are never replaced.
func ownedEntry(name string) bool {
	return !strings.HasPrefix(name, "_") && !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "run-")
}

// checkOutput refuses an output directory that holds something else than a relatex output: it
// must be new, empty, hold an earlier conversion (its manifest.json), or hold only relatex's own
// directories and the run-* directories of the older layout.
func checkOutput(output string) {
	info, err := os.Lstat(output)
	if err != nil {
		return
	}
	if !info.IsDir() {
		rec.Fail("Conversion output is not a directory: %s", output)
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		rec.Fail("%v", err)
	}
	for _, entry := range entries {
		if entry.Name() == "manifest.json" {
			return
		}
	}
	for _, entry := range entries {
		if ownedEntry(entry.Name()) {
			rec.Fail("Conversion output is not empty and not a relatex output (%s holds %s)", output, entry.Name())
		}
	}
}

// publishInto makes the staged conversion the content of output: a new output directory is the
// staged one; an existing one loses the products of the earlier conversion and takes the staged
// entries, leaving relatex's own directories alone.
func publishInto(staged, output string) {
	if _, err := os.Lstat(output); err != nil {
		rename(staged, output)
		return
	}
	clearStale(output)
	old, err := os.ReadDir(output)
	if err != nil {
		rec.Fail("%v", err)
	}
	for _, entry := range old {
		if ownedEntry(entry.Name()) {
			removeAll(filepath.Join(output, entry.Name()))
		}
	}
	fresh, err := os.ReadDir(staged)
	if err != nil {
		rec.Fail("%v", err)
	}
	for _, entry := range fresh {
		rename(filepath.Join(staged, entry.Name()), filepath.Join(output, entry.Name()))
	}
}
