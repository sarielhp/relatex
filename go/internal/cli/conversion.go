package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/engine"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/papersource"
	"github.com/sarielhp/relatex/internal/pipeline"
	"github.com/sarielhp/relatex/internal/rec"
)

// keptAttempts is ProductCLI::KEPT_ATTEMPTS.
const keptAttempts = 2

// resumedKeys are the configuration keys a resumed run takes from its attempt's configuration.
var resumedKeys = []string{"formats", "jobs", "layout", "synchronize", "source_fonts", "allow_fallback", "compile_typst",
	"verify_browser", "synctex", "layout_viewer", "author_math", "main_ignore", "accessible", "web_report", "split"}

// run is ProductCLI#convert: the options over a resumed attempt's settings and the user's
// configuration, the output, work and cache roots, one conversion and the project cache settled.
func (f *convertFlags) run(rest []string) ojson.Object {
	if f.refused {
		rec.Fail("%s", acceptanceRefusal)
	}
	options := map[string]any{"engine": papersource.DefaultEngine()}
	previous := ojson.Object{}
	if attempt, _ := f.given["resume"].(string); attempt != "" {
		previous = resumedSettings(attempt, options)
	}
	maps.Copy(options, f.given)
	// A format flag selects the formats: a resumed attempt's own list gives way to it.
	if f.given["formats"] == nil && f.given["extra_formats"] != nil {
		delete(options, "formats")
	}
	input := "."
	if len(rest) == 1 {
		input = rest[0]
	} else if attempt, ok := options["resume"].(string); ok {
		input = resumedInput(attempt, previous)
	}
	input = texFileInput(input, options)
	// rlx builds the PDF and its SyncTeX file; Markdown, HTML and JSON are asked for by naming a format.
	options["products_only"] = Name == "rlx" && options["formats"] == nil && options["extra_formats"] == nil
	applyUserConfig(options, input)
	started := time.Now()
	friendly := options["report"] != true && Name == "rlx"
	engine.Quiet = friendly
	f.human = options["report"] != true && Name != "rlx"
	if f.human {
		defer startHuman(inputLabel(input)).finish()
	}
	result := runConversion(options, input)
	if friendly {
		summary = friendlySummary(published, time.Since(started))
	}
	return result
}

// texFileInput turns a .tex file given as the input into its directory with that file as the
// main file (`relatex paper.tex`, as one would call lualatex); a ZIP or directory passes as is.
func texFileInput(input string, options map[string]any) string {
	info, err := os.Stat(input)
	if err != nil || !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(input), ".tex") {
		return input
	}
	name := filepath.Base(input)
	if main, _ := options["main"].(string); main != "" && main != name {
		fail(fmt.Sprintf("--main %s names another file than the input %s", main, name))
	}
	options["main"] = name
	return filepath.Dir(input)
}

// summary replaces the JSON run report on stdout when rlx has something friendlier to say.
var summary string

// resumedSettings merges a resumed attempt's configuration, engine and main file into the
// options and returns its run report.
func resumedSettings(attempt string, options map[string]any) ojson.Object {
	root := realpath(attempt)
	pipeline.ExpandAtRest(root)
	previous, _ := readOrdered(filepath.Join(root, "run.json")).(ojson.Object)
	saved, _ := previous.Get("configuration").(ojson.Object)
	for _, pair := range saved {
		if slices.Contains(resumedKeys, pair.Key()) {
			options[pair.Key()] = pair.Value()
		}
	}
	if previous.Has("engine") {
		options["engine"] = previous.Get("engine")
	}
	options["main"] = resumedMain(root, previous)
	return previous
}

// resumedMain is ProductCLI#resumed_main.
func resumedMain(root string, previous ojson.Object) any {
	if main := previous.Get("main"); main != nil {
		return main
	}
	path := filepath.Join(root, "layers/01-input/input/inventory.json")
	if !regular(path) {
		return nil
	}
	inventory, _ := readOrdered(path).(ojson.Object)
	return inventory.Get("main")
}

