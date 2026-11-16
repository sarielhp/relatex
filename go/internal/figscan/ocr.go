package figscan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/command"
)

// SourceOCR is a picture (a PNG or JPEG figure) whose only geometry is the text OCR read in it
// (decision 123, amendment 4): its labels, with no shapes.
const SourceOCR = "ocr"

// ocrNote marks a label OCR read in the text form; ocrVerticalNote one it read in the picture
// turned a quarter, text set vertically (a vertical axis title, decisions 133 and 138).
const (
	ocrNote         = "read by OCR"
	ocrVerticalNote = "read by OCR, set vertically"
)

// turnedPass offsets the block numbers of the second pass (twice it, the third), so their lines
// never join the first pass's or each other.
const turnedPass = 1 << 20

// OCR limits: tesseract's sparse-text mode (as the figure-type survey ran it), words it is at least
// this confident of, pictures narrower than ocrMinWidth read at twice their size, and one run's time.
const (
	ocrConfidence = 60
	ocrMinWidth   = 1200
	ocrTimeout    = 90 * time.Second
)

// ocrWord is one word of tesseract's TSV: its line (block, paragraph, line), text, pixel box and
// confidence.
type ocrWord struct {
	line       [3]int
	text       string
	box        [4]int // left, top, width, height in pixels, y down
	confidence float64
	vertical   bool // read in the picture turned a quarter: the text runs bottom to top or top to bottom
}

// ScanPicture reads the words tesseract finds in a picture as a figure of labels only (Source
// "ocr"), each line of words one label at its centre, its text marked as read by OCR. Without
// tesseract, or when it reads no word, it returns nil and no error: the figure has no geometry.
func ScanPicture(picture image.Image) (*Figure, error) {
	tool := command.Resolve("tesseract")
	if tool == "" {
		return nil, nil
	}
	factor := 1
	if picture.Bounds().Dx() < ocrMinWidth {
		picture, factor = doubled(picture), 2
	}
	tsv, err := runTesseract(tool, picture)
	if err != nil {
		return nil, err
	}
	words := parseTSV(tsv)
	// A second pass on the picture turned a quarter clockwise reads text set bottom to top (a
	// vertical axis title), which the first pass reads as stray letters or not at all; a third on
	// the picture turned a quarter counterclockwise reads text set top to bottom (a right-hand axis
	// title, a turned table heading; decision 138).
	bounds := picture.Bounds()
	turnedTSV, err := runTesseract(tool, turnedClockwise(picture))
	if err != nil {
		return nil, err
	}
	words = mergeVertical(words, parseTSV(turnedTSV), fromClockwise(bounds.Dy()), turnedPass)
	turnedTSV, err = runTesseract(tool, turnedCounterclockwise(picture))
	if err != nil {
		return nil, err
	}
	words = mergeVertical(words, parseTSV(turnedTSV), fromCounterclockwise(bounds.Dx()), 2*turnedPass)
	if len(words) == 0 {
		return nil, nil
	}
	figure := ocrFigure(words, bounds.Dx(), bounds.Dy())
	figure.Size = [2]float64{float64(bounds.Dx() / factor), float64(bounds.Dy() / factor)}
	return figure, nil
}

// runTesseract runs tesseract's sparse-text mode on picture and returns its TSV.
func runTesseract(tool string, picture image.Image) ([]byte, error) {
	dir := command.Scratch("figscan-ocr-")
	defer os.RemoveAll(dir)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "picture.png")
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), ocrTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, path, "stdout", "--psm", "11", "tsv")
	cmd.Env = command.Environment(nil)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tesseract: %v: %s", err, firstLine(stderr.String()))
	}
	if out.Len() == 0 {
		return nil, errors.New("tesseract: no output")
	}
	return out.Bytes(), nil
}

