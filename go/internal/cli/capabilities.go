package cli

import (
	"path/filepath"
	"strings"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/exportwriter"
	"github.com/sarielhp/relatex/internal/mathml"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/pipeline"
	"github.com/sarielhp/relatex/internal/rec"
)

// layerOrder is LayerRun::ORDER.
var layerOrder = []string{"input", "engine", "reconcile", "layout", "synchronize", "export", "publish"}

func words(values ...string) []any {
	list := make([]any, len(values))
	for index, value := range values {
		list[index] = value
	}
	return list
}

// capabilityLayers is Capabilities::LAYERS.
func capabilityLayers() ojson.Object {
	return ojson.Object{
		{"input", words("zip", "directory", "immutable_snapshot", "main_selection")},
		{"engine", words("ordinary_pdf", "external_json", "bibliography", "convergence", "bounded_parallel_builds")},
		{"reconcile", words("structure", "math", "references", "notes", "citations", "fonts", "figures", "tables", "metadata",
			"executed_links", "png", "jpeg", "pdf_figures", "affine_placements", "rich_cells", "spans", "multipage_tables")},
		{"layout", words("page_vectors", "glyph_geometry", "pdf_reconstruction")},
		{"synchronize", words("word_boxes", "source_lines", "source_spans", "forward_search", "inverse_search",
			"portable_native_synctex", "enhanced_word_synctex", "source_viewer", "editor_opt_in")},
		{"export", words("json", "json_gzip", "html", "markdown", "typst", "pdf", "epub", "native_mathml", "native_typst_math")},
		{"publish", words("portable_assets", "atomic_output", "loss_ledger")},
	}
}

// capabilityReaders is Capabilities::READERS.
func capabilityReaders() ojson.Object {
	return ojson.Object{{"document", words("paper-0.1", "paper-0.2")}, {"bundle", words("bundle-1.0", "bundle-1.1", "bundle-1.2")},
		{"layout", words("layout-1.0", "layout-1.1", "layout-1.2")},
		{"source_map", words("source-map-1.0", "source-map-1.1", "source-map-1.2", "source-map-2.0")},
		{"media", words("legacy-png", "media-1")}, {"table", words("legacy-grid", "table-2")},
		{"table_paint", words("legacy", "table-paint-2", "table-paint-3", "table-paint-4")},
		{"table_font_bindings", words("pdf-table-fonts-1")}, {"text_paint", words("legacy", "text-paint-2")}}
}

// mathSupport is MathProjectionSupport.matrix.
func mathSupport() ojson.Object {
	attributes := ojson.Object{{"math", words("display")},
		{"mo", words("lspace", "rspace", "stretchy", "fence", "symmetric", "movablelimits", "minsize", "maxsize")},
		{"mfrac", words("linethickness", "bevelled")}, {"mover", words("accent")}, {"munder", words("accentunder")},
		{"munderover", words("accent", "accentunder")}, {"mspace", words("width", "height", "depth")},
		{"mpadded", words("width", "height", "depth", "lspace", "voffset")},
		{"mtable", words("columnalign", "columnspacing", "rowalign", "rowspacing")},
		{"mtr", words("columnalign", "rowalign")}, {"mtd", words("columnalign", "rowalign", "columnspan", "rowspan")}}
	defaults := ojson.Object{{"dir", "ltr"}, {"bevelled", "false"}, {"columnspan", "1"}, {"rowspan", "1"}, {"frame", "none"},
		{"rowlines", "none"}, {"columnlines", "none"}}
	return ojson.Object{{"schema_version", "math-projection-support-1"}, {"retained_elements", words(mathml.PaperElements...)},
		{"retained_attributes", words(mathml.PaperAttributes...)}, {"native_attributes", attributes},
		{"styles", words("mathvariant", "displaystyle", "scriptlevel", "mathsize", "mathcolor", "mathbackground", "dir")},
		{"metadata", words("xmlns", "class", "intent")},
		{"tex_native_mathml_attributes", words("mathsize", "mathcolor", "mathbackground", "minsize", "maxsize", "height", "depth",
			"voffset")},
		{"tex_dialects", words("tex", "pandoc")}, {"markdown", "Pandoc TeX math or structured native MathML; no formula image"},
		{"conditional_defaults", defaults}, {"qualification", "Implementation coverage; renderer validation is separate"}}
}

