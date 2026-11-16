package engine

import (
	"fmt"
	"path/filepath"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
)

// The index (decision 084): a build whose pass wrote JOB.idx (\makeindex) has makeindex sort
// it into JOB.ind, which \printindex typesets on the next pass, as latexmk does. The request is
// the .idx's digest: makeindex runs again only when the entries changed.

// indexRequest is the SHA-256 of the build's JOB.idx, "" without one.
func indexRequest(b build) string {
	path := filepath.Join(b.output, b.job+".idx")
	if !isFile(path) {
		return ""
	}
	return command.FileDigest(path)
}

// indexDigest is the SHA-256 of the build's JOB.ind, "" without one.
func indexDigest(b build) string {
	path := filepath.Join(b.output, b.job+".ind")
	if !isFile(path) {
		return ""
	}
	return command.FileDigest(path)
}

// updateIndex runs makeindex when the pass's entries differ from the ones last sorted, and
// returns the request now answered.
func (p *Probe) updateIndex(b build, indexed string) string {
	request := indexRequest(b)
	if request == "" || request == indexed {
		return indexed
	}
	p.runIndex(b)
	return request
}

// runIndex is makeindex in the sandbox, in the output directory (its .idx), with the paper's
// directory on the style path; its log and receipt numbered beside the build's output. A failed
// run typesets whatever .ind it left, with an alert.
func (p *Probe) runIndex(b build) {
	executable := command.Sibling(p.doctor.Engine, "makeindex")
	if !command.IsExecutable(executable) {
		rec.Fail("Selected TeX installation lacks makeindex")
	}
	arguments := []string{executable, "-q", "-o", b.job + ".ind", "-t", b.job + ".ilg", b.job + ".idx"}
	environment := p.environment(b, 1)
	environment["INDEXSTYLE"] = searchPath(b.working, b.source)
	wrapped := p.doctor.WrapIn(arguments, b.source, b.output, b.output, environment)
	receipts, err := filepath.Glob(filepath.Join(b.output, "makeindex-*.json"))
	check(err)
	log := filepath.Join(b.output, fmt.Sprintf("makeindex-%d", len(receipts)+1))
	stage := command.Run(wrapped, b.output, log+".log", p.remaining(), environment)
	command.WriteReceipt(log+".json", stage)
	if succeeded(stage) && isFile(filepath.Join(b.output, b.job+".ind")) {
		return
	}
	if stage["timeout"] == true {
		rec.Fail("makeindex timed out; see %s.log", log)
	}
	degradation := rec.Map{"layer": "engine", "level": "alert", "reason": "makeindex failed on the index entries",
		"effect": "the index is missing or incomplete"}
	p.lock.Lock()
	defer p.lock.Unlock()
	for _, existing := range p.degradations {
		if rec.Equal(existing, degradation) {
			return
		}
	}
	p.degradations = append(p.degradations, degradation)
}