// resumedInput is ProductCLI#resumed_input: the attempt's input snapshot when its bytes are
// unchanged, else the original input when it still exists.
func resumedInput(attempt string, previous ojson.Object) string {
	root := realpath(attempt)
	snapshot := filepath.Join(root, "layers/01-input/input/source.zip")
	if regular(snapshot) {
		inventory, _ := readOrdered(filepath.Join(root, "layers/01-input/input/inventory.json")).(ojson.Object)
		if !inventory.Has("zip_sha256") {
			rec.Fail("key not found: \"zip_sha256\"")
		}
		sum := sha256.Sum256(readFile(snapshot))
		if hex.EncodeToString(sum[:]) != inventory.Get("zip_sha256") {
			rec.Fail("Retained resume input bytes changed; supply a ZIP/directory explicitly")
		}
		return snapshot
	}
	if original, ok := previous.Get("input").(string); ok {
		if _, err := os.Stat(original); err == nil {
			return original
		}
	}
	rec.Fail("Resume has no retained input; supply a ZIP/directory explicitly")
	return ""
}

// applyUserConfig is ProductCLI#apply_user_config!: the user's configuration under the flags
// and a resumed run's settings; a format flag such as --html selects the formats (the configured
// ones apply only when the command line names none).
func applyUserConfig(options map[string]any, input string) {
	settings, _ := loadConfig(input)
	configureBrowser(settings)
	formats, ok := options["formats"].([]any)
	extra, _ := options["extra_formats"].([]any)
	if !ok && len(extra) == 0 {
		formats, _ = settings.Get("formats").([]any)
	}
	delete(options, "extra_formats")
	options["formats"] = union(formats, extra)
	if options["output"] == nil && options["product"] == nil {
		options["output"] = expandPath(rec.AsString(settings.Get("output")))
	}
	defaults := map[string]any{"jobs": settings.Get("jobs"), "work_mode": settings.Get("work"),
		"main_ignore": settings.Get("main").(ojson.Object).Get("ignore")}
	for key, value := range defaults {
		if options[key] == nil {
			options[key] = value
		}
	}
	for _, key := range []string{"author_math", "source_fonts"} {
		if _, given := options[key]; !given {
			options[key] = settings.Get(key)
		}
	}
}

// union is Array#|: the values of both lists in order, each once.
func union(lists ...[]any) []any {
	values := []any{}
	for _, list := range lists {
		for _, value := range list {
			if !slices.Contains(values, value) {
				values = append(values, value)
			}
		}
	}
	return values
}

// runConversion is the second half of ProductCLI#convert: the output, work and cache roots
// (the project cache when "work" is "cache"), the conversion, and the project cache settled.
func runConversion(options map[string]any, input string) ojson.Object {
	productsOnly, _ := options["products_only"].(bool)
	places := conversionPlaces(options, input, productsOnly)
	if places.made {
		// A directory the run had to make goes again when it stays empty: after a failure.
		defer os.Remove(places.workspace)
	}
	cached, scratch, roots, output := places.cached, places.scratch, places.roots, places.output
	work, chosen := options["work"].(string)
	if !chosen {
		work = workspacePath(scratch, "_work")
	}
	settled := false
	if cached && !chosen {
		defer retireUnsettled(work, &settled)
	}
	if !within(work, roots) {
		rec.Fail("--work must be within the output root or the project cache")
	}
	checkedStageCache(options, scratch, roots, productsOnly)
	exclusions := slices.Clone(roots)
	if executable, set := os.LookupEnv("RELATEX_EXECUTABLE"); set {
		exclusions = append(exclusions, executable)
	}
	command.Trace("conversion begins")
	result, err := pipeline.Convert(conversionOptions(options, input, output, work, exclusions))
	command.Trace("conversion ends")
	if err != nil {
		panic(err)
	}
	settled = true
	if cached && !chosen {
		if productsOnly && !reportRequested(options) {
			// The attempt of a successful rlx run holds nothing anyone reads: its products are beside the paper.
			pipeline.ReleaseAttempts()
			retireAttempts(work, 0)
		} else {
			settleProject(work, rec.AsString(options["cache"]), scratch)
		}
	}
	command.Trace("project settled")
	return result
}

// runPlaces are the directories of a run: what it delivers to, its product, the directory it owns
// outright and the project scratch, and the roots a --work or --cache must lie within.
type runPlaces struct {
	output, workspace, scratch string
	roots                      []string
	cached, made               bool
}

