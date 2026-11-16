package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

// seedFiles are the convergence files and the generated bibliography: what a settled build
// hands to relatex's builds.
var seedFiles = regexp.MustCompile(`\.(?:aux|toc|out|lof|lot|bcf|run\.xml|nav|snm|bbl|ind)\z`)

// settleVersion names the settled-build state's format.
const settleVersion = "relatex-settle-4"

// Seed is the paper settled before relatex's own builds: a plain LuaLaTeX build, with its
// bibliography, run until the cross-reference files stop changing, in a directory of
// relatex's cache that persists between conversions (it replaces the leeni seed). Its
// converged files seed both builds: a build whose first pass leaves them unchanged needs no
// second one, and one that rewrites them just converges as it would have without the seed.
type Seed struct {
	files map[string]string
	names []string
}

// seedDirectory is the settled build's directory: one per set of file names and main file.
func seedDirectory(manifest rec.Map, main string) string {
	names := []any{}
	for _, value := range rec.FetchList(manifest, "files") {
		names = append(names, rec.Fetch(rec.AsMap(value), "path"))
	}
	slices.SortFunc(names, func(a, b any) int { return strings.Compare(rec.AsString(a), rec.AsString(b)) })
	key, err := canonjson.Marshal([]any{main, names})
	check(err)
	return filepath.Join(command.CacheRoot(), "settled", hexDigest(key)[:16])
}

// settleKey is what a settled build depends on: the main file, every source file's content
// and the toolchain. An unchanged paper reuses its settled build without running TeX.
func (p *Probe) settleKey() string {
	files := []any{}
	for _, value := range rec.FetchList(p.manifest, "files") {
		file := rec.AsMap(value)
		if file["directory"] == true {
			continue
		}
		files = append(files, []any{rec.Fetch(file, "path"), rec.Fetch(file, "sha256")})
	}
	key, err := canonjson.Marshal([]any{settleVersion, p.main, files, p.doctor.Fingerprint()})
	check(err)
	return hexDigest(key)
}

// prepareSeed settles the paper in directory, or reuses its settled build; nil when the
// build failed or settled nothing. directory/settle.json reports what happened either way.
func (p *Probe) prepareSeed(directory string) *Seed {
	check(os.MkdirAll(directory, 0o755))
	key := p.settleKey()
	started := time.Now()
	report := rec.Map{"schema_version": settleVersion, "key": key}
	seed := reusedSeed(directory, key)
	if seed != nil {
		report["status"] = "reused"
	} else {
		seed = p.settle(directory, key, report)
	}
	report["seconds"] = time.Since(started).Seconds()
	writeJSON(filepath.Join(directory, "settle.json"), report)
	if seed == nil || len(seed.files) == 0 {
		return nil
	}
	return seed
}

// reusedSeed is the settled build of an earlier conversion with the same key, when its files
// are still those it recorded.
func reusedSeed(directory, key string) *Seed {
	state, err := os.ReadFile(filepath.Join(directory, "settled.json"))
	if err != nil {
		return nil
	}
	value, err := canonjson.Decode(strings.NewReader(string(state)))
	recorded, ok := value.(rec.Map)
	if err != nil || !ok || recorded["schema_version"] != settleVersion || recorded["key"] != key {
		return nil
	}
	files, _ := recorded["files"].(rec.Map)
	seed := &Seed{files: map[string]string{}}
	for _, name := range sortedNames(files) {
		path := filepath.Join(directory, "build", name)
		if !isFile(path) || command.FileDigest(path) != files[name] {
			return nil
		}
		seed.files[name] = path
		seed.names = append(seed.names, name)
	}
	return seed
}

