// Package papersource is the Go port of Relatex::PaperSource: a completed, immutable
// sandboxed probe reinspected into the paper document. Saved success flags are not
// qualification inputs; historical producer and current consumer stay distinct.
package papersource

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/artifacts"
	"github.com/sarielhp/relatex/internal/assets"
	"github.com/sarielhp/relatex/internal/authorhints"
	"github.com/sarielhp/relatex/internal/authormath"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/contrast"
	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/formulatex"
	"github.com/sarielhp/relatex/internal/frontmatter"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/inspect"
	"github.com/sarielhp/relatex/internal/legacyinput"
	"github.com/sarielhp/relatex/internal/paintmodel"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/pdfpaint"
	"github.com/sarielhp/relatex/internal/reading"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/sourcefonts"
	"github.com/sarielhp/relatex/internal/stream"
	"github.com/sarielhp/relatex/internal/textstyle"
)

var (
	strategies         = []string{"control", "hooks", "tagging", "bridge"}
	externalStrategies = []string{"control", "external"}
	logWarnings        = regexp.MustCompile(`(?i)undefined references|Citation .*undefined|Reference .*undefined|Rerun to get|Please rerun|rerun LaTeX`)
	inputDirective     = regexp.MustCompile(`(?m)^([^%\n]*?)\\(?:input|include)\s*\{([^{}]+)\}`)
	unsafeName         = regexp.MustCompile(`[\\\x00-\x1f]`)
)

// Source is one probe being reinspected.
type Source struct {
	Root        string // the repository root (schemas, profiles, consumer files)
	directory   string
	sourceFonts bool
	authorMath  bool
	engine      string
	diagnostics *diagnostics.Diagnostics

	manifestBytes []byte
	manifest      rec.Map
	selected      rec.Map
	report        rec.Map
	adapter       string
	main          string
	job           string
	hints         authorhints.Reader
	inputSHA      string
	stages        map[string][]rec.Map
	producer      any

	streamPath     string
	pageCountProof any
	inspectionSHA  string
	tablePaintDir  any
	sourceFontsDir any
	textPaintDir   any
	// blockPages is the first page of each reading block, by inspection key (chapterpages.go).
	blockPages map[any]int64
	// pageSets are the pages of each reading block and pageLabels the printed page numbers (pagelist.go).
	pageSets   map[any][]int64
	pageLabels map[int64]string
	// slotBlocks and retention bind the glyph map to the document (glyphowners.go).
	slotBlocks map[reading.SlotKey]string
	retention  rec.Map
	// pictures are the drawn pictures cropped from the PDF, by file name (glyphowners.go).
	pictures map[string][]assets.DrawnPicture
	// crops are those pictures' cropped PDFs, by reading file name (decision 137).
	crops map[string][]byte
}

// Options are PaperSource.new's keywords: source_fonts:, author_math:, engine: and hints:.
type Options struct {
	SourceFonts, AuthorMath bool
	Engine                  string
	// Hints reads the author's hint files (decision 122); nil keeps no speech hints.
	Hints authorhints.Reader
}

// New is PaperSource.new(directory, source_fonts:, diagnostics:, author_math:, engine:).
func New(root, directory string, options Options, diags *diagnostics.Diagnostics) (source *Source) {
	defer invalid("Invalid paper evidence: ")
	resolved, err := fspath.Real(directory)
	check(err)
	s := &Source{Root: root, directory: resolved, sourceFonts: options.SourceFonts, authorMath: options.AuthorMath,
		engine: options.Engine, diagnostics: diags, hints: options.Hints}
	if s.engine == "" {
		s.engine = DefaultEngine()
	}
	s.manifestBytes, err = os.ReadFile(filepath.Join(s.directory, "artifact-manifest.json"))
	check(err)
	manifest, ok := decode(s.manifestBytes).(rec.Map)
	if !ok || len(manifest) == 0 {
		rec.Fail("Empty paper artifact inventory")
	}
	s.manifest = manifest
	s.verifyArtifacts()
	s.selected = rec.AsMap(s.readJSON("selected-paper.json"))
	s.report = rec.AsMap(s.readJSON("probe.json"))
	built := s.buildInventory()
	if !rec.Equal(s.selected["id"], s.report["id"]) || !rec.Equal(s.selected["draw"], s.report["draw"]) {
		rec.Fail("Paper identity or build inventory mismatch")
	}
	s.main = rec.FetchString(s.selected, "selected_main")
	s.job = strings.TrimSuffix(filepath.Base(s.main), filepath.Ext(s.main))
	s.inputSHA = digest(s.sourceArtifact(s.main))
	if digest(s.readArtifact("input/source.zip")) != rec.FetchString(s.selected, "zip_sha256") {
		rec.Fail("Frozen paper ZIP mismatch")
	}
	s.stages = map[string][]rec.Map{}
	for _, strategy := range built {
		s.stages[strategy] = s.verifyBuild(strategy)
	}
	s.producer = s.readJSON("source-manifest.json")
	return s
}