// conversionPlaces settles the run's directories and records the product in options. The workspace is
// the output directory, or the product when nothing is delivered (the development tools); made says
// the run created it.
func conversionPlaces(options map[string]any, input string, productsOnly bool) runPlaces {
	places := runPlaces{cached: options["work_mode"] == "cache"}
	places.output, _ = options["output"].(string)
	if productsOnly {
		places.output = ""
	}
	// The paper's own directory is never a workspace: the product and the work stay in the project cache
	// (decision 191, amendment of 2026-11-16), and the output root is only where the formats are put.
	paper := places.output != "" && outputHoldsInput(places.output, input)
	if paper {
		places.cached = true
	}
	if places.cached {
		places.scratch = workspaceRoot(projectScratch(input))
	}
	product, _ := options["product"].(string)
	if product == "" && !productsOnly {
		product = defaultProduct(places.output, places.scratch)
	}
	options["product"] = product
	places.workspace = places.output
	if paper {
		places.workspace = ""
	}
	if places.workspace == "" {
		places.workspace = product
	}
	if places.workspace != "" {
		_, missing := os.Stat(places.workspace)
		places.workspace, places.made = workspaceRoot(places.workspace), missing != nil
	}
	if !places.cached {
		places.scratch = places.workspace
	}
	places.roots = []string{places.scratch}
	// An empty product is none (rlx builds no product): expanded, it would be the current directory, the paper itself.
	for _, root := range []string{places.workspace, product} {
		if root == "" {
			continue
		}
		if root = expandPath(root); !within(root, places.roots) {
			places.roots = append(places.roots, root)
		}
	}
	return places
}

// checkedStageCache sets the stage cache: the project's by default, off for a products-only build,
// and a named one only inside the roots.
func checkedStageCache(options map[string]any, scratch string, roots []string, productsOnly bool) {
	if options["cache"] == nil {
		options["cache"] = workspacePath(scratch, "_cache/stages")
		// What rlx builds is its PDF and SyncTeX; the stage cache of a build costs more to write than it saves.
		if productsOnly {
			options["cache"] = "off"
		}
	}
	if options["cache"] == "off" {
		return
	}
	cache := expandPath(rec.AsString(options["cache"]))
	if !within(cache, roots) {
		rec.Fail("--cache must be within the output root or the project cache")
	}
	options["cache"] = cache
}

// retireUnsettled is the ensure of ProductCLI#convert: a failed run in the project cache
// still retires old attempts.
func retireUnsettled(work string, settled *bool) {
	if !*settled {
		retireAttempts(work, keptAttempts)
	}
}

// formatChoices are the formats' own choices: how a book is split.
func formatChoices(o *pipeline.Options, options map[string]any) {
	if mode, ok := options["split"].(string); ok {
		o.Split = mode
	}
}