func sortedNames(files rec.Map) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// settle builds the paper plainly (no collector) in directory/build until it converges, in
// the builds' own sandbox, and records the settled files under key.
func (p *Probe) settle(directory, key string, report rec.Map) (seed *Seed) {
	defer p.unsettled(&seed, report)
	check(os.RemoveAll(filepath.Join(directory, "settled.json")))
	b := p.settleBuild(directory)
	passes, converged := p.settlePasses(b)
	report["passes"] = int64(passes)
	if !converged {
		report["status"] = "failed"
		report["reason"] = "the build did not converge"
		return nil
	}
	seed = collectSeed(b, isFile(filepath.Join(b.working, b.job+".bbl")) && p.bibliographyRequest(b) == nil)
	files := rec.Map{}
	for _, name := range seed.names {
		files[name] = command.FileDigest(seed.files[name])
	}
	writeJSON(filepath.Join(directory, "settled.json"), rec.Map{"schema_version": settleVersion, "key": key, "files": files})
	report["status"] = "settled"
	report["files"] = len(seed.names)
	return seed
}

// unsettled turns a failed settling build into no seed: relatex's builds converge without one.
func (p *Probe) unsettled(seed **Seed, report rec.Map) {
	recovered := recover()
	if recovered == nil {
		return
	}
	failure, ok := recovered.(rec.Failure)
	if !ok || failure.Lookup {
		panic(recovered)
	}
	report["status"], report["reason"] = "failed", failure.Message
	*seed = nil
}

// settleBuild is the settling build's working copy (fresh) and output (kept between
// conversions, so a paper that changed a little converges from its last state).
func (p *Probe) settleBuild(directory string) build {
	source := filepath.Join(directory, "working")
	output := filepath.Join(directory, "build")
	check(os.RemoveAll(source))
	check(os.MkdirAll(output, 0o755))
	copyTree(filepath.Join(p.Directory, "input", "files"), source)
	job := JobName(p.main)
	for _, extension := range stale {
		check(os.RemoveAll(filepath.Join(p.workingIn(source), job+"."+extension)))
	}
	stalePasses, err := filepath.Glob(filepath.Join(output, "pass-*"))
	check(err)
	for _, path := range stalePasses {
		check(os.Remove(path))
	}
	check(os.WriteFile(filepath.Join(output, "inject.tex"), []byte(p.wrapper("control")), 0o644))
	return build{strategy: "settle", source: source, output: output, collector: filepath.Join(source, "_relatex_probe"), job: job,
		working: p.workingIn(source)}
}

// settlePasses runs the plain passes, the bibliography whenever its request changes, until
// a pass leaves the cross-reference files as they were and its log asks for no rerun. The
// bibliography's alerts belong to relatex's own builds, not to this one.
func (p *Probe) settlePasses(b build) (int, bool) {
	p.lock.Lock()
	alerts := len(p.degradations)
	p.lock.Unlock()
	defer func() {
		p.lock.Lock()
		p.degradations = p.degradations[:alerts]
		p.lock.Unlock()
	}()
	fonts := texsandbox.NewFontGenerator(p.doctor.Engine)
	defer func() {
		p.recordGeneratedFonts(fonts.Generated)
	}()
	previous := convergenceState(b)
	var bibliography []string
	indexed := ""
	for pass := 1; pass <= 6; pass++ {
		readIndex := indexDigest(b)
		if !succeeded(p.texPass(b, pass, fonts)) {
			return pass, false
		}
		if indexed = p.updateIndex(b, indexed); indexDigest(b) != readIndex {
			previous = nil
		}
		state := convergenceState(b)
		if signature := p.bibliographyRequest(b); signature != nil && !slices.Equal(signature, bibliography) {
			p.runBibliography(b, signature[0])
			bibliography = signature
			previous = nil
		}
		log := readLog(filepath.Join(b.output, fmt.Sprintf("pass-%d.log", pass)))
		if previous != nil && rec.Equal(state, previous) && !rerunRequest.MatchString(log) {
			return pass, true
		}
		previous = state
	}
	return 6, false
}

// collectSeed is the settled build's files. A bibliography the builds read from the paper
// (supplied: its own JOB.bbl, with no processor to run) is not seeded: the source map can
// point into the paper's file.
func collectSeed(b build, supplied bool) *Seed {
	matches, err := filepath.Glob(filepath.Join(b.output, globEscape(b.job)+".*"))
	check(err)
	slices.Sort(matches)
	seed := &Seed{files: map[string]string{}}
	for _, path := range matches {
		if supplied && filepath.Base(path) == b.job+".bbl" {
			continue
		}
		if isFile(path) && seedFiles.MatchString(path) {
			seed.files[filepath.Base(path)] = path
			seed.names = append(seed.names, filepath.Base(path))
		}
	}
	return seed
}

