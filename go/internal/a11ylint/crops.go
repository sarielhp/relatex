package a11ylint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	runtimetools "github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/sourcemap"
)

// CropResolution is the resolution, in pixels per inch, of the review page's PDF crops.
const CropResolution = 110

// CropPadding is how far, in PDF points, a crop reaches past the item's words.
const CropPadding = 12.0

// MaxCrops bounds the crops one review page build renders; the rest are listed without one. A book has
// hundreds of items (the author's book: 457, about a minute per 200 crops), so the bound covers a book.
const MaxCrops = 1000

// cropLookahead is how many source lines below an item's line the search for its words may go
// (a \includegraphics line has no words; its caption begins on the next line or soon after).
const cropLookahead = 3

// cropRun is how many consecutive source lines of words one crop gathers once it finds them.
const cropRun = 8

// cropTimeout bounds one external crop program (decision 037: a page build must not hang).
const cropTimeout = 20 * time.Second

// Box is a rectangle in PDF points, top-left origin, x0 <= x1 and y0 <= y1.
type Box struct {
	MinX, MinY, MaxX, MaxY float64
}

// PixelRegion is the same rectangle in output pixels at CropResolution.
type PixelRegion struct {
	X, Y, Width, Height int
}

// Crop is the part of one PDF page a review item concerns: the item's index in the page's item
// list, its page number, its padded box in PDF points and the PNG file name it is written as.
type Crop struct {
	Item int
	Page int
	Box  Box
	PNG  string
	pdf  string // the export's ordinary PDF, the pages the reader sees
	// region is Box at CropResolution, the pixel rectangle pdftocairo crops.
	region PixelRegion
}

// Renderer draws one page region of a PDF to a PNG file.
type Renderer interface {
	Render(pdf, output string, page int, region PixelRegion) error
}

// Poppler is the pdftocairo renderer (poppler is an allowed external program).
type Poppler struct{}