// invalid is the rescue of KeyError, TypeError, JSON::ParserError and SystemCallError: the
// evidence is invalid or incomplete.
func invalid(prefix string) {
	recovered := recover()
	if recovered == nil {
		return
	}
	switch failure := recovered.(type) {
	case rec.Failure:
		if failure.Lookup {
			panic(rec.Failure{Message: prefix + failure.Message})
		}
	case systemError:
		panic(rec.Failure{Message: prefix + failure.Error()})
	}
	panic(recovered)
}

// systemError is an operating-system or JSON error, which Ruby raises as SystemCallError or
// JSON::ParserError.
type systemError struct{ error }

func check(err error) {
	if err != nil {
		panic(systemError{err})
	}
}

func decode(bytes []byte) any {
	value, err := canonjson.Decode(strings.NewReader(string(bytes)))
	check(err)
	return value
}

func digest(bytes []byte) string {
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:])
}

// sourceArtifact is the paper file NAME (relative to the archive) as the builds read it, which the
// pass stream, the document identity and every reading of the source mean by the input: the
// archive's bytes, re-encoded to UTF-8 when the probe recorded that (decision 143; legacyinput.AsBuilt
// checks the record against those bytes).
func (s *Source) sourceArtifact(name string) []byte {
	return legacyinput.AsBuilt(s.readArtifact("input/files/"+name), name, s.report)
}

func (s *Source) buildInventory() []string {
	inventory := []string{}
	for name := range rec.FetchMap(s.report, "results") {
		inventory = append(inventory, name)
	}
	slices.Sort(inventory)
	if sorted := slices.Sorted(slices.Values(strategies)); slices.Equal(inventory, sorted) {
		s.adapter = "bridge"
		return strategies
	}
	if !slices.Equal(inventory, slices.Sorted(slices.Values(externalStrategies))) {
		rec.Fail("Paper identity or build inventory mismatch")
	}
	s.adapter = "external"
	return externalStrategies
}

// relative is ArtifactResolver.relative!.
func relative(name string) {
	if name == "" || strings.HasPrefix(name, "/") || unsafeName.MatchString(name) {
		rec.Fail("Unsafe logical artifact name")
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			rec.Fail("Unsafe logical artifact name")
		}
	}
}

// artifactPath is ArtifactResolver#resolve: an inventoried file, unchanged, inside the probe.
func (s *Source) artifactPath(name string) string {
	name = artifacts.Canonical(name)
	relative(name)
	expected, ok := s.manifest[name]
	if !ok {
		rec.Fail("Uninventoried artifact: %s", name)
	}
	if entry, isMap := expected.(rec.Map); isMap {
		expected = rec.Fetch(entry, "sha256")
	}
	path, err := fspath.Real(filepath.Join(s.directory, name))
	check(err)
	if !fspath.Inside(s.directory, path) {
		rec.Fail("Artifact escapes immutable root")
	}
	if command.FileDigest(path) != expected {
		rec.Fail("Changed artifact: %s", name)
	}
	return path
}

func (s *Source) readArtifact(name string) []byte {
	bytes, err := os.ReadFile(s.artifactPath(name))
	check(err)
	return bytes
}

