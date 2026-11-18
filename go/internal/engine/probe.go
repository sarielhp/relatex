// Package engine is the Go port of the 02-engine stage: Relatex::SourceProbe and
// PaperBuilds (the ordinary and external LuaLaTeX builds of a paper, each run until its
// cross-reference files converge, in the TeX sandbox), with the leeni seed, the EPS
// conversion, the bibliography runs and the description of a failed build.
package engine

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sarielhp/relatex/internal/artifacts"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

// Root is the repository root: the collector (tex/) and the consumer files live there.
var Root string

var (
	strategies = []string{"control", "external"}
	// TeX takes a main file whose .tex extension is in any case (vstatl.TEX).
	safeMainName = regexp.MustCompile(`\A[A-Za-z0-9_./ -]+\.(?i:tex)\z`)
)

// frozen is SourceIdentity::FROZEN: recognized archives keep their historical identity.
var frozen = []rec.Map{
	{"zip_sha256": "470d0f8421d1718d69e12012ce18c9fec6fae93e6b4df4bd5d56e378e065b0ca", "main": "main.tex", "id": "2102.05264",
		"draw": int64(7)},
	{"zip_sha256": "40db3ac58feb7438348aeef6b6d81977fee5c80f3b1864517c5dafb58c31ed79", "main": "2009.09958.tex", "id": "2009.09958",
		"draw": int64(12)},
}

// SourceIdentity is SourceIdentity.for: a recognized archive's id and draw, else the archive
// digest and draw 0.
func SourceIdentity(manifest rec.Map) rec.Map {
	for _, entry := range frozen {
		if rec.Equal(entry["zip_sha256"], manifest["zip_sha256"]) && rec.Equal(entry["main"], manifest["main"]) {
			return rec.Map{"id": entry["id"], "draw": entry["draw"]}
		}
	}
	return rec.Map{"id": rec.Fetch(manifest, "zip_sha256"), "draw": int64(0)}
}

// Options are what engine_stage gives SourceProbe.
type Options struct {
	Archive    string // the input stage's snapshot directory (input/)
	Engine     string
	Jobs       int64
	AuthorMath bool
	// ExternalOnly builds only the instrumented build, converged by itself: no settled seed, no
	// ordinary build to compare it with and no manifests. A run that wants only the PDF and its SyncTeX.
	ExternalOnly bool
	// Host, when given, is the author's own build the builds reproduce (decision 101).
	Host *Host
}

// Probe is one SourceProbe: the attempt directory with the paper's builds.
type Probe struct {
	Directory    string
	doctor       *texsandbox.Doctor
	options      Options
	manifest     rec.Map
	main         string
	entry        rec.Map
	deadline     time.Time
	seed         *Seed
	seedDir      string
	lock         sync.Mutex
	degradations []any
	// compatibility are the shims of the compatibility layer applied, each with its cause (decision 082).
	compatibility []rec.Map
	// reencoded are the source files the inputenc shim re-encoded to UTF-8 (decision 143).
	reencoded []any
	// generatedFonts are TeX font metrics generated with mktextfm during the build (decision 158).
	generatedFonts []string
	// ipeGraphics are the .ipe figures prepareEPS converted, each with what the file held (decision 194).
	ipeGraphics []any
}

// NewProbe is SourceProbe.new(archive:, work:, engine:, jobs:, inspect_stream: false,
// author_math:): the attempt directory with the snapshot, the sandbox checked, EPS figures
// converted, the leeni seed, and the doctor, source and selection records.
func NewProbe(options Options, work string) *Probe {
	doctor := texsandbox.Current(options.Engine)
	if doctor.Report["ready"] != true {
		rec.Fail("Selected engine is missing collection prerequisites")
	}
	jobs := buildJobs(options.Jobs)
	manifest := readJSON(filepath.Join(options.Archive, "inventory.json"))
	p := &Probe{doctor: doctor, options: options, manifest: manifest, deadline: time.Now().Add(900 * time.Second),
		main: rec.FetchString(manifest, "main"), degradations: []any{}}
	p.options.Jobs = jobs
	if !safeMainName.MatchString(p.main) {
		rec.Fail("Main filename contains unsupported TeX input characters")
	}
	p.entry = SourceIdentity(manifest)
	absolute, err := filepath.Abs(work)
	check(err)
	p.Directory = command.MakeTemp(absolute, "source-probe-")
	copyTree(options.Archive, filepath.Join(p.Directory, "input"))
	command.Trace("engine: probe directory ready")
	p.verifySandbox()
	command.Trace("engine: sandbox verified")
	p.prepareEPS()
	command.Trace("engine: eps ready")
	p.seedDir = seedDirectory(manifest, p.main)
	if options.ExternalOnly {
		p.seed = p.prepareIncrementalSeed(p.seedDir)
	} else {
		p.seed = p.prepareSeed(p.seedDir)
	}
	command.Trace("engine: seed ready")
	writeJSON(filepath.Join(p.Directory, "doctor.json"), doctor.Report)
	if !options.ExternalOnly {
		writeJSON(filepath.Join(p.Directory, "source-manifest.json"), rec.Map{"version": version(), "toolchain": doctor.Fingerprint(),
			"files": artifacts.FileManifest(Root, "lib", "tex", "schemas", "profiles", "test")})
	}
	selected := rec.Map{"selected_main": p.main, "zip_sha256": rec.Fetch(manifest, "zip_sha256")}
	for key, value := range p.entry {
		selected[key] = value
	}
	writeJSON(filepath.Join(p.Directory, "selected-paper.json"), selected)
	return p
}