// globEscape quotes the glob metacharacters of a literal name.
func globEscape(name string) string {
	return strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`).Replace(name)
}

// Apply copies the seed into a build's output directory.
func (s *Seed) Apply(output string) {
	for _, name := range s.names {
		check(copyFile(s.files[name], filepath.Join(output, name)))
	}
}

// BibliographySignature is the bibliography request the seed answers, so a build whose
// control file is unchanged does not run biber again.
func (s *Seed) BibliographySignature(job string) []string {
	bcf, hasControl := s.files[job+".bcf"]
	if _, hasBibliography := s.files[job+".bbl"]; !hasControl || !hasBibliography {
		return nil
	}
	return []string{"biber", command.FileDigest(bcf)}
}

// prepareIncrementalSeed is the seed of a build that wants one instrumented pass, like latexmk's
// second run: the converged files of the last such build of this paper, whatever its sources then
// were (a build that finds them stale just goes on to another pass). Without one, the paper is
// settled plainly as for any build.
func (p *Probe) prepareIncrementalSeed(directory string) *Seed {
	if seed := keptSeed(directory, p.bibliographyInputs()); seed != nil {
		return seed
	}
	return p.prepareSeed(directory)
}

// bibliographyInputs is what the bibliography is made from besides the build's own requests: the
// databases and styles. A kept seed holds a bibliography made from other ones is of no use, because
// the processor reruns only when the build's request changes.
func (p *Probe) bibliographyInputs() string {
	parts := []any{}
	for _, value := range rec.FetchList(p.manifest, "files") {
		file := rec.AsMap(value)
		path := rec.AsString(rec.Fetch(file, "path"))
		if file["directory"] != true && bibliographyFile.MatchString(path) {
			parts = append(parts, []any{path, rec.Fetch(file, "sha256")})
		}
	}
	key, err := canonjson.Marshal(parts)
	check(err)
	return hexDigest(key)
}

// bibliographyFile names the files a bibliography is made from.
var bibliographyFile = regexp.MustCompile(`\.(?:bib|bst|bbx|cbx|lbx|dbx)\z`)

// keptSeed is the files recorded in directory/settled.json when they are all still there.
func keptSeed(directory, bibliography string) *Seed {
	state, err := os.ReadFile(filepath.Join(directory, "settled.json"))
	if err != nil {
		return nil
	}
	value, err := canonjson.Decode(strings.NewReader(string(state)))
	recorded, ok := value.(rec.Map)
	if err != nil || !ok || recorded["schema_version"] != settleVersion {
		return nil
	}
	files, _ := recorded["files"].(rec.Map)
	if len(files) == 0 || recorded["bibliography"] != bibliography {
		return nil
	}
	return reusedSeed(directory, rec.AsString(recorded["key"]))
}

// keepSeed records the instrumented build's converged files as the next build's seed.
func (p *Probe) keepSeed() {
	output := filepath.Join(p.Directory, "external", "output")
	job := JobName(p.main)
	source := filepath.Join(p.Directory, "external", "working")
	b := build{strategy: "external", source: source, output: output, job: job, working: p.workingIn(source)}
	seed := collectSeed(b, isFile(filepath.Join(b.working, job+".bbl")) && p.bibliographyRequest(b) == nil)
	if len(seed.names) == 0 {
		return
	}
	kept := filepath.Join(p.seedDir, "build")
	check(os.MkdirAll(kept, 0o755))
	files := rec.Map{}
	for _, name := range seed.names {
		check(copyFile(seed.files[name], filepath.Join(kept, name)))
		files[name] = command.FileDigest(filepath.Join(kept, name))
	}
	writeJSON(filepath.Join(p.seedDir, "settled.json"), rec.Map{"schema_version": settleVersion, "key": p.settleKey(), "bibliography": p.bibliographyInputs(), "files": files})
}
