package assets

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// spPerBP is RenderedGraphics::SP_PER_BP (a literal, as in Ruby).
const spPerBP = 65_781.76

var pageSize = regexp.MustCompile(`(?m)^Page\s+(\d+) size:\s+([\d.]+) x ([\d.]+) pts`)

// ApplyRendered is RenderedGraphics.apply!: each rendered graphic is cropped out of the
// reference PDF and its reading span becomes an ordinary PDF image, in place. It returns
// the cropped PDFs by reading filename.
func ApplyRendered(reading rec.Map, pdf string, diags *diagnostics.Diagnostics) map[string][]byte {
	files, _ := CropRendered(reading, pdf, diags)
	return files
}

// CropRendered is ApplyRendered that also returns each cropped picture's page and box, by reading
// filename in reading order: the tagged PDF finds the picture's painting there (glyphowner.Picture).
func CropRendered(reading rec.Map, pdf string, diags *diagnostics.Diagnostics) (map[string][]byte, map[string][]DrawnPicture) {
	files, pictures := map[string][]byte{}, map[string][]DrawnPicture{}
	spans := applyOrcid(renderedSpans(reading), files)
	if len(spans) == 0 {
		return files, pictures
	}
	pages := []int64{}
	for _, span := range spans {
		pages = append(pages, rec.FetchInt(span, "page"))
	}
	sizes := pageSizes(pdf, pages)
	directory := command.Scratch("rendered-graphics-")
	boxes := make([][4]int64, len(spans))
	for index, span := range spans {
		size, ok := sizes[rec.FetchInt(span, "page")]
		if !ok {
			panic(rec.Failure{Message: fmt.Sprintf("key not found: %d", rec.FetchInt(span, "page")), Lookup: true})
		}
		boxes[index] = cropBox(span, size.height)
	}
	// Each crop is a pdftocairo and a qpdf run on its own files: they run side by side, and the first
	// failing crop in order is the one raised.
	crops := make([][]byte, len(spans))
	failures := make([]any, len(spans))
	limit := make(chan struct{}, runtime.NumCPU())
	var group sync.WaitGroup
	for index, span := range spans {
		group.Add(1)
		go func() {
			defer group.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			defer func() { failures[index] = recover() }()
			crops[index] = crop(pdf, span, boxes[index], sizes[rec.FetchInt(span, "page")].landscape(), directory)
		}()
	}
	group.Wait()
	for _, failure := range failures {
		if failure != nil {
			panic(failure)
		}
	}
	for index, span := range spans {
		data, box := crops[index], boxes[index]
		height := sizes[rec.FetchInt(span, "page")].height
		// The baseline keeps its distance from the box bottom; rounding outward lowers the crop.
		region := rec.FetchList(span, "region_sp")
		lowered := rec.AsInt(region[1]) - int64(math.Round((height-float64(box[1])-float64(box[3]))*spPerBP))
		drawn := DrawnPicture{Page: int(rec.FetchInt(span, "page")), Box: pictureBox(region, height)}
		renderedImage(span, data, box, rec.FetchInt(span, "depth_sp")+lowered)
		name := rec.FetchString(span, "filename")
		files[name], pictures[name] = data, append(pictures[name], drawn)
	}
	diags.Add("figure.rendered_from_pdf", fmt.Sprintf("%d figure(s) without an inspectable source rendered from the PDF", len(spans)), nil)
	return files, pictures
}

// pictureBox is a drawn picture's region (left, bottom, right, top in sp from the page's lower-left
// corner) as left, top, right, bottom in bp from the top-left corner, to the hundredth: where the
// tagged PDF's route 2 finds its painting (glyphowner.Picture).
func pictureBox(region []any, pageHeight float64) [4]float64 {
	bp := func(index int) float64 {
		number, _ := rec.Number(region[index])
		return number / spPerBP
	}
	round := func(value float64) float64 { return math.Round(value*100) / 100 }
	return [4]float64{round(bp(0)), round(pageHeight - bp(3)), round(bp(2)), round(pageHeight - bp(1))}
}

