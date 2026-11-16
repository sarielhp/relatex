package alttext

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/media"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// turnedFigureDocument is a document whose one figure is page one of pagePDF (a blue square at
// x 10-50, y 20-60 of a 200 pt page) included with \includegraphics[angle=-90].
func turnedFigureDocument(t *testing.T) rec.Map {
	t.Helper()
	original := mediamodel.Encoded(pagePDF(t), "application/pdf")
	sha := original["sha256"].(string)
	size := geometry.BPToSP(200)
	turn := rec.Map{"kind": "rotate", "cos": int64(0), "sin": int64(-1), "origin_x": int64(0), "origin_y": int64(0),
		"width": size, "height": int64(0), "depth": size, "input_width": size, "input_height": size, "input_depth": int64(0)}
	placement := media.Placement(rec.Map{"page": int64(1), "pagebox": "cropbox", "natural_width": size, "natural_height": size},
		rec.Map{"width": size, "height": size, "viewport": []any{0.0, 0.0, 200.0, 200.0}, "clip": false}, []any{turn})
	id := rec.FetchString(placement, "id")
	display := mediamodel.Encoded([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>`), "image/svg+xml")
	display["placement"] = id
	asset := rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": sha, "metadata": rec.Map{},
		"placements": rec.Map{id: placement}, "representations": rec.Map{"original": original, "display:" + id: display}}
	nodes := rec.Map{"p1": rec.Map{"id": "p1", "type": "paragraph", "content": []any{rec.Map{"type": "image", "id": "i1",
		"asset": sha, "source": "figures/turned.pdf", "placement": placement}}}}
	return rec.Map{"language": rec.Map{"tag": "en"}, "assets": rec.Map{"items": rec.Map{sha: asset}},
		"nodes": nodes, "block_order": []any{"p1"}}
}

// TestTurnedFigureIsDescribedAsPrinted: a figure the paper prints turned (angle=-90, decision 123
// amendment 1) reaches the describer turned: its picture and its geometry, which says so.
func TestTurnedFigureIsDescribedAsPrinted(t *testing.T) {
	document := turnedFigureDocument(t)
	figure := figdesc.Figures(document)[0]
	if figure.Turns != 3 {
		t.Fatalf("turns %d, want 3 (a quarter turn clockwise)", figure.Turns)
	}
	if command.Resolve("pdftocairo") != "" {
		path, mime, err := writeImage(document, figure, t.TempDir())
		if err != nil || mime != "image/png" || !strings.HasSuffix(path, "figure-turned.png") {
			t.Fatalf("image %q %q %v", path, mime, err)
		}
		picture := decodePNG(t, path)
		// The square's centre (30, 160) pt from the top left, at 200 dpi, lands at (H-1-y, x).
		size := picture.Bounds().Dx()
		if !blue(picture.At(size-1-444, 83)) || blue(picture.At(83, 444)) {
			t.Errorf("the square is not where a clockwise quarter turn puts it")
		}
	}
	if command.Resolve("mutool") == "" {
		t.Skip("mutool not installed")
	}
	scanned, err := ScanFigure(document, figure, t.TempDir())
	if err != nil || scanned == nil {
		t.Fatalf("scan: %v", err)
	}
	if !strings.Contains(scanned.Text(), "turned 90 degrees clockwise as the paper prints it") {
		t.Errorf("text:\n%s", scanned.Text())
	}
	seen := 0
	for _, node := range scanned.Nodes {
		for _, part := range node.Parts {
			for _, p := range part.Points {
				seen++
				// Drawn at x 5-25, y 10-30 of 100; turned clockwise: x 10-30, y 75-95.
				if p[0] < 9.9 || p[0] > 30.1 || p[1] < 74.9 || p[1] > 95.1 {
					t.Errorf("point %v is not turned", p)
				}
			}
		}
	}
	if seen == 0 {
		t.Errorf("no shape read:\n%s", scanned.Text())
	}
}

func decodePNG(t *testing.T, path string) image.Image {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	picture, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return picture
}

func blue(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return b > 0xc000 && r < 0x4000 && g < 0x4000
}

// cropPDF is a one-page PDF whose media box (200 pt) holds a leftover square outside its crop box
// (the lower-left 100 pt), as drawing programs leave them.
func cropPDF() []byte {
	content := "0 0 1 rg 10 10 40 40 re f 150 150 40 40 re f"
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /CropBox [0 0 100 100] /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)}
	data := "%PDF-1.7\n"
	offsets := []int{}
	for index, object := range objects {
		offsets = append(offsets, len(data))
		data += fmt.Sprintf("%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	start := len(data)
	data += fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		data += fmt.Sprintf("%010d 00000 n \n", offset)
	}
	return []byte(data + fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, start))
}

// TestWholePageRendersItsCropBox: the describer sees what the paper prints, the crop box, not the
// leftovers of the media box around it (decision 123 amendment 3).
func TestWholePageRendersItsCropBox(t *testing.T) {
	tool := command.Resolve("pdftocairo")
	if tool == "" {
		t.Skip("pdftocairo not installed")
	}
	directory := t.TempDir()
	path, target := filepath.Join(directory, "in.pdf"), filepath.Join(directory, "out")
	if err := os.WriteFile(path, cropPDF(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command(tool, rasterArgs(&figdesc.Figure{Page: 1, PageBox: "cropbox"}, path, target)...).Run(); err != nil {
		t.Fatal(err)
	}
	picture := decodePNG(t, target+".png")
	if size := picture.Bounds(); size.Dx() != 278 || size.Dy() != 278 {
		t.Errorf("rendered %v, want the 100 pt crop box at 200 dpi (278 px)", size)
	}
}

// TestTurnImageMovesEachPixel: a 3 × 2 picture with one marked corner, turned by each quarter.
func TestTurnImageMovesEachPixel(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 3, 2))
	mark := color.RGBA{R: 255, A: 255}
	picture.Set(2, 0, mark) // top right
	cases := []struct {
		quarters, width, x, y int
	}{
		{0, 3, 2, 0}, {1, 2, 0, 0}, {2, 3, 0, 1}, {3, 2, 1, 2}, {-1, 2, 1, 2},
	}
	for _, test := range cases {
		turned := turnImage(picture, test.quarters)
		if turned.Bounds().Dx() != test.width || turned.At(test.x, test.y) != color.Color(color.RGBA{R: 255, A: 255}) {
			t.Errorf("quarters %d: size %v, mark not at (%d, %d)", test.quarters, turned.Bounds(), test.x, test.y)
		}
	}
}
