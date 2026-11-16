package cli

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/pipeline"
	"github.com/sarielhp/relatex/internal/split"
)

// convertDescription and convertLong are the convert command's help (ProductCLI::CONVERT_BANNER).
const convertDescription = "Convert a paper (a ZIP, a directory or a .tex file) into Markdown, HTML and the other formats"

const convertLong = "Defaults: input '.', Markdown and HTML, output/ holding only the formats asked for; the full product is kept in the project cache\n" +
	"Main file: --main, else the only top-level .tex besides prefix.tex and prelim.tex, else the only .tex that begins a document\n" +
	"TeX search path: your TEXINPUTS, read from the main file's directory as lualatex reads it (--hermetic, or an archive input: the paper's own files and the TeX tree only)"

// convertFlags is what a convert command line asked for: the options as the configuration reads
// them (only the ones given), and whether it named an option of the archived acceptance suite.
type convertFlags struct {
	given   map[string]any
	refused bool
	// human is a conversion that prints for a person (the JSON run report is --report).
	human bool
}

func newConvertFlags() *convertFlags { return &convertFlags{given: map[string]any{}} }

// set returns the handler of an option that stores one string under key.
func (f *convertFlags) set(key string) func(string) error {
	return func(value string) error { f.given[key] = value; return nil }
}

// addFormats appends formats to the extra formats, in the order the options come.
func (f *convertFlags) addFormats(formats ...string) {
	extra, _ := f.given["extra_formats"].([]any)
	for _, format := range formats {
		extra = append(extra, format)
	}
	f.given["extra_formats"] = extra
}

// spec is the convert command; the root program has the same options and work.
func (f *convertFlags) spec(replay bool) commandSpec {
	return commandSpec{name: "convert", description: convertDescription, long: convertLong,
		usage: Name + " convert [ZIP_OR_DIRECTORY] [options]", options: f.options(replay),
		args: clihelp.MaximumNArgs(1), run: f.run}
}

// replaySpec is relatex replay ATTEMPT: convert --resume ATTEMPT.
func (f *convertFlags) replaySpec() commandSpec {
	return commandSpec{name: "replay", description: "Continue the conversion of an earlier attempt, reusing its finished stages",
		long: "Continues the conversion of an earlier attempt directory (the path printed as \"Conversion attempt:\")\n" +
			"with its settings, reusing its finished stages; the same as relatex convert --resume ATTEMPT.\n" +
			"Options as for convert.",
		usage: Name + " replay ATTEMPT [--force STAGES] [options]", options: f.options(true), args: clihelp.ExactArgs(1),
		run: func(words []string) ojson.Object {
			f.given["resume"] = words[0]
			return f.run(nil)
		}}
}

// options are the convert options; replay takes the attempt as its word, not as --resume.
func (f *convertFlags) options(replay bool) []clihelp.Option {
	list := []clihelp.Option{
		valueFlag("-o, --output <dir>", "Where the requested formats go (default: the current directory), each in its own entry: html/, typst/, markdown/, NAME.epub, NAME-tagged.pdf; with -o DIR they go in DIR; "+
			"relatex replaces what it wrote there before and touches nothing else", f.set("output")),
		valueFlag("--product <dir>", "Write the full product here (document, layout, records and every format; for the development tools) "+
			"instead of the project cache, and no output directory unless -o is given", f.set("product")),
		valueFlag("--work <dir>", "This attempt's directory, inside the output directory or the project cache", f.set("work")),
		valueFlag("--main <file>", "The main .tex file, a path inside the paper", f.set("main")),
		valueFlag("--engine <path>", "The lualatex to run (default RELATEX_ENGINE, else ~/.texlive's, else lualatex on PATH)",
			f.set("engine")),
		integerFlag("--jobs <n>", "Parallel LaTeX builds, 1 to 8 (default 2)", func(value int64) { f.given["jobs"] = value }),
		valueFlag("--cache <dir>", "Stage cache, inside the output directory or the project cache, or off", f.set("cache")),
		switchFlag("--no-cache", "Run every stage afresh and write no stage cache", func() { f.given["cache"] = "off" }),
		listFlag("--force <stages>", "Rerun these stages and the ones after them: input, engine, reconcile, layout, synchronize, "+
			"export, publish, or export.FORMAT", func(list []string) { f.given["force"] = names(list) }),
		listFlag("--format <formats>", "Write these formats (comma-separated) instead of the configured ones",
			func(list []string) { f.given["formats"] = names(list) }),
	}
	if !replay {
		list = append(list, valueFlag("--resume <attempt>", "Continue an earlier attempt directory with its settings and finished stages",
			f.set("resume")))
	}
	list = append(list, f.formatSwitches()...)
	list = append(list, f.synctexOptions()...)
	list = append(list, f.outputOptions()...)
	list = append(list, f.contentOptions()...)
	return append(list, f.acceptanceOptions()...)
}

