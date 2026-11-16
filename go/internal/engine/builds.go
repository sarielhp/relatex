package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/artifacts"
	"github.com/sarielhp/relatex/internal/assets"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

var (
	// rerunRequest is LaTeX asking for another pass: a build that still asks after its last pass did
	// not converge. undefinedReferences is a reference or citation left undefined, which a draft keeps
	// on every pass once its aux files are stable: the build converged, and the conversion reports the
	// reference (reference.unresolved) instead of failing. logErrors is PaperBuilds::LOG_ERRORS.
	rerunRequest        = regexp.MustCompile(`(?i)Rerun to get|Please rerun|rerun LaTeX`)
	undefinedReferences = regexp.MustCompile(`(?i)undefined references|Citation .*undefined|Reference .*undefined`)
	logErrors           = regexp.MustCompile(`^!|^l\.[0-9]+|^relatex: unsupported graphic:`)
	// convergence is PaperCheck::CONVERGENCE: the files whose stability ends the passes.
	convergence = regexp.MustCompile(`\.(?:aux|toc|out|lof|lot|bcf|run\.xml|nav|snm)\z`)
	// stale is PaperCheck::STALE: products of the author's own builds, set aside before ours.
	stale = artifacts.AuthorBuildProducts
)

// build is PaperBuilds::Build. working is TeX's working directory: the main file's directory in
// the source, as `cd sub && lualatex main.tex` would build a main file in a subdirectory.
type build struct {
	strategy, source, output, collector, job, working string
}

// variant is SourceProbe#variant: one build, its diagnosis and the earlier passes' streams
// pruned; a failure of the build is its result.
func (p *Probe) variant(strategy string) (result rec.Map) {
	defer p.failedBuild(&result)
	b := p.prepareVariant(strategy)
	if p.seed != nil {
		p.seed.Apply(b.output)
		// A shipped JOB.bbl is read from the source: a seeded one (an earlier attempt's processor
		// output) would shadow it (second cohort 310).
		if shippedBibliography(b) {
			check(removeIfPresent(filepath.Join(b.output, b.job+".bbl")))
		}
	}
	command.Trace("engine: " + strategy + " prepared")
	stages := p.compileVariant(b)
	command.Trace("engine: " + strategy + " compiled")
	result = p.diagnose(b, stages)
	command.Trace("engine: " + strategy + " diagnosed")
	pruneStreams(b, len(stages))
	state := "failed"
	if result["compiled"] == true {
		state = "compiled"
	}
	if (state == "compiled" && !Quiet) || (state != "compiled" && !QuietFailures) {
		fmt.Fprintf(os.Stderr, "%s: %s\n", strategy, state)
	}
	return result
}

// failedBuild is variant's rescue of Relatex::Failure.
func (p *Probe) failedBuild(result *rec.Map) {
	recovered := recover()
	if recovered == nil {
		return
	}
	failure, ok := recovered.(rec.Failure)
	if !ok || failure.Lookup {
		panic(recovered)
	}
	*result = rec.Map{"compiled": false, "error": failure.Message}
}

// prepareVariant is PaperBuilds#prepare_variant: the working copy with the collector, the
// author's stale products set aside, the wrapper and the working inventory.
func (p *Probe) prepareVariant(strategy string) build {
	source := filepath.Join(p.Directory, strategy, "working")
	output := filepath.Join(p.Directory, strategy, "output")
	check(os.MkdirAll(output, 0o755))
	copyTree(filepath.Join(p.Directory, "input", "files"), source)
	if hasShim(p.compatibility, "inputenc") {
		p.recordReencoded(reencodeLegacyInput(source))
	}
	collector := filepath.Join(source, "_relatex_probe")
	if _, err := os.Lstat(collector); err == nil {
		rec.Fail("Collector directory collision")
	}
	copyTree(filepath.Join(Root, "tex"), collector)
	job := JobName(p.main)
	for _, extension := range stale {
		path := filepath.Join(p.workingIn(source), job+"."+extension)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			check(os.Rename(path, filepath.Join(output, "supplied."+extension)))
		}
	}
	check(os.WriteFile(filepath.Join(output, "inject.tex"), []byte(p.wrapper(strategy)), 0o644))
	writeJSON(filepath.Join(output, "working-manifest.json"), artifacts.FileManifest(source))
	return build{strategy: strategy, source: source, output: output, collector: collector, job: job, working: p.workingIn(source)}
}

