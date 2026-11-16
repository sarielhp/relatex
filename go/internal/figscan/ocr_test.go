package figscan

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/command"
)

// ocrTSV is tesseract's TSV for a 400 × 200 picture: two words on one line, one word on another,
// one word below the confidence floor, and the page/block rows that are not words.
const ocrTSV = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
	"1\t1\t0\t0\t0\t0\t0\t0\t400\t200\t-1\t\n" +
	"2\t1\t1\t0\t0\t0\t20\t10\t200\t30\t-1\t\n" +
	"5\t1\t1\t1\t1\t1\t20\t10\t80\t30\t96.5\tTime\n" +
	"5\t1\t1\t1\t1\t2\t110\t12\t60\t28\t91.0\t(s)\n" +
	"5\t1\t2\t1\t1\t1\t300\t150\t40\t20\t88.2\tLoad\n" +
	"5\t1\t3\t1\t1\t1\t200\t100\t10\t10\t31.0\t~\n" +
	"5\t1\t4\t1\t1\t1\t250\t60\t10\t10\t95.0\t \n"

func TestParseTSVKeepsConfidentWords(t *testing.T) {
	words := parseTSV([]byte(ocrTSV))
	texts := []string{}
	for _, word := range words {
		texts = append(texts, word.text)
	}
	if strings.Join(texts, "|") != "Time|(s)|Load" {
		t.Fatalf("words %q", texts)
	}
	if words[1].box != [4]int{110, 12, 60, 28} || words[1].line != [3]int{1, 1, 1} {
		t.Errorf("second word %+v", words[1])
	}
}

func TestOCRFigureJoinsLinesIntoLabels(t *testing.T) {
	figure := ocrFigure(parseTSV([]byte(ocrTSV)), 400, 200)
	if figure.Source != SourceOCR || figure.Extent != [2]float64{100, 50} {
		t.Fatalf("source %q extent %v", figure.Source, figure.Extent)
	}
	if len(figure.Labels) != 2 {
		t.Fatalf("labels %+v", figure.Labels)
	}
	// "Time (s)" spans x 20-170, y 10-40 (down): centre (95, 25), y up 175; scale 100/400.
	first := figure.Labels[0]
	if first.Text != "Time (s)" || first.Point != (Point{23.8, 43.8}) {
		t.Errorf("first label %+v", first)
	}
	if second := figure.Labels[1]; second.Text != "Load" || second.Point != (Point{80, 10}) {
		t.Errorf("second label %+v", second)
	}
	text := figure.Text()
	for _, want := range []string{"source ocr", "\npicture\n", `read by OCR "Time (s)"`, `read by OCR "Load"`,
		"shapes: 0 of 0 shown; labels: 2", "shapes: none read (OCR reads only the text)\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("text form lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "PDF page") || strings.Contains(text, "label read by OCR") {
		t.Errorf("text form names a page or lists the labels as shapes:\n%s", text)
	}
}

func TestOCRBoxIsInPixels(t *testing.T) {
	figure := ocrFigure(parseTSV([]byte(ocrTSV)), 400, 200)
	figure.Size = [2]float64{400, 200}
	if !strings.Contains(figure.Text(), "box 400 x 200 px;") {
		t.Errorf("box line:\n%s", figure.Text())
	}
}

func TestOCRFigureTurnsWithThePaper(t *testing.T) {
	figure := ocrFigure(parseTSV([]byte(ocrTSV)), 400, 200)
	figure.Size = [2]float64{400, 200}
	figure.Turn(3)
	if figure.Extent != [2]float64{50, 100} || figure.Size != [2]float64{200, 400} {
		t.Fatalf("extent %v size %v", figure.Extent, figure.Size)
	}
	if figure.Labels[1].Point != (Point{10, 20}) {
		t.Errorf("turned label %+v", figure.Labels[1])
	}
}