// conversionOptions are GeneralConversion.new's arguments from the command's options.
func conversionOptions(options map[string]any, input, output, work string, exclusions []string) pipeline.Options {
	for _, key := range []string{"verification_profile", "authored_proof", "frozen_pdf"} {
		if options[key] != nil {
			rec.Fail("%s", acceptanceRefusal)
		}
	}
	o := pipeline.Defaults()
	o.Root, o.Input, o.Output, o.Work = Root, input, output, work
	o.Records = filepath.Join(projectScratch(input), "outputs")
	o.Product, _ = options["product"].(string)
	o.Main, _ = options["main"].(string)
	o.Engine, _ = options["engine"].(string)
	o.Formats = textList(options["formats"])
	o.ProductsOnly, _ = options["products_only"].(bool)
	o.Jobs, _ = options["jobs"].(int64)
	flags := map[string]*bool{"layout": &o.Layout, "synchronize": &o.Synchronize, "source_fonts": &o.SourceFonts,
		"compile_typst": &o.CompileTypst, "verify_browser": &o.VerifyBrowser, "layout_viewer": &o.LayoutViewer,
		"accessible": &o.Accessible}
	for key, target := range flags {
		if value, ok := options[key].(bool); ok {
			*target = value
		}
	}
	// The word SyncTeX comes from TeX's glyph map and the source map; the layout (mutool's traces and
	// page SVGs, for the renderings) is not needed to write it.
	if o.ProductsOnly {
		o.Layout = false
	}
	o.AllowFallback, _ = options["allow_fallback"].(string)
	o.WebReport, _ = options["web_report"].(string)
	if synctex, ok := options["synctex"].(string); ok {
		o.Synctex = synctex
	}
	// Run as rlx, relatex works like latexmk: it writes the PDF, a word-level SyncTeX file and the
	// bibliography beside the main file. As relatex it leaves them in the output directory unless
	// --publish says otherwise.
	publish := Name == "rlx"
	if value, ok := options["publish"].(bool); ok {
		publish = value
	}
	o.Publish = &publish
	if _, chosen := options["synctex"].(string); !chosen && publish {
		o.Synctex = "word"
	}
	// Like lualatex, relatex reads the caller's TEXINPUTS (decision 103), unless the run is hermetic.
	if hermetic, _ := options["hermetic"].(bool); !hermetic {
		o.TexInputs = os.Getenv("TEXINPUTS")
		o.BstInputs = os.Getenv("BSTINPUTS")
		o.BibInputs = os.Getenv("BIBINPUTS")
	}
	o.SnapshotExclusions = exclusions
	o.Cache = rec.AsString(options["cache"])
	o.Resume, _ = options["resume"].(string)
	if force, ok := options["force"].([]any); ok {
		o.Force = textList(force)
	}
	if authorMath, ok := options["author_math"].(bool); ok {
		o.AuthorMath = &authorMath
	}
	if ignore, ok := options["main_ignore"].([]any); ok {
		o.MainIgnore = textList(ignore)
	}
	formatChoices(&o, options)
	o.Capabilities = capabilityReport(true)
	if conversionHuman != nil {
		o.Started = func(directory string) { conversionHuman.attempt = directory }
	} else if report, _ := options["report"].(bool); report || Name != "rlx" {
		o.Started = func(directory string) { warn(messages, "Conversion attempt: "+directory) }
	}
	o.Published = func(paths []string) { published = paths }
	o.Unchanged = func() { upToDate = true }
	if adjustConversion != nil {
		adjustConversion(&o)
	}
	return o
}

// adjustConversion, when set, changes the conversion a command runs after its options are read
// (a command built on a conversion: the environment it reproduces and the hooks that keep what it built).
var adjustConversion func(*pipeline.Options)

// textList is a list of strings from a JSON or option list; nil entries become "".
func textList(value any) []string {
	list, _ := value.([]any)
	texts := make([]string, 0, len(list))
	for _, item := range list {
		text, _ := item.(string)
		texts = append(texts, text)
	}
	return texts
}

// settleProject is ProductCLI#settle_project: after a successful run in the project cache, old
// attempts go and the kept ones and the stage cache are compressed.
func settleProject(work, cache, root string) {
	defer uncompressed()
	attempts := retireAttempts(work, keptAttempts)
	pipeline.CompressAtRest(attempts)
	if cache != "off" && within(cache, []string{root}) {
		pipeline.NewStageCache(cache, "", filepath.Join(Root, "tex")).Settle(keptAttempts)
	}
}

// uncompressed reports a settling failure, which leaves the run itself successful.
func uncompressed() {
	recovered := recover()
	if recovered == nil {
		return
	}
	switch typed := recovered.(type) {
	case rec.Failure:
		warn(messages, "Project cache left uncompressed: "+typed.Message)
	case systemFailure:
		warn(messages, "Project cache left uncompressed: "+typed.message)
	default:
		panic(recovered)
	}
}

// retireAttempts is ProductCLI#retire_attempts: the newest attempts stay, the rest go. An attempt
// another process still works in is neither retired, compressed nor counted (pipeline.AttemptActive).
func retireAttempts(work string, keep int) []string {
	matches, _ := filepath.Glob(filepath.Join(work, "conversion-*"))
	type attempt struct {
		path     string
		modified time.Time
	}
	attempts := []attempt{}
	for _, path := range matches {
		info, err := os.Lstat(path)
		if err == nil && info.IsDir() && !pipeline.AttemptActive(path) {
			attempts = append(attempts, attempt{path, info.ModTime()})
		}
	}
	slices.SortStableFunc(attempts, func(a, b attempt) int { return b.modified.Compare(a.modified) })
	kept := []string{}
	for index, attempt := range attempts {
		if index < keep {
			kept = append(kept, attempt.path)
			continue
		}
		if err := os.RemoveAll(attempt.path); err != nil {
			panic(systemCall(err))
		}
		_ = os.Remove(pipeline.AttemptLockPath(attempt.path))
	}
	return kept
}

