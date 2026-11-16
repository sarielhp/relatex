package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// Reconstruct is PDFReconstruction.new(layout).write(directory): page graphics rebuilt from
// the layout's SVGs with mutool; the original PDF is not used.
func Reconstruct(layout rec.Map, directory string) rec.Map {
	if layout == nil || !slices.Contains(layoutVersions, rec.Fetch(layout, "schema_version")) {
		rec.Fail("Missing or unsupported reconstruction layout")
	}
	tool := command.Resolve("mutool")
	if tool == "" {
		rec.Fail("PDF reconstruction requires mutool")
	}
	check(os.MkdirAll(directory, 0o755))
	toolHash := command.FileDigest(tool)
	count := LayoutPageCount(layout)
	if count == 0 {
		rec.Fail("Reconstruction page inventory is incomplete")
	}
	files := []string{}
	// A stored layout is read part by part (decision 087).
	EachLayoutPages(layout, func(first int, pages []any) {
		for index, value := range pages {
			page := rec.AsMap(value)
			if !rec.Equal(rec.Fetch(page, "number"), int64(first+index+1)) {
				rec.Fail("Reconstruction page inventory is incomplete")
			}
			name := fmt.Sprintf("page-%06d", rec.FetchInt(page, "number"))
			svg := reconstructionSVG(page)
			check(os.WriteFile(filepath.Join(directory, name+".svg"), svg, 0o644))
			runTool(tool, directory, name, "convert", "-F", "pdf", "-o", name+".pdf", name+".svg")
			files = append(files, name+".pdf")
		}
	})
	runTool(tool, directory, "merge", append([]string{"merge", "-o", "reconstructed.pdf"}, files...)...)
	if command.FileDigest(tool) != toolHash {
		rec.Fail("Reconstruction helper changed")
	}
	return rec.Map{"file": "reconstructed.pdf", "pages": int64(count), "helper_sha256": toolHash,
		"source_pdf_sha256": rec.Fetch(layout, "pdf_sha256"), "fidelity": "unverified until rendered comparison",
		"losses": []any{rec.Map{"node": "document", "original": "searchable source PDF text and interactive annotations",
			"replacement": "outlined vector page graphics", "reason": "Layout reconstruction preserves page graphics",
			"properties": []any{"text_search", "editability", "annotations"}}}}
}

// reconstructionSVG is the page's SVG, checked against its hash ("Reconstruction SVG hash
// differs") and validated.
func reconstructionSVG(page rec.Map) []byte {
	svg := decodeSVG(rec.FetchString(page, "svg"))
	if hexSum(svg) != rec.Fetch(page, "svg_sha256") {
		rec.Fail("Reconstruction SVG hash differs")
	}
	return mediamodel.ValidateSVG(svg)
}

func runTool(tool, directory, name string, arguments ...string) {
	stage := command.Run(append([]string{tool}, arguments...), directory, filepath.Join(directory, name+".log"), 120*time.Second, nil)
	writeJSON(filepath.Join(directory, name+"-stage.json"), stageOrder(stage))
	if !rec.Equal(stage["exit"], int64(0)) || stage["timeout"] != false {
		rec.Fail("Reconstruction %s failed", name)
	}
}
