package browser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/html"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// A figure's name no longer carries its content, so a changed file is caught against the
// document (PaperImages.verify_files!).
func TestVerifyImageFilesReadsTheDocumentNotTheName(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>`)
	original := mediamodel.Encoded([]byte("source pdf"), "application/pdf")
	shown := mediamodel.Encoded(svg, "image/svg+xml")
	asset := rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": original["sha256"],
		"representations": rec.Map{"original": original, "html": shown, "markdown": shown, "typst": shown}}
	span := rec.Map{"id": "n-0-image", "type": "image", "asset": asset["sha256"], "width_sp": int64(655_360),
		"height_sp": int64(655_360), "source": "figs/plot.pdf"}
	document := rec.Map{
		"block_order": []any{"n-0"},
		"nodes":       rec.Map{"n-0": rec.Map{"id": "n-0", "content": []any{span}}},
		"assets":      rec.Map{"items": rec.Map{rec.FetchString(asset, "sha256"): asset}},
	}
	directory := t.TempDir()
	html.WriteImageFiles(document, directory)
	verifyImageFiles(document, directory)

	changed := filepath.Join(directory, "figs/plot.svg")
	if err := os.WriteFile(changed, append(svg, ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() {
		problem := recover()
		if problem == nil {
			t.Fatal("a changed figure file went unnoticed")
		}
		if failure, ok := problem.(rec.Failure); !ok || failure.Message != "Linked image file changed: figs/plot.svg" {
			t.Fatalf("recovered %v", problem)
		}
	}()
	verifyImageFiles(document, directory)
}

// A QED mark drawn as a picture (the duck, decision 176) shows its image as an img: the page's images count it,
// as PaperImages.occurrences does (books/0004: "Browser changed paper image count").
func TestImageSpansCountASymbolsImage(t *testing.T) {
	duck := rec.Map{"type": "image", "id": "duck", "asset": "a"}
	document := rec.Map{"block_order": []any{"p"}, "nodes": rec.Map{"p": rec.Map{"id": "p", "type": "paragraph",
		"content": []any{rec.Map{"type": "text", "text": "Done."}, rec.Map{"type": "symbol", "name": "qed", "image": duck},
			rec.Map{"type": "symbol", "name": "qed", "text": "■"}}}}}
	spans := imageSpans(document)
	if len(spans) != 1 || spans[0]["id"] != "duck" {
		t.Fatalf("image spans %v", spans)
	}
}