func (s *Source) readJSON(name string) any { return decode(s.readArtifact(name)) }

func (s *Source) verifyArtifacts() {
	current, err := os.ReadFile(filepath.Join(s.directory, "artifact-manifest.json"))
	check(err)
	if string(current) != string(s.manifestBytes) {
		rec.Fail("Paper artifact manifest changed during export")
	}
	for name := range s.manifest {
		s.artifactPath(name)
	}
}

func (s *Source) verifyBuild(strategy string) []rec.Map {
	result := rec.FetchMap(rec.FetchMap(s.report, "results"), strategy)
	count, isInt := rec.Fetch(result, "passes").(int64)
	// A build seeded with converged cross-reference files records them as pass 0.
	first := int64(2)
	if _, seeded := s.manifest[strategy+"/output/pass-0-state.json"]; seeded {
		first = 1
	}
	if !isInt || count < first || count > 5 {
		rec.Fail("Invalid paper pass count")
	}
	stages := []rec.Map{}
	for pass := int64(1); pass <= count; pass++ {
		stage := rec.AsMap(s.readJSON(fmt.Sprintf("%s/output/pass-%d-stage.json", strategy, pass)))
		exit, exitInt := rec.Fetch(stage, "exit").(int64)
		if !exitInt || (exit != 0 && !s.settledPass(strategy, pass, count, stage)) || rec.Fetch(stage, "timeout") != false ||
			rec.Fetch(stage, "signal") != nil {
			rec.Fail("Paper build stage failed")
		}
		stages = append(stages, stage)
	}
	previous := s.readJSON(fmt.Sprintf("%s/output/pass-%d-state.json", strategy, count-1))
	final, isMap := s.readJSON(fmt.Sprintf("%s/output/pass-%d-state.json", strategy, count)).(rec.Map)
	if !isMap || len(final) == 0 || !rec.Equal(previous, final) {
		rec.Fail("Paper auxiliary state did not converge")
	}
	for name, sha := range final {
		if digest(s.readArtifact(strategy+"/output/"+name)) != sha {
			rec.Fail("Paper final auxiliary hash mismatch")
		}
	}
	log := strings.ToValidUTF8(string(s.readArtifact(fmt.Sprintf("%s/output/pass-%d.log", strategy, count))), "�")
	if logWarnings.MatchString(log) {
		rec.Fail("Paper has unresolved references or rerun warnings")
	}
	if !strings.HasPrefix(string(s.readArtifact(fmt.Sprintf("%s/output/%s.pdf", strategy, s.job))), "%PDF-") {
		rec.Fail("Paper PDF missing or invalid")
	}
	return stages
}

// settlingFatal is a stop no aux file can settle (decision 082).
var settlingFatal = regexp.MustCompile(`(?m)^! Emergency stop|Fatal error occurred|\(job aborted`)

// settledPass: a failed first pass of an unseeded build is accepted only as a settled one
// (decision 082): its receipt says so, later passes follow, and its own log shows no fatal stop.
func (s *Source) settledPass(strategy string, pass, count int64, stage rec.Map) bool {
	_, seeded := s.manifest[strategy+"/output/pass-0-state.json"]
	if pass != 1 || count < 2 || seeded || stage["settled"] != true {
		return false
	}
	log := strings.ToValidUTF8(string(s.readArtifact(strategy+"/output/pass-1.log")), "�")
	return !settlingFatal.MatchString(log)
}

// packageSubstitutes are the compatibility shims that stand in for a package TeX Live lacks, with what the
// warning says (decision 194).
var packageSubstitutes = map[string]string{
	"psfig":    "psfig.sty is not in TeX Live: \\psfig is epsfig's \\includegraphics substitute",
	"ltexpprt": "ltexpprt.sty (the LaTeX 2.09 SIAM proceedings style) is not in TeX Live: taken as loaded, the class's page layout stands",
}