// workingIn is TeX's working directory in a copy of the source: the main file's directory.
func (p *Probe) workingIn(source string) string {
	return filepath.Join(source, filepath.Dir(p.main))
}

// searchPath is a kpathsea path of the directories, each once, ending in the default path.
func searchPath(directories ...string) string {
	entries := []string{}
	seen := map[string]bool{}
	for _, directory := range directories {
		if !seen[directory] {
			seen[directory] = true
			entries = append(entries, directory)
		}
	}
	return command.SearchList(append(entries, "")...)
}

// wrapper is SourceProbe#paper_wrapper: the external build loads the collector first.
func (p *Probe) wrapper(strategy string) string {
	prefix := ""
	if strategy != "control" {
		// relatex-preamble keeps the paragraphs the preamble sets in inserts (footnote text) for the
		// collector to read once it starts.
		prefix = "\\directlua{ltd_preamble = require('relatex-preamble')}\n" +
			"\\AddToHook{begindocument/before}{\\RequirePackage{relatex-external}}\n"
		if os.Getenv("RELATEX_LUAPROF") != "" {
			prefix = "\\directlua{dofile(\"_relatex_probe/relatex-prof.lua\"); relatex_prof_mark(\"start\")}\n" +
				"\\AddToHook{begindocument/before}{\\directlua{relatex_prof_mark(\"begindocument/before\")}}\n" +
				"\\AddToHook{begindocument/end}{\\directlua{relatex_prof_mark(\"begindocument/end\"); relatex_prof_wrap()}}\n" +
				"\\AddToHook{shipout/before}{\\directlua{relatex_prof_mark(\"shipout\")}}\n" + prefix
		}
	}
	return p.compatibilityLine() + p.slidePrefix() + prefix + "\\input{" + filepath.Base(p.main) + "}\n"
}

func (p *Probe) minimumPasses() int {
	if p.seed != nil {
		return 1
	}
	return 2
}

// compileVariant is SourceProbe#compile_variant: passes until the cross-reference files stop
// changing and the log asks for no rerun, the bibliography rerun whenever its request changes.
func (p *Probe) compileVariant(b build) []rec.Map {
	stages := []rec.Map{}
	fonts := texsandbox.NewFontGenerator(p.doctor.Engine)
	defer func() {
		p.recordGeneratedFonts(fonts.Generated)
	}()
	var previous rec.Map
	var bibliography []string
	indexed := ""
	if p.seed != nil {
		previous = convergenceState(b)
		writeJSON(filepath.Join(b.output, "pass-0-state.json"), previous)
		bibliography = p.seed.BibliographySignature(b.job)
	}
	for pass := 1; pass <= 5; pass++ {
		read, readIndex := bibliographyDigest(b), indexDigest(b)
		stage := p.texPass(b, pass, fonts)
		stages = append(stages, stage)
		state := saveState(b, pass)
		if !passed(stage) {
			break
		}
		// A source that ships its JOB.bbl is typeset with it, as the ordinary build and arXiv do:
		// running the processor again could print what the author's never did (second cohort 310:
		// a \url its shipped bibliography spells \texttt). One biblatex rejects as another format
		// version still needs the processor (paper 40).
		log := readLog(filepath.Join(b.output, fmt.Sprintf("pass-%d.log", pass)))
		rejected := shippedBibliography(b) && rejectedBibliography.MatchString(strings.ReplaceAll(log, "\n", ""))
		honoured := shippedBibliography(b) && !rejected
		// The seed's processor output was removed for the shipped one; its request alone runs nothing.
		if rejected && bibliographyDigest(b) == "" {
			bibliography = nil
		}
		if signature := p.bibliographyRequest(b); !honoured && signature != nil && !slices.Equal(signature, bibliography) {
			p.runBibliography(b, signature[0])
			bibliography = signature
		}
		indexed = p.updateIndex(b, indexed)
		// A bibliography or index the pass did not read (the processor rewrote it) needs another pass.
		changed := bibliographyDigest(b) != read || indexDigest(b) != readIndex
		if pass >= p.minimumPasses() && previous != nil && rec.Equal(state, previous) && !changed && !rerunRequest.MatchString(log) {
			break
		}
		previous = state
	}
	return stages
}

