package cli

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/artifacts"
	"github.com/sarielhp/relatex/internal/bundle"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/sourcemap"
)

// query is a sync request: forward (a source selection) or inverse (a PDF point).
type query struct {
	given                                  map[string]bool
	file                                   string
	line, column, endLine, endColumn, page int64
	horizontal, vertical                   float64
}

type editorOptions struct {
	open                bool
	sourceRoot, output  string
	arguments           []any
	hasArguments        bool
	match, origin       int64
	hasMatch, hasOrigin bool
}

const syncUsage = "DOCUMENT.json[.gz] (--source FILE --line N [--column N] | --page N --x BP --y BP)"

// syncSpec is ProductCLI#synchronize: the words a source selection or a PDF point names.
func syncSpec() commandSpec {
	q := &query{given: map[string]bool{}}
	editor := &editorOptions{}
	return commandSpec{name: "sync", description: "Find the words a source selection or a PDF point names", usage: Name + " sync " + syncUsage,
		options: syncOptions(q, editor), args: clihelp.ExactArgs(1), human: syncHuman,
		run: func(rest []string) ojson.Object { return synchronize(rest[0], *q, *editor) }}
}

func synchronize(path string, q query, editor editorOptions) ojson.Object {
	payload := readBundle(path)
	bundle.Validate(payload)
	sourceMap, ok := payload["source_map"].(rec.Map)
	if !ok {
		rec.Fail("Bundle has no source synchronization data")
	}
	direction := "inverse"
	required, allowed := []string{"page", "horizontal", "vertical"}, []string{"page", "horizontal", "vertical"}
	if q.given["file"] {
		direction = "forward"
		required, allowed = []string{"file", "line"}, []string{"file", "line", "column", "end_line", "end_column"}
	}
	for _, key := range required {
		if !q.given[key] {
			fail("a " + direction + " query needs " + strings.Join(required, ", ") + " (usage: " + Name + " sync " + syncUsage + ")")
		}
	}
	for key := range q.given {
		if !contains(allowed, key) {
			fail("a " + direction + " query takes only " + strings.Join(allowed, ", ") + " (usage: " + Name + " sync " + syncUsage + ")")
		}
	}
	matches := inverse(sourceMap, q)
	if direction == "forward" {
		matches = forward(sourceMap, q)
	}
	report := ojson.Object{{"status", "success"}, {"direction", direction}, {"matches", matches},
		{"pdf_sha256", rec.Fetch(sourceMap, "pdf_sha256")}}
	if editor.open {
		report = append(report, ojson.Pair{"editor", openEditor(matches, sourceMap, editor)})
	}
	return report
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func syncOptions(q *query, editor *editorOptions) []clihelp.Option {
	integer := func(key, spec, description string, target *int64) clihelp.Option {
		return integerFlag(spec, description, func(value int64) { *target, q.given[key] = value, true })
	}
	return []clihelp.Option{
		valueFlag("--source <file>", "Forward: the source file of the selection", func(value string) error {
			q.file, q.given["file"] = normalizeSourceArg(value, fspath.Slash), true
			return nil
		}),
		integer("line", "--line <n>", "Forward: the first selected line", &q.line),
		integer("column", "--column <n>", "Forward: the first selected column", &q.column),
		integer("end_line", "--end-line <n>", "Forward: the last selected line", &q.endLine),
		integer("end_column", "--end-column <n>", "Forward: the last selected column", &q.endColumn),
		integer("page", "--page <n>", "Inverse: the PDF page", &q.page),
		numberFlag("--x <bp>", "Inverse: the horizontal position on the page, in bp", func(value float64) { q.horizontal, q.given["horizontal"] = value, true }),
		numberFlag("--y <bp>", "Inverse: the vertical position on the page, in bp", func(value float64) { q.vertical, q.given["vertical"] = value, true }),
		switchFlag("--open-editor", "Open the first match in the editor", func() { editor.open = true }),
		textFlag("--source-root <dir>", "The directory the source paths are relative to", &editor.sourceRoot),
		valueFlag("--editor-argv <json>", "The editor's command line as a JSON list", func(text string) error {
			value, err := ojson.Decode(strings.NewReader(text))
			if err != nil {
				panic(rec.Failure{Message: jsonMessage(err)})
			}
			list, ok := value.([]any)
			editor.arguments, editor.hasArguments = list, true
			if !ok {
				editor.arguments = nil
			}
			return nil
		}),
		integerFlag("--match-index <n>", "Open this match rather than the first", func(value int64) { editor.match, editor.hasMatch = value, true }),
		integerFlag("--origin-index <n>", "Open this origin of the match", func(value int64) { editor.origin, editor.hasOrigin = value, true }),
		textFlag("-o, --output <dir>", "Where the editor's request is written", &editor.output),
	}
}

// forward is SourceMap.forward: the words whose origins overlap the selection.
func forward(sourceMap rec.Map, q query) []any {
	finish := q.line
	if q.given["end_line"] {
		finish = q.endLine
	}
	if !(q.line > 0 && finish >= q.line) {
		rec.Fail("Invalid source selection lines")
	}
	if (q.given["column"] && q.column <= 0) || (q.given["end_column"] && q.endColumn <= 0) {
		rec.Fail("Invalid source selection column")
	}
	start := 1.0
	if q.given["column"] {
		start = float64(q.column)
	}
	finishColumn := math.Inf(1)
	switch {
	case q.given["end_column"]:
		finishColumn = float64(q.endColumn)
	case q.given["column"] && !q.given["end_line"]:
		finishColumn = float64(q.column + 1)
	}
	if finish == q.line && finishColumn <= start {
		rec.Fail("Reversed source selection")
	}
	matches := []any{}
	for _, word := range sourcemap.Selection(sourceMap) {
		for _, origin := range rec.FetchList(word, "origins") {
			if overlaps(rec.AsMap(origin), q.file, [2]float64{float64(q.line), float64(finish)}, [2]float64{start, finishColumn}) {
				matches = append(matches, word)
				break
			}
		}
	}
	return narrowest(matches, q)
}

func normalizeSourceArg(value string, slash func(string) string) string { return slash(value) }

// overlaps is SourceMap.origin_overlaps?: an origin covers line..end_line, its characters the
// first column on its first line and the end column on its last.
func overlaps(origin rec.Map, file string, lines, columns [2]float64) bool {
	first, _ := rec.Number(rec.Fetch(origin, "line"))
	last := first
	if value, ok := origin["end_line"]; ok {
		last, _ = rec.Number(value)
	}
	if rec.Fetch(origin, "file") != file || first > lines[1] || last < lines[0] {
		return false
	}
	characters, ok := origin["characters"].([]any)
	if !ok {
		return true
	}
	begin, _ := rec.Number(characters[0])
	end, _ := rec.Number(characters[1])
	begin, end = begin+1, end+1
	return compare(first, begin, lines[1], columns[1]) < 0 && compare(last, end, lines[0], columns[0]) > 0
}

// compare is [a, b] <=> [c, d].
func compare(a, b, c, d float64) int {
	switch {
	case a < c:
		return -1
	case a > c:
		return 1
	case b < d:
		return -1
	case b > d:
		return 1
	}
	return 0
}

// inverse is SourceMap.inverse: the words with a box on the page around the point.
func inverse(sourceMap rec.Map, q query) []any {
	matches := []any{}
	if q.given["file"] {
		return matches
	}
	for _, word := range sourcemap.Selection(sourceMap) {
		for _, box := range boxes(word, q.page) {
			if contains4(rec.AsList(box), q.horizontal, q.vertical) {
				matches = append(matches, word)
				break
			}
		}
	}
	return matches
}

// boxes is SourceWordGroups.boxes: a word's (or its fragments') boxes on the page.
func boxes(word rec.Map, page int64) []any {
	if _, fragmented := word["fragments"]; !fragmented && rec.Equal(rec.Fetch(word, "page"), page) {
		return rec.FetchList(word, "boxes")
	}
	result := []any{}
	fragments, _ := word["fragments"].([]any)
	for _, value := range fragments {
		fragment := rec.AsMap(value)
		if rec.Equal(rec.Fetch(fragment, "page"), page) {
			result = append(result, rec.FetchList(fragment, "boxes")...)
		}
	}
	return result
}

// contains4 is Geometry.contains?.
func contains4(values []any, x, y float64) bool {
	box := make([]float64, 4)
	for index := range box {
		number, ok := rec.Number(rec.Index(values, index))
		if !ok || math.IsInf(number, 0) || math.IsNaN(number) {
			rec.Fail("Nonfinite geometry coordinate")
		}
		box[index] = number
	}
	if len(values) != 4 {
		rec.Fail("Invalid geometry vector")
	}
	if !(box[0] <= box[2] && box[1] <= box[3]) {
		rec.Fail("Reversed geometry box")
	}
	return x >= box[0] && x <= box[2] && y >= box[1] && y <= box[3]
}

// openEditor is ProductCLI#open_source_editor with SourceEditor#open: an explicit local
// opening of the selected origin's file.
func openEditor(matches []any, sourceMap rec.Map, options editorOptions) ojson.Object {
	if options.sourceRoot == "" || !options.hasArguments {
		rec.Fail("Opening an editor requires --source-root and --editor-argv; placeholders: {file}, {line}, {column}")
	}
	selected := rec.AsMap(candidate(matches, options.match, options.hasMatch, "--match-index"))
	origin := rec.AsMap(candidate(rec.FetchList(selected, "origins"), options.origin, options.hasOrigin, "--origin-index"))
	output := options.output
	if output == "" {
		// The editor's scratch is relatex's own, in the cache: never in the calling directory.
		output = filepath.Join(command.CacheRoot(), "editor")
	}
	scratch := filepath.Join(workspaceRoot(output), "_scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		panic(systemFailureAt(err, "rb_dir_s_mkdir"))
	}
	workspace := makeTemporary(scratch, "editor-")
	root := realpath(options.sourceRoot)
	resolver := artifacts.New(root, hashes(rec.FetchMap(sourceMap, "source_files")), nil)
	words := []string{}
	for _, argument := range options.arguments {
		text, ok := argument.(string)
		if !ok {
			rec.Fail("Editor arguments must be a nonempty string array")
		}
		words = append(words, text)
	}
	if len(words) == 0 {
		rec.Fail("Editor arguments must be a nonempty string array")
	}
	return launch(resolver, origin, words, workspace)
}

var placeholder = regexp.MustCompile(`\{(?:file|line|column)\}`)

func launch(resolver *artifacts.Resolver, origin rec.Map, words []string, workspace string) ojson.Object {
	file := resolver.Resolve(rec.FetchString(origin, "file"))
	column, ok := origin["native_column"]
	if !ok {
		characters, has := origin["characters"].([]any)
		first := int64(0)
		if has {
			first = rec.AsInt(characters[0])
		}
		column = first + 1
	}
	replacements := map[string]string{"{file}": file, "{line}": rec.ToS(rec.Fetch(origin, "line")), "{column}": rec.ToS(column)}
	arguments := make([]any, len(words))
	for index, word := range words {
		words[index] = placeholder.ReplaceAllStringFunc(word, func(key string) string { return replacements[key] })
	}
	executable := command.Executable(words[0])
	if executable == "" {
		rec.Fail("Editor executable unavailable")
	}
	words[0] = executable
	for index, word := range words {
		arguments[index] = word
	}
	log := filepath.Join(workspace, "editor.log")
	logFile, err := os.Create(log)
	if err != nil {
		panic(systemCall(err))
	}
	defer logFile.Close()
	environment := map[string]string{"TMPDIR": filepath.Join(workspace, "_tmp"), "XDG_CACHE_HOME": filepath.Join(workspace, "_cache"),
		"XDG_CONFIG_HOME": filepath.Join(workspace, "_config")}
	if runtime.GOOS == "windows" {
		environment["TMP"], environment["TEMP"] = environment["TMPDIR"], environment["TMPDIR"]
	}
	for _, directory := range environment {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			panic(systemFailureAt(err, "rb_dir_s_mkdir"))
		}
	}
	process := exec.Command(words[0], words[1:]...)
	process.Env = command.Environment(environment)
	process.Stdout, process.Stderr = logFile, logFile
	process.SysProcAttr = command.DetachedAttributes()
	if err := process.Start(); err != nil {
		panic(systemCall(err))
	}
	go func() { _ = process.Wait() }()
	return ojson.Object{{"status", "launched"}, {"pid", int64(process.Process.Pid)}, {"arguments", arguments}, {"log", log},
		{"source_precision", rec.Fetch(origin, "precision")}, {"scope", "Explicit local editor opening; no content qualification"}}
}