// recordIpeGraphics reports each .ipe graphic the engine did not leave to LaTeX as it is (decision 194): converted
// from the PostScript or PDF it holds (warning), or a placeholder box when it holds neither (alert).
func (s *Source) recordIpeGraphics() {
	graphics, _ := s.report["ipe_graphics"].([]any)
	for _, value := range graphics {
		entry := rec.AsMap(value)
		if rec.ToS(entry["how"]) == "placeholder" {
			s.diagnostics.Add("figure.ipe_placeholder", fmt.Sprintf("%s is an Ipe drawing in a format relatex cannot read (neither PDF nor EPS): a placeholder box stands in", rec.ToS(entry["path"])), nil)
			continue
		}
		s.diagnostics.Add("figure.ipe_converted", fmt.Sprintf("%s is an Ipe graphic, read as the %s it holds", rec.ToS(entry["path"]), rec.ToS(entry["how"])), nil)
	}
}

// recordCompatibility: the compatibility layer's shims and a settled first pass's errors, as
// informational diagnostics (decision 082).
func (s *Source) recordCompatibility() {
	compatibility, _ := s.report["compatibility"].([]any)
	for _, value := range compatibility {
		request := rec.AsMap(value)
		if substitute, ok := packageSubstitutes[rec.ToS(request["shim"])]; ok {
			s.diagnostics.Add("source.package_substituted", fmt.Sprintf("%s (%s)", substitute, rec.ToS(request["cause"])), nil)
			continue
		}
		s.diagnostics.Add("source.engine_compatibility", fmt.Sprintf("built under LuaLaTeX with the %s compatibility shim (%s)",
			rec.ToS(request["shim"]), rec.ToS(request["cause"])), nil)
	}
	s.recordIpeGraphics()
	reencoded, _ := s.report["reencoded_inputs"].([]any)
	for _, value := range reencoded {
		entry := rec.AsMap(value)
		s.diagnostics.Add("source.engine_compatibility", fmt.Sprintf("%s was re-encoded from %s to UTF-8 before the build (decision 143)",
			rec.ToS(entry["path"]), rec.ToS(entry["encoding"])), nil)
	}
	generated, _ := s.report["generated_fonts"].([]any)
	if len(generated) > 0 {
		names := make([]string, 0, len(generated))
		for _, value := range generated {
			if name := rec.ToS(value); name != "" {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			s.diagnostics.Add("fonts.generated_metrics", fmt.Sprintf("generated missing TeX font metrics with mktextfm: %s (decision 158)",
				strings.Join(names, ", ")), nil)
		}
	}
	results, _ := s.report["results"].(rec.Map)
	for _, strategy := range []string{"control", "external"} {
		build, _ := results[strategy].(rec.Map)
		if errors, _ := build["settling_errors"].([]any); len(errors) > 0 {
			s.diagnostics.Add("source.settling_errors", fmt.Sprintf("the %s build's first pass, before the aux files existed, reported %d error kinds "+
				"that later passes did not (first: %s)", strategy, len(errors), rec.ToS(errors[0])), nil)
		}
	}
}

func (s *Source) pdf() string {
	return s.artifactPath(fmt.Sprintf("%s/output/%s.pdf", s.adapter, s.job))
}

// Document is PaperSource#document.
func (s *Source) Document() (document rec.Map) {
	defer invalid("Incomplete paper export evidence: ")
	s.verifyArtifacts()
	s.recordCompatibility()
	stages := s.stages[s.adapter]
	pass := len(stages)
	s.streamPath = fmt.Sprintf("%s/output/pass-%d.jsonl", s.adapter, pass)
	loaded := s.loadStream(stages[pass-1])
	command.Trace("reconcile: stream loaded")
	header := loaded.Header()
	if header["adapter"] != s.adapter || header["input"] != s.inputSHA || !rec.Equal(header["pass"], int64(pass)) {
		rec.Fail("Paper stream identity mismatch")
	}
	if stream.Enabled(header, "page-totals-0.1") {
		// The page count is a proof beside the stream; the stream is the one already read (verifyArtifacts
		// still finds a file that changed meanwhile).
		s.countPages()
	}
	command.Trace("reconcile: pages counted")
	inspection, slots := inspect.Read(loaded.Records, s.pageCountProof)
	s.slotBlocks, s.retention = slots, rec.FetchMap(inspection, "math_retention")
	command.Trace("reconcile: inspected")
	s.inspectionSHA, _ = canonjson.Digest(inspection)
	reading := rec.FetchMap(inspection, "reading")
	s.blockPages = firstPages(reading)
	s.pageSets, s.pageLabels = blockPageSets(reading), printedLabels(loaded.Records)
	rendered, pictures := assets.CropRendered(reading, s.pdf(), s.diagnostics)
	s.pictures, s.crops = pictures, rendered
	mediaAssets := assets.FromReading(reading, func(name string) []byte {
		if bytes, ok := rendered[name]; ok {
			return bytes
		}
		return s.readArtifact(fmt.Sprintf("%s/working/%s", s.adapter, name))
	})
	command.Trace("reconcile: assets")
	identity := rec.Map{"id": rec.Fetch(s.selected, "id"), "input_sha256": s.inputSHA}
	hints := s.authorHints()
	document = paperdoc.New(inspection, identity, mediaAssets, s.diagnostics, hints)
	command.Trace("reconcile: document built")
	if s.adapter == "external" {
		s.projectFrontmatter(loaded.Records, document)
	}
	if s.authorMath && s.adapter == "external" {
		s.authorMathSource(inspection, document)
	}
	command.Trace("reconcile: author math")
	s.formulaTeX(document)
	command.Trace("reconcile: formula TeX")
	s.addSpeechHints(document, hints)
	s.addColourAttestations(document, hints)
	s.alignParagraphs(document)
	s.captureTablePaint(reading, document)
	command.Trace("reconcile: table paint")
	if s.sourceFonts {
		s.captureSourceFonts(loaded.Records, document)
	}
	command.Trace("reconcile: fonts")
	// Pictures drawn by TeX show their labels as text in the document's fonts (decision 137).
	assets.TextPictures(document, s.crops, s.diagnostics)
	command.Trace("reconcile: picture text")
	s.recordInkAdjustments(document)
	paperdoc.Validate(document)
	command.Trace("reconcile: validated")
	s.verifyArtifacts()
	return document
}

func (s *Source) loadStream(stage rec.Map) *stream.Stream {
	loaded, err := stream.Load(s.artifactPath(s.streamPath), s.Root, stage)
	if err != nil {
		var failure rec.Failure
		if errors.As(err, &failure) {
			panic(failure)
		}
		check(err)
	}
	return loaded
}

func (s *Source) countPages() {
	saved := rec.AsMap(s.readJSON(s.adapter + "/output/physical-page-count/proof.json"))
	output := filepath.Join(command.Scratch("page-count-consumer-"), "count")
	proof := assets.CapturePageCount(s.pdf(), output)
	for _, key := range []string{"pages", "pdf_sha256", "helper"} {
		_, inProof := proof[key]
		_, inSaved := saved[key]
		if inProof != inSaved || !rec.Equal(proof[key], saved[key]) {
			rec.Fail("Independent producer/consumer PDF page counts differ")
		}
	}
	s.pageCountProof = proof
}

// DefaultEngine is Relatex.default_engine: RELATEX_ENGINE, else the lualatex the configuration's
// "tools" section names, else the TeX Live tree's lualatex.
func DefaultEngine() string {
	if engine := os.Getenv("RELATEX_ENGINE"); engine != "" {
		return engine
	}
	if engine := command.ConfiguredTool("lualatex"); engine != "" {
		return engine
	}
	engine := command.TeXLiveBin("lualatex")
	if command.IsExecutable(engine) {
		return engine
	}
	return "lualatex"
}

// authorMathSource is PaperSource#author_math! (decision 064): each formula's TeX, cut from the
// source and verified by typesetting it again; what stays derived from MathML is counted.
func (s *Source) authorMathSource(inspection, document rec.Map) {
	authormath.ApplySource(inspection, document, s.sourceFile)
	nodes := authormath.MathNodes(inspection, document)
	authormath.Verify(document, nodes, command.Scratch("math-verification-"), s.engine)
	counts := map[string]int{}
	order := []string{}
	for _, node := range nodes {
		reason, ok := rec.AsMap(node["tex"])["reason"].(string)
		if !ok {
			continue
		}
		if counts[reason] == 0 {
			order = append(order, reason)
		}
		counts[reason]++
	}
	for _, reason := range order {
		plural := "s"
		if counts[reason] == 1 {
			plural = ""
		}
		s.diagnostics.Add(reason, fmt.Sprintf("%d formula%s keep the TeX derived from MathML in Markdown", counts[reason], plural), nil)
	}
}

// formulaTeX gives each formula the TeX written from its MathML that round-trips (decision 141);
// how many formulas have none is one whatever-level diagnostic.
func (s *Source) formulaTeX(document rec.Map) {
	nodes := rec.AsMap(document["nodes"])
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	math := []rec.Map{}
	for _, id := range ids {
		if node, ok := nodes[id].(rec.Map); ok && node["type"] == "math" {
			math = append(math, node)
		}
	}
	summary := formulatex.Check(math, command.Scratch("formula-tex-"), s.engine)
	if missing := summary.Formulas - summary.Verified; missing > 0 {
		s.diagnostics.Add("math.formula_tex_unverified", fmt.Sprintf("%d of %d formulas have no TeX written from their MathML: "+
			"it did not typeset to the same MathML (most often: %s)", missing, summary.Formulas, summary.Classes()[0]), nil)
	}
}

// authorHints reads the author's hints (decision 122) from the paper's sources and relatex/hints.json;
// nil when the caller gives no reader, and a file that cannot be read is reported, never dropped
// silently. The speech and alt sections share the one read, so its precedence holds for both.
func (s *Source) authorHints() *authorhints.Hints {
	if s.hints == nil {
		return nil
	}
	hints, err := s.readHints()
	if err != nil {
		s.diagnostics.Add("source.author_hints_unread",
			"the author's hints were not read, so no speech or alt hint applies: "+err.Error(), nil)
		return nil
	}
	return hints
}

// addSpeechHints keeps the author's speech hints (decision 122) in the document, where
// mathintent.MacrosOf gives them to the formulas' intent. A paper without hints keeps no field.
func (s *Source) addSpeechHints(document rec.Map, hints *authorhints.Hints) {
	if hints == nil {
		return
	}
	speech := rec.Map{}
	for key, hint := range hints.Sections["speech"] {
		speech[key] = hint.Value
	}
	if len(speech) > 0 {
		document["math_speech"] = speech
	}
}

// addColourAttestations keeps the author's colour-caption attestations (decision 188) in the
// document, where the accessibility report's use_of_color check reads them; a paper without any
// keeps no field.
func (s *Source) addColourAttestations(document rec.Map, hints *authorhints.Hints) {
	if hints == nil {
		return
	}
	attested := rec.Map{}
	for key, hint := range hints.Sections[authorhints.ColourCaptionsSection] {
		attested[key] = strings.TrimSpace(hint.Value)
	}
	if len(attested) > 0 {
		document["colour_captions"] = attested
	}
}

// readHints reads the paper's hint comments and relatex/hints.json against the hints schema.
func (s *Source) readHints() (*authorhints.Hints, error) {
	bytes, err := os.ReadFile(filepath.Join(s.Root, "schemas", "relatex-hints.schema.json"))
	if err != nil {
		return nil, err
	}
	value, err := canonjson.Decode(strings.NewReader(string(bytes)))
	if err != nil {
		return nil, err
	}
	return authorhints.Read(filepath.Join(s.directory, "input", "files"), s.main, s.hints, rec.AsMap(value))
}

// sourceFile is a file the collector read, as the builds read it (sourceArtifact); not the paper's
// file otherwise.
func (s *Source) sourceFile(file string) (string, bool) {
	name := strings.TrimPrefix(file, "./")
	if _, ok := s.manifest["input/files/"+name]; !ok {
		return "", false
	}
	return authormath.Scrub(s.sourceArtifact(name)), true
}

// rescued runs a capture that degrades: a Failure inside it is handed to degrade.
func rescued(capture func(), degrade func(message string)) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		failure, ok := recovered.(rec.Failure)
		if !ok {
			panic(recovered)
		}
		degrade(failure.Message)
	}()
	capture()
}

