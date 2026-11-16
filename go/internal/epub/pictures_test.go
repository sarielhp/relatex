package epub

import (
	"testing"

	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// TestPackageLeavesOutFilesNoImgShows: the file of a figure shown as inline SVG (decision 146) and
// every original stay in the export directory but not in the package; a figure an img shows is
// carried.
func TestPackageLeavesOutFilesNoImgShows(t *testing.T) {
	items, content := rec.Map{}, []any{}
	for index, source := range []string{"rendered/a.pdf", "rendered/b.pdf"} {
		original := mediamodel.Encoded([]byte("%PDF "+source), "application/pdf")
		shown := mediamodel.Encoded([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="`+string(rune('1'+index))+`"/>`), "image/svg+xml")
		items[rec.ToS(original["sha256"])] = rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": original["sha256"],
			"representations": rec.Map{"original": original, "html": shown}}
		content = append(content, rec.Map{"type": "image", "id": "i" + source, "asset": original["sha256"], "source": source})
	}
	document := rec.Map{"block_order": []any{"b"}, "nodes": rec.Map{"b": rec.Map{"id": "b", "content": content}},
		"assets": rec.Map{"items": items}}
	displayed, originals := figures.Plan(document)
	if len(displayed) != 2 || len(originals) != 2 {
		t.Fatalf("plan %v %v", displayed, originals)
	}
	var shownFile, inlineFile string
	for _, relative := range displayed {
		if shownFile == "" || relative < shownFile {
			inlineFile, shownFile = shownFile, relative
		} else {
			inlineFile = relative
		}
	}
	p := preparePage(`<!DOCTYPE html><html lang="en"><head><title>T</title></head><body><main><p><img src="` + shownFile +
		`" alt="x"/><svg xmlns="http://www.w3.org/2000/svg" data-kind="image" data-src="` + inlineFile + `"><text>y</text></svg></p></main></body></html>`)
	left := unshownFiles(document, p)
	if left[shownFile] || !left[inlineFile] || len(left) != 3 {
		t.Errorf("left out %v (shown %s, inline %s)", left, shownFile, inlineFile)
	}
}
