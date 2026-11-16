package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/authordir"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// UserConfig: the user's defaults for conversions, ~/.config/relatex/config.jsonc
// ($XDG_CONFIG_HOME aware), then a paper directory's own relatex/config.jsonc, each over the
// built-in defaults. Command-line flags override both. The files are JSON with // and /* */
// comments and trailing commas (JSONC); an unknown key or a wrong type stops the conversion
// with the file named, so a typo never silently changes a conversion.

const projectConfig = authordir.Config

// exportFormats is BundleExport::FORMATS.
var exportFormats = []string{"json", "json.gz", "html", "markdown", "typst", "typst-source", "pdf", "epub", "pdf-ua"}

// separateBuilds are the formats --all leaves out: each is a LuaLaTeX build of its own (the
// tagged PDF, decision 097), asked for by name.
var separateBuilds = []string{"pdf-ua"}

// configTemplate is UserConfig::TEMPLATE.
const configTemplate = `// relatex configuration. Global: ~/.config/relatex/config.jsonc; a paper directory's
// relatex/config.jsonc overrides it, and command-line flags override both.
{
  "main": {
    // Files never taken as the main .tex file: the preamble and preliminaries beside it.
    "ignore": ["prefix.tex", "prelim.tex"]
  },
  // Formats written by default: markdown, html, typst-source, typst, json, json.gz, pdf, epub, pdf-ua.
  // Flags such as --html add to them; --format replaces them.
  "formats": ["markdown", "html"],
  // The author's TeX in Markdown math (decision 064): true, false, or null for "with Markdown, HTML or EPUB".
  "author_math": null,
  // Where results go, relative to the directory relatex runs in: "." puts markdown/, html/, typst/ and
  // NAME.epub right there, and nothing else (relatex keeps its records in the project cache).
  "output": ".",
  // Build layers and the stage cache (for --resume and fast reruns): "cache" keeps them under
  // ~/.cache/relatex/projects/, so the output holds only results; "output" keeps them in _product/ of
  // an output directory given with -o (never in the paper's own directory, which uses the cache).
  "work": "cache",
  // Parallel LaTeX builds.
  "jobs": 2,
  // Link the paper's own fonts in HTML and Markdown.
  "source_fonts": true,
  // The browser that verifies the HTML (--verify-browser). Without an endpoint relatex starts the
  // local Chromium; an endpoint is a remote Chrome's DevTools address, by IP, such as
  // "http://192.0.2.10:9223" (RELATEX_BROWSER_ENDPOINT overrides it). "sessions" caps the
  // concurrent pages (default 6 remote, half the processors locally); "token_file" names a file
  // holding a bearer token, for an endpoint that requires one (sent to ace_endpoint too).
  // "ace_endpoint" is DAISY Ace as a service (ace-http) for the development tools' EPUB checks,
  // such as "http://192.0.2.10:9223/ace" (RELATEX_ACE_ENDPOINT overrides it); without it, or
  // when the service fails, the local Ace runs.
  "browser": {"endpoint": null, "sessions": null, "token_file": null, "ace_endpoint": null},
  // Figure descriptions (relatex alt draft). "command" is the program that describes one figure:
  // a list, the program and its arguments; relatex appends the figure's file and passes the
  // prompt on its standard input. "uploads": true when it sends figures off this machine (each
  // paper then needs your approval once: relatex alt draft --allow-upload); false for a local
  // model. "model" is the name the drafts record (default: the first of "models").
  // "endpoint", "models" and "key_file" are the describing service's settings, handed to the
  // command as RELATEX_DESCRIBER_ENDPOINT, RELATEX_DESCRIBER_MODELS (comma-separated) and
  // RELATEX_DESCRIBER_KEY_FILE, for example "http://tqed:4000/v1/chat/completions",
  // ["lumen/qwen3.8-flash"] and "~/.config/auth/litellm_api_key". The API key stays in its own
  // file, yours alone (chmod 600), so this file can be shared; relatex checks that file's mode
  // and never reads it. Global file only: a paper's file may not set it.
  "alt_text": {"command": null, "uploads": true, "model": null, "timeout_seconds": 120},
  // Programs relatex starts, by path, when PATH and TeX Live's own directory do not find them
  // (on Windows "C:\\Users\\me\\mupdf\\mutool.exe": backslashes doubled). null finds the program
  // as usual; a path that names no program is missing, never replaced by another one. "verapdf" is the
  // PDF/UA validator (its launcher script, such as "/opt/verapdf/verapdf"); without it the PDF/UA-2 claim
  // is withdrawn. RELATEX_ENGINE, RELATEX_CHROMIUM and RELATEX_<NAME> (RELATEX_MUTOOL, RELATEX_VERAPDF, ...)
  // override these; relatex doctor shows
  // where each program came from. Global file only: a paper's file may not set it.
  "tools": {"lualatex": null, "mutool": null, "qpdf": null, "pdfinfo": null, "pdftocairo": null, "pdftotext": null, "pdftoppm": null, "tesseract": null, "typst": null, "browser": null, "verapdf": null}
}
`