// captureTablePaint: tables whose native paint cannot be bound to the PDF render from their
// grid alone.
func (s *Source) captureTablePaint(reading, document rec.Map) {
	rescued(func() {
		paint, directory := pdfpaint.CaptureTables(s.pdf(), reading, document)
		if paint != nil {
			document["table_paint"] = paint
		}
		if directory != "" {
			s.tablePaintDir = directory
		}
	}, func(message string) {
		delete(document, "table_paint")
		s.diagnostics.Add("table.paint_unverified",
			fmt.Sprintf("table layout not verified against the PDF; rendered from its grid (%s)", message), nil)
	})
}

// captureSourceFonts: the text paint (each glyph's ink, the page canvas and the fills behind
// faint text) is read from the PDF's drawing operations and needs no font file, so it stays
// when only the source fonts fail (decision 113). A text paint that cannot be proven drops the
// fonts with it, as before. Without verified source fonts the reading copy uses the browser's
// fonts: every character stays, glyph shapes and widths differ.
func (s *Source) captureSourceFonts(records []rec.Map, document rec.Map) {
	painted := false
	rescued(func() {
		paint, directory := pdfpaint.CaptureText(s.pdf(), paintmodel.PDFMedia(document))
		document["text_paint"] = paint
		command.Trace("reconcile: text paint")
		s.textPaintDir = directory
		painted = true
	}, func(message string) {
		delete(document, "text_paint")
		s.textPaintDir = nil
		// The fonts were never tried: say what failed (the paint), not that the fonts did.
		s.sourceFontsDir = nil
		s.diagnostics.Add("text.paint_unverified", fmt.Sprintf("text colours not read from the PDF; source fonts not linked (%s)", message), nil)
	})
	if !painted {
		return
	}
	rescued(func() {
		sources := []rec.Map{}
		for _, record := range records {
			if record["code"] == "external_text_font_source" {
				sources = append(sources, record)
			}
		}
		// LaTeX opened the font files from its working directory: a fontspec Path=./fonts/ is relative to it.
		working := filepath.Join(s.directory, s.adapter, "working")
		fonts, fontsDirectory := sourcefonts.Capture(s.pdf(), document, sources, working)
		if fonts != nil {
			document["fonts"] = fonts
		}
		s.sourceFontsDir = fontsDirectory
	}, func(message string) {
		delete(document, "fonts")
		s.fontsUnavailable(message)
	})
}

