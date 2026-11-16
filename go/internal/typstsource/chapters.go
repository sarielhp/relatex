package typstsource

import (
	"fmt"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

type bookFile struct {
	path    string
	content string
}

type unitSpec struct {
	front bool
	nodes []string
}

type renderedUnit struct {
	prefix  string
	body    string
	isFront bool
}

var ruleKeys = []string{"heading", "equation", "figure:image", "figure:table"}

func (e *exporter) hasChapters() bool {
	for id := range e.nodes {
		node := e.node(id)
		if toS(node["type"]) == "section" && dig(node, "semantics", "counter") == "chapter" {
			return true
		}
	}
	return false
}

func (e *exporter) renderBook() (string, []bookFile) {
	e.labelNumbers = map[string]string{}
	e.labelsMerged = map[string]bool{}
	e.placed = map[string]string{}
	e.writtenImages = map[string]string{}
	var main string
	var files []bookFile
	for range 2 {
		e.placedBefore = e.placed
		e.placed = map[string]string{}
		main, files = e.renderBookPass()
	}
	e.report = e.buildReport()
	return main, files
}

func (e *exporter) renderBookPass() (string, []bookFile) {
	e.resetState()
	e.activeRules = map[string]string{}
	e.imagePrefix = "../"
	defer func() {
		e.activeRules = nil
		e.imagePrefix = ""
	}()
	emitted := map[string]string{}
	units := e.bookUnits()
	rendered := make([]renderedUnit, 0, len(units)+1)
	for _, unit := range units {
		e.resetUnitTheorems()
		prefix := e.mainRulePrefix(emitted)
		body := e.renderNodeSlice(unit.nodes)
		if body == "" && !unit.front {
			continue
		}
		rendered = append(rendered, renderedUnit{prefix: prefix, body: body, isFront: unit.front})
	}
	settings := e.settings()
	front := e.frontMatter()
	return e.assembleBook(settings, front, rendered)
}

func (e *exporter) assembleBook(settings, front string, rendered []renderedUnit) (string, []bookFile) {
	if front != "" {
		if len(rendered) > 0 && rendered[0].isFront {
			rendered[0].body = joinPresent("\n\n", front, rendered[0].body)
		} else {
			rendered = append([]renderedUnit{{body: front, isFront: true}}, rendered...)
		}
	}
	chapters := make([]renderedUnit, 0, len(rendered))
	for _, unit := range rendered {
		if unit.body != "" {
			chapters = append(chapters, unit)
		}
	}
	width := 2
	if len(chapters) >= 100 {
		width = 3
	}
	sections := joinNonEmpty(header, settings, e.bibliographySetup())
	head := "#import \"../" + preambleFile + "\": *\n"
	if e.styledBib != nil {
		head += "#import \"../" + SupportDirectory + "/" + bibliographyFile + "\": pergamon\n"
	}
	files := make([]bookFile, 0, len(chapters))
	for index, unit := range chapters {
		relPath := fmt.Sprintf("chapters/%0*d.typ", width, index+1)
		include := "#include " + typstString(relPath)
		if unit.prefix != "" {
			include = unit.prefix + "\n" + include
		}
		sections = append(sections, include)
		files = append(files, bookFile{
			path:    relPath,
			content: head + "\n" + unit.body + "\n",
		})
	}
	return strings.Join(sections, "\n\n") + "\n", files
}

func joinNonEmpty(items ...string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func (e *exporter) bookUnits() []unitSpec {
	kids := children(e.node(rec.Fetch(e.document, "root")))
	units := []unitSpec{}
	pending := []string{}
	sawSection := false
	for _, id := range kids {
		node := e.node(id)
		if toS(node["type"]) != "section" {
			pending = append(pending, id)
			continue
		}
		if !sawSection {
			sawSection = true
			units = append(units, unitSpec{front: true, nodes: pending})
			pending = nil
		} else if len(pending) > 0 {
			units = append(units, unitSpec{nodes: pending})
			pending = nil
		}
		if e.isPartSection(node) {
			units = append(units, e.partUnits(node)...)
		} else {
			units = append(units, unitSpec{nodes: []string{id}})
		}
	}
	if !sawSection {
		units = append(units, unitSpec{front: true, nodes: pending})
	} else if len(pending) > 0 {
		units = append(units, unitSpec{nodes: pending})
	}
	return units
}

func (e *exporter) isPartSection(node rec.Map) bool {
	if dig(node, "semantics", "counter") == "part" {
		return true
	}
	for _, cid := range children(node) {
		child := e.node(cid)
		if toS(child["type"]) == "section" && dig(child, "semantics", "counter") == "chapter" {
			return true
		}
	}
	return false
}

func (e *exporter) partUnits(section rec.Map) []unitSpec {
	units := []unitSpec{}
	pending := []string{}
	for _, cid := range children(section) {
		if toS(e.node(cid)["type"]) != "section" {
			pending = append(pending, cid)
			continue
		}
		if len(pending) > 0 {
			units = append(units, unitSpec{nodes: pending})
			pending = nil
		}
		units = append(units, unitSpec{nodes: []string{cid}})
	}
	if len(pending) > 0 {
		units = append(units, unitSpec{nodes: pending})
	}
	return units
}

func (e *exporter) resetUnitTheorems() {
	for _, existing := range e.theoremKinds {
		existing.defined = false
		existing.used = false
	}
}

func (e *exporter) trackRule(line string) {
	if e.activeRules == nil {
		return
	}
	switch {
	case strings.HasPrefix(line, "#set heading("):
		e.activeRules["heading"] = line
	case strings.HasPrefix(line, "#set math.equation("):
		e.activeRules["equation"] = line
	case strings.HasPrefix(line, "#show figure.where(kind: image):"):
		e.activeRules["figure:image"] = line
	case strings.HasPrefix(line, "#show figure.where(kind: table):"):
		e.activeRules["figure:table"] = line
	}
}

func (e *exporter) mainRulePrefix(emitted map[string]string) string {
	lines := []string{}
	for _, key := range ruleKeys {
		active := e.activeRules[key]
		if active != "" && emitted[key] != active {
			lines = append(lines, active)
			emitted[key] = active
		}
	}
	return strings.Join(lines, "\n")
}