// Render runs pdftocairo for one page and one region, without a shell, with a list of
// arguments, under a cropTimeout. The caller lowers this process's priority first, so the
// program inherits it (go/internal/cli's lowerPriority, as relatex synctex does).
func (Poppler) Render(pdf, output string, page int, region PixelRegion) error {
	ctx, cancel := context.WithTimeout(context.Background(), cropTimeout)
	defer cancel()
	// -singlefile without the extension writes exactly output's .png.
	base := strings.TrimSuffix(output, ".png")
	arguments := []string{"-png", "-r", strconv.Itoa(CropResolution),
		"-x", strconv.Itoa(region.X), "-y", strconv.Itoa(region.Y),
		"-W", strconv.Itoa(region.Width), "-H", strconv.Itoa(region.Height),
		"-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-singlefile", pdf, base}
	tool := runtimetools.Resolve("pdftocairo")
	if tool == "" {
		return errors.New("pdftocairo not found (RELATEX_PDFTOCAIRO, PATH)")
	}
	cmd := exec.CommandContext(ctx, tool, arguments...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("pdftocairo timed out after %s", cropTimeout)
		}
		return fmt.Errorf("pdftocairo: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// Crops is the part of the export's PDF each item concerns, from the conversion's source map
// (words with a page, a box in points and their source lines): at most MaxCrops, in item order,
// the items without a box left out. product is the conversion's product directory, holding the
// export's ordinary PDF.
func Crops(paper Paper, items []Item, product string) (crops []Crop, err error) {
	defer rec.Recover(&err)
	crops = cropBoxes(paper.SourceMap, items)
	if len(crops) == 0 {
		return nil, nil
	}
	pdf, err := exportPDF(product)
	if err != nil {
		return nil, err
	}
	for index := range crops {
		crops[index].pdf = pdf
	}
	return crops, nil
}

// RenderCrops writes each crop's PNG under destination with renderer.
func RenderCrops(crops []Crop, destination string, renderer Renderer) (int, error) {
	if len(crops) == 0 {
		return 0, nil
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return 0, err
	}
	for _, crop := range crops {
		output := filepath.Join(destination, crop.PNG)
		if err := renderer.Render(crop.pdf, output, crop.Page, crop.region); err != nil {
			return 0, fmt.Errorf("crop of page %d for item %d: %w", crop.Page, crop.Item+1, err)
		}
	}
	return len(crops), nil
}

// exportPDF is the export's ordinary PDF in the product directory: the one PDF the reader sees.
func exportPDF(product string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(product, "*.pdf"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no PDF in the product directory %s; the review page cannot crop the pages", product)
	}
	slices.Sort(matches)
	return matches[0], nil
}

// sourceWord is one word of the source map and where its source lines name it.
type wordRows map[string]map[int][]rec.Map

// lineIndex is the source map's words by file and source line, in reading order.
func lineIndex(m rec.Map) wordRows {
	index := wordRows{}
	for _, word := range sourcemap.Selection(m) {
		for _, value := range rec.FetchList(word, "origins") {
			origin := rec.AsMap(value)
			file := rec.AsString(rec.Fetch(origin, "file"))
			first := int(rec.FetchInt(origin, "line"))
			last := first
			if value, ok := origin["end_line"]; ok {
				last = int(rec.AsInt(value))
			}
			rows := index[file]
			if rows == nil {
				rows = map[int][]rec.Map{}
				index[file] = rows
			}
			for line := first; line <= last; line++ {
				rows[line] = append(rows[line], word)
			}
		}
	}
	return index
}

// pageSizes are the map's page dimensions in points by page number.
func pageSizes(m rec.Map) map[int]pageSize {
	sizes := map[int]pageSize{}
	for _, value := range rec.FetchList(m, "pages") {
		page := rec.AsMap(value)
		number := int(rec.FetchInt(page, "number"))
		width, _ := rec.Number(rec.Fetch(page, "width"))
		height, _ := rec.Number(rec.Fetch(page, "height"))
		sizes[number] = pageSize{width: width, height: height}
	}
	return sizes
}

type pageSize struct {
	width, height float64
}

// cropBoxes is the pure part of Crops: the items that have a box in the source map, padded and
// clamped to their page, named in order.
func cropBoxes(sourceMap rec.Map, items []Item) []Crop {
	if len(sourceMap) == 0 {
		return nil
	}
	rows, sizes := lineIndex(sourceMap), pageSizes(sourceMap)
	crops := []Crop{}
	for index, item := range items {
		if len(crops) == MaxCrops {
			break
		}
		crop, ok := itemCrop(index, item, rows, sizes)
		if !ok {
			continue
		}
		crop.PNG = fmt.Sprintf("crop-%03d.png", len(crops)+1)
		crops = append(crops, crop)
	}
	return crops
}

// itemCrop is one item's page and padded box: the words of its line, or of its caption below it,
// on the page of the first of them. Language and math items concern the whole document and get none
// (the review page says "document-level").
func itemCrop(index int, item Item, rows wordRows, sizes map[int]pageSize) (Crop, bool) {
	if item.Kind == "language" || item.Kind == "math" {
		return Crop{}, false
	}
	line := item.Line
	if line <= 0 {
		line = item.NoteLine
	}
	if line <= 0 {
		return Crop{}, false
	}
	words := placeWords(rows[item.File], line)
	if len(words) == 0 && item.CropLine > 0 {
		words = placeWords(rows[item.File], item.CropLine)
	}
	if len(words) == 0 {
		return Crop{}, false
	}
	page := int(rec.FetchInt(words[0], "page"))
	size, ok := sizes[page]
	if page <= 0 || !ok || size.width <= 0 || size.height <= 0 {
		return Crop{}, false
	}
	box, ok := wordBox(words, page)
	if !ok {
		return Crop{}, false
	}
	box = box.Padded(CropPadding, size.width, size.height)
	return Crop{Item: index, Page: page, Box: box, region: box.Region(CropResolution)}, true
}

// placeWords is the words of an item's line; when the line has none (a command that draws
// nothing itself), the words of the caption below it, as far as the run of lines goes.
func placeWords(rows map[int][]rec.Map, line int) []rec.Map {
	if words := rows[line]; len(words) > 0 {
		return words
	}
	for start := line + 1; start <= line+cropLookahead; start++ {
		if len(rows[start]) == 0 {
			continue
		}
		words := append([]rec.Map{}, rows[start]...)
		for next := start + 1; next < start+cropRun && len(rows[next]) > 0; next++ {
			words = append(words, rows[next]...)
		}
		return words
	}
	return nil
}

// wordBox is the union of the words' boxes on page.
func wordBox(words []rec.Map, page int) (Box, bool) {
	box, found := Box{}, false
	for _, word := range words {
		if int(rec.FetchInt(word, "page")) != page {
			continue
		}
		for _, value := range rec.FetchList(word, "boxes") {
			one, ok := boxOf(value)
			if !ok {
				continue
			}
			if !found {
				box, found = one, true
				continue
			}
			box = box.Union(one)
		}
	}
	return box, found
}

// boxOf is one [x0, y0, x1, y1] box of the map, when it is finite and ordered.
func boxOf(value any) (Box, bool) {
	values := rec.AsList(value)
	coordinate := func(index int) (float64, bool) {
		number, ok := rec.Number(rec.Index(values, index))
		return number, ok && !math.IsInf(number, 0) && !math.IsNaN(number)
	}
	box := Box{}
	var ok [4]bool
	box.MinX, ok[0] = coordinate(0)
	box.MinY, ok[1] = coordinate(1)
	box.MaxX, ok[2] = coordinate(2)
	box.MaxY, ok[3] = coordinate(3)
	if len(values) != 4 || !ok[0] || !ok[1] || !ok[2] || !ok[3] || box.MinX > box.MaxX || box.MinY > box.MaxY {
		return Box{}, false
	}
	return box, true
}

// Union is the smallest box holding both.
func (b Box) Union(other Box) Box {
	return Box{MinX: math.Min(b.MinX, other.MinX), MinY: math.Min(b.MinY, other.MinY),
		MaxX: math.Max(b.MaxX, other.MaxX), MaxY: math.Max(b.MaxY, other.MaxY)}
}

// Padded grows the box by pad points on every side and clamps it to the page.
func (b Box) Padded(pad, width, height float64) Box {
	padded := Box{
		MinX: clamp(math.Max(0, b.MinX-pad), 0, width),
		MinY: clamp(math.Max(0, b.MinY-pad), 0, height),
		MaxX: clamp(math.Min(width, b.MaxX+pad), 0, width),
		MaxY: clamp(math.Min(height, b.MaxY+pad), 0, height),
	}
	if padded.MaxX-padded.MinX < 0.5 {
		padded.MaxX = clamp(padded.MinX+0.5, 0, width)
		padded.MinX = clamp(padded.MaxX-0.5, 0, width)
	}
	if padded.MaxY-padded.MinY < 0.5 {
		padded.MaxY = clamp(padded.MinY+0.5, 0, height)
		padded.MinY = clamp(padded.MaxY-0.5, 0, height)
	}
	return padded
}

// Region is the box in pixels at dpi, at least one pixel wide and high.
func (b Box) Region(dpi int) PixelRegion {
	scale := float64(dpi) / 72
	region := PixelRegion{
		X:      max(0, int(math.Floor(b.MinX*scale))),
		Y:      max(0, int(math.Floor(b.MinY*scale))),
		Width:  max(1, int(math.Ceil((b.MaxX-b.MinX)*scale))),
		Height: max(1, int(math.Ceil((b.MaxY-b.MinY)*scale))),
	}
	return region
}

func clamp(value, low, high float64) float64 {
	return math.Min(math.Max(value, low), high)
}
