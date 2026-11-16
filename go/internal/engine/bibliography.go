package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
)

var (
	bibliographyRecord = regexp.MustCompile(`\\(?:bibdata|bibstyle|citation)\{`)
	bibliographyData   = regexp.MustCompile(`\\bibdata\{([^}]*)\}`)
)

// bibliographyRequest is SourceProbe#bibliography_request: [processor, signature] of what the
// build asks the bibliography processor for, nil when it asks for nothing (or reads a
// supplied JOB.bbl instead).
func (p *Probe) bibliographyRequest(b build) []string {
	bcf := filepath.Join(b.output, b.job+".bcf")
	if isFile(bcf) {
		if suppliedBibliography(b, len(bibFiles(b.source)) == 0) {
			return nil
		}
		return []string{"biber", command.FileDigest(bcf)}
	}
	aux := filepath.Join(b.output, b.job+".aux")
	if !isFile(aux) {
		return nil
	}
	records := []string{}
	for _, line := range lines(readLog(aux)) {
		if bibliographyRecord.MatchString(line) {
			records = append(records, line)
		}
	}
	joined := strings.Join(records, "")
	databases := []string{}
	for _, match := range bibliographyData.FindAllStringSubmatch(joined, -1) {
		for _, name := range strings.Split(match[1], ",") {
			databases = append(databases, rec.Strip(name))
		}
	}
	if len(databases) == 0 {
		return nil
	}
	missing := false
	for _, name := range databases {
		if !strings.HasSuffix(name, ".bib") {
			name += ".bib"
		}
		missing = missing || !isFile(filepath.Join(b.working, name))
	}
	if suppliedBibliography(b, missing) {
		return nil
	}
	return []string{"bibtex", hexDigest([]byte(joined))}
}

// bibFiles is Dir.glob('**/*.bib', base: source).
func bibFiles(source string) []string {
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
		if strings.HasSuffix(entry.Name(), ".bib") {
			found = append(found, path)
		}
		return nil
	})
	check(err)
	return found
}

// suppliedBibliography: an arXiv-style paper ships its JOB.bbl without the databases that
// made it; LaTeX reads the supplied file instead of a failing BibTeX run.
func suppliedBibliography(b build, databasesMissing bool) bool {
	return databasesMissing && isFile(filepath.Join(b.working, b.job+".bbl"))
}

// runBibliography is SourceProbe#run_bibliography: biber or BibTeX in the sandbox, its log
// and receipt numbered beside the build's output.
func (p *Probe) runBibliography(b build, name string) {
	executable := command.Sibling(p.doctor.Engine, name)
	if !command.IsExecutable(executable) {
		rec.Fail("Selected TeX installation lacks %s", name)
	}
	arguments := []string{executable, b.job}
	if name == "biber" {
		// The control file by path, not --input-directory: biber resolves a database written as
		// "./book.bib" against its input directory, and the author meant the source directory
		// (biber runs there; the class notes, 2026-10-30). Bare names still search BIBINPUTS.
		arguments = []string{executable, "--output-directory", b.output, filepath.Join(b.output, b.job+".bcf")}
	}
	environment := p.environment(b, 1)
	environment["BIBINPUTS"], environment["BSTINPUTS"] = searchPath(b.working, b.source, b.output), searchPath(b.working, b.source)
	// biber runs where TeX ran (the main file's directory), BibTeX in the output (its .aux).
	working := b.output
	if name == "biber" {
		working = b.working
	}
	wrapped := p.doctor.WrapIn(arguments, b.source, b.output, working, environment)
	receipts, err := filepath.Glob(filepath.Join(b.output, name+"-*.json"))
	check(err)
	log := filepath.Join(b.output, fmt.Sprintf("%s-%d", name, len(receipts)+1))
	stage := command.Run(wrapped, working, log+".log", p.remaining(), environment)
	command.WriteReceipt(log+".json", stage)
	if !succeeded(stage) {
		p.bibliographyErrors(b, name, stage, log)
	}
}

// bibliographyErrors is SourceProbe#bibliography_errors: a bibliography with warnings or
// errors is typeset from the .bbl the processor still wrote; on errors a supplied JOB.bbl
// stands in for it, with an alert. A fatal error, a timeout or no .bbl stops the conversion.
func (p *Probe) bibliographyErrors(b build, name string, stage rec.Map, log string) {
	written := filepath.Join(b.output, b.job+".bbl")
	exit, isInt := stage["exit"].(int64)
	if stage["timeout"] == true || !isInt || exit < 1 || exit > 2 || !isFile(written) {
		rec.Fail("%s failed; see %s.log", name, log)
	}
	if name == "bibtex" && exit == 1 {
		return
	}
	supplied := filepath.Join(b.working, b.job+".bbl")
	effect := "entries it rejected are missing"
	if isFile(supplied) {
		check(copyFile(supplied, written))
		effect = "the paper's supplied " + b.job + ".bbl is typeset instead"
	}
	degradation := rec.Map{"layer": "engine", "level": "alert", "reason": name + " reported errors in the bibliography database",
		"effect": effect}
	p.lock.Lock()
	defer p.lock.Unlock()
	for _, existing := range p.degradations {
		if rec.Equal(existing, degradation) {
			return
		}
	}
	p.degradations = append(p.degradations, degradation)
}