func candidate(candidates []any, index int64, given bool, flag string) any {
	if len(candidates) == 0 {
		rec.Fail("Source query has no candidates")
	}
	if !given && len(candidates) > 1 {
		rec.Fail("Source query is ambiguous; select %s", flag)
	}
	if index < 0 || index >= int64(len(candidates)) {
		rec.Fail("Invalid %s", flag)
	}
	return candidates[index]
}

// extent is the source range of an origin: first line and column, last line and column.
type extent struct{ line, column, endLine, endColumn float64 }

func extentOf(origin rec.Map) (extent, bool) {
	first, _ := rec.Number(rec.Fetch(origin, "line"))
	last := first
	if value, ok := origin["end_line"]; ok {
		last, _ = rec.Number(value)
	}
	characters, ok := origin["characters"].([]any)
	if !ok {
		return extent{}, false
	}
	begin, _ := rec.Number(characters[0])
	end, _ := rec.Number(characters[1])
	return extent{first, begin, last, end}, true
}

// inside is whether e lies within outer and is not the same range.
func (e extent) inside(outer extent) bool {
	return e != outer && compare(outer.line, outer.column, e.line, e.column) <= 0 &&
		compare(e.endLine, e.endColumn, outer.endLine, outer.endColumn) <= 0
}

// narrowest drops, among the math words a selection overlaps, those whose range holds the
// range of another match: clicking on g(s) also overlaps the equation number, whose range is
// the whole row, and the token the user chose is the better answer.
func narrowest(matches []any, q query) []any {
	lines := [2]float64{float64(q.line), float64(max(q.line, q.endLine))}
	ranges := make([]extent, len(matches))
	math := make([]bool, len(matches))
	for index, word := range matches {
		for _, origin := range rec.FetchList(rec.AsMap(word), "origins") {
			if found, ok := extentOf(rec.AsMap(origin)); ok && rec.AsMap(origin)["precision"] == "math" &&
				rec.Fetch(rec.AsMap(origin), "file") == q.file && found.line <= lines[1] && found.endLine >= lines[0] {
				ranges[index], math[index] = found, true
				break
			}
		}
	}
	kept := []any{}
	for index, word := range matches {
		wider := false
		for other := range matches {
			wider = wider || (math[index] && math[other] && ranges[other].inside(ranges[index]))
		}
		if !wider {
			kept = append(kept, word)
		}
	}
	return kept
}