// turnedClockwise is the picture turned a quarter clockwise: pixel (x, y) of a picture h pixels
// high goes to (h-1-y, x).
func turnedClockwise(picture image.Image) image.Image {
	bounds := picture.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	result := image.NewRGBA(image.Rect(0, 0, height, width))
	for y := range height {
		for x := range width {
			result.Set(height-1-y, x, picture.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return result
}

// turnedCounterclockwise is the picture turned a quarter counterclockwise: pixel (x, y) of a
// picture w pixels wide goes to (y, w-1-x).
func turnedCounterclockwise(picture image.Image) image.Image {
	bounds := picture.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	result := image.NewRGBA(image.Rect(0, 0, height, width))
	for y := range height {
		for x := range width {
			result.Set(y, width-1-x, picture.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return result
}

// fromClockwise maps a word box (left, top, width, height) of the picture turned clockwise back
// to the picture, height pixels high: x from its top, y from its left.
func fromClockwise(height int) func([4]int) [4]int {
	return func(box [4]int) [4]int {
		left, top, wide, high := box[0], box[1], box[2], box[3]
		return [4]int{top, height - left - wide, high, wide}
	}
}

// fromCounterclockwise maps a word box of the picture turned counterclockwise back to the
// picture, width pixels wide: x from its bottom, y from its left.
func fromCounterclockwise(width int) func([4]int) [4]int {
	return func(box [4]int) [4]int {
		left, top, wide, high := box[0], box[1], box[2], box[3]
		return [4]int{width - top - high, left, high, wide}
	}
}

// mergeVertical adds to the words read so far those of a turned pass that read vertical text in
// a place nothing was read: each turned word's box is mapped back to the picture (back), and a
// word is kept only when it is at least as tall as it is wide there, overlaps no word already
// read, and its line holds a word (wordLike) rather than only the stray pairs of letters OCR makes
// of turned curves and ticks. Its line keeps apart from the other passes' (pass offsets its block).
func mergeVertical(words, turned []ocrWord, back func([4]int) [4]int, pass int) []ocrWord {
	read := slices.Clone(words)
	kept := []ocrWord{}
	lines := map[[3]int]bool{}
	for _, word := range turned {
		word.box = back(word.box)
		if word.box[3] < word.box[2] || overlapsAny(word.box, read) {
			continue
		}
		word.line[0] += pass
		word.vertical = true
		kept = append(kept, word)
		lines[word.line] = lines[word.line] || wordLike(word.text)
	}
	for _, word := range kept {
		if lines[word.line] {
			words = append(words, word)
		}
	}
	return words
}

// wordLike is a word of at least three characters, two of them letters ("Count", "[pA]"), not the
// "oa", "Nn" or "©" OCR reads in a turned plot's curves.
func wordLike(text string) bool {
	letters := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return utf8.RuneCountInString(text) >= 3 && letters >= 2
}

// overlapsAny says whether box overlaps the box of any word.
func overlapsAny(box [4]int, words []ocrWord) bool {
	for _, word := range words {
		other := word.box
		if box[0] < other[0]+other[2] && other[0] < box[0]+box[2] && box[1] < other[1]+other[3] && other[1] < box[1]+box[3] {
			return true
		}
	}
	return false
}

// doubled is a picture at twice its size, each pixel a 2 × 2 block.
func doubled(picture image.Image) image.Image {
	bounds := picture.Bounds()
	source := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(source, source.Bounds(), picture, bounds.Min, draw.Src)
	result := image.NewRGBA(image.Rect(0, 0, 2*bounds.Dx(), 2*bounds.Dy()))
	for y := range result.Bounds().Dy() {
		for x := range result.Bounds().Dx() {
			result.Set(x, y, source.At(x/2, y/2))
		}
	}
	return result
}

// parseTSV reads the words (level 5) of tesseract's TSV that it is confident of and that have text.
func parseTSV(tsv []byte) []ocrWord {
	words := []ocrWord{}
	for _, row := range strings.Split(string(tsv), "\n") {
		fields := strings.Split(strings.TrimRight(row, "\r"), "\t")
		if len(fields) < 12 || fields[0] != "5" {
			continue
		}
		numbers := make([]int, 10)
		for index := range numbers {
			numbers[index], _ = strconv.Atoi(fields[index])
		}
		confidence, err := strconv.ParseFloat(fields[10], 64)
		text := strings.TrimSpace(fields[11])
		if err != nil || confidence < ocrConfidence || text == "" {
			continue
		}
		words = append(words, ocrWord{line: [3]int{numbers[2], numbers[3], numbers[4]}, text: text,
			box: [4]int{numbers[6], numbers[7], numbers[8], numbers[9]}, confidence: confidence})
	}
	return words
}

// ocrFigure is the words of a width × height picture as a figure of labels: the words of one line,
// in order, joined into one label at the centre of their box, positions normalized as figscan's
// (the longer side 0–100, y up).
func ocrFigure(words []ocrWord, width, height int) *Figure {
	box := Box{X1: float64(width), Y1: float64(height)}
	figure := &Figure{Source: SourceOCR, Page: 1, box: box,
		Detail: fmt.Sprintf("tesseract OCR, words of confidence %d or more", ocrConfidence)}
	scale := box.scale()
	for _, line := range ocrLines(words) {
		left, top, right, bottom := line.box[0], line.box[1], line.box[2], line.box[3]
		// Twice the centre in whole pixels, y up, then halved: no float product feeds a sum.
		x := float64(left+right) / 2
		y := float64(2*height-top-bottom) / 2
		point := roundPoint(Point{float64(x * scale), float64(y * scale)})
		note := ocrNote
		if line.vertical {
			note = ocrVerticalNote
		}
		figure.Nodes = append(figure.Nodes, &Node{Kind: KindText, Text: line.text, PrintedNote: note, Point: &point})
	}
	figure.Extent = [2]float64{round1(float64(width) * scale), round1(float64(height) * scale)}
	finish(figure)
	if figure.Nodes == nil {
		figure.Nodes = []*Node{}
	}
	return figure
}

// ocrLine is one line of words: their text and the box around them (left, top, right, bottom).
type ocrLine struct {
	text string
	box  [4]int
	// vertical: the line was read in the picture turned a quarter (text set bottom to top or top to bottom).
	vertical bool
}

// ocrLines joins the words of each line, lines in the order tesseract read them.
func ocrLines(words []ocrWord) []ocrLine {
	lines := []ocrLine{}
	index := map[[3]int]int{}
	for _, word := range words {
		right, bottom := word.box[0]+word.box[2], word.box[1]+word.box[3]
		at, seen := index[word.line]
		if !seen {
			index[word.line] = len(lines)
			lines = append(lines, ocrLine{text: word.text, box: [4]int{word.box[0], word.box[1], right, bottom}, vertical: word.vertical})
			continue
		}
		line := &lines[at]
		line.text += " " + word.text
		line.box = [4]int{min(line.box[0], word.box[0]), min(line.box[1], word.box[1]), max(line.box[2], right), max(line.box[3], bottom)}
	}
	return slices.Clip(lines)
}