// fontsUnavailable: the document links no source font; the browser's fonts substitute.
func (s *Source) fontsUnavailable(message string) {
	s.sourceFontsDir = nil
	s.diagnostics.Add("fonts.source_unavailable", fmt.Sprintf("source fonts not linked; browser fonts substitute (%s)", message), nil)
}

// recordInkAdjustments: a text colour too faint on the light page is shown with the smallest
// lightness change that reads, and the document records each one (plan accessibility §7.4
// decision 1), so the paint checks accept exactly those.
func (s *Source) recordInkAdjustments(document rec.Map) {
	record := contrast.Adjustments(document)
	if record == nil {
		return
	}
	document[contrast.Field] = record
	count := int64(len(rec.FetchList(record, "items")))
	s.diagnostics.Add("color.low_contrast", fmt.Sprintf("%d text colors read below 4.5:1 on the light page or on their fills; "+
		"shown with the smallest lightness change that reads (recorded in ink_adjustments)", count), count)
}

// withInputs: front matter is often split into the paper's own files
// (\input{sections/abstract}); they are read in place, two levels deep. Anything else stays
// as written.
func (s *Source) withInputs(source string, depth int) string {
	if depth == 0 {
		return source
	}
	return inputDirective.ReplaceAllStringFunc(source, func(match string) string {
		parts := inputDirective.FindStringSubmatch(match)
		name := rec.Strip(parts[2])
		for _, file := range []string{name, name + ".tex"} {
			input := s.inputName(file)
			if _, ok := s.manifest["input/files/"+input]; ok && input != "" {
				return parts[1] + s.withInputs(string(s.sourceArtifact(input)), depth-1)
			}
		}
		return match
	})
}

