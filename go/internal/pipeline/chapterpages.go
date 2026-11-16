package pipeline

import (
	"path/filepath"
	"slices"

	"github.com/sarielhp/relatex/internal/epub"
	"github.com/sarielhp/relatex/internal/glyphowner"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/papersource"
	"github.com/sarielhp/relatex/internal/rec"
)

// readChapterPages are the pages the reconciled document's parts and chapters begin on, in
// increasing order, from the reconcile stage's papersource.ChapterPagesFile; none when a resumed
// stage from before decision 087 has no such file (the layout is then cut by size alone).
func (c *conversion) readChapterPages() []int64 {
	receipt := c.pipeline.receipt("reconcile")
	outputs, _ := receipt.Get("outputs").(ojson.Object)
	if !outputs.Has(papersource.ChapterPagesFile) {
		return nil
	}
	recorded := c.readStageJSON("reconcile", papersource.ChapterPagesFile)
	if recorded["schema_version"] != papersource.ChapterPagesVersion {
		rec.Fail("Unsupported chapter pages schema")
	}
	pages := []int64{}
	for _, value := range rec.FetchList(recorded, "headings") {
		pages = append(pages, rec.FetchInt(rec.AsMap(value), "page"))
	}
	slices.Sort(pages)
	return slices.Compact(pages)
}

// readPageList is where each page of the PDF begins in the reading order, from the reconcile
// stage's papersource.PageListFile; none when a resumed stage from before the page list has no
// such file (the package then has no page list).
func (c *conversion) readPageList() []epub.PageMark {
	receipt := c.pipeline.receipt("reconcile")
	outputs, _ := receipt.Get("outputs").(ojson.Object)
	if !outputs.Has(papersource.PageListFile) {
		return nil
	}
	recorded := c.readStageJSON("reconcile", papersource.PageListFile)
	if recorded["schema_version"] != papersource.PageListVersion {
		rec.Fail("Unsupported page list schema")
	}
	marks := []epub.PageMark{}
	for _, value := range rec.FetchList(recorded, "pages") {
		entry := rec.AsMap(value)
		marks = append(marks, epub.PageMark{Page: rec.FetchInt(entry, "page"), Label: rec.FetchString(entry, "label"),
			Node: rec.FetchString(entry, "node")})
	}
	return marks
}

// publishGlyphOwners copies the reconcile stage's glyph owners (glyphowner.File, decision 105) into
// the export beside document.json.gz; a resumed stage from before them, or a build without a glyph
// map, has none.
func (c *conversion) publishGlyphOwners(root string) {
	receipt := c.pipeline.receipt("reconcile")
	outputs, _ := receipt.Get("outputs").(ojson.Object)
	if !outputs.Has(glyphowner.File) {
		return
	}
	copyFile(c.stageArtifact("reconcile", glyphowner.File), filepath.Join(root, glyphowner.File), 0o644)
}
