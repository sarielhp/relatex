package engine

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/rec"
)

var (
	// followUps are the lines TeX prints after the real error (TeXFailure::FOLLOW_UPS).
	followUps = regexp.MustCompile(`\A!\s+(?:==> Fatal error occurred|Emergency stop)`)
	// context is TeXFailure::CONTEXT: the line number and the text read so far.
	context = regexp.MustCompile(`\Al\.(\d+)[ \t\n\v\f\r]?(.*)\z`)
	// collectorFailure is PaperBuilds::COLLECTOR_FAILURE.
	collectorFailure = regexp.MustCompile(`\Arelatex: unsupported graphic:`)
	// sourceFile is the files TeXFailure.location searches.
	sourceFile = regexp.MustCompile(`\.(?:tex|sty|cls|ltx|def)\z`)
)

// engineFailure is GeneralConversion#engine_failure: the ordinary build's own LaTeX error when
// it failed, as the author would see it; else what kept the builds from converging.
func engineFailure(directory string, results rec.Map) string {
	for _, strategy := range []string{"control", "external"} {
		build, ok := results[strategy].(rec.Map)
		if !ok || build["compiled"] == true {
			continue
		}
		errors, _ := build["errors"].([]any)
		if text := describeTeXFailure(stringList(errors), filepath.Join(directory, strategy, "working")); text != "" {
			return text
		}
	}
	details := []string{}
	for _, strategy := range []string{"control", "external"} {
		build, _ := results[strategy].(rec.Map)
		errors, _ := build["errors"].([]any)
		for _, line := range stringList(errors) {
			if collectorFailure.MatchString(line) && !slices.Contains(details, line) {
				details = append(details, line)
			}
		}
	}
	if len(details) == 0 {
		if text := unconverged(directory, results); text != "" {
			return text
		}
	}
	return strings.Join(append([]string{"Ordinary/external builds did not both compile and converge"}, details...), ": ")
}

// undefinedWarning is LaTeX's (and natbib's) warning for a reference or citation it could not
// resolve: "Reference `sec:x' on page 1 undefined", "Citation `knuth84' on page 2 undefined".
var undefinedWarning = regexp.MustCompile("(Reference|Citation) [`']([^']*)' on page \\S+ undefined")

// shownUndefined is how many undefined names the failure lists.
const shownUndefined = 5

// unconverged describes builds that compiled but did not converge: LaTeX still asked for another
// run after the last pass of a build (the ordinary one first), with the references and citations it
// left undefined, the first few by name; "" when no build compiled with its log at hand. References
// a draft leaves undefined on stable aux files are no failure (reference.unresolved).
func unconverged(directory string, results rec.Map) string {
	for _, strategy := range []string{"control", "external"} {
		build, _ := results[strategy].(rec.Map)
		if build["compiled"] != true || build["unresolved_references_or_rerun"] != true {
			continue
		}
		passes, _ := build["passes"].(int64)
		log := filepath.Join(directory, strategy, "output", "pass-"+strconv.FormatInt(passes, 10)+".log")
		if data, err := os.ReadFile(log); err == nil {
			return unresolvedFailure(string(data), passes, filepath.Base(log))
		}
	}
	return ""
}

// unresolvedFailure is the failure for a last pass whose log is text.
func unresolvedFailure(text string, passes int64, log string) string {
	names := []string{}
	for _, match := range undefinedWarning.FindAllStringSubmatch(text, -1) {
		name := strings.ToLower(match[1]) + " " + match[2]
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	after := " after " + strconv.FormatInt(passes, 10) + " LaTeX passes"
	asked := "The builds did not converge: LaTeX still asked for another run" + after
	if len(names) == 0 {
		return asked + " (see " + log + ")"
	}
	shown := strings.Join(names[:min(len(names), shownUndefined)], ", ")
	if len(names) > shownUndefined {
		shown += " and " + strconv.Itoa(len(names)-shownUndefined) + " more"
	}
	return asked + "; undefined: " + shown
}

func stringList(values []any) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = rec.AsString(value)
	}
	return result
}

// describeTeXFailure is TeXFailure.describe: the first error LuaLaTeX reported, with the file
// and line where TeX stopped; "" when the log holds no error.
func describeTeXFailure(errors []string, source string) string {
	index := slices.IndexFunc(errors, func(line string) bool { return strings.HasPrefix(line, "!") && !followUps.MatchString(line) })
	if index < 0 {
		return ""
	}
	message := rec.Strip(strings.TrimPrefix(errors[index], "!"))
	text := "LuaLaTeX failed on the paper: " + message
	for _, line := range errors[index:] {
		match := context.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		number, err := strconv.Atoi(match[1])
		check(err)
		return text + "\n  at " + location(source, number, match[2]) + ": " + rec.Strip(match[2])
	}
	return text
}

// location is TeXFailure.location: the paper file holding the text on that line, else the
// line alone.
func location(source string, number int, text string) string {
	probe := []rune(rec.Strip(text))
	probe = probe[:min(len(probe), 40)]
	for _, name := range sourceFiles(source) {
		line, ok := lineAt(filepath.Join(source, name), number)
		if ok && len(probe) > 0 && strings.Contains(line, string(probe)) {
			return name + ":" + strconv.Itoa(number)
		}
	}
	return "line " + strconv.Itoa(number)
}

// sourceFiles is Dir.glob('**/*.{tex,sty,cls,ltx,def}', base: source).sort.
func sourceFiles(source string) []string {
	found := []string{}
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != source && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if sourceFile.MatchString(entry.Name()) && !entry.IsDir() {
			relative, _ := filepath.Rel(source, path)
			found = append(found, fspath.Slash(relative))
		}
		return nil
	})
	check(err)
	slices.Sort(found)
	return found
}

func lineAt(path string, number int) (string, bool) {
	file, err := os.Open(path)
	check(err)
	defer file.Close()
	reader := bufio.NewReader(file)
	for at := 1; ; at++ {
		line, err := reader.ReadString('\n')
		if at == number && (line != "" || err == nil) {
			return line, true
		}
		if err != nil {
			return "", false
		}
	}
}

// macroDefinition is MathMacros::DEFINITION: a definition of a named command.
var macroDefinition = regexp.MustCompile(
	`\\(?:(?:re|provide)?newcommand\*?|DeclareRobustCommand\*?|DeclareMathOperator\*?|DeclarePairedDelimiter(?:XPP|X)?|def|gdef|edef|xdef|let)` +
		`[ \t\n\v\f\r]*\{?[ \t\n\v\f\r]*\\([A-Za-z]+)`)

var macroSource = regexp.MustCompile(`\.(?:tex|sty|cls|def|ltx)\z`)

// writeMacroNames is MathMacros.write_names: the candidate macro names defined anywhere in
// the paper's own files (not the collector's), one per line.
func writeMacroNames(directory, path string) {
	names := []string{}
	err := filepath.WalkDir(directory, func(file string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if file != directory && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !macroSource.MatchString(entry.Name()) || strings.Contains(file, "_relatex_probe") {
			return nil
		}
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			return readErr
		}
		for _, match := range macroDefinition.FindAllStringSubmatch(strings.ToValidUTF8(string(data), "�"), -1) {
			if !slices.Contains(names, match[1]) {
				names = append(names, match[1])
			}
		}
		return nil
	})
	check(err)
	slices.Sort(names)
	check(os.WriteFile(path, []byte(strings.Join(names, "\n")+"\n"), 0o644))
}
