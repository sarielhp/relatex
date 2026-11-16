package a11ylint

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// cropWord is one source-map word: a page, a box in points and an origin line.
func cropWord(page int, line int, file string, box [4]float64) rec.Map {
	return rec.Map{"id": fmt.Sprintf("g%d", line), "page": int64(page),
		"boxes":   []any{[]any{box[0], box[1], box[2], box[3]}},
		"origins": []any{rec.Map{"file": file, "line": int64(line)}}}
}

// cropMap is a source map over one page with the given words.
func cropMap(width, height float64, words ...rec.Map) rec.Map {
	list := make([]any, len(words))
	for index, word := range words {
		list[index] = word
	}
	return rec.Map{"schema_version": "source-map-2.0", "logical_words": []any{}, "words": list,
		"pages": []any{rec.Map{"number": int64(1), "width": width, "height": height}}}
}

// TestCropBoxesPadClampAndNameEachItemLine: a crop is the item's words' box grown by CropPadding
// and clamped to the page, in item order, named crop-NNN.png; a language or math item gets none
// even when its line has words (document-level).
func TestCropBoxesPadClampAndNameEachItemLine(t *testing.T) {
	sourceMap := cropMap(612, 792,
		cropWord(1, 5, "main.tex", [4]float64{100, 200, 150, 210}),
		cropWord(1, 6, "main.tex", [4]float64{2, 3, 20, 8}))
	items := []Item{
		{Kind: "heading", File: "main.tex", Line: 5},
		{Kind: "language", File: "main.tex", Line: 5},
		{Kind: "heading", File: "main.tex", Line: 6},
		{Kind: "math", File: "main.tex", Line: 6},
	}
	crops := cropBoxes(sourceMap, items)
	if len(crops) != 2 {
		t.Fatalf("%d crops, want 2: %+v", len(crops), crops)
	}
	first, second := crops[0], crops[1]
	if first.Item != 0 || first.Page != 1 || first.PNG != "crop-001.png" {
		t.Errorf("first crop %+v", first)
	}
	if want := (Box{MinX: 88, MinY: 188, MaxX: 162, MaxY: 222}); first.Box != want {
		t.Errorf("first box %v, want %v", first.Box, want)
	}
	if want := (Box{MinX: 0, MinY: 0, MaxX: 32, MaxY: 20}); second.Box != want {
		t.Errorf("clamped box %v, want %v", second.Box, want)
	}
	if second.Item != 2 || second.PNG != "crop-002.png" {
		t.Errorf("second crop %+v", second)
	}
}

// TestCropBoxesFindTheCaptionBelowTheLine: a \includegraphics line has no words; the crop is the
// caption's words below it, up to cropRun consecutive lines.
func TestCropBoxesFindTheCaptionBelowTheLine(t *testing.T) {
	sourceMap := cropMap(612, 792,
		cropWord(1, 8, "main.tex", [4]float64{100, 200, 200, 210}),
		cropWord(1, 9, "main.tex", [4]float64{100, 212, 200, 222}))
	crops := cropBoxes(sourceMap, []Item{{Kind: "figure", File: "main.tex", Line: 6}})
	if len(crops) != 1 || crops[0].Box.MinY != 188 || crops[0].Box.MaxY != 234 {
		t.Fatalf("caption crop %+v", crops)
	}
}

// TestCropBoxesLookaheadAndRunStop: words further below than cropLookahead give no crop, and a
// caption run ends at the first line without words.
func TestCropBoxesLookaheadAndRunStop(t *testing.T) {
	far := cropMap(612, 792, cropWord(1, 10, "main.tex", [4]float64{100, 200, 200, 210}))
	if crops := cropBoxes(far, []Item{{Kind: "figure", File: "main.tex", Line: 6}}); len(crops) != 0 {
		t.Errorf("words 4 lines below gave a crop: %+v", crops)
	}
	gap := cropMap(612, 792,
		cropWord(1, 7, "main.tex", [4]float64{100, 200, 200, 210}),
		cropWord(1, 9, "main.tex", [4]float64{100, 500, 200, 510}))
	crops := cropBoxes(gap, []Item{{Kind: "figure", File: "main.tex", Line: 6}})
	if len(crops) != 1 || crops[0].Box.MaxY != 222 {
		t.Errorf("caption run crossed the gap: %+v", crops)
	}
}

// TestCropBoxesStopAtMaxCrops: at most MaxCrops crops are made; the rest are listed without one.
func TestCropBoxesStopAtMaxCrops(t *testing.T) {
	words := make([]rec.Map, 0, MaxCrops+20)
	items := make([]Item, 0, MaxCrops+20)
	for line := 1; line <= MaxCrops+20; line++ {
		words = append(words, cropWord(1, line, "main.tex", [4]float64{10, 10, 20, 20}))
		items = append(items, Item{Kind: "heading", File: "main.tex", Line: line})
	}
	if crops := cropBoxes(cropMap(612, 792, words...), items); len(crops) != MaxCrops {
		t.Errorf("%d crops, want %d", len(crops), MaxCrops)
	}
}