// styleCatalog is the records' text style catalog, as the front matter's page decorations read it.
func styleCatalog(records []rec.Map) frontmatter.Styles { return textstyle.NewCatalog(records) }

// projectFrontmatter: front matter the projection cannot type stays in the body as
// ordinary text.
func (s *Source) projectFrontmatter(records []rec.Map, document rec.Map) {
	rescued(func() {
		main := string(s.sourceArtifact(s.main))
		text := s.withInputs(main, 2)
		parser := frontmatter.NewTexMetadata(text).WithStyles(s.ownStyles(text))
		front := frontmatter.ProjectWith(parser, styleCatalog, records, main, s.main, rec.FetchMap(document, "identity"), document)
		if front == nil {
			// A source declaring no title, authors or abstract still has its page headers and
			// footers (a WSPC date head): they are kept on their own.
			s.keepPageDecorations(records, document)
			return
		}
		if unowned, ok := front["decorations_unowned"]; ok {
			delete(front, "decorations_unowned")
			if unowned != nil && unowned != false {
				s.diagnostics.Add("frontmatter.decoration_unowned",
					fmt.Sprintf("%v header or footer glyphs have no reading owner and are not kept", unowned), nil)
			}
		}
		if missing, ok := front["abstract_missing"]; ok {
			delete(front, "abstract_missing")
			if missing != nil && missing != false {
				s.diagnostics.Add("frontmatter.abstract_missing", "the abstract is declared but empty; title and authors are typed alone", nil)
			}
		}
		document["frontmatter"] = front
		// Validated here, so front matter that projects but does not validate stays body text.
		frontmatter.Validate(document)
	}, func(message string) {
		delete(document, "frontmatter")
		s.keepPageDecorations(records, document)
		s.diagnostics.Add("frontmatter.unsupported", fmt.Sprintf("title, authors and abstract stay untyped body text (%s)", message), nil)
	})
}