// configDefaults is UserConfig::DEFAULTS, in its order.
func configDefaults() ojson.Object {
	return ojson.Object{{"main", ojson.Object{{"ignore", []any{"prefix.tex", "prelim.tex"}}}}, {"formats", []any{"markdown", "html"}},
		{"author_math", nil}, {"output", "."}, {"work", "cache"}, {"jobs", int64(2)}, {"source_fonts", true},
		{"browser", ojson.Object{{"endpoint", nil}, {"sessions", nil}, {"token_file", nil}, {"ace_endpoint", nil}}},
		{"alt_text", ojson.Object{{"command", nil}, {"uploads", true}, {"model", nil}, {"timeout_seconds", int64(120)}}},
		{"tools", toolDefaults()}}
}

// globalConfigPath is UserConfig.global_path.
func globalConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if _, set := os.LookupEnv("XDG_CONFIG_HOME"); !set {
		base = defaultConfigBase()
	}
	return filepath.Join(expandPath(base), "relatex", "config.jsonc")
}

// defaultConfigBase is the user's configuration directory: ~/.config on Unix, %APPDATA% on
// Windows (decision 129).
func defaultConfigBase() string {
	if runtime.GOOS == "windows" {
		if base, err := os.UserConfigDir(); err == nil {
			return base
		}
	}
	return "~/.config"
}

// expandPath is File.expand_path: ~ is the home directory, relative paths are under the
// working directory.
func expandPath(path string) string {
	path = fspath.ExpandHome(path)
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolute
}

// projectHeader opens a paper's relatex/config.jsonc.
const projectHeader = `// relatex settings for this paper. They override the global configuration
// (~/.config/relatex/config.jsonc), and command-line flags override both. Every setting below is
// commented out, so this file changes nothing: uncomment a setting to set it for this paper.
`

// projectTemplate is the global template with every setting commented out: a paper's file
// overrides the global one key by key, so a setting written out would hide the global value.
func projectTemplate() string {
	lines := strings.Split(configTemplate, "\n")
	var builder strings.Builder
	builder.WriteString(projectHeader)
	globalOnly := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// alt_text and tools belong to the global file only: their comments and lines are left out.
		globalOnly = globalOnly || strings.HasPrefix(trimmed, "// Figure descriptions") ||
			strings.HasPrefix(trimmed, "// Programs relatex starts")
		if globalOnly {
			globalOnly = !strings.HasPrefix(trimmed, `"alt_text"`) && !strings.HasPrefix(trimmed, `"tools"`)
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(line, " "):
			continue // the global template's own header
		case trimmed == "" || trimmed == "{" || trimmed == "}" && line == "}" || strings.HasPrefix(trimmed, "//"):
			builder.WriteString(line)
		default:
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			builder.WriteString(indent + "// " + trimmed)
		}
		builder.WriteString("\n")
	}
	return strings.TrimSuffix(builder.String(), "\n")
}

// writeTemplate writes a commented template; an existing file is never replaced.
func writeTemplate(path, template string) string {
	if _, err := os.Stat(path); err == nil {
		rec.Fail("%s already exists", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(systemCall(err))
	}
	if err := os.WriteFile(path, []byte(template), 0o644); err != nil {
		panic(systemCall(err))
	}
	return path
}

// loadConfig is UserConfig.load: the merged settings and the files they came from.
func loadConfig(directory string) (ojson.Object, []any) {
	files := []any{}
	candidates := []string{globalConfigPath()}
	if info, err := os.Stat(directory); directory != "" && err == nil && info.IsDir() {
		candidates = append(candidates, filepath.Join(directory, projectConfig))
	}
	settings := configDefaults()
	for _, path := range candidates {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, path)
		values := readConfig(path)
		if path != candidates[0] {
			refuseProjectAltText(values, path)
			refuseProjectTools(values, path)
		}
		settings = mergeSettings(settings, values)
	}
	return settings, files
}

func readConfig(path string) ojson.Object {
	bytes, err := os.ReadFile(path)
	if err != nil {
		panic(systemCall(err))
	}
	value, err := ojson.Decode(strings.NewReader(stripComments(string(bytes))))
	if err != nil {
		rec.Fail("%s: not valid JSONC (%s)", path, jsonMessage(err))
	}
	values, ok := value.(ojson.Object)
	if !ok {
		rec.Fail("%s: the configuration must be a JSON object", path)
	}
	validateConfig(values, path)
	return values
}

// jsonMessage is the first line of a JSON parse error.
func jsonMessage(err error) string {
	text, _, _ := strings.Cut(strings.TrimPrefix(err.Error(), "decode json: "), "\n")
	return strings.TrimSpace(text)
}

