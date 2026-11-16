package authormath

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

// restarts: TeX stops a run after 100 errors in one paragraph, or at an emergency stop; the
// run then starts again after the candidate it stopped in, a few times at most.
const restarts = 6

var (
	stoppedFile   = regexp.MustCompile(`\((?:\./)?(c\d{5})\.tex`)
	candidateName = regexp.MustCompile(`\Ac\d{5}\z`)
)

// Typesetter is the retypesetting run both checks share: the author's TeX (decision 064) and the
// formula TeX written from MathML (decision 141). Each candidate is a file NAME.tex in Directory
// (named by CandidateID); one LuaLaTeX run through the paper's own math collector inputs them all
// after Header (the class, packages and macros), and restarts after a candidate TeX gave up on.
type Typesetter struct {
	Directory string
	Header    string
	Doctor    *texsandbox.Doctor
	Attempt   string // RELATEX_ATTEMPT, the run's name in the collector's records
}

// CandidateID is the file name of the index-th candidate (from 0).
func CandidateID(index int) string { return fmt.Sprintf("c%05d", index+1) }

// Typeset is each candidate's MathML, its formulas in the order TeX made them, by candidate id.
// A candidate TeX did not typeset has no entry.
func (t *Typesetter) Typeset(ids []string) map[string]any {
	results := map[string]any{}
	remaining := ids
	fonts := texsandbox.NewFontGenerator(t.Doctor.Engine)
	for attempt := range restarts {
		if len(remaining) == 0 {
			break
		}
		t.write(remaining)
		t.run(attempt, fonts)
		for id, mathml := range t.formulas(attempt) {
			results[id] = mathml
		}
		stopped := t.stoppedIn()
		if stopped == "" {
			break
		}
		remaining = after(remaining, stopped)
	}
	return results
}

// after is the ids after stopped; none when stopped is not among them.
func after(ids []string, stopped string) []string {
	for index, id := range ids {
		if id == stopped {
			return ids[index+1:]
		}
	}
	return nil
}

// stoppedIn is the candidate a stopped run was reading, from the files its log shows opened.
func (t *Typesetter) stoppedIn() string {
	bytes, err := os.ReadFile(filepath.Join(t.Directory, "paper.log"))
	if err != nil {
		return ""
	}
	log := Scrub(bytes)
	if !strings.Contains(log, "Fatal error occurred") && !strings.Contains(log, "Emergency stop") {
		return ""
	}
	matches := stoppedFile.FindAllStringSubmatch(log, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1][1]
}

func (t *Typesetter) write(ids []string) {
	inputs := make([]string, len(ids))
	for index, id := range ids {
		inputs[index] = "\\input{" + id + "}"
	}
	writeFile(filepath.Join(t.Directory, "paper.tex"),
		t.Header+"\n\\begin{document}\n"+strings.Join(inputs, "\n")+"\n\\end{document}\n")
	writeFile(filepath.Join(t.Directory, "inject.tex"),
		"\\AddToHook{begindocument/before}{\\RequirePackage{relatex-external}}\n\\input{paper.tex}\n")
}

func (t *Typesetter) run(attempt int, fonts *texsandbox.FontGenerator) {
	tex := filepath.Join(Root, "tex")
	environment := texsandbox.WithGeneratedFonts(map[string]string{"TEXINPUTS": command.SearchList(".", tex, ""), "LUAINPUTS": command.SearchList(tex, ""),
		"TEXFORMATS": filepath.Dir(t.Doctor.FormatPath()), "TEXMFVAR": filepath.Join(t.Directory, "texmf-var"),
		"TEXMFCACHE":     texsandbox.SeedFontCache(filepath.Join(t.Directory, "texmf-cache"), t.Doctor),
		"RELATEX_STREAM": filepath.ToSlash(filepath.Join(t.Directory, fmt.Sprintf("stream-%d.jsonl", attempt))),
		"RELATEX_MATHML": filepath.ToSlash(filepath.Join(t.Directory, "paper-luamml-mathml.html")), "RELATEX_ATTEMPT": t.Attempt,
		"RELATEX_PASS": "1", "RELATEX_INPUT": command.FileDigest(filepath.Join(t.Directory, "paper.tex")),
		"RELATEX_STRATEGY": "external", "RELATEX_AUTHOR_MATH": "1"})
	arguments := []string{t.Doctor.Engine, "-fmt=lualatex", "-interaction=nonstopmode", "-no-shell-escape", "-jobname=paper",
		"inject.tex"}
	logPath := filepath.Join(t.Directory, "paper.run.log")
	deadline := time.Now().Add(600 * time.Second)
	fonts.RunPass(logPath, deadline, func() rec.Map {
		return command.Run(arguments, t.Directory, logPath, 600*time.Second, environment)
	}, nil)
}

// formulas: candidate id => the MathML of its formulas, in order. Each candidate file's formulas
// carry it as their source; an alignment's rows share the entity of its cells.
func (t *Typesetter) formulas(attempt int) map[string]any {
	file, err := os.Open(filepath.Join(t.Directory, fmt.Sprintf("stream-%d.jsonl", attempt)))
	if err != nil {
		return map[string]any{}
	}
	defer file.Close()
	math := []rec.Map{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<30)
	for scanner.Scan() {
		value, decodeErr := canonjson.Decode(strings.NewReader(scanner.Text()))
		row, isMap := value.(rec.Map)
		if decodeErr == nil && isMap && row["kind"] == "math" && truthy(row["formula"]) {
			math = append(math, row)
		}
	}
	entities := map[any]string{}
	for _, row := range math {
		location, _ := row["source"].(rec.Map)
		entity := rec.Fetch(row, "entity")
		if truthy(location["group"]) && entities[entity] == "" {
			entities[entity] = candidateOf(row)
		}
	}
	found := map[string]any{}
	for _, row := range math {
		location, hasSource := row["source"].(rec.Map)
		name := entities[rec.Fetch(row, "entity")]
		if truthy(row["source"]) {
			name = ""
			if !hasSource || !truthy(location["group"]) {
				name = candidateOf(row)
			}
		}
		if name != "" {
			list, _ := found[name].([]any)
			found[name] = append(list, rec.Fetch(row, "mathml"))
		}
	}
	return found
}

func candidateOf(row rec.Map) string {
	location, _ := row["source"].(rec.Map)
	name := filepath.Base(fmt.Sprint(nilToEmpty(location["file"])))
	name = strings.TrimSuffix(name, ".tex")
	if candidateName.MatchString(name) {
		return name
	}
	return ""
}

func nilToEmpty(value any) any {
	if value == nil {
		return ""
	}
	return value
}

// CollectorDigest is the digest of the collector's files (tex/*), which a cached retypesetting
// depends on.
func CollectorDigest() string { return collectorDigest() }

// Difference is "" when two MathML trees have the same structure (decision 064's projection:
// element names, text and mathvariant), else where they first differ.
func Difference(original, typeset string) string { return difference(original, typeset) }
