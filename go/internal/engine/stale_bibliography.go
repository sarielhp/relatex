package engine

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/sarielhp/relatex/internal/rec"
)

var (
	biblatexBibliography = regexp.MustCompile(`\\(?:refsection|sortlist|datalist)\b|biblatex bbl format`)
	loadsBiblatex        = regexp.MustCompile(`biblatex`)
	biblatexSources      = regexp.MustCompile(`\.(?:tex|cls|sty|bbx|cbx|def)\z`)
)

// staleBiblatexBibliography: JOB.bbl in the main file's directory is in biblatex format while no
// file of the source mentions biblatex, so the document is a BibTeX one and the file is a leftover
// of another build (P8). LaTeX would stop on its \sortlist; BibTeX writes the real one.
func staleBiblatexBibliography(b build) bool {
	path := filepath.Join(b.working, b.job+".bbl")
	if !isFile(path) || !biblatexBibliography.MatchString(readLog(path)) {
		return false
	}
	loaded := false
	err := filepath.WalkDir(b.source, func(file string, entry os.DirEntry, err error) error {
		if err != nil || loaded {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "_relatex_probe" {
				return filepath.SkipDir
			}
			return nil
		}
		if biblatexSources.MatchString(entry.Name()) {
			loaded = loadsBiblatex.MatchString(readLog(file))
		}
		return nil
	})
	check(err)
	return !loaded
}

// setAsideStaleBibliography moves a stale biblatex JOB.bbl out of the working copy, as the other
// products of the author's builds are, and warns naming it.
func (p *Probe) setAsideStaleBibliography(b build) {
	if staleBiblatexBibliography(b) {
		p.setAsideBibliography(b, "is in biblatex format but the document loads no biblatex", "BibTeX writes the bibliography")
	}
}

// setAsideRejectedBibliography: biblatex refused the shipped JOB.bbl (another format version) yet
// TeX would still read it from the source beside the processor's fresh one and stop on its
// \sortlist; it is set aside so the processor's file is the one read.
func (p *Probe) setAsideRejectedBibliography(b build) {
	p.setAsideBibliography(b, "is in a biblatex format version this biblatex rejects", "the processor writes the bibliography")
}

func (p *Probe) setAsideBibliography(b build, reason, writer string) {
	name := b.job + ".bbl"
	check(os.Rename(filepath.Join(b.working, name), filepath.Join(b.output, "supplied.bbl")))
	degradation := rec.Map{"layer": "engine", "level": "warning", "reason": name + " " + reason,
		"effect": "it is ignored and " + writer}
	p.lock.Lock()
	defer p.lock.Unlock()
	for _, existing := range p.degradations {
		if rec.Equal(existing, degradation) {
			return
		}
	}
	p.degradations = append(p.degradations, degradation)
}