func TestDoubledRepeatsEachPixel(t *testing.T) {
	picture := image.NewGray(image.Rect(0, 0, 2, 1))
	picture.Pix[1] = 200
	twice := doubled(picture)
	if twice.Bounds().Dx() != 4 || twice.Bounds().Dy() != 2 {
		t.Fatalf("bounds %v", twice.Bounds())
	}
	if r, _, _, _ := twice.At(3, 1).RGBA(); r>>8 != 200 {
		t.Errorf("pixel (3, 1) %d", r>>8)
	}
	if r, _, _, _ := twice.At(1, 1).RGBA(); r>>8 != 0 {
		t.Errorf("pixel (1, 1) %d", r>>8)
	}
}

// TestScanPictureReadsTheLabelsOfARenderedFigure renders a gnuplot figure (tick numbers and an
// axis title) to a PNG and reads it back by OCR.
func TestScanPictureReadsTheLabelsOfARenderedFigure(t *testing.T) {
	if command.Resolve("tesseract") == "" || command.Resolve("pdftocairo") == "" {
		t.Skip("tesseract or pdftocairo missing")
	}
	if _, err := os.Stat(filepath.Join("testdata", "gnuplot-Fig3.pdf")); err != nil {
		t.Skip("the figure fixtures are not in this tree (they are not published)")
	}
	out := filepath.Join(t.TempDir(), "figure")
	render := exec.Command(command.Resolve("pdftocairo"), "-png", "-r", "150", "-singlefile",
		filepath.Join("testdata", "gnuplot-Fig3.pdf"), out)
	if data, err := render.CombinedOutput(); err != nil {
		t.Fatalf("pdftocairo: %v %s", err, data)
	}
	data, err := os.ReadFile(out + ".png")
	if err != nil {
		t.Fatal(err)
	}
	picture, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	figure, err := ScanPicture(picture)
	if err != nil || figure == nil {
		t.Fatalf("figure %v, error %v", figure, err)
	}
	bounds := picture.Bounds()
	if figure.Source != SourceOCR || figure.Size != [2]float64{float64(bounds.Dx()), float64(bounds.Dy())} {
		t.Fatalf("source %q size %v, picture %v", figure.Source, figure.Size, bounds)
	}
	found := map[string]bool{}
	for _, label := range figure.Labels {
		found[label.Text] = true
		if label.Point[0] < 0 || label.Point[0] > figure.Extent[0] || label.Point[1] < 0 || label.Point[1] > figure.Extent[1] {
			t.Errorf("label %q at %v outside %v", label.Text, label.Point, figure.Extent)
		}
	}
	for _, want := range []string{"300", "250", "200"} {
		if !found[want] {
			t.Errorf("OCR missed %q; read %v", want, found)
		}
	}
	// The vertical axis title is read by the turned pass (decision 133).
	vertical := map[string]bool{}
	for _, label := range figure.Labels {
		if label.Note == ocrVerticalNote {
			vertical[label.Text] = true
		}
	}
	if !vertical["Dipole Strength"] {
		t.Errorf("the turned pass missed the axis title; vertical labels %v", vertical)
	}
}

// The turned pass's words come back to the picture's pixels; only vertical words where the first
// pass read nothing are added, on lines of their own, marked as set vertically (decision 133).
func TestMergeVerticalAddsOnlyNewVerticalWords(t *testing.T) {
	// A 400 × 200 picture: the first pass read "Time" at x 20-100, y 10-40.
	first := []ocrWord{{line: [3]int{1, 1, 1}, text: "Time", box: [4]int{20, 10, 80, 30}}}
	turned := []ocrWord{
		// Turned picture (200 × 400): "Count" at x 50-150, y 5-25 maps to x 5-25, y 50-150: vertical, new.
		{line: [3]int{1, 1, 1}, text: "Count", box: [4]int{50, 5, 100, 20}},
		// "Ti" at x 170-190, y 20-50 maps to x 20-50, y 10-30: overlaps "Time", already read.
		{line: [3]int{1, 1, 2}, text: "Ti", box: [4]int{170, 20, 20, 30}},
		// "wide" at x 10-20, y 200-260 maps to x 200-260, y 180-190: wider than tall, not vertical text.
		{line: [3]int{2, 1, 1}, text: "wide", box: [4]int{10, 200, 10, 60}},
	}
	// "Nn" at x 100-130, y 300-320 maps to x 300-320, y 70-100: vertical and new, but alone on its
	// line and no word: OCR's reading of a turned curve.
	turned = append(turned, ocrWord{line: [3]int{3, 1, 1}, text: "Nn", box: [4]int{100, 300, 30, 20}})
	merged := mergeVertical(first, turned, fromClockwise(200), turnedPass)
	if len(merged) != 2 || merged[1].text != "Count" {
		t.Fatalf("merged %+v", merged)
	}
	if count := merged[1]; count.box != [4]int{5, 50, 20, 100} || !count.vertical || count.line[0] != 1+turnedPass {
		t.Errorf("vertical word %+v", count)
	}
	figure := ocrFigure(merged, 400, 200)
	if text := figure.Text(); !strings.Contains(text, `read by OCR, set vertically "Count"`) || !strings.Contains(text, `read by OCR "Time"`) {
		t.Errorf("text form:\n%s", text)
	}
}