// bibliographyDigest is the SHA-256 of the build's JOB.bbl, "" without one.
func bibliographyDigest(b build) string {
	path := filepath.Join(b.output, b.job+".bbl")
	if !isFile(path) {
		return ""
	}
	return command.FileDigest(path)
}

// readLog is a log as Ruby reads it for matching: invalid bytes replaced (String#scrub).
func readLog(path string) string {
	data, err := os.ReadFile(path)
	check(err)
	return strings.ToValidUTF8(string(data), "\uFFFD")
}

// texPass runs one pass. A pass that starts without JOB.aux runs on past errors and may settle
// (decision 082); every other pass halts on its first error. Missing TFMs trigger mktextfm and
// rerun the pass until all needed fonts are generated, nothing new is recovered, or the build cap
// is reached (decision 158).
func (p *Probe) texPass(b build, pass int, fonts *texsandbox.FontGenerator) rec.Map {
	log := filepath.Join(b.output, fmt.Sprintf("pass-%d.log", pass))
	settling := false
	return fonts.RunPass(log, p.deadline, func() rec.Map {
		var stage rec.Map
		stage, settling, _ = p.runTexPass(b, pass)
		return stage
	}, func() {
		if settling {
			removeIfPresent(filepath.Join(b.output, b.job+".aux"))
		}
	})
}

func (p *Probe) runTexPass(b build, pass int) (rec.Map, bool, string) {
	environment := p.environment(b, pass)
	settling := !isFile(filepath.Join(b.output, b.job+".aux"))
	arguments := p.texPassArguments(b, settling)
	wrapped := p.doctor.WrapWith(arguments, b.source, b.output, b.working, environment, p.options.Host.mounts())
	log := filepath.Join(b.output, fmt.Sprintf("pass-%d.log", pass))
	stage := command.Run(wrapped, b.working, log, p.remaining(), environment)
	command.WriteReceipt(filepath.Join(b.output, fmt.Sprintf("pass-%d-stage.json", pass)), stage)
	if settling && settles(stage, b, log) {
		// The receipt says so, for the evidence verifiers (papersource, PaperSource).
		stage["settled"] = true
		writeJSON(filepath.Join(b.output, fmt.Sprintf("pass-%d-stage.json", pass)), stage)
	}
	return stage, settling, log
}

func (p *Probe) texPassArguments(b build, settling bool) []string {
	arguments := []string{p.doctor.Engine, "-fmt=lualatex", "-interaction=nonstopmode"}
	if !settling {
		arguments = append(arguments, "-halt-on-error")
	}
	// TeX reads its command line as TeX: a backslash would start a control sequence, so the
	// paths it gets use "/" on every platform.
	return append(arguments, "-no-shell-escape",
		"-synctex=1", "-recorder", "-jobname="+b.job, "-output-directory="+filepath.ToSlash(b.output),
		filepath.ToSlash(filepath.Join(b.output, "inject.tex")))
}

// settlingFatal is PaperCheck::SETTLING_FATAL: a stop no aux file can settle.
var settlingFatal = regexp.MustCompile(`(?m)^! Emergency stop|Fatal error occurred|\(job aborted`)

// settles: a failed first pass that wrote JOB.aux without a fatal stop.
func settles(stage rec.Map, b build, log string) bool {
	if succeeded(stage) || stage["timeout"] == true || !isFile(filepath.Join(b.output, b.job+".aux")) {
		return false
	}
	return !settlingFatal.MatchString(readLog(log))
}

// passed: a pass that succeeded or settled.
func passed(stage rec.Map) bool { return succeeded(stage) || stage["settled"] == true }

