package alttext

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// pngFigureDocument is a document whose one figure is a PNG with printed text: a gnuplot figure
// rendered by pdftocairo.
func pngFigureDocument(t *testing.T) rec.Map {
	t.Helper()
	if _, err := os.Stat(filepath.Join("..", "figscan", "testdata", "gnuplot-Fig3.pdf")); err != nil {
		t.Skip("the figure fixtures are not in this tree (they are not published)")
	}
	out := filepath.Join(t.TempDir(), "plot")
	render := exec.Command(command.Resolve("pdftocairo"), "-png", "-r", "150", "-singlefile",
		filepath.Join("..", "figscan", "testdata", "gnuplot-Fig3.pdf"), out)
	if data, err := render.CombinedOutput(); err != nil {
		t.Fatalf("pdftocairo: %v %s", err, data)
	}
	data, err := os.ReadFile(out + ".png")
	if err != nil {
		t.Fatal(err)
	}
	original := mediamodel.Encoded(data, "image/png")
	sha := original["sha256"].(string)
	asset := rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": sha, "representations": rec.Map{"original": original,
		"html": original}}
	return rec.Map{"language": rec.Map{"tag": "en"}, "assets": rec.Map{"items": rec.Map{sha: asset}},
		"block_order": []any{"p1"},
		"nodes": rec.Map{"p1": rec.Map{"id": "p1", "type": "paragraph", "content": []any{
			rec.Map{"type": "image", "id": "i1", "asset": sha, "source": "figs/plot.png"}}}}}
}

func TestDrafterSendsTheTextOCRReadInARasterFigure(t *testing.T) {
	if command.Resolve("tesseract") == "" || command.Resolve("pdftocairo") == "" {
		t.Skip("tesseract or pdftocairo missing")
	}
	script, _ := stub(t, `prompt=$(cat)
test -s "$RELATEX_FIGURE_GEOMETRY" || exit 7
case "$prompt" in *"Geometry of the figure"*) ;; *) exit 8;; esac
grep -q "source ocr" "$RELATEX_FIGURE_GEOMETRY" || exit 9
grep -q 'read by OCR "300"' "$RELATEX_FIGURE_GEOMETRY" || exit 10
echo "Labels seen."`)
	sidecar, err := figdesc.Read(filepath.Join(t.TempDir(), figdesc.FileName))
	if err != nil {
		t.Fatal(err)
	}
	drafter := &Drafter{Command: shellCommand(t, script), Model: "stub", Timeout: 30 * time.Second, Today: "2026-11-12"}
	outcome, err := drafter.Run(pngFigureDocument(t), sidecar)
	if err != nil || len(outcome.Drafted) != 1 {
		t.Fatalf("outcome %+v %v", outcome, err)
	}
	if entry := sidecar.Entries["plot"]; entry == nil || entry.RestedOn != RestedOCR || entry.Alt != "Labels seen." {
		t.Fatalf("entry %+v", entry)
	}
}

// bitmapPDF wraps a PNG picture as a PDF whose one page is that picture alone (as a scanned or
// exported raster figure is): one RGB image covering the page.
func bitmapPDF(t *testing.T, data []byte) []byte {
	t.Helper()
	picture, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	bounds := picture.Bounds()
	var pixels bytes.Buffer
	deflate := zlib.NewWriter(&pixels)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := picture.At(x, y).RGBA()
			if _, err := deflate.Write([]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := deflate.Close(); err != nil {
		t.Fatal(err)
	}
	width, height := bounds.Dx(), bounds.Dy()
	content := fmt.Sprintf("q %d 0 0 %d 0 0 cm /Im0 Do Q", width, height)
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Resources << /XObject << /Im0 5 0 R >> >> /Contents 4 0 R >>", width, height),
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\nstream\n%s\nendstream",
			width, height, pixels.Len(), pixels.String())}
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	offsets := []int{}
	for index, object := range objects {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	start := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, start)
	return out.Bytes()
}

// A PDF figure that is one bitmap is read by OCR like a PNG figure (decision 133): its geometry is
// the text in the picture.
func TestScanFigureReadsTheTextOfABitmapPDF(t *testing.T) {
	if command.Resolve("tesseract") == "" || command.Resolve("pdftocairo") == "" || command.Resolve("mutool") == "" {
		t.Skip("tesseract, pdftocairo or mutool missing")
	}
	picture, _ := figdesc.Figures(pngFigureDocument(t))[0].Image(pngFigureDocument(t))
	original := mediamodel.Encoded(bitmapPDF(t, picture), "application/pdf")
	sha := original["sha256"].(string)
	asset := rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": sha, "representations": rec.Map{"original": original,
		"html": mediamodel.Encoded(picture, "image/png")}}
	document := rec.Map{"language": rec.Map{"tag": "en"}, "assets": rec.Map{"items": rec.Map{sha: asset}},
		"block_order": []any{"p1"},
		"nodes": rec.Map{"p1": rec.Map{"id": "p1", "type": "paragraph", "content": []any{
			rec.Map{"type": "image", "id": "i1", "asset": sha, "source": "figs/plot.pdf"}}}}}
	scanned, err := ScanFigure(document, figdesc.Figures(document)[0], t.TempDir())
	if err != nil || scanned == nil {
		t.Fatalf("scan %v %v", scanned, err)
	}
	text := scanned.Text()
	for _, want := range []string{"source ocr", "in the PDF's one image", `read by OCR "300"`} {
		if !strings.Contains(text, want) {
			t.Errorf("geometry lacks %q:\n%s", want, text)
		}
	}
}
