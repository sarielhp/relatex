package cli

import (
	"io"
	"reflect"
	"slices"
	"testing"
)

// parseLine reads a command line against the whole command tree without running a command: the
// session holds the options a convert asked for, and the words and command path it named.
func parseLine(arguments ...string) (*session, error) {
	current := newSession(io.Discard, io.Discard)
	current.parseOnly = true
	return current, current.app().Execute(legacySynctex(arguments))
}

// parsedConvert is what `relatex [convert] ARGS` reads from its arguments: the options (without the
// engine default) and the remaining words, before any command logic runs.
func parsedConvert(t *testing.T, arguments ...string) (map[string]any, []string) {
	t.Helper()
	current, err := parseLine(arguments...)
	if err != nil {
		t.Fatalf("relatex convert %v: %v", arguments, err)
	}
	return current.convert.given, current.words
}

func formatList(names ...string) []any {
	list := []any{}
	for _, name := range names {
		list = append(list, name)
	}
	return list
}

type convertCase struct {
	name      string
	arguments []string
	options   map[string]any
	rest      []string
}

// convertCases are the invocations of the convert command found in tools/, lib/, test/ and the
// documentation (the 2026-11 option inventory), and the spellings OptionParser
// accepts that a user's script may rely on.
func convertCases() []convertCase {
	return []convertCase{
		{"cohort_product", []string{"paper.zip", "--main", "main.tex", "--html", "--markdown", "--typst", "--json", "--epub",
			"--output", "out", "--hermetic", "--author-math", "--verify-browser"},
			map[string]any{"main": "main.tex", "extra_formats": formatList("html", "markdown", "typst-source", "json", "epub"),
				"output": "out", "hermetic": true, "author_math": true, "verify_browser": true}, []string{"paper.zip"}},
		{"cohort_product no author math", []string{"p.zip", "--main", "m.tex", "--format", "pdf-ua", "--web-report", "a.json",
			"--output", "o", "--hermetic", "--no-author-math"},
			map[string]any{"main": "m.tex", "formats": formatList("pdf-ua"), "web_report": "a.json", "output": "o",
				"hermetic": true, "author_math": false}, []string{"p.zip"}},
		{"format list", []string{"./d", "--main", "main.tex", "--format", "html,json,epub", "--output", "o", "--no-publish"},
			map[string]any{"main": "main.tex", "formats": formatList("html", "json", "epub"), "output": "o", "publish": false},
			[]string{"./d"}},
		{"check_sync_math", []string{"./d", "--main", "main.tex", "--html", "--json", "--output", "d/out"},
			map[string]any{"main": "main.tex", "extra_formats": formatList("html", "json"), "output": "d/out"}, []string{"./d"}},
		{"pdf-ua", []string{"./d", "--main", "main.tex", "--format", "json", "--pdf-ua", "--no-layout", "--output", "o"},
			map[string]any{"main": "main.tex", "formats": formatList("json"), "extra_formats": formatList("pdf-ua"),
				"layout": false, "synchronize": false, "output": "o"}, []string{"./d"}},
		{"zoo_run", []string{"./d", "--main", "main.tex", "--html", "--output", "o", "--hermetic", "--no-author-math",
			"--no-layout", "--no-publish"},
			map[string]any{"main": "main.tex", "extra_formats": formatList("html"), "output": "o", "hermetic": true,
				"author_math": false, "layout": false, "synchronize": false, "publish": false}, []string{"./d"}},
		{"texinputs_test", []string{"./d", "--main", "main.tex", "--json", "--no-layout", "--output", "o"},
			map[string]any{"main": "main.tex", "extra_formats": formatList("json"), "layout": false, "synchronize": false,
				"output": "o"}, []string{"./d"}},
		{"time_paper (no command)", []string{"--no-cache", "--no-layout"},
			map[string]any{"cache": "off", "layout": false, "synchronize": false}, nil},
		{"package_smoke missing engine", []string{"--engine", "/nonexistent/relatex-engine"},
			map[string]any{"engine": "/nonexistent/relatex-engine"}, nil},
		{"check_verapdf", []string{"./d", "--main", "main.tex", "--format", "pdf-ua", "--no-publish", "--output", "o"},
			map[string]any{"main": "main.tex", "formats": formatList("pdf-ua"), "publish": false, "output": "o"}, []string{"./d"}},
		{"documented", []string{"paper.zip", "--main", "paper.tex", "--format", "html", "--output", "reading"},
			map[string]any{"main": "paper.tex", "formats": formatList("html"), "output": "reading"}, []string{"paper.zip"}},
		{"replay", []string{"--resume", "attempt", "--force", "export,publish"},
			map[string]any{"resume": "attempt", "force": formatList("export", "publish")}, nil},
		{"equals forms", []string{"--output=o", "--jobs=4", "--split=none", "--pdf-ua=route2", "--no-source-fonts", "--synctex"},
			map[string]any{"output": "o", "jobs": int64(4), "split": "none", "extra_formats": formatList("pdf-ua"),
				"source_fonts": false, "synctex": "word"}, nil},
		{"legacy synctex", []string{"--synctex", "native", "--accessible"},
			map[string]any{"synctex": "native", "extra_formats": formatList("html", "epub", "pdf-ua"), "verify_browser": true,
				"accessible": true}, nil},
		{"short output", []string{"-o", "p", "--no-source-fonts", "--no-cache", "--html"},
			map[string]any{"output": "p", "source_fonts": false, "cache": "off", "extra_formats": formatList("html")}, nil},
		{"short output joined", []string{"-oq"}, map[string]any{"output": "q"}, nil},
		{"options after the input, double dash", []string{"./d", "--html", "--", "--odd.zip"},
			map[string]any{"extra_formats": formatList("html")}, []string{"./d", "--odd.zip"}},
	}
}