// TestRegionIsTheBoxInPixels: the pixel rectangle is the points box at CropResolution, at least
// one pixel wide and high.
func TestRegionIsTheBoxInPixels(t *testing.T) {
	if got := (Box{MinX: 88, MinY: 188, MaxX: 162, MaxY: 222}).Region(CropResolution); got != (PixelRegion{X: 134, Y: 287, Width: 114, Height: 52}) {
		t.Errorf("region %+v", got)
	}
	if got := (Box{MinX: 1, MinY: 1, MaxX: 1, MaxY: 1}).Region(CropResolution); got.Width != 1 || got.Height != 1 {
		t.Errorf("degenerate region %+v", got)
	}
}

// fakeRenderer records the crops it is asked to draw and writes a stand-in PNG.
type fakeRenderer struct {
	calls []fakeCall
}

type fakeCall struct {
	pdf, output string
	page        int
	region      PixelRegion
}

func (f *fakeRenderer) Render(pdf, output string, page int, region PixelRegion) error {
	f.calls = append(f.calls, fakeCall{pdf: pdf, output: output, page: page, region: region})
	return os.WriteFile(output, []byte("\x89PNG\r\n\x1a\n"), 0o644)
}

// TestRenderCropsDrawsEachCropThroughTheRenderer: Crops names the export's PDF and each crop, and
// RenderCrops hands every one to the renderer, writing it under destination.
func TestRenderCropsDrawsEachCropThroughTheRenderer(t *testing.T) {
	product := t.TempDir()
	pdf := filepath.Join(product, "paper.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceMap := cropMap(612, 792, cropWord(1, 4, "main.tex", [4]float64{100, 200, 150, 210}))
	paper := Paper{SourceMap: sourceMap}
	items := []Item{{Kind: "heading", File: "main.tex", Line: 4}}
	crops, err := Crops(paper, items, product)
	if err != nil {
		t.Fatal(err)
	}
	if len(crops) != 1 || crops[0].pdf != pdf {
		t.Fatalf("crops %+v", crops)
	}
	destination := filepath.Join(t.TempDir(), "crops")
	renderer := &fakeRenderer{}
	rendered, err := RenderCrops(crops, destination, renderer)
	if err != nil || rendered != 1 || len(renderer.calls) != 1 {
		t.Fatalf("rendered %d, err %v, calls %+v", rendered, err, renderer.calls)
	}
	call := renderer.calls[0]
	if call.pdf != pdf || call.page != 1 || call.output != filepath.Join(destination, "crop-001.png") {
		t.Errorf("call %+v", call)
	}
	if call.region != (PixelRegion{X: 134, Y: 287, Width: 114, Height: 52}) {
		t.Errorf("region %+v", call.region)
	}
	if _, err := os.Stat(call.output); err != nil {
		t.Errorf("written crop: %v", err)
	}
}

// TestCropsNeedTheExportPDF: boxes without the export's PDF are an error, not a silent skip.
func TestCropsNeedTheExportPDF(t *testing.T) {
	sourceMap := cropMap(612, 792, cropWord(1, 4, "main.tex", [4]float64{10, 10, 20, 20}))
	_, err := Crops(Paper{SourceMap: sourceMap}, []Item{{Kind: "heading", File: "main.tex", Line: 4}}, t.TempDir())
	if err == nil {
		t.Fatal("no error without a PDF")
	}
}

// minimalPDF is a hand-made one-page PDF (200 by 200 points) with offsets, enough for poppler.
func minimalPDF() []byte {
	objects := []string{
		"<</Type/Catalog/Pages 2 0 R>>",
		"<</Type/Pages/Kids[3 0 R]/Count 1>>",
		"<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]>>",
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for index, body := range objects {
		offsets[index] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", index+1, body)
	}
	start := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<</Size %d/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, start)
	return out.Bytes()
}

// TestCropsDrawRealPDFPageWhenPopplerIsInstalled: the default renderer makes a PNG of a real
// one-page PDF (skipped when pdftocairo is not on PATH).
func TestCropsDrawRealPDFPageWhenPopplerIsInstalled(t *testing.T) {
	if _, err := exec.LookPath("pdftocairo"); err != nil {
		t.Skip("pdftocairo is not on PATH")
	}
	product := t.TempDir()
	if err := os.WriteFile(filepath.Join(product, "paper.pdf"), minimalPDF(), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceMap := cropMap(200, 200, cropWord(1, 4, "main.tex", [4]float64{50, 50, 100, 80}))
	crops, err := Crops(Paper{SourceMap: sourceMap}, []Item{{Kind: "heading", File: "main.tex", Line: 4}}, product)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "crops")
	rendered, err := RenderCrops(crops, destination, Poppler{})
	if err != nil || rendered != 1 {
		t.Fatalf("rendered %d, err %v", rendered, err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "crop-001.png"))
	if err != nil || !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Errorf("crop %d bytes, err %v", len(data), err)
	}
}