// The counterclockwise pass reads text set top to bottom: its boxes come back from the right
// edge, and a word the clockwise pass already placed there is not added twice (decision 138).
func TestMergeVerticalReadsTextSetTopToBottom(t *testing.T) {
	// A 400 × 200 picture turned counterclockwise is 200 × 400: "Load" at x 50-150, y 10-30 there
	// maps to x 400-30 … 400-10 = 370-390, y 50-150: a word set top to bottom at the right edge.
	turned := []ocrWord{{line: [3]int{1, 1, 1}, text: "Load", box: [4]int{50, 10, 100, 20}}}
	merged := mergeVertical(nil, turned, fromCounterclockwise(400), 2*turnedPass)
	if len(merged) != 1 || merged[0].box != [4]int{370, 50, 20, 100} || !merged[0].vertical || merged[0].line[0] != 1+2*turnedPass {
		t.Fatalf("merged %+v", merged)
	}
	if again := mergeVertical(merged, turned, fromCounterclockwise(400), 2*turnedPass); len(again) != 1 {
		t.Errorf("a word read already was added again: %+v", again)
	}
}

// labelsPDF is a one-page PDF (600 × 400 pt) printing Helvetica text: a horizontal axis title,
// one set bottom to top on the left and one set top to bottom on the right.
func labelsPDF() []byte {
	content := "BT /F1 26 Tf 1 0 0 1 170 30 Tm (Time in seconds) Tj ET\n" +
		"BT /F1 26 Tf 0 1 -1 0 50 90 Tm (Voltage level) Tj ET\n" +
		"BT /F1 26 Tf 0 -1 1 0 560 330 Tm (Photon counts) Tj ET\n"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 400] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		"<< /Length " + strconv.Itoa(len(content)) + " >>\nstream\n" + content + "endstream",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
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