// capabilityReport is Capabilities.report(public:).
func capabilityReport(public bool) ojson.Object {
	helpers := ojson.Object{}
	for _, tool := range []string{"mutool", "synctex", "typst", "bibtex", "biber", "qpdf", "pdftotext", "pdftoppm", "bwrap"} {
		path := helperPath(tool)
		var value any
		switch {
		case public:
			value = path != ""
		case path != "":
			value = path
		}
		helpers = append(helpers, ojson.Pair{tool, value})
	}
	stages := []any{}
	for _, name := range layerOrder {
		if name != "input" && name != "export" && name != "publish" {
			stages = append(stages, name)
		}
	}
	return ojson.Object{{"schema_version", int64(1)}, {"version", version()}, {"layers", capabilityLayers()},
		{"availability", "implemented"}, {"qualification", "Independent per-input verification is separate from feature availability"},
		{"readers", capabilityReaders()}, {"acceptance_profiles", words("authored-workflow-1", "frozen-draw-7", "frozen-draw-12")},
		{"math_support", mathSupport()},
		{"reuse", ojson.Object{{"stages", stages}, {"format_caches", words(exportwriter.Formats...)}, {"acceptance_reused", false},
			{"resume", true}, {"replay", true}}},
		{"stopped", words("stress_tests", "automatic_stress_triggers", "broad_sampling")}, {"helpers", helpers}}
}

func capabilitiesSpec() commandSpec {
	return commandSpec{name: "capabilities", description: "Print what this build implements, as JSON", usage: Name + " capabilities",
		args: clihelp.NoArgs, human: capabilitiesHuman, run: func([]string) ojson.Object { return capabilityReport(false) }}
}

// layersSpec is ProductCLI#layers: an attempt's pipeline record, or one stage's receipt.
func layersSpec() commandSpec {
	stage := ""
	options := []clihelp.Option{textChoice("--stage <name>", "One stage's receipt: "+strings.Join(layerOrder, ", "), layerOrder, &stage)}
	return commandSpec{name: "layers", description: "List an attempt's stages and files, or one stage's receipt",
		usage: Name + " layers ATTEMPT [--stage NAME]", options: options, args: clihelp.ExactArgs(1),
		human: layersHuman, run: func(rest []string) ojson.Object { return layers(rest[0], stage) }}
}

func layers(attempt, stage string) ojson.Object {
	root := realpath(attempt)
	pipeline.ExpandAtRest(root)
	pipeline, ok := readOrdered(filepath.Join(root, "layers/pipeline.json")).(ojson.Object)
	if !ok {
		rec.Fail("undefined method 'fetch'")
	}
	if stage == "" {
		return pipeline
	}
	stages, _ := pipeline.Get("stages").([]any)
	if !pipeline.Has("stages") {
		panic(rec.Failure{Message: `key not found: "stages"`, Lookup: true})
	}
	for _, row := range stages {
		receipt, isObject := row.(ojson.Object)
		if isObject && receipt.Get("layer") == stage {
			return receipt
		}
		if isObject && !receipt.Has("layer") {
			panic(rec.Failure{Message: `key not found: "layer"`, Lookup: true})
		}
	}
	rec.Fail("Requested stage has not completed")
	return nil
}

// configSpec is ProductCLI#config_command: the settings a conversion there would use, and
// their files; --init writes the global template, --init-project the directory's own file.
func configSpec() commandSpec {
	target := ""
	options := []clihelp.Option{
		switchFlag("--init", "Write the global configuration file from the template", func() { target = "global" }),
		switchFlag("--init-project", "Write DIRECTORY/"+projectConfig+" with every setting commented out (it overrides nothing until you uncomment one)",
			func() { target = "project" }),
	}
	return commandSpec{name: "config", description: "Show the settings a conversion would use and their files; write a configuration file",
		usage: Name + " config [DIRECTORY] [--init | --init-project]", options: options, args: clihelp.MaximumNArgs(1),
		human: configHuman, run: func(rest []string) ojson.Object { return configCommand(rest, &target) }}
}

func configCommand(rest []string, target *string) ojson.Object {
	directory := "."
	if len(rest) == 1 {
		directory = rest[0]
	}
	report := ojson.Object{{"status", "success"}}
	switch *target {
	case "global":
		report = append(report, ojson.Pair{"written", writeTemplate(globalConfigPath(), configTemplate)})
	case "project":
		report = append(report, ojson.Pair{"written", writeTemplate(filepath.Join(directory, projectConfig), projectTemplate())})
	}
	settings, files := loadConfig(directory)
	return append(report, ojson.Pair{"files", files}, ojson.Pair{"settings", settings})
}