// buildJobs is BuildPool.count.
func buildJobs(value int64) int64 {
	if value < 1 || value > 8 {
		rec.Fail("Build jobs must be between 1 and 8")
	}
	return value
}

func version() string {
	data, err := os.ReadFile(filepath.Join(Root, "VERSION"))
	check(err)
	return rec.Strip(string(data))
}

func check(err error) {
	if err != nil {
		rec.Fail("%v", err)
	}
}

func readJSON(path string) rec.Map {
	file, err := os.Open(path)
	check(err)
	defer file.Close()
	value, err := canonjson.Decode(file)
	check(err)
	return rec.AsMap(value)
}

// writeJSON is Relatex.write_canonical_json.
func writeJSON(path string, value any) { check(canonjson.WriteFile(path, value)) }

// verifySandbox is TeXSandbox#verify!: /usr/bin/true runs in the sandbox.
func (p *Probe) verifySandbox() {
	if !texsandbox.Sandboxed {
		return
	}
	source := filepath.Join(p.Directory, "sandbox-check")
	environment := map[string]string{"TEXMFVAR": filepath.Join(source, "var"), "TEXMFCACHE": filepath.Join(source, "cache")}
	wrapped := p.doctor.Wrap([]string{"/usr/bin/true"}, source, source, environment)
	stage := command.Run(wrapped, source, filepath.Join(source, "check.log"), 10*time.Second, nil)
	command.WriteReceipt(filepath.Join(source, "stage.json"), stage)
	if !succeeded(stage) {
		rec.Fail("TeX sandbox is unavailable; see %s/check.log", source)
	}
}

// succeeded is stage.fetch('exit')&.zero? && !stage.fetch('timeout').
func succeeded(stage rec.Map) bool {
	return rec.Equal(stage["exit"], int64(0)) && stage["timeout"] != true
}

// passSeconds bounds one engine pass: a long paper's instrumented pass may take minutes on a busy
// machine (second cohort 482: 63 pages, 95 s); the attempt deadline bounds the total.
const passSeconds = 300

// remaining is SourceProbe#remaining_timeout: at most passSeconds, within the attempt's deadline.
func (p *Probe) remaining() time.Duration {
	left := time.Until(p.deadline)
	if left <= 0 {
		rec.Fail("Attempt deadline exceeded")
	}
	return min(passSeconds*time.Second, left)
}

// strategies are the builds this probe makes: the ordinary one and the instrumented one, or the instrumented one alone.
func (p *Probe) strategies() []string {
	if p.options.ExternalOnly {
		return []string{"external"}
	}
	return strategies
}