// formatSwitches are the options that add one format each.
func (f *convertFlags) formatSwitches() []clihelp.Option {
	list := []clihelp.Option{}
	for _, flag := range [][2]string{{"--markdown", "markdown"}, {"--typst", "typst-source"}, {"--typst-layout", "typst"},
		{"--html", "html"}, {"--json", "json"}, {"--json-gz", "json.gz"}, {"--pdf", "pdf"}, {"--epub", "epub"}} {
		format := flag[1]
		list = append(list, switchFlag(flag[0], "Generate "+format, func() { f.addFormats(format) }))
	}
	return list
}

// synctexOptions are the SyncTeX file's: --synctex is the most precise file, built from the word map
// (word); --synctex-native keeps TeX's own records; --no-synctex (the default) writes none.
func (f *convertFlags) synctexOptions() []clihelp.Option {
	return []clihelp.Option{
		toggleFlag("--[no-]synctex", "SyncTeX file beside the PDF from the word map, the most precise; --no-synctex writes none (default)",
			func(value bool) { f.given["synctex"] = map[bool]string{true: "word", false: "none"}[value] }),
		switchFlag("--synctex-native", "SyncTeX file as TeX recorded it, retargeted to the packaged sources",
			func() { f.given["synctex"] = "native" }),
	}
}

// outputOptions are the options of what is written besides the formats.
func (f *convertFlags) outputOptions() []clihelp.Option {
	return []clihelp.Option{
		toggleFlag("--[no-]publish", "Copy the PDF, .synctex.gz and .bbl beside the main file, as latexmk does (the default when run as rlx)",
			func(value bool) { f.given["publish"] = value }),
		switchFlag("--hermetic", "Build without the caller's TEXINPUTS: only the paper's own files and the TeX tree "+
			"(an archive always builds so)", func() { f.given["hermetic"] = true }),
		toggleFlag("--[no-]report", "Print the JSON run report on stdout, as tools read it, instead of the one-line result",
			func(value bool) { f.given["report"] = value }),
		optionalFlag("--pdf-ua [route]", "Also build the tagged PDF (PDF/UA-2), validated by veraPDF when installed: "+
			"the ordinary PDF tagged from relatex's model (route, auto or route2, names that one route)", "auto",
			func(route string) error {
				if !slices.Contains(pipeline.PDFUARoutes, route) {
					return fmt.Errorf("the route is one of [%s]", strings.Join(pipeline.PDFUARoutes, ", "))
				}
				f.addFormats("pdf-ua")
				return nil
			}),
		switchFlag("--accessible", "An accessibility conversion: HTML, EPUB and the tagged PDF (as --pdf-ua), the page measured "+
			"in a browser (--verify-browser), and accessibility.html, the report to keep: the claim each output reaches or why not, "+
			"the open items with their source lines, where the descriptions came from", func() {
			f.addFormats("html", "epub", "pdf-ua")
			f.given["verify_browser"], f.given["accessible"] = true, true
		}),
		valueFlag("--web-report <file>", "accessibility.json of the same document, from an earlier conversion: the web claim "+
			"the tagged PDF's PDF/UA-2 identification is judged against when this conversion builds no html", f.set("web_report")),
		switchFlag("--all", "Generate every format but the separate builds: "+strings.Join(everyFormatName(), ", "),
			func() { f.given["formats"] = everyFormat() }),
		switchFlag("--layout-viewer", "Also write layout.html, the page graphics with a button per word (for development)",
			func() { f.given["layout_viewer"] = true }),
		choiceFlag("--split <mode>", "A book (a document with \\chapter) as a contents page and one file per chapter (chapters, the default) "+
			"or one page (none)", split.Modes, func(value string) { f.given["split"] = value }),
		switchFlag("--no-layout", "Skip the page graphics; also skips the word-to-source map",
			func() { f.given["layout"], f.given["synchronize"] = false, false }),
		switchFlag("--no-sync", "Skip the word-to-source map", func() { f.given["synchronize"] = false }),
	}
}

