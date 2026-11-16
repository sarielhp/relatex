package assets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/command"
)

// A drawn picture's region (sp from the page's lower-left corner) is its box in bp from the
// top-left corner, as the glyph owners' positions are.
func TestPictureBoxIsTopLeftBigPoints(t *testing.T) {
	region := []any{int64(100 * spPerBP), int64(200 * spPerBP), int64(300 * spPerBP), 250.5 * spPerBP}
	if got := pictureBox(region, 792); got != [4]float64{100, 541.5, 300, 592} {
		t.Fatalf("box %v", got)
	}
}

// duplicateKeyPDF is a one-page PDF whose resources name one XObject twice, as pdftocairo
// writes them ("/x7 7 0 R /x7 7 0 R").
func duplicateKeyPDF() []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources 4 0 R /Contents 5 0 R >>",
		"<< /XObject << /x7 6 0 R /x7 6 0 R >> >>",
		"<< /Length 8 >>\nstream\n/x7 Do \nendstream",
		"<< /Type /XObject /Subtype /Form /BBox [0 0 1 1] /Length 0 >>\nstream\n\nendstream",
	}
	return pdfOf(objects)
}

// pdfOf is a PDF of the numbered objects, the first its catalog.
func pdfOf(objects []string) []byte {
	var body strings.Builder
	body.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects))
	for index, object := range objects {
		offsets[index] = body.Len()
		fmt.Fprintf(&body, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	start := body.Len()
	fmt.Fprintf(&body, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&body, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&body, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, start)
	return []byte(body.String())
}

// A Cairo crop with a duplicated resource key normalizes (fifth cohort 1355): qpdf exits 3
// with only that warning and writes the dictionary once.
func TestNormalizedCropAcceptsDuplicatedKeys(t *testing.T) {
	if command.Resolve("qpdf") == "" {
		t.Skip("qpdf not installed")
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "r1.pdf")
	if err := os.WriteFile(path, duplicateKeyPDF(), 0o600); err != nil {
		t.Fatal(err)
	}
	output := normalizedCrop(path, directory, "r1")
	if info, err := os.Stat(output); err != nil || info.Size() == 0 {
		t.Fatalf("no normalized crop: %v", err)
	}
	if log, _ := os.ReadFile(filepath.Join(directory, "r1.qpdf.log")); !strings.Contains(string(log), "duplicated key /x7") {
		t.Fatalf("qpdf did not report the duplicated key: %q", log)
	}
}

func TestOnlyDuplicateKeyWarnings(t *testing.T) {
	duplicate := "WARNING: r1.pdf (object 3 0, offset 388): dictionary has duplicated key /x7; " +
		"last occurrence overrides earlier ones\nqpdf: operation succeeded with warnings\n"
	other := duplicate + "WARNING: r1.pdf: file is damaged\n"
	// A Windows qpdf writes CRLF line ends.
	crlf := strings.ReplaceAll(duplicate, "\n", "\r\n")
	cases := map[string]bool{duplicate: true, crlf: true, other: false, "qpdf: operation succeeded with warnings\n": false}
	for text, want := range cases {
		if got := onlyDuplicateKeyWarnings(text); got != want {
			t.Errorf("onlyDuplicateKeyWarnings(%q) = %v, want %v", text, got, want)
		}
	}
}
