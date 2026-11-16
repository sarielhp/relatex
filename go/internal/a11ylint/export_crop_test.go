package a11ylint

import (
	"strings"
	"testing"
)

// A table item whose own line draws nothing is cropped at its float's caption, and the review page
// states whether the export would earn the claim.
func TestExportTableCropsAtItsCaption(t *testing.T) {
	sourceMap := cropMap(600, 800, cropWord(1, 8, "main.tex", [4]float64{100, 300, 400, 320}))
	items := []Item{{Kind: "table", File: "main.tex", Line: 4, CropLine: 8}, {Kind: "table", File: "main.tex", Line: 4}}
	crops := cropBoxes(sourceMap, items)
	if len(crops) != 1 || crops[0].Item != 0 || crops[0].Page != 1 {
		t.Fatalf("crops = %+v", crops)
	}
	paper := exportPaper(t, exportReport())
	page, _ := ReviewPage(paper, Lint(paper, Options{}), "")
	if !strings.Contains(page, "the export earns the WCAG claim") {
		t.Error("the review page has no export summary")
	}
}
