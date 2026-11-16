package assets

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/xmltree"
)

var pageBoxes = map[string]string{"mediabox": "MediaBox", "cropbox": "CropBox", "bleedbox": "BleedBox", "trimbox": "TrimBox",
	"artbox": "ArtBox"}

var jpegFrames = []byte{0xc0, 0xc1, 0xc2, 0xc3, 0xc5, 0xc6, 0xc7, 0xc9, 0xca, 0xcb, 0xcd, 0xce, 0xcf}

// svgLimit: a PDF figure whose vector SVG would exceed this (dense plots: one 805 KB PDF
// became a 38 MB SVG) is drawn as a PNG at twice the browser's CSS pixel density instead.
const (
	svgLimit  = 4 * 1024 * 1024
	rasterDPI = 192
)

// oversizedPage is the message of MediaDerivatives::Oversized: a page conversion larger than
// the extraction limit.
const oversizedPage = "Selected PDF media page is excessive"

var lengthSuffix = regexp.MustCompile(`(?:pt|px)\z`)

// Derivatives is MediaDerivatives: one captured source with a representation per
// page/crop/transform placement.
type Derivatives struct {
	data  []byte
	mime  string
	png   *mediamodel.PNG
	asset rec.Map
	pages map[[2]any][]byte
}

// NewDerivatives is MediaDerivatives.new(bytes, expected).
func NewDerivatives(data []byte, expected rec.Map) *Derivatives {
	if !rec.Equal(int64(len(data)), rec.Fetch(expected, "bytes")) || hexDigest(data) != rec.Fetch(expected, "sha256") {
		rec.Fail("Media source bytes differ from collected identity")
	}
	d := &Derivatives{data: data, mime: rec.FetchString(expected, "mime_type"), pages: map[[2]any][]byte{}}
	if d.mime == "image/png" {
		d.png = mediamodel.PNGFromBytes(data, expected)
	}
	d.asset = rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": rec.Fetch(expected, "sha256"), "metadata": d.metadata(),
		"placements": rec.Map{}, "representations": rec.Map{"original": mediamodel.Encoded(data, d.mime)}}
	return d
}

// Add is MediaDerivatives#add: the asset with one more placement and its display SVG.
func (d *Derivatives) Add(placement rec.Map) rec.Map {
	mediamodel.ValidatePlacement(placement)
	id := rec.FetchString(placement, "id")
	placements := rec.FetchMap(d.asset, "placements")
	if _, ok := placements[id]; ok {
		return d.asset
	}
	placements[id] = placement
	representation := mediamodel.Encoded(d.svg(placement), "image/svg+xml")
	representation["placement"] = id
	rec.FetchMap(d.asset, "representations")["display:"+id] = representation
	return d.asset
}

func (d *Derivatives) metadata() rec.Map {
	switch d.mime {
	case "image/png":
		return rec.Map{"pixel_width": d.png.Metadata["pixel_width"], "pixel_height": d.png.Metadata["pixel_height"],
			"trailing_bytes": d.png.Metadata["trailing_bytes"]}
	case "image/jpeg":
		width, height := d.jpegDimensions()
		if width <= 0 || height <= 0 || width*height > mediamodel.PixelLimit {
			rec.Fail("JPEG source exceeds pixel limit")
		}
		return rec.Map{"pixel_width": width, "pixel_height": height}
	case "application/pdf":
		if !bytes.HasPrefix(d.data, []byte("%PDF-")) {
			rec.Fail("Expected a PDF media source")
		}
		return rec.Map{}
	}
	rec.Fail("Unsupported collected media type")
	return nil
}

func (d *Derivatives) jpegDimensions() (int64, int64) {
	data := d.data
	if !bytes.HasPrefix(data, []byte{0xff, 0xd8}) {
		rec.Fail("Expected a JPEG source")
	}
	position := 2
	for position < len(data) {
		if data[position] != 0xff {
			rec.Fail("Invalid JPEG marker boundary")
		}
		for position < len(data) && data[position] == 0xff {
			position++
		}
		if position >= len(data) {
			rec.Fail("Invalid JPEG segment length")
		}
		marker := data[position]
		position++
		if marker == 0x01 || (marker >= 0xd0 && marker <= 0xd8) {
			continue
		}
		if marker == 0xda || marker == 0xd9 {
			break
		}
		if position+2 > len(data) {
			rec.Fail("Invalid JPEG segment length")
		}
		length := int(binary.BigEndian.Uint16(data[position:]))
		if length < 2 || position+length > len(data) {
			rec.Fail("Invalid JPEG segment length")
		}
		if bytes.IndexByte(jpegFrames, marker) >= 0 && length >= 8 {
			height := int64(binary.BigEndian.Uint16(data[position+3:]))
			width := int64(binary.BigEndian.Uint16(data[position+5:]))
			return width, height
		}
		position += length
	}
	rec.Fail("No supported JPEG frame dimensions")
	return 0, 0
}

