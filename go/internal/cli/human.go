package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sarielhp/relatex/internal/engine"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/pipeline"
	"github.com/sarielhp/relatex/internal/rec"
)

// The default output of a conversion is for a person: a busy indicator on a terminal, and at the end
// one line saying where the result is, how many errors and warnings there were and where the report
// is. --report keeps the JSON run report on stdout, as the tools read it.

// maxErrorLines is how many error-level diagnostics the end lists before pointing to the report.
const maxErrorLines = 10

// stageWords are the stages as a person reads them.
var stageWords = map[string]string{"input": "reading the input", "engine": "typesetting", "reconcile": "analyzing",
	"layout": "measuring the pages", "synchronize": "mapping words to the source", "export": "writing the formats",
	"publish": "publishing"}

// humanRun is a conversion whose output is for a person.
type humanRun struct {
	attempt  string // the attempt directory, once the conversion has one
	progress *progress
}

// conversionHuman is the running conversion that prints for a person, nil for --report and rlx.
var conversionHuman *humanRun

// startHuman begins a conversion's output for a person: a busy indicator when stderr is a terminal.
func startHuman(label string) *humanRun {
	run := &humanRun{}
	engine.Quiet = true
	engine.QuietFailures = true
	if colourfulTerminal(messages) {
		run.progress = startProgress(messages.(*os.File), label)
		pipeline.Progress = run.progress.stage
	}
	conversionHuman = run
	return run
}

// finish stops the busy indicator.
func (h *humanRun) finish() {
	pipeline.Progress = nil
	if h.progress != nil {
		h.progress.stop()
	}
}

// colourfulTerminal is whether the writer is a terminal.
func colourfulTerminal(out any) bool {
	file, ok := out.(*os.File)
	return ok && file != nil && isTerminal(file)
}

// progress is the busy indicator: a spinner and the stage the conversion is in.
type progress struct {
	out   *os.File
	label string
	lock  sync.Mutex
	text  string
	done  chan struct{}
	ended sync.WaitGroup
}

func startProgress(out *os.File, label string) *progress {
	p := &progress{out: out, label: label, done: make(chan struct{})}
	p.text = "starting"
	p.ended.Add(1)
	go p.spin()
	return p
}

// stage is the pipeline's hook: the stage begins.
func (p *progress) stage(name string, index, total int) {
	words := stageWords[name]
	if words == "" {
		words = name
	}
	p.lock.Lock()
	p.text = fmt.Sprintf("%s (%d/%d)", words, index, total)
	p.lock.Unlock()
}

func (p *progress) spin() {
	defer p.ended.Done()
	frames := []string{"|", "/", "-", "\\"}
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	for tick := 0; ; tick++ {
		p.lock.Lock()
		text := p.text
		p.lock.Unlock()
		_, _ = fmt.Fprintf(p.out, "\r\x1b[2K%s converting %s: %s", frames[tick%len(frames)], p.label, text)
		select {
		case <-p.done:
			_, _ = fmt.Fprint(p.out, "\r\x1b[2K")
			return
		case <-ticker.C:
		}
	}
}

// stop clears the indicator.
func (p *progress) stop() {
	close(p.done)
	p.ended.Wait()
}

// inputLabel names the input for the indicator: the file or directory, without its path.
func inputLabel(input string) string {
	if absolute, err := filepath.Abs(input); err == nil {
		return filepath.Base(absolute)
	}
	return input
}

// reportFile is the run report written in the attempt directory, "" when there is none.
func reportFile(attempt string) string {
	if attempt == "" {
		return ""
	}
	path := filepath.Join(attempt, "run.json")
	if regular(path) {
		return path
	}
	return ""
}

// countText is "1 error", "2 errors".
func countText(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

// diagnosticCounts are the errors (fatal and error) and the warnings (alert and warning) of a run.
func diagnosticCounts(result ojson.Object) (errors, warnings int) {
	counts, _ := result.Get("diagnostics").(ojson.Object)
	number := func(level string) int {
		value, _ := counts.Get(level).(int64)
		return int(value)
	}
	return number("fatal") + number("error"), number("alert") + number("warning")
}

var errorLevelPattern = regexp.MustCompile(`: (fatal|error)\[`)

// errorLines are the error-level diagnostics (fatal and error) of a run.
func errorLines(result ojson.Object) []string {
	var lines []string
	for _, line := range diagnosticLines(result) {
		if errorLevelPattern.MatchString(line) {
			lines = append(lines, line)
		}
	}
	return lines
}

// humanErrors is the short list of error-level diagnostics: at most maxErrorLines, then how many
// more the report holds.
func humanErrors(result ojson.Object, report string, colour bool) string {
	lines := errorLines(result)
	if len(lines) == 0 {
		return ""
	}
	shown := lines[:min(len(lines), maxErrorLines)]
	text := diagnosticsText(shown, colour)
	if more := len(lines) - len(shown); more > 0 {
		text += fmt.Sprintf("\nand %d more in %s", more, report)
	}
	return text + "\n"
}

// requestedPlace is where a person finds what was asked for: the one deliverable when a single
// format was written, else the output directory; the product directory when nothing was delivered.
func requestedPlace(result ojson.Object) string {
	outputs, _ := result.Get("outputs").([]any)
	if len(outputs) == 1 {
		return rec.ToS(outputs[0])
	}
	if output := rec.ToS(result.Get("output")); output != "" {
		if paper := rec.ToS(result.Get("input")); paper != "" && len(outputs) > 1 && outputHoldsInput(output, paper) {
			// The output root is the paper's own directory: name the entries, not the directory.
			names := make([]string, len(outputs))
			for index, item := range outputs {
				names[index] = rec.ToS(item)
			}
			return strings.Join(names, ", ")
		}
		return output
	}
	return rec.ToS(result.Get("product"))
}

// humanSummary is the one line a conversion ends with.
func humanSummary(result ojson.Object, colour bool) string {
	errors, warnings := diagnosticCounts(result)
	report := reportFile(rec.ToS(result.Get("directory")))
	details := countText(errors, "error") + ", " + countText(warnings, "warning")
	if report != "" {
		details += "; report " + report
	}
	if result.Get("status") == "failed" {
		reason := "see the report"
		if lines := errorLines(result); len(lines) > 0 {
			reason = lines[0]
		}
		return paint("Conversion failed", levelColours["error"], colour) + ": " + reason + " (" + details + ")"
	}
	return paint("Conversion succeeded", "32", colour) + ": " + requestedPlace(result) + " (" + details + ")"
}

// humanFailure is what stops a conversion, for a person: the first line of the failure and the
// report that holds the rest.
func humanFailure(message string, colour bool) string {
	lines := messageLines(shorten(message))
	head := ""
	if len(lines) > 0 {
		head = lines[0]
	}
	text := paint("Conversion failed", levelColours["error"], colour) + ": " + head
	if rest, said := strings.CutPrefix(head, "Conversion failed"); said {
		text = paint("Conversion failed", levelColours["error"], colour) + rest
	}
	if report := reportFile(conversionHuman.attempt); report != "" {
		text += " (report " + report + ")"
	}
	if len(lines) > 1 {
		text += "\n" + strings.Join(lines[1:], "\n")
	}
	return text
}
