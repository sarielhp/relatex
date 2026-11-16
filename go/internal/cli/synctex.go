package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/engine"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/papersource"
	"github.com/sarielhp/relatex/internal/pipeline"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/synctexout"
)

// synctexLong is the synctex command's help (decision 101).
const synctexLong = "Gives the PDF another tool built (leeni, latexmk) relatex's word-level SyncTeX file: builds the paper's instrumented build alone, waits for PDF to be written after it started, and installs NAME.synctex.gz beside it (atomically) only when PDF is byte for byte the PDF relatex built, its trailer /ID aside. The bibliography is the .bbl beside the main file; SOURCE_DATE_EPOCH, FORCE_SOURCE_DATE, TZ and TEXINPUTS are the other build's, so give both builds the same. With --replace-pdf (for a preview built by another engine, XeLaTeX) relatex's own PDF and SyncTeX file replace the pair at PDF, when PDF is still the file it waited for and the sources are as its build read them. Exit status: 0 installed, 2 refused (the PDFs differ; nothing written), 3 no new PDF within --wait, 4 superseded (--replace-pdf: PDF or a source changed after relatex saw it; nothing written), 1 failed; SIGTERM or SIGINT stops it with every program it started. It runs at nice 19"

// synctexWaitSeconds is how long relatex synctex waits for the other tool's PDF by default.
const synctexWaitSeconds = 600

// synctexRun is one relatex synctex: what it was asked, and what its build made.
type synctexRun struct {
	started time.Time
	input   string // the paper's directory, absolute
	pdf     string // the other tool's PDF, absolute
	wait    time.Duration
	options map[string]any
	built   pipeline.Built
	ours    []byte // the PDF relatex built
	synctex []byte // its word-level SyncTeX file
	replace bool   // --replace-pdf: install relatex's PDF with its SyncTeX file
	seen    chan awaited
}

// awaited is the other tool's PDF as it came: its digest, or why it did not.
type awaited struct {
	digest string
	err    error
}

// synctexRequest is what a synctex command line asked for.
type synctexRequest struct {
	main, target, engine string
	wait                 int64
	replace              bool
}

// synctexSpec is relatex synctex: the word-level SyncTeX for a PDF another tool built.
func synctexSpec() commandSpec {
	request := synctexRequest{wait: synctexWaitSeconds}
	options := []clihelp.Option{
		textFlag("--main <file>", "The main .tex file, a path inside the paper", &request.main),
		clihelp.Required(textFlag("--for <pdf>", "The PDF the other tool builds", &request.target)),
		integerFlag("--wait <seconds>", fmt.Sprintf("How long to wait for PDF to be written after relatex started (default %d)",
			synctexWaitSeconds), func(value int64) { request.wait = value }),
		textFlag("--engine <path>", "The lualatex to run (default RELATEX_ENGINE, else ~/.texlive's, else lualatex on PATH)", &request.engine),
		switchFlag("--replace-pdf", "Install relatex's PDF with its SyncTeX file in place of PDF (a preview another engine built) "+
			"when PDF is still the file it waited for; exit 4, nothing written, when it changed", func() { request.replace = true }),
	}
	return commandSpec{name: "synctex", description: "Give a PDF another tool built relatex's word-level SyncTeX file", long: synctexLong,
		usage: Name + " synctex [DIRECTORY] --main FILE --for PDF [--wait SECONDS] [--replace-pdf]", options: options,
		args: clihelp.MaximumNArgs(1), run: func(rest []string) ojson.Object { return synctexCommand(rest, request) }}
}

// synctexCommand runs it.
func synctexCommand(rest []string, request synctexRequest) ojson.Object {
	run := &synctexRun{started: time.Now(), replace: request.replace}
	lowerPriority()
	run.configure(rest, request)
	stop := stopOnSignal()
	defer stop()
	if run.replace {
		run.watchPDF()
	}
	run.build()
	var name string
	if run.replace {
		name = run.replacePair(run.seenPDF())
	} else {
		run.awaitPDF()
		run.verify()
		name = run.install()
	}
	elapsed := time.Since(run.started).Round(time.Second)
	panic(&exitRequest{stdout: fmt.Sprintf("relatex synctex: installed %s (%s)\n", name, elapsed)})
}

// configure reads the command's request; a .tex file as the input names the main file.
func (r *synctexRun) configure(rest []string, request synctexRequest) {
	if request.wait < 0 {
		fail("--wait takes a number of seconds, not " + fmt.Sprint(request.wait))
	}
	input := "."
	if len(rest) == 1 {
		input = rest[0]
	}
	program := request.engine
	if program == "" {
		program = papersource.DefaultEngine()
	}
	r.options = map[string]any{"engine": program, "synctex": "word", "publish": false, "work_mode": "cache"}
	if request.main != "" {
		r.options["main"] = request.main
	}
	input = texFileInput(input, r.options)
	r.input = realpath(input)
	r.pdf = expandPath(request.target)
	r.wait = time.Duration(request.wait) * time.Second
}

