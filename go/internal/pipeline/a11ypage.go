package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/a11ylint"
	"github.com/sarielhp/relatex/internal/a11yreport"
	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// writeAccessibilityPage writes accessibility.html, the export's report page (decision 112), into
// the export root when html or pdf-ua was exported: the claims of the page, the EPUB package and
// the tagged PDF, their open items, the source lint's items and the provenance counts. An
// accessibility conversion (--accessible) only adds the outputs it claims on.
func (c *conversion) writeAccessibilityPage(root string, results ojson.Object) {
	if !results.Has("html") && !results.Has("pdf-ua") {
		return
	}
	main := rec.ToS(c.archive.Manifest["main"])
	inputs := a11yreport.Inputs{Title: c.documentTitle(main), Main: main, Version: c.version(), Date: time.Now().Format("2006-01-02"),
		Document: c.document, EPUB: results.Has("epub")}
	if web, ok := readJSONMap(filepath.Join(root, "accessibility.json")); ok {
		inputs.Web = web
	}
	if tagged, ok := results.Get("pdf-ua").(ojson.Object); ok {
		inputs.Tagged = tagged
	}
	var paper a11ylint.Paper
	inputs.Lint, inputs.LintNote, paper = c.sourceLint(main, inputs.Web)
	inputs.LintSummary = a11ylint.ExportSummary(paper)
	if err := os.WriteFile(filepath.Join(root, a11yreport.FileName), []byte(a11yreport.Page(inputs)), 0o644); err != nil {
		rec.Fail("%v", err)
	}
}

// sourceLint is the accessibility lint of the author's source directory with this conversion's
// figures, or why there is none (an archive input has no lines to point to).
// The paper is returned with it, for the summary of what the export still lacks (a11ylint.ExportSummary).
func (c *conversion) sourceLint(main string, web rec.Map) ([]a11ylint.Item, string, a11ylint.Paper) {
	directory := c.inputDirectory()
	if directory == "" {
		return nil, "The input was an archive: run relatex lint --accessibility in the source directory for the items with their lines.", a11ylint.Paper{}
	}
	files, err := a11ylint.Load(directory, main)
	if err != nil {
		return nil, "The source could not be read for the lint: " + err.Error(), a11ylint.Paper{}
	}
	sidecar, err := figdesc.Read(filepath.Join(directory, figdesc.FileName))
	if err != nil {
		return nil, "The lint could not read " + figdesc.FileName + ": " + err.Error(), a11ylint.Paper{}
	}
	paper := a11ylint.Paper{Files: files, Sidecar: sidecar, Directory: directory, SourceMap: c.sourceMap}.WithConversion(c.document)
	if web != nil {
		paper = paper.WithExport(web, c.document)
	}
	return a11ylint.Lint(paper, a11ylint.Options{}), "", paper
}

// documentTitle is the document's title, else its main file's name.
func (c *conversion) documentTitle(main string) string {
	// A document without a title block has no "frontmatter" key; its title is then the file name,
	// never an invented one.
	front, _ := c.document["frontmatter"].(rec.Map)
	if title := strings.TrimSpace(rec.ToS(front["title"])); title != "" {
		return title
	}
	return filepath.Base(main)
}

// version is relatex's version (the VERSION file of its resources).
func (c *conversion) version() string {
	data, err := os.ReadFile(filepath.Join(c.options.Root, "VERSION"))
	if err != nil {
		return "(unknown version)"
	}
	return strings.TrimSpace(string(data))
}

// readJSONMap reads a JSON object written earlier in the run.
func readJSONMap(path string) (rec.Map, bool) {
	value, err := ojson.ReadFile(path)
	if err != nil {
		return nil, false
	}
	parsed, ok := ojson.Plain(value).(rec.Map)
	return parsed, ok
}