// A title set top to bottom (a right-hand axis) is read by the counterclockwise pass, beside the
// horizontal title and the one set bottom to top, each once (decision 138).
func TestScanPictureReadsTextSetTopToBottom(t *testing.T) {
	if command.Resolve("tesseract") == "" || command.Resolve("pdftocairo") == "" {
		t.Skip("tesseract or pdftocairo missing")
	}
	directory := t.TempDir()
	path, out := filepath.Join(directory, "labels.pdf"), filepath.Join(directory, "labels")
	if err := os.WriteFile(path, labelsPDF(), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := exec.Command(command.Resolve("pdftocairo"), "-png", "-r", "100", "-singlefile", path, out).CombinedOutput(); err != nil {
		t.Fatalf("pdftocairo: %v %s", err, data)
	}
	data, err := os.ReadFile(out + ".png")
	if err != nil {
		t.Fatal(err)
	}
	picture, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	figure, err := ScanPicture(picture)
	if err != nil || figure == nil {
		t.Fatalf("figure %v, error %v", figure, err)
	}
	read := map[string]Label{}
	for _, label := range figure.Labels {
		read[label.Text] = label
	}
	horizontal, up, down := read["Time in seconds"], read["Voltage level"], read["Photon counts"]
	if horizontal.Note != ocrNote || up.Note != ocrVerticalNote || down.Note != ocrVerticalNote {
		t.Fatalf("labels %+v", figure.Labels)
	}
	if up.Point[0] > figure.Extent[0]/2 || down.Point[0] < figure.Extent[0]/2 {
		t.Errorf("bottom to top at %v (left), top to bottom at %v (right) of %v", up.Point, down.Point, figure.Extent)
	}
	if len(figure.Labels) != 3 {
		t.Errorf("each title once, nothing else: %+v", figure.Labels)
	}
}

func TestTurnedCounterclockwiseMovesEachPixel(t *testing.T) {
	picture := image.NewGray(image.Rect(0, 0, 3, 2))
	picture.Pix[0*3+2] = 200 // (2, 0): top right goes to top left of the turned 2 × 3 picture
	turned := turnedCounterclockwise(picture)
	if turned.Bounds().Dx() != 2 || turned.Bounds().Dy() != 3 {
		t.Fatalf("bounds %v", turned.Bounds())
	}
	if r, _, _, _ := turned.At(0, 0).RGBA(); r>>8 != 200 {
		t.Errorf("pixel (0, 0) %d", r>>8)
	}
}

func TestTurnedClockwiseMovesEachPixel(t *testing.T) {
	picture := image.NewGray(image.Rect(0, 0, 3, 2))
	picture.Pix[0*3+2] = 200 // (2, 0): top right goes to bottom right of the turned 2 × 3 picture
	turned := turnedClockwise(picture)
	if turned.Bounds().Dx() != 2 || turned.Bounds().Dy() != 3 {
		t.Fatalf("bounds %v", turned.Bounds())
	}
	if r, _, _, _ := turned.At(1, 2).RGBA(); r>>8 != 200 {
		t.Errorf("pixel (1, 2) %d", r>>8)
	}
}

func TestScanPictureOfABlankPictureHasNoGeometry(t *testing.T) {
	if command.Resolve("tesseract") == "" {
		t.Skip("tesseract missing")
	}
	picture := image.NewGray(image.Rect(0, 0, 300, 200))
	for index := range picture.Pix {
		picture.Pix[index] = 255
	}
	figure, err := ScanPicture(picture)
	if err != nil || figure != nil {
		t.Errorf("figure %+v, error %v", figure, err)
	}
}

// TestRunTesseractUsesRunScratch verifies runTesseract writes its temporary PNG under
// RELATEX_RUNS_DIR via command.Scratch and removes it on return (decision 138 E).
func TestRunTesseractUsesRunScratch(t *testing.T) {
	base := t.TempDir()
	t.Setenv("RELATEX_RUNS_DIR", base)
	seen := filepath.Join(t.TempDir(), "seen")
	tool := stubTesseract(t, filepath.Join(t.TempDir(), "tesseract"), seen)
	picture := image.NewGray(image.Rect(0, 0, 8, 8))
	if _, err := runTesseract(tool, picture); err != nil {
		t.Fatalf("runTesseract: %v", err)
	}
	recorded, err := os.ReadFile(seen)
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimSpace(string(recorded))
	resolved, _ := filepath.EvalSymlinks(base)
	if !strings.HasPrefix(path, base) && !strings.HasPrefix(path, resolved) {
		t.Fatalf("runTesseract wrote %s outside RELATEX_RUNS_DIR %s", path, base)
	}
	if left, _ := filepath.Glob(filepath.Join(base, "figscan-ocr-*")); len(left) != 0 {
		t.Fatalf("runTesseract left temporary directories: %v", left)
	}
}

func stubTesseract(t *testing.T, basePath, seen string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := basePath + ".bat"
		body := "@echo off\r\necho %~1> \"" + seen + "\"\r\necho level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\r\n"
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	body := "#!/bin/sh\nprintf '%s\\n' \"$1\" > \"" + seen + "\"\nprintf 'level\\tpage_num\\tblock_num\\tpar_num\\tline_num\\tword_num\\tleft\\ttop\\twidth\\theight\\tconf\\ttext\\n'\n"
	if err := os.WriteFile(basePath, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return basePath
}