// build is the products-only conversion: the instrumented build alone, seeded, and the word SyncTeX
// writer, in the project cache; nothing is published. What it built is kept in memory.
func (r *synctexRun) build() {
	r.options["products_only"] = true
	r.options["product"] = filepath.Join(projectScratch(r.input), "_synctex")
	stopping.outputDirectory(r.options["product"].(string))
	applyUserConfig(r.options, r.input)
	engine.Quiet = true
	adjustConversion = r.adjust
	defer func() { adjustConversion = nil }()
	runConversion(r.options, r.input)
	if r.ours == nil || r.synctex == nil {
		rec.Fail("relatex synctex: the build wrote no PDF or no SyncTeX file")
	}
}

// adjust gives the conversion the other tool's environment, the files it must not read, and the
// hooks that keep the attempt (for a signal) and what it built.
func (r *synctexRun) adjust(o *pipeline.Options) {
	main, _ := r.options["main"].(string)
	o.Host = engine.NewHost(os.Getenv("TEXINPUTS"), filepath.Join(r.input, filepath.Dir(main)))
	synctex := synctexout.Path(r.pdf)
	o.SnapshotExclusions = append(o.SnapshotExclusions, r.pdf, r.pdf+".tmp", synctex, synctex+".tmp")
	o.Started = func(directory string) { stopping.attempt(directory) }
	o.Published = nil
	o.Products = r.keep
}

// keep holds what the build made: the attempt goes when the run ends.
func (r *synctexRun) keep(built pipeline.Built) {
	r.built = built
	if built.PDF == "" || built.Synctex == "" {
		return
	}
	r.ours, r.synctex = readFile(built.PDF), readFile(built.Synctex)
}

// awaitPDF waits for the other tool's PDF (exit status 3 when it does not come within --wait).
func (r *synctexRun) awaitPDF() {
	if err := synctexout.Await(r.pdf, r.started, r.wait); err != nil {
		panic(&exitRequest{stderr: "relatex synctex: " + err.Error() + "\n", status: 3})
	}
}

// verify refuses (exit status 2) a PDF that is not, byte for byte but its /ID, the one relatex built.
func (r *synctexRun) verify() {
	built := synctexout.Built{Input: r.input, Sources: r.built.Sources, PDF: r.ours,
		Scratch: func() string { return projectScratch(r.input) }}
	err := built.Verify(r.pdf)
	var refused *synctexout.Refused
	switch {
	case err == nil:
	case errors.As(err, &refused):
		panic(&exitRequest{stderr: "relatex synctex: " + err.Error() + "\n", status: 2})
	default:
		panic(systemCall(err))
	}
}

// install puts the SyncTeX file beside the PDF, atomically; a signal removes the partial file.
func (r *synctexRun) install() string {
	name, err := synctexout.Install(r.pdf, r.synctex, func(path string) { stopping.temporary(path) })
	if err != nil {
		panic(systemCall(err))
	}
	return name
}

// watchPDF waits for the other tool's PDF beside the build (--replace-pdf) and keeps the digest of
// the first one that came: the PDF the pair may replace.
func (r *synctexRun) watchPDF() {
	r.seen = make(chan awaited, 1)
	go func() {
		err := synctexout.Await(r.pdf, r.started, r.wait)
		result := awaited{err: err}
		if err == nil {
			result.digest = synctexout.Digest(r.pdf)
		}
		r.seen <- result
	}()
}

// seenPDF is the digest of the PDF watchPDF saw (exit status 3 when it did not come within --wait).
func (r *synctexRun) seenPDF() string {
	result := <-r.seen
	if result.err != nil {
		panic(&exitRequest{stderr: "relatex synctex: " + result.err.Error() + "\n", status: 3})
	}
	return result.digest
}

// replacePair puts relatex's PDF and SyncTeX file in place of the other tool's pair, only while
// that PDF is still the one seen and the sources are as the build read them (else exit status 4).
func (r *synctexRun) replacePair(seen string) string {
	built := synctexout.Built{Input: r.input, Sources: r.built.Sources}
	name, err := synctexout.Replace(synctexout.Replacement{PDF: r.pdf, Ours: r.ours, Synctex: r.synctex,
		Current: func() error { return built.Current(r.pdf, seen) }, Partial: stopping.temporary,
		Uninterrupted: stopping.uninterrupted})
	var superseded *synctexout.Superseded
	switch {
	case err == nil:
		return name
	case errors.As(err, &superseded):
		panic(&exitRequest{stderr: "relatex synctex: " + err.Error() + "\n", status: 4})
	default:
		panic(systemCall(err))
	}
}