var configChecks = []struct {
	name, expected string
	test           func(any) bool
}{
	{"formats", "a list of formats (" + strings.Join(exportFormats, ", ") + ")", func(value any) bool {
		list, ok := value.([]any)
		if !ok || len(list) == 0 {
			return false
		}
		for _, item := range list {
			text, isString := item.(string)
			if !isString || !slices.Contains(exportFormats, text) {
				return false
			}
		}
		return true
	}},
	{"author_math", "true, false or null", func(value any) bool { return value == true || value == false || value == nil }},
	{"output", "a directory name", func(value any) bool { text, ok := value.(string); return ok && text != "" }},
	{"work", `"cache" or "output"`, func(value any) bool { return value == "cache" || value == "output" }},
	{"jobs", "a number from 1 to 8", func(value any) bool { number, ok := value.(int64); return ok && number >= 1 && number <= 8 }},
	{"source_fonts", "true or false", func(value any) bool { return value == true || value == false }},
}

func validateConfig(values ojson.Object, path string) {
	unknown := []string{}
	known := configDefaults()
	for _, pair := range values {
		if !known.Has(pair.Key()) {
			unknown = append(unknown, rec.InspectString(pair.Key()))
		}
	}
	if len(unknown) > 0 {
		rec.Fail("%s: unknown setting %s", path, strings.Join(unknown, ", "))
	}
	if values.Has("main") {
		validateMain(values.Get("main"), path)
	}
	if values.Has("browser") {
		validateBrowser(values.Get("browser"), path)
	}
	if values.Has("alt_text") {
		validateAltText(values.Get("alt_text"), path)
	}
	if values.Has("tools") {
		validateTools(values.Get("tools"), path)
	}
	for _, pair := range values {
		for _, check := range configChecks {
			if check.name == pair.Key() && !check.test(pair.Value()) {
				rec.Fail("%s: \"%s\" must be %s", path, check.name, check.expected)
			}
		}
	}
}

func validateMain(value any, path string) {
	main, ok := value.(ojson.Object)
	valid := ok
	for _, pair := range main {
		valid = valid && pair.Key() == "ignore"
	}
	if !valid {
		rec.Fail("%s: \"main\" takes only \"ignore\"", path)
	}
	if !main.Has("ignore") {
		return
	}
	list, isList := main.Get("ignore").([]any)
	for _, item := range list {
		if _, isString := item.(string); !isString {
			isList = false
		}
	}
	if !isList {
		rec.Fail("%s: \"main.ignore\" must be a list of file names", path)
	}
}

// mergeSettings is UserConfig.merge: nested objects merged, other values replaced.
func mergeSettings(base, override ojson.Object) ojson.Object {
	merged := append(ojson.Object{}, base...)
	for _, pair := range override {
		old, isObject := merged.Get(pair.Key()).(ojson.Object)
		if next, nested := pair.Value().(ojson.Object); isObject && nested {
			merged = merged.Set(pair.Key(), mergeSettings(old, next))
			continue
		}
		merged = merged.Set(pair.Key(), pair.Value())
	}
	return merged
}

// stripComments is UserConfig.strip_comments: JSONC to JSON in one pass that tracks strings,
// a comma held until the next significant character shows it was trailing.
func stripComments(content string) string {
	var out strings.Builder
	inString, escaped, pendingComma := false, false, false
	characters := []rune(content)
	for index := 0; index < len(characters); {
		character := characters[index]
		if inString {
			out.WriteRune(character)
			switch {
			case escaped:
				escaped = false
			case character == '\\':
				escaped = true
			case character == '"':
				inString = false
			}
			index++
			continue
		}
		if skip := commentEnd(characters, index); skip >= 0 {
			index = skip
			continue
		}
		if pendingComma && !rubyWhitespace(character) {
			if character != '}' && character != ']' {
				out.WriteByte(',')
			}
			pendingComma = false
		}
		if character == ',' {
			pendingComma = true
		} else {
			inString = character == '"'
			out.WriteRune(character)
		}
		index++
	}
	if pendingComma {
		out.WriteByte(',')
	}
	return out.String()
}

func rubyWhitespace(character rune) bool { return strings.ContainsRune(" \t\n\v\f\r", character) }

// commentEnd is where a comment starting at index ends, or -1.
func commentEnd(characters []rune, index int) int {
	if index+1 >= len(characters) || characters[index] != '/' {
		return -1
	}
	switch characters[index+1] {
	case '/':
		for end := index; end < len(characters); end++ {
			if characters[end] == '\n' {
				return end
			}
		}
		return len(characters)
	case '*':
		for end := index + 2; end+1 < len(characters); end++ {
			if characters[end] == '*' && characters[end+1] == '/' {
				return end + 2
			}
		}
		return len(characters)
	}
	return -1
}