// formatG is DecimalFormat.g(number, 12).
func formatG(value any) string {
	number, ok := rec.Number(value)
	if !ok {
		rec.Fail("can't convert %T into Float", value)
	}
	return rec.FormatG(number, 12)
}

func subtract(left, right any) any {
	a, isInt := left.(int64)
	b, isOtherInt := right.(int64)
	if isInt && isOtherInt {
		return a - b
	}
	x, _ := rec.Number(left)
	y, _ := rec.Number(right)
	return x - y
}

func (d *Derivatives) svg(placement rec.Map) []byte {
	box := rec.FetchList(placement, "source_box_bp")
	width := subtract(box[2], box[0])
	height := subtract(box[3], box[1])
	var content string
	if d.mime == "application/pdf" {
		content = d.pdfContent(placement, width, height)
	} else {
		content = d.raster(width, height)
	}
	matrix := []string{}
	for _, number := range rec.FetchList(placement, "matrix") {
		matrix = append(matrix, formatG(number))
	}
	canvas := []string{}
	for _, key := range []string{"width_sp", "height_sp"} {
		value, _ := rec.Number(rec.Fetch(placement, key))
		canvas = append(canvas, rubyString(geometry.SPToBP(value)))
	}
	clip := ""
	if rec.Fetch(placement, "clip") == true {
		viewport := rec.FetchList(placement, "viewport_bp")
		left, top, right, bottom := viewport[0], viewport[1], viewport[2], viewport[3]
		clip = "<defs><clipPath id=\"viewport\"><rect x=\"" + rubyString(left) + "\" y=\"" + rubyString(top) + "\" " +
			"width=\"" + rubyString(subtract(right, left)) + "\" height=\"" + rubyString(subtract(bottom, top)) + "\"/></clipPath></defs>"
		content = "<g clip-path=\"url(#viewport)\">" + content + "</g>"
	}
	svg := "<svg xmlns=\"http://www.w3.org/2000/svg\" xmlns:xlink=\"http://www.w3.org/1999/xlink\" " +
		"width=\"" + canvas[0] + "pt\" height=\"" + canvas[1] + "pt\" viewBox=\"0 0 " + strings.Join(canvas, " ") + "\">" +
		clip + "<g transform=\"matrix(" + strings.Join(matrix, " ") + ")\">" + content + "</g></svg>"
	return mediamodel.ValidateSVG([]byte(svg))
}

// pdfContent is vector when it is a reasonable size, a raster otherwise (also when the
// vector form is beyond the extraction limit).
func (d *Derivatives) pdfContent(placement rec.Map, width, height any) (content string) {
	defer d.rasterWhenOversized(placement, width, height, &content)
	content = d.pdfPage(placement, width, height)
	if len(content) > svgLimit {
		return d.pdfRaster(placement, width, height)
	}
	return content
}

func (d *Derivatives) rasterWhenOversized(placement rec.Map, width, height any, content *string) {
	recovered := recover()
	if recovered == nil {
		return
	}
	if failure, ok := recovered.(rec.Failure); !ok || failure.Message != oversizedPage {
		panic(recovered)
	}
	*content = d.pdfRaster(placement, width, height)
}

func imageElement(width, height any, uri string) string {
	return "<image width=\"" + rubyString(width) + "\" height=\"" + rubyString(height) +
		"\" preserveAspectRatio=\"none\" xlink:href=\"" + uri + "\"/>"
}

func (d *Derivatives) raster(width, height any) string {
	item := rec.FetchMap(rec.FetchMap(d.asset, "representations"), "original")
	if d.png != nil {
		item = mediamodel.Encoded(mediamodel.DataURIBytes(rec.FetchString(d.png.Descriptor(), "data_uri")), d.mime)
	}
	return imageElement(width, height, rec.FetchString(item, "data_uri"))
}