// TestConvertInvocationsKeepTheirParse must pass unchanged across any change of the parsing layer.
func TestConvertInvocationsKeepTheirParse(t *testing.T) {
	for _, c := range convertCases() {
		options, rest := parsedConvert(t, c.arguments...)
		if !reflect.DeepEqual(options, c.options) || !slices.Equal(rest, c.rest) {
			t.Errorf("%s: relatex %v\n parsed %#v %v\n  wants %#v %v", c.name, c.arguments, options, rest, c.options, c.rest)
		}
	}
}

// TestConvertParseErrorsKeepTheirStatus pins the exit status of a wrong option: 2.
func TestConvertParseErrorsKeepTheirStatus(t *testing.T) {
	for _, arguments := range [][]string{{"--bogus"}, {"--jobs", "x"}, {"--main"}, {"--split", "zzz"}, {"--hermetic=1"}} {
		if status, _, stderr := run(t, arguments...); status != 2 || stderr == "" {
			t.Errorf("relatex %v: status %d, stderr %q", arguments, status, stderr)
		}
	}
}

// TestDroppedSpellingsAreUsageErrors: the spellings OptionParser accepted and clihelp does not (the
// author dropped them on purpose, 2026-11-15) are usage errors now, status 2: an abbreviated flag,
// a prefix of a choice, a flag in the wrong case or with `_` for `-`, Ruby's number forms, and a
// first word that is neither a command nor a path.
func TestDroppedSpellingsAreUsageErrors(t *testing.T) {
	for _, arguments := range [][]string{{"--no-sou"}, {"--out", "o"}, {"--no_cache"}, {"--HTML"}, {"--Output", "o"},
		{"--split", "no"}, {"--allow-fallback", "im"}, {"--jobs", "0x10"}, {"--jobs", "0b11"}, {"--jobs", "1_000"},
		{"--jobs", "1.5"}, {"--format"}, {"unknown-word"}} {
		if status, _, stderr := run(t, arguments...); status != 2 || stderr == "" {
			t.Errorf("relatex %v: status %d, stderr %q", arguments, status, stderr)
		}
	}
}

// TestRubyNumbersNowReadAsDecimal: `010` was eight in OptionParser and is ten now.
func TestRubyNumbersNowReadAsDecimal(t *testing.T) {
	if options, _ := parsedConvert(t, "--jobs", "010"); options["jobs"] != int64(10) {
		t.Errorf("--jobs 010: %v", options["jobs"])
	}
}

// TestBareInvocations pins the program-level forms the tools use.
func TestBareInvocations(t *testing.T) {
	if status, stdout, _ := run(t, "--version"); status != 0 || stdout != "relatex "+version()+"\n" {
		t.Errorf("--version: status %d, stdout %q", status, stdout)
	}
	// -h was an ambiguous option (status 2); it is the concise help now.
	if status, stdout, _ := run(t, "-h"); status != 0 || stdout == "" {
		t.Errorf("-h: status %d, stdout %q", status, stdout)
	}
	if status, stdout, _ := run(t, "--help"); status != 0 || stdout == "" {
		t.Errorf("--help: status %d, stdout %q", status, stdout)
	}
	if status, stdout, _ := run(t, "convert", "--help"); status != 0 || stdout == "" {
		t.Errorf("convert --help: status %d, stdout %q", status, stdout)
	}
}
