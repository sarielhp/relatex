package html

import (
	"os"
	"path/filepath"

	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// WriteImageFiles is PaperImages.write_files!: every displayed image of the document, once,
// under its figure name in directory/figs, then each figure's original beside it.
func WriteImageFiles(document rec.Map, directory string) {
	displayed, originals := figures.Plan(document)
	for _, span := range figures.Occurrences(document) {
		digest, _ := figures.Identity(document, span)
		relative, named := displayed[digest]
		if !named {
			continue
		}
		data, _ := figures.Displayed(document, span)
		writeOnce(filepath.Join(directory, relative), data)
	}
	writeOriginals(document, directory, originals)
}

// writeOriginals is PaperImages.write_originals!: a figure's original (a source PDF, a cropped
// JPEG) is kept beside what the HTML displays.
func writeOriginals(document rec.Map, directory string, originals map[string]string) {
	items, _ := rec.FetchMap(document, "assets")["items"].(rec.Map)
	for _, source := range figures.Originals(document) {
		relative, named := originals[source.Digest]
		if !named {
			continue
		}
		asset := rec.AsMap(items[source.Asset])
		figure := mediamodel.NewMediaAsset(asset)
		writeOnce(filepath.Join(directory, relative), figure.Bytes(figure.Representation("original", nil)))
	}
}

func writeOnce(path string, data []byte) {
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		rec.Fail("%v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		rec.Fail("%v", err)
	}
}

// Root is the repository root, where lib/relatex/preview.js is read; the export writer sets it.
var Root string

// WritePreviewScript writes preview.js, the reference previews of the reading page, beside it.
func WritePreviewScript(directory string) {
	script, err := os.ReadFile(filepath.Join(Root, "lib", "relatex", "preview.js"))
	if err != nil {
		rec.Fail("%v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "preview.js"), script, 0o644); err != nil {
		rec.Fail("%v", err)
	}
}