func (d *Derivatives) pdfPage(placement rec.Map, width, height any) string {
	key := [2]any{rec.Fetch(placement, "page"), rec.Fetch(placement, "pagebox")}
	page, ok := d.pages[key]
	if !ok {
		page = d.extractPage(key[0], rec.AsString(key[1]))
		d.pages[key] = page
	}
	root := parseREXML(string(page))
	if _, has := root.attribute("viewBox"); !has {
		size := []string{}
		for _, name := range []string{"width", "height"} {
			value, found := root.attribute(name)
			if !found {
				panic(rec.Failure{Message: fmt.Sprintf("key not found: %q", name), Lookup: true})
			}
			number, err := strconv.ParseFloat(strings.TrimSpace(lengthSuffix.ReplaceAllString(value, "")), 64)
			if err != nil {
				rec.Fail("invalid value for Float(): %q", value)
			}
			size = append(size, rubyString(number))
		}
		root.setAttribute("viewBox", "0 0 "+strings.Join(size, " "))
	}
	root.setAttribute("width", rubyString(width))
	root.setAttribute("height", rubyString(height))
	root.setAttribute("preserveAspectRatio", "none")
	var out strings.Builder
	root.write(&out)
	return out.String()
}

func (d *Derivatives) pdfRaster(placement rec.Map, width, height any) string {
	data := d.renderPage(rec.Fetch(placement, "page"), rec.FetchString(placement, "pagebox"), "png", []string{"-r", strconv.Itoa(rasterDPI)})
	png := mediamodel.PNGFromBytes(data, rec.Map{})
	size := rec.AsInt(png.Metadata["png_bytes"])
	return imageElement(width, height, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(data[:size]))
}

// extractPage: a glyph mapped to a control character is U+FFFD in the figure's data-text, which
// an XML reader could not otherwise read (fifth cohort 1457).
func (d *Derivatives) extractPage(page any, pagebox string) []byte {
	return mediamodel.ValidateSVG(xmltree.Characters(d.renderPage(page, pagebox, "svg", []string{"-O", "text=path"})))
}

// recoverableLine is a message mutool prints for content it repaired while drawing the page (a
// shading's stitching function beyond its limits); it then exits 1 with the page drawn.
var recoverableLine = regexp.MustCompile(`\A(?:warning|syntax error): `)

// recoveredSyntax: mutool's log holds only recoverable messages, at least one of them a syntax
// error, so its failing exit reports repaired content, not a missing page (third cohort 786).
// A document it cannot open ("format error", "cannot draw") still fails.
func recoveredSyntax(log string) bool {
	data, err := os.ReadFile(log)
	if err != nil {
		return false
	}
	syntax := false
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if !recoverableLine.MatchString(line) {
			return false
		}
		syntax = syntax || strings.HasPrefix(line, "syntax error: ")
	}
	return syntax
}

func (d *Derivatives) renderPage(page any, pagebox, format string, options []string) []byte {
	helper := command.Resolve("mutool")
	if helper == "" {
		rec.Fail("PDF media conversion requires mutool")
	}
	fingerprint := command.FileDigest(helper)
	directory := command.Scratch("media-pdf-")
	if err := os.WriteFile(filepath.Join(directory, "source.pdf"), d.data, 0o644); err != nil {
		rec.Fail("%v", err)
	}
	box, ok := pageBoxes[pagebox]
	if !ok {
		panic(rec.Failure{Message: fmt.Sprintf("key not found: %q", pagebox), Lookup: true})
	}
	drawArgs := append([]string{"-F", format}, options...)
	drawArgs = append(drawArgs, "-b", box, "-o", "page."+format, "source.pdf", rubyString(page))
	args := command.MutoolDrawCommand(helper, drawArgs...)
	stage := command.Run(args, directory, filepath.Join(directory, "media.log"), 120*time.Second, nil)
	stage["helper_sha256"] = fingerprint
	writeJSON(filepath.Join(directory, "stage.json"), stage)
	exited := rec.Equal(stage["exit"], int64(0)) || recoveredSyntax(filepath.Join(directory, "media.log"))
	if !exited || stage["timeout"] == true || command.FileDigest(helper) != fingerprint {
		rec.Fail("Selected PDF media page extraction failed")
	}
	path := filepath.Join(directory, "page."+format)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		rec.Fail("Selected PDF media page is missing")
	}
	if info.Size() > mediamodel.MaxBytes {
		rec.Fail(oversizedPage)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	return data
}