// Probe is PaperBuilds#probe: both builds (BuildPool.map over the strategies), the report and
// the attempt's artifact inventory.
func (p *Probe) Probe() rec.Map {
	started := time.Now()
	command.Trace("engine: builds begin")
	built := p.buildWithCompatibility()
	command.Trace("engine: builds end")
	results := rec.Map{}
	for index, strategy := range p.strategies() {
		results[strategy] = built[index]
	}
	report := rec.Map{"id": rec.Fetch(p.entry, "id"), "draw": rec.Fetch(p.entry, "draw"), "qualified": false,
		"scope":                  "Diagnostic extraction; independent paper coverage and fidelity still require review",
		"equation_alias_adapter": false, "amsmath_adapter": false, "footnote_adapter": true, "graphics_adapter": true,
		"table_adapter": true, "page_total_adapter": true, "source_repairs": int64(0), "results": results,
		"layout_adapters": rec.Map{"classic_table_rules": false}, "compatibility": p.compatibilityRecord(), "reencoded_inputs": p.reencodedRecord(),
		"generated_fonts": p.generatedFontsRecord(), "ipe_graphics": p.ipeGraphics,
		"execution": rec.Map{"jobs": p.options.Jobs, "cache": "disabled for diagnostic paper probes",
			"seconds": time.Since(started).Seconds()}}
	writeJSON(filepath.Join(p.Directory, "probe.json"), report)
	if !p.options.ExternalOnly {
		writeJSON(filepath.Join(p.Directory, "artifact-manifest.json"), artifacts.FileManifest(p.Directory))
	}
	return report
}

// buildAll is BuildPool.map: the strategies built concurrently, at most jobs at a time, the
// results in order; a failure of the pool itself is raised for the first strategy that had one.
func (p *Probe) buildAll() []rec.Map {
	builds := p.strategies()
	results := make([]rec.Map, len(builds))
	failures := make([]any, len(builds))
	slots := make(chan struct{}, p.options.Jobs)
	var group sync.WaitGroup
	for index, strategy := range builds {
		group.Add(1)
		go func() {
			defer group.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			defer captured(&failures[index])
			results[index] = p.variant(strategy)
		}()
	}
	group.Wait()
	for _, failure := range failures {
		if failure != nil {
			panic(failure)
		}
	}
	return results
}

// captured keeps a goroutine's panic for the pool to raise.
func captured(failure *any) {
	if recovered := recover(); recovered != nil {
		*failure = recovered
	}
}

func (p *Probe) recordGeneratedFonts(names []string) {
	if len(names) == 0 {
		return
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	for _, name := range names {
		if !slices.Contains(p.generatedFonts, name) {
			p.generatedFonts = append(p.generatedFonts, name)
		}
	}
}

func (p *Probe) generatedFontsRecord() []any {
	p.lock.Lock()
	defer p.lock.Unlock()
	sorted := slices.Clone(p.generatedFonts)
	slices.Sort(sorted)
	record := make([]any, 0, len(sorted))
	for _, name := range sorted {
		record = append(record, name)
	}
	return record
}

// Products are what engine_stage records of a probe: its directory, the external build's PDF
// and the degradations.
func (p *Probe) Products() rec.Map {
	relative := filepath.Base(p.Directory)
	job := JobName(p.main)
	p.lock.Lock()
	defer p.lock.Unlock()
	return rec.Map{"probe": relative, "pdf": path.Join(relative, "external/output", job+".pdf"),
		"degradations": append([]any{}, p.degradations...)}
}

// Run is GeneralConversion#engine_stage's block: the probe in root, both builds compiled
// without unresolved references, engine-products.json, and the stage result.
func Run(options Options, root string) rec.Map {
	probe := NewProbe(options, root)
	result := probe.Probe()
	results := rec.FetchMap(result, "results")
	for _, strategy := range probe.strategies() {
		build := rec.FetchMap(results, strategy)
		if build["compiled"] != true || build["unresolved_references_or_rerun"] == true {
			rec.Fail("%s", engineFailure(probe.Directory, results))
		}
	}
	if probe.options.ExternalOnly {
		probe.keepSeed()
	}
	products := probe.Products()
	writeJSON(filepath.Join(root, "engine-products.json"), products)
	result["products"] = products
	return result
}

// JobName is the job LuaTeX gives a main file: its base name without the extension, whatever
// the extension's case (vstatl.TEX gives vstatl).
func JobName(main string) string {
	base := filepath.Base(main)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

var errNotDirectory = errors.New("not a directory")

// copyTree is FileUtils.cp_r(from, to) for a new to: the tree with each file's permissions.
func copyTree(from, to string) {
	info, err := os.Stat(from)
	check(err)
	if !info.IsDir() {
		rec.Fail("%v: %v", from, errNotDirectory)
	}
	err = filepath.WalkDir(from, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, _ := filepath.Rel(from, path)
		target := filepath.Join(to, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
	check(err)
}

func copyFile(from, to string) error {
	info, err := os.Stat(from)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, info.Mode().Perm())
}

// seedFontCache is FontCache.seed!: a build's TEXMFCACHE with the shared font-name database.
func seedFontCache(cache string, p *Probe) string { return texsandbox.SeedFontCache(cache, p.doctor) }