// unsafeName are the characters a project scratch name replaces.
var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// defaultProduct is the directory of the full product when none is named: beside the project's work in
// the cache, or, when "work" is "output", in the output directory as relatex's own _product.
func defaultProduct(output, scratch string) string {
	if scratch != "" {
		return filepath.Join(scratch, "product")
	}
	return filepath.Join(output, "_product")
}

// lastProduct is where the last conversion of a paper directory kept its full product: the project
// cache, or output/_product when "work" is "output".
func lastProduct(directory string, settings ojson.Object) string {
	if settings.Get("work") == "output" {
		if output := expandPath(rec.AsString(settings.Get("output"))); !outputHoldsInput(output, directory) {
			return filepath.Join(output, "_product")
		}
	}
	return filepath.Join(projectScratch(directory), "product")
}

// outputHoldsInput is whether the output root is the paper's directory or a directory above it: the
// default, where the formats go beside the paper. That root is not a workspace and has no _product.
func outputHoldsInput(output, input string) bool {
	paper := realpath(input)
	if info, err := os.Stat(paper); err != nil || !info.IsDir() {
		paper = filepath.Dir(paper)
	}
	root := expandPath(output) // absolute: -o . is the paper's directory too; it may not exist yet
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return fspath.Within(root, paper)
}

// projectScratch is ProductCLI#project_scratch: one directory per input under the cache's
// projects, named for it and keyed by its real path. RELATEX_PROJECTS_DIR moves the projects
// root: the cohort harness puts it in a run's scratch, deleted with the run (decision 090).
func projectScratch(input string) string {
	resolved := realpath(input)
	sum := sha256.Sum256([]byte(resolved))
	name := unsafeName.ReplaceAllString(filepath.Base(resolved), "_")
	root := os.Getenv("RELATEX_PROJECTS_DIR")
	if root == "" {
		root = filepath.Join(command.CacheRoot(), "projects")
	}
	return filepath.Join(root, name+"-"+hex.EncodeToString(sum[:])[:12])
}

// workspacePath is RunWorkspace#path: a path within the root that passes no symlink.
func workspacePath(root, relative string) string {
	candidate := expandPath(filepath.Join(root, relative))
	if candidate == root {
		return root
	}
	if !fspath.Inside(root, candidate) {
		rec.Fail("Workspace path escapes root")
	}
	current := root
	for _, part := range strings.Split(fspath.Name(root, candidate), "/") {
		current = filepath.Join(current, part)
		if info, err := os.Lstat(current); err == nil && info.Mode()&os.ModeSymlink != 0 {
			rec.Fail("Workspace path contains a symlink")
		}
	}
	return candidate
}

// within is ProductCLI#within?.
func within(path string, roots []string) bool {
	expanded := expandPath(path)
	for _, root := range roots {
		if fspath.Within(root, expanded) {
			return true
		}
	}
	return false
}

// upToDate is whether the last conversion found its products already made from these sources.
var upToDate bool

// published is what the last conversion copied beside the main file; rlx reports it instead of the run report.
var published []string

// friendlySummary is rlx's output: the files it wrote, relative to where it was run when they are
// under it, and how long it took. "" when nothing was published.
func friendlySummary(paths []string, elapsed time.Duration) string {
	if len(paths) == 0 {
		return ""
	}
	directory, _ := os.Getwd()
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		if relative, err := filepath.Rel(directory, path); err == nil && !strings.HasPrefix(relative, "..") {
			path = relative
		}
		names = append(names, path)
	}
	if upToDate {
		return fmt.Sprintf("up to date: %s", strings.Join(names, ", "))
	}
	return fmt.Sprintf("wrote %s (%s)", strings.Join(names, ", "), elapsed.Round(time.Second))
}

// reportRequested is whether --report asked for the run report (and the attempt behind it).
func reportRequested(options map[string]any) bool {
	report, _ := options["report"].(bool)
	return report
}