// settlingErrors are a settled first pass's errors, recorded though they did not decide the build.
func settlingErrors(b build, stages []rec.Map) []any {
	errors := []any{}
	if len(stages) == 0 || stages[0]["settled"] != true {
		return errors
	}
	for _, line := range lines(readLog(filepath.Join(b.output, "pass-1.log"))) {
		line = rec.Strip(line)
		if strings.HasPrefix(line, "! ") && !slices.Contains(errors, any(line)) && len(errors) < 12 {
			errors = append(errors, line)
		}
	}
	return errors
}

// environment is PaperBuilds#environment_for: the collector's inputs and records, the build's
// private TeX caches, and the author-math requests.
func (p *Probe) environment(b build, pass int) map[string]string {
	cache := filepath.Join(p.Directory, b.strategy, "texmf-cache")
	seeded := "0"
	if p.seed != nil {
		seeded = "1"
	}
	environment := texsandbox.WithGeneratedFonts(map[string]string{"PATH": command.ToolPath(p.doctor.Engine),
		"TEXINPUTS": command.SearchList(".", b.collector, ""), "LUAINPUTS": command.SearchList(b.collector, ""),
		"TEXFORMATS": filepath.Dir(p.doctor.FormatPath()),
		"TEXMFVAR":   filepath.Join(p.Directory, b.strategy, "texmf-var"), "TEXMFCACHE": seedFontCache(cache, p),
		"RELATEX_STREAM": filepath.ToSlash(filepath.Join(b.output, fmt.Sprintf("pass-%d.jsonl", pass))),
		"RELATEX_MATHML": filepath.ToSlash(filepath.Join(b.output, b.job+"-luamml-mathml.html")), "RELATEX_ATTEMPT": filepath.Base(p.Directory),
		"RELATEX_PASS": fmt.Sprint(pass), "RELATEX_INPUT": command.FileDigest(filepath.Join(b.source, p.main)),
		"RELATEX_STRATEGY": b.strategy, "RELATEX_SEEDED": seeded})
	// The collector names the files TeX read from a main file's subdirectory as the source does.
	if directory := path.Dir(p.main); directory != "." {
		environment["RELATEX_MAIN_DIRECTORY"] = directory
	}
	// A build that wants only the SyncTeX needs the glyph map and the math records: the collector skips
	// the readings of paragraphs, pages, tables, citations and the rest.
	if p.options.ExternalOnly {
		environment["RELATEX_LEAN"] = "1"
	}
	if host := p.options.Host; host != nil && len(host.SearchPath) > 0 {
		environment["TEXINPUTS"] = host.searchPath(b.collector)
	}
	if p.options.AuthorMath {
		names := filepath.Join(b.output, "math-macro-names.txt")
		if _, err := os.Stat(names); err != nil {
			writeMacroNames(b.source, names)
		}
		environment["RELATEX_AUTHOR_MATH"], environment["RELATEX_MATH_MACROS"] = "1", names
	}
	return environment
}

// convergenceState is PaperBuilds#convergence_state: the convergence files of the build's
// output with their digests, not the per-pass copies or the supplied ones.
func convergenceState(b build) rec.Map {
	state := rec.Map{}
	for name, digest := range artifacts.FileManifest(b.output) {
		base := filepath.Base(name)
		if convergence.MatchString(name) && !strings.HasPrefix(base, "pass-") && !strings.HasPrefix(base, "supplied.") {
			state[name] = digest
		}
	}
	return state
}

func saveState(b build, pass int) rec.Map {
	state := convergenceState(b)
	writeJSON(filepath.Join(b.output, fmt.Sprintf("pass-%d-state.json", pass)), state)
	aux := filepath.Join(b.output, b.job+".aux")
	if info, err := os.Stat(aux); err == nil && info.Mode().IsRegular() {
		check(copyFile(aux, filepath.Join(b.output, fmt.Sprintf("pass-%d.aux", pass))))
	}
	return state
}

// pruneStreams is PaperBuilds#prune_streams: only the final pass's collector streams stay.
func pruneStreams(b build, final int) {
	for pass := 1; pass < final; pass++ {
		base := filepath.Join(b.output, fmt.Sprintf("pass-%d.jsonl", pass))
		matches, err := filepath.Glob(base + ".*")
		check(err)
		for _, path := range append([]string{base}, matches...) {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				check(err)
			}
		}
	}
}