// DrawnPicture is a rendered graphic's page and box (left, top, right, bottom in bp from the
// top-left corner).
type DrawnPicture struct {
	Page int
	Box  [4]float64
}

func renderedSpans(reading rec.Map) []rec.Map {
	blocks := rec.FetchList(reading, "blocks")
	for _, note := range rec.FetchList(rec.FetchMap(reading, "notes"), "notes") {
		blocks = append(blocks[:len(blocks):len(blocks)], rec.FetchList(rec.AsMap(note), "blocks")...)
	}
	spans := []rec.Map{}
	for _, block := range blocks {
		spans = append(spans, inline.OfType(rec.FetchList(rec.AsMap(block), "content"), "rendered_graphic")...)
	}
	return spans
}

// pageBox is a page's size in bp, as pdfinfo prints it.
type pageBox struct{ width, height float64 }

// landscape: pdftocairo turns the paper of a page wider than tall, so a crop of such a page names
// its paper height first.
func (box pageBox) landscape() bool { return box.width > box.height }

func pageSizes(pdf string, pages []int64) map[int64]pageBox {
	tool := command.Resolve("pdfinfo")
	if tool == "" {
		rec.Fail("Rendered figures require pdfinfo")
	}
	first, last := pages[0], pages[0]
	for _, page := range pages {
		first, last = min(first, page), max(last, page)
	}
	output := command.Capture(tool, "-f", strconv.FormatInt(first, 10), "-l", strconv.FormatInt(last, 10), pdf)
	sizes := map[int64]pageBox{}
	for _, match := range pageSize.FindAllStringSubmatch(output, -1) {
		page, _ := strconv.ParseInt(match[1], 10, 64)
		width, widthErr := strconv.ParseFloat(match[2], 64)
		height, err := strconv.ParseFloat(match[3], 64)
		if err != nil || widthErr != nil {
			rec.Fail("invalid value for Float(): %q", match[0])
		}
		sizes[page] = pageBox{width, height}
	}
	return sizes
}

// cropBox is the box in bp, rounded outward to whole points (pdftocairo takes integers).
func cropBox(span rec.Map, pageHeight float64) [4]int64 {
	region := rec.FetchList(span, "region_sp")
	values := make([]float64, 4)
	for index := range values {
		number, _ := rec.Number(region[index])
		values[index] = number / spPerBP
	}
	left, bottom, right, top := values[0], values[1], values[2], values[3]
	x := int64(math.Floor(left))
	y := int64(math.Floor(pageHeight - top))
	return [4]int64{x, y, max(int64(math.Ceil(right))-x, 1), max(int64(math.Ceil(pageHeight-bottom))-y, 1)}
}

func crop(pdf string, span rec.Map, box [4]int64, landscape bool, directory string) []byte {
	tool := command.Resolve("pdftocairo")
	if tool == "" {
		rec.Fail("Rendered figures require pdftocairo")
	}
	page := rubyString(rec.Fetch(span, "page"))
	id := rec.FetchString(span, "id")
	name := id + ".pdf"
	number := func(value int64) string { return strconv.FormatInt(value, 10) }
	paperWidth, paperHeight := box[2], box[3]
	if landscape {
		paperWidth, paperHeight = paperHeight, paperWidth
	}
	args := []string{tool, "-pdf", "-f", page, "-l", page, "-x", number(box[0]), "-y", number(box[1]), "-W", number(box[2]),
		"-H", number(box[3]), "-paperw", number(paperWidth), "-paperh", number(paperHeight), "-nocenter", "-noshrink", pdf, name}
	stage := command.Run(args, directory, filepath.Join(directory, id+".log"), 120*time.Second, nil)
	path := filepath.Join(directory, name)
	info, err := os.Stat(path)
	if !rec.Equal(stage["exit"], int64(0)) || err != nil || !info.Mode().IsRegular() {
		rec.Fail("Figure crop from the PDF failed")
	}
	data, err := os.ReadFile(normalizedCrop(path, directory, id))
	if err != nil {
		rec.Fail("%v", err)
	}
	return data
}