// keepPageDecorations: the page headers, footers and publisher notices are not front matter,
// so they stay when it fails. They are kept as page_decorations; when they cannot be typed
// either, nothing is kept.
func (s *Source) keepPageDecorations(records []rec.Map, document rec.Map) {
	rescued(func() {
		rows := frontmatter.PageDecorations(styleCatalog, records, rec.FetchMap(document, "identity"), document)
		if len(rows) > 0 {
			document["page_decorations"] = rows
		}
	}, func(string) {})
}

// Provenance is PaperSource#provenance.
func (s *Source) Provenance() rec.Map {
	if s.streamPath == "" {
		rec.Fail("Paper stream must be inspected first")
	}
	s.verifyArtifacts()
	return rec.Map{"id": rec.Fetch(s.selected, "id"), "draw": rec.Fetch(s.selected, "draw"), "qualified": false,
		"artifact_manifest_sha256": digest(s.manifestBytes), "stream_sha256": rec.Fetch(s.manifest, s.streamPath),
		"inspection_sha256": s.inspectionSHA, "input_sha256": s.inputSHA, "archive_sha256": rec.Fetch(s.selected, "zip_sha256"),
		"pdf_page_count": s.pageCountProof, "table_paint_directory": s.tablePaintDir,
		"source_fonts_directory": s.sourceFontsDir, "text_paint_directory": s.textPaintDir,
		"producer": s.producer, "consumer": rec.Map{"version": s.version(), "files": s.consumerFiles()},
		"scope": "Historical probe reinspection; no current engine qualification or complete-paper acceptance implied"}
}

func (s *Source) version() string {
	bytes, err := os.ReadFile(filepath.Join(s.Root, "VERSION"))
	check(err)
	return rec.Strip(string(bytes))
}

// consumerFiles is every file of lib, tex, schemas, profiles and test with its SHA-256.
func (s *Source) consumerFiles() rec.Map {
	return artifacts.FileManifest(s.Root, "lib", "tex", "schemas", "profiles", "test")
}
