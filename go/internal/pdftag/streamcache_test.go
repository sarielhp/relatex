package pdftag

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The stream cache must read every non-image stream once: a form XObject painted at many CTMs is
// one qpdf run, and an image XObject is never kept: the 0950 PDF holds gigabytes of image data
// (a11y-108).

// xobjectRefs finds the form and image XObject references (qpdf renumbers the fixture's objects
// when it generates object streams).
func xobjectRefs(t *testing.T, file *pdfFile) (form, image string) {
	t.Helper()
	for key, value := range file.objects {
		entry, _ := value.(map[string]any)
		stream, _ := entry["stream"].(map[string]any)
		dict, _ := stream["dict"].(map[string]any)
		ref, _ := strings.CutPrefix(key, "obj:")
		switch dict["/Subtype"] {
		case "/Form":
			form = ref
		case "/Image":
			image = ref
		}
	}
	if form == "" || image == "" {
		t.Fatalf("fixture has no form %q or image %q XObject", form, image)
	}
	return form, image
}

// objectArg is a reference as --json-object takes it ("N 0 R" as "N,0").
func objectArg(ref string) string {
	fields := strings.Fields(ref)
	return strings.Join(fields[:2], ",")
}

// objectRuns counts the qpdf runs that fetched one object's stream data.
func objectRuns(t *testing.T, calls, object string) int {
	t.Helper()
	data, err := os.ReadFile(calls)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return strings.Count(string(data), "--json-object="+objectArg(object))
}

// writeStreamFixture writes a one-page PDF with a form XObject painted at three CTMs and an
// image XObject, rewritten by qpdf so its cross-reference table is sound.
func writeStreamFixture(t *testing.T, qpdf, directory string) string {
	t.Helper()
	content := "q 1 0 0 1 10 10 cm /Fm1 Do Q q 2 0 0 2 40 40 cm /Fm1 Do Q q 1 0 0 1 80 80 cm /Fm1 Do Q"
	form := "BT /F1 5 Tf (a) Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R " +
			"/Resources << /XObject << /Fm1 5 0 R /Im1 6 0 R >> /Font << /F1 7 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length %d "+
			"/Resources << /Font << /F1 7 0 R >> >> >>\nstream\n%s\nendstream", len(form), form),
		"<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray " +
			"/BitsPerComponent 8 /Length 1 >>\nstream\n\x00\nendstream",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var out strings.Builder
	out.WriteString("%PDF-2.0\n")
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
	raw, pdf := filepath.Join(directory, "raw.pdf"), filepath.Join(directory, "paper.pdf")
	if err := os.WriteFile(raw, []byte(out.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if message, err := runQPDF(qpdf, "--object-streams=generate", raw, pdf); err != nil {
		t.Fatalf("qpdf: %v: %s", err, message)
	}
	return pdf
}

// TestStreamCacheReadsAFormOnce interprets a page whose form is painted at three CTMs: the form's
// stream data is fetched once, and the glyphs are as many as the three visits paint.
func TestStreamCacheReadsAFormOnce(t *testing.T) {
	qpdf, err := exec.LookPath("qpdf")
	if err != nil {
		t.Skip("qpdf not installed")
	}
	directory := t.TempDir()
	pdf := writeStreamFixture(t, qpdf, directory)
	wrapper, calls := countingWrapper(t, qpdf)
	file, err := readPDF(wrapper, pdf)
	if err != nil {
		t.Fatal(err)
	}
	form, _ := xobjectRefs(t, file)
	pages := file.pages()
	if len(pages) != 1 {
		t.Fatalf("%d pages, want 1", len(pages))
	}
	data, err := file.contentData(pages[0])
	if err != nil {
		t.Fatal(err)
	}
	operations, err := parseContent(data)
	if err != nil {
		t.Fatal(err)
	}
	in := newInterpreter(file, pages[0].resources)
	items, err := in.run(operations)
	if err != nil {
		t.Fatal(err)
	}
	painted, objects := 0, 0
	for _, entry := range items {
		if entry.kind == kindObject {
			objects++
			painted += entry.count
		}
	}
	if objects != 3 || painted != 3 || in.glyphs != 3 {
		t.Fatalf("%d form Do's painting %d glyphs (%d counted)", objects, painted, in.glyphs)
	}
	if runs := objectRuns(t, calls, form); runs != 1 {
		t.Fatalf("form XObject fetched in %d qpdf runs, want 1", runs)
	}
	content, _ := pages[0].dict["/Contents"].(string)
	if runs := objectRuns(t, calls, content); runs != 1 {
		t.Fatalf("content stream fetched in %d qpdf runs, want 1", runs)
	}
	if _, err := file.contentData(pages[0]); err != nil {
		t.Fatal(err)
	}
	if runs := objectRuns(t, calls, content); runs != 1 {
		t.Fatalf("second content read ran qpdf again (%d runs)", runs)
	}
}

// TestStreamCacheSkipsImages reads an image XObject twice: it is not cached, so both reads run
// qpdf rather than keeping its data (the 0950 memory case).
func TestStreamCacheSkipsImages(t *testing.T) {
	qpdf, err := exec.LookPath("qpdf")
	if err != nil {
		t.Skip("qpdf not installed")
	}
	directory := t.TempDir()
	pdf := writeStreamFixture(t, qpdf, directory)
	wrapper, calls := countingWrapper(t, qpdf)
	file, err := readPDF(wrapper, pdf)
	if err != nil {
		t.Fatal(err)
	}
	_, image := xobjectRefs(t, file)
	for range 2 {
		if _, err := file.streamData(image); err != nil {
			t.Fatal(err)
		}
	}
	if runs := objectRuns(t, calls, image); runs != 2 {
		t.Fatalf("image XObject fetched in %d qpdf runs, want 2 (not cached)", runs)
	}
}