func (f *convertFlags) contentOptions() []clihelp.Option {
	return []clihelp.Option{
		toggleFlag("--[no-]source-fonts", "Link the paper's own fonts in HTML and Markdown (default); --no-source-fonts uses browser fonts",
			func(value bool) { f.given["source_fonts"] = value }),
		// Decision 064: on by default when Markdown, HTML or EPUB is written.
		toggleFlag("--[no-]author-math", "The author's TeX in Markdown math", func(value bool) { f.given["author_math"] = value }),
		choiceFlag("--allow-fallback <mode>", "image: Markdown and Typst show a formula they cannot express as a picture "+
			"(not accessible) instead of failing", []string{"image"}, func(value string) { f.given["allow_fallback"] = value }),
		switchFlag("--compile-typst", "Compile the Typst exports with typst; the conversion fails if that fails",
			func() { f.given["compile_typst"] = true }),
		switchFlag("--verify-browser", "Check the finished page in a browser and measure its accessibility",
			func() { f.given["verify_browser"] = true }),
	}
}

// acceptanceRefusal answers the options and the command of the archived Ruby program's
// acceptance suite (decision 069): the product does not run it.
const acceptanceRefusal = "independent acceptance (--verify, --authored-proof, --frozen-pdf, relatex verify) is not part of " +
	"relatex; it is a development tool of the relatex repository (tools/run_paper). " +
	"To check the page in a browser, convert with --verify-browser"

// acceptanceOptions are the archived acceptance suite's options, hidden from the help: given, they
// stop the conversion with acceptanceRefusal rather than being reported as unknown.
func (f *convertFlags) acceptanceOptions() []clihelp.Option {
	list := []clihelp.Option{}
	for _, spec := range []string{"--verify <profile>", "--authored-proof <dir>", "--frozen-pdf <file>"} {
		option := valueFlag(spec, "Refused: the archived acceptance suite", func(string) error { f.refused = true; return nil })
		option.Hidden = true
		list = append(list, option)
	}
	return list
}

func regular(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// legacySynctex maps the former `--synctex MODE` and `--synctex=MODE` to the options that replace
// them: word is plain --synctex, native is --synctex-native, none is --no-synctex.
func legacySynctex(arguments []string) []string {
	modes := map[string]string{"word": "--synctex", "native": "--synctex-native", "none": "--no-synctex"}
	result := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if mode, joined := strings.CutPrefix(argument, "--synctex="); joined && modes[mode] != "" {
			result = append(result, modes[mode])
			continue
		}
		if argument == "--synctex" && index+1 < len(arguments) && modes[arguments[index+1]] != "" {
			result = append(result, modes[arguments[index+1]])
			index++
			continue
		}
		result = append(result, argument)
	}
	return result
}

// everyFormat is the --all selection: every export format but the separate builds.
func everyFormat() []any {
	return names(everyFormatName())
}

func everyFormatName() []string {
	names := []string{}
	for _, format := range exportFormats {
		if !slices.Contains(separateBuilds, format) {
			names = append(names, format)
		}
	}
	return names
}