// diagnose is PaperBuilds#diagnose: whether the build compiled, its errors and dependencies,
// and for the external build its physical page count.
func (p *Probe) diagnose(b build, stages []rec.Map) rec.Map {
	compiled := len(stages) >= p.minimumPasses() && succeeded(stages[len(stages)-1])
	for _, stage := range stages {
		compiled = compiled && passed(stage)
	}
	if info, err := os.Stat(filepath.Join(b.output, b.job+".pdf")); err != nil || !info.Mode().IsRegular() {
		compiled = false
	}
	log := readLog(filepath.Join(b.output, fmt.Sprintf("pass-%d.log", len(stages))))
	dependencies := []any{}
	if recorder := filepath.Join(b.output, b.job+".fls"); isFile(recorder) {
		dependencies = recordedDependencies(recorder, b.source, b.working)
	}
	writeJSON(filepath.Join(b.output, "dependencies.json"), dependencies)
	errors := []any{}
	for _, line := range lines(log) {
		if logErrors.MatchString(line) && len(errors) < 12 {
			errors = append(errors, rec.Strip(line))
		}
	}
	result := rec.Map{"compiled": compiled, "passes": int64(len(stages)), "unresolved_references_or_rerun": rerunRequest.MatchString(log),
		"undefined_references": undefinedReferences.MatchString(log),
		"errors":               errors}
	if settled := settlingErrors(b, stages); len(settled) > 0 {
		result["settling_errors"] = settled
	}
	if !isFile(filepath.Join(b.output, fmt.Sprintf("pass-%d.jsonl", len(stages)))) || b.strategy != "external" || !compiled {
		return result
	}
	return pageCount(b, result)
}

// pageCount is the physical page count of the external build's PDF; a failure rejects the
// stream as Ruby's rescue does.
func pageCount(b build, result rec.Map) (counted rec.Map) {
	defer rejectedStream(result, &counted)
	assets.CapturePageCount(filepath.Join(b.output, b.job+".pdf"), filepath.Join(b.output, "physical-page-count"))
	return result
}

func rejectedStream(result rec.Map, counted *rec.Map) {
	recovered := recover()
	if recovered == nil {
		return
	}
	failure, ok := recovered.(rec.Failure)
	if !ok || failure.Lookup {
		panic(recovered)
	}
	merged := rec.Map{}
	for key, value := range result {
		merged[key] = value
	}
	merged["stream"], merged["extraction_error"] = "rejected", failure.Message
	*counted = merged
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// lines is String#lines: each line with its newline.
func lines(text string) []string {
	result := strings.SplitAfter(text, "\n")
	if len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}
	return result
}

// recordedDependencies is Evidence.dependencies: the files the recorder saw the build read
// from outside its working copy, with their digests.
func recordedDependencies(recorder, source, working string) []any {
	seen := map[string]bool{}
	result := []any{}
	for _, line := range lines(readLog(recorder)) {
		if !strings.HasPrefix(line, "INPUT ") {
			continue
		}
		path := artifacts.ExpandPath(rec.Strip(strings.TrimPrefix(line, "INPUT ")), working)
		if seen[path] {
			continue
		}
		seen[path] = true
		if fspath.Inside(source, path) || !isFile(path) {
			continue
		}
		result = append(result, rec.Map{"path": path, "sha256": command.FileDigest(path)})
	}
	return result
}

func hexDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Quiet leaves out the progress line of a build that compiled; a failed build is always reported.
var Quiet bool

// QuietFailures also leaves out the progress line of a build that failed: the default output of a conversion
// is the result line, the short error list and the evidence path; the failure stays in the run report and the logs.
var QuietFailures bool

// rejectedBibliography is biblatex refusing a JOB.bbl written for another format version, matched
// with the log's lines joined: TeX wraps it at 79 columns (second cohort 473).
var rejectedBibliography = regexp.MustCompile(`Package biblatex Warning: File '[^']+\.bbl' is wrong format version`)

// shippedBibliography: whether the source holds the build's own JOB.bbl.
func shippedBibliography(b build) bool {
	return isFile(filepath.Join(b.working, b.job+".bbl"))
}

// removeIfPresent removes a file; a missing one is no error.
func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