// normalizedCrop: Cairo writes the crop with its creation date and a random file ID; without
// them, and with an ID derived from the content, the crop is the same bytes in every run.
func normalizedCrop(path, directory, id string) string {
	tool := command.Resolve("qpdf")
	if tool == "" {
		rec.Fail("Rendered figures require qpdf")
	}
	output := filepath.Join(directory, id+".normalized.pdf")
	log := filepath.Join(directory, id+".qpdf.log")
	stage := command.Run([]string{tool, "--deterministic-id", "--remove-info", path, output}, directory, log,
		60*time.Second, nil)
	info, err := os.Stat(output)
	if err != nil || !info.Mode().IsRegular() || !normalizationSucceeded(stage["exit"], log) {
		rec.Fail("Figure crop normalization failed")
	}
	return output
}

// qpdfWarning is one warning line of qpdf; qpdfDuplicateKey is the one warning a Cairo crop
// may draw, since Cairo can list one XObject twice in a resource dictionary
// ("/x7 7 0 R /x7 7 0 R"). qpdf writes the dictionary once and exits 3.
var (
	qpdfWarning      = regexp.MustCompile(`(?m)^WARNING: .*$`)
	qpdfDuplicateKey = regexp.MustCompile(`: dictionary has duplicated key /\S+; last occurrence overrides earlier ones$`)
)

// normalizationSucceeded: qpdf exits 0, or 3 (written with warnings) when every warning is a
// duplicated dictionary key.
func normalizationSucceeded(exit any, log string) bool {
	if rec.Equal(exit, int64(0)) {
		return true
	}
	if !rec.Equal(exit, int64(3)) {
		return false
	}
	text, err := os.ReadFile(log)
	return err == nil && onlyDuplicateKeyWarnings(string(text))
}

func onlyDuplicateKeyWarnings(text string) bool {
	warnings := qpdfWarning.FindAllString(text, -1)
	for _, warning := range warnings {
		// A Windows qpdf ends its lines with CRLF.
		if !qpdfDuplicateKey.MatchString(strings.TrimSuffix(warning, "\r")) {
			return false
		}
	}
	return len(warnings) > 0
}

// renderedImage is span.replace: the span becomes the cropped PDF image.
func renderedImage(span rec.Map, data []byte, box [4]int64, depth int64) {
	placement := renderedPlacement(box[2], box[3], depth)
	sha := hexDigest(data)
	from := rec.Map{}
	for _, key := range []string{"page", "graphic"} {
		if value, ok := span[key]; ok {
			from[key] = value
		}
	}
	id := rec.Fetch(span, "id")
	for key := range span {
		delete(span, key)
	}
	replacement := rec.Map{"type": "image", "id": id, "filename": "rendered/" + sha + ".pdf", "sha256": sha,
		"bytes": int64(len(data)), "mime_type": "application/pdf", "placement": placement, "width_sp": placement["width_sp"],
		"height_sp": placement["height_sp"], "depth_sp": placement["depth_sp"], "rendered_from": from}
	for key, value := range replacement {
		span[key] = value
	}
}

func renderedPlacement(width, height, depth int64) rec.Map {
	identity := []any{1.0, 0.0, 0.0, 1.0, 0.0, 0.0}
	data := rec.Map{"schema_version": mediamodel.PlacementVersion, "page": int64(1), "pagebox": "mediabox",
		"source_box_bp": []any{int64(0), int64(0), width, height}, "viewport_bp": []any{int64(0), int64(0), width, height},
		"clip": false, "matrix": identity, "width_sp": int64(math.Round(float64(width) * spPerBP)),
		"height_sp": int64(math.Round(float64(height) * spPerBP)), "depth_sp": depth, "operations": []any{}}
	id, err := canonjson.Digest(data)
	if err != nil {
		rec.Fail("%v", err)
	}
	data["id"] = id
	mediamodel.ValidatePlacement(data)
	return data
}
