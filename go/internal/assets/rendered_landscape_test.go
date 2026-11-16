package assets

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
)

// A crop of a landscape page keeps its region's width and height (second cohort 0469, slides
// 0027): pdftocairo turns the paper for a page wider than tall, which made the crop tall and
// narrow and cut the picture.
func TestCropOfALandscapePageKeepsItsSize(t *testing.T) {
	if command.Resolve("pdftocairo") == "" || command.Resolve("qpdf") == "" || command.Resolve("pdfinfo") == "" {
		t.Skip("pdftocairo, qpdf or pdfinfo not installed")
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "page.pdf")
	page := pdfOf([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 400] /Contents 4 0 R >>",
		"<< /Length 22 >>\nstream\n0 0 600 400 re f\n\nendstream",
	})
	if err := os.WriteFile(path, page, 0o600); err != nil {
		t.Fatal(err)
	}
	sizes := pageSizes(path, []int64{1})
	if !sizes[1].landscape() {
		t.Fatalf("page sizes %v, want a landscape page 1", sizes)
	}
	data := crop(path, rec.Map{"page": int64(1), "id": "r1"}, [4]int64{10, 20, 300, 100}, sizes[1].landscape(), directory)
	output := filepath.Join(directory, "out.pdf")
	if err := os.WriteFile(output, data, 0o600); err != nil {
		t.Fatal(err)
	}
	info := command.Capture(command.Resolve("pdfinfo"), output)
	if !regexp.MustCompile(`Page size:\s+300 x 100 pts`).MatchString(info) {
		t.Fatalf("crop page size, want 300 x 100: %s", info)
	}
}
