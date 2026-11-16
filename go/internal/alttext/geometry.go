package alttext

import (
	"bufio"
	"bytes"
	"image"
	_ "image/jpeg" // decodes JPEG figures for OCR
	_ "image/png"  // decodes PNG figures for OCR
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/figscan"
	"github.com/sarielhp/relatex/internal/rec"
)

// What a draft rested on (decision 123 point 5): the figure's embedded Ipe source, a trace of its
// PDF, a PDF that is one bitmap, the text OCR read in a raster figure (decision 123, amendment 4),
// or only the picture (a raster figure OCR read nothing in, or a PDF no reader understood).
const (
	RestedIpe    = "ipe"
	RestedTrace  = "trace"
	RestedBitmap = "bitmap"
	RestedOCR    = "ocr"
	RestedImage  = "image"
)

// geometryFile is the name of the geometry's text form in the describer's directory.
const geometryFile = "geometry.txt"

// Geometry is what figscan read of a figure: its text form, empty when there is none, what a draft
// from it rests on, and for an Ipe view which view it is, the layers it shows and the printed text
// of its labels (decision 133: they tell the view apart from the others of its file).
type Geometry struct {
	Text     string
	RestedOn string
	View     string
	Layers   []string
	Labels   []string
}

// viewLabelLimit bounds the labels named when a view is told apart from the others.
const viewLabelLimit = 12

// geometryOf is the describer's geometry of a scanned figure.
func geometryOf(scanned *figscan.Figure) Geometry {
	found := Geometry{Text: scanned.Text(), RestedOn: scanned.Source}
	if scanned.Source != figscan.SourceIpe {
		return found
	}
	found.View, found.Layers = scanned.View, scanned.Layers
	for _, label := range scanned.Labels {
		if len(found.Labels) == viewLabelLimit {
			break
		}
		// A label's LaTeX source is no name for the reader: only printed or plain text is listed.
		text := label.Printed
		if text == "" && !label.Math && !strings.ContainsAny(label.Text, `\$`) {
			text = label.Text
		}
		if text = strings.TrimSpace(text); text != "" && !slices.Contains(found.Labels, text) {
			found.Labels = append(found.Labels, text)
		}
	}
	return found
}

// ScanFigure reads the geometry of a PDF figure at its own page and box (decision 123 point 3), or
// the text OCR reads in a PNG or JPEG figure (decision 123, amendment 4); any other figure, or a
// raster one without tesseract, has none (nil, nil). Its tools work in directories made under temp,
// the run's own temporary directory (never the system's default).
func ScanFigure(document rec.Map, figure *figdesc.Figure, temp string) (*figscan.Figure, error) {
	data, mime := figure.Image(document)
	var scanned *figscan.Figure
	var err error
	switch mime {
	case "application/pdf":
		page := max(figure.Page, 1)
		scanned, err = figscan.Scan(data, page, scanBox(data, page, figure, temp))
		// A PDF that is one bitmap has no paths to read, only the picture's text (decision 133).
		if err == nil && scanned != nil && scanned.Source == figscan.SourceBitmap {
			if read := scanPDFPicture(data, figure, temp); read != nil {
				scanned = read
			}
		}
	case "image/png", "image/jpeg":
		scanned, err = scanPicture(data)
	}
	if err != nil || scanned == nil {
		return nil, err
	}
	// The paper may print it turned (\includegraphics[angle=-90]): the geometry is turned with it,
	// as the picture is (decision 123, amendment 3).
	scanned.Turn(figure.Turns)
	return scanned, nil
}

// scanPicture reads the text in a PNG or JPEG picture by OCR.
func scanPicture(data []byte) (*figscan.Figure, error) {
	picture, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return figscan.ScanPicture(picture)
}

// scanPDFPicture reads by OCR the text in a PDF figure that is one bitmap: the figure's page and box
// rendered as the describer sees it (rasterArgs), read like a PNG figure. nil when pdftocairo or
// tesseract is missing, the render fails or OCR reads no word; the figure then rests on its bitmap.
func scanPDFPicture(data []byte, figure *figdesc.Figure, temp string) *figscan.Figure {
	tool := command.Resolve("pdftocairo")
	if tool == "" {
		return nil
	}
	directory, err := os.MkdirTemp(temp, "ocr-")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(directory)
	path, target := filepath.Join(directory, "figure.pdf"), filepath.Join(directory, "figure")
	if os.WriteFile(path, data, 0o600) != nil || exec.Command(tool, rasterArgs(figure, path, target)...).Run() != nil {
		return nil
	}
	rendered, err := os.ReadFile(target + ".png")
	if err != nil {
		return nil
	}
	read, err := scanPicture(rendered)
	if err != nil || read == nil {
		return nil
	}
	read.Detail += ", in the PDF's one image rendered at " + rasterDPI + " dpi"
	return read
}

// scanBox is the figure's box in PDF points, from its box measured from the top-left corner of
// the page box; nil (the page's own box) for a whole page, or when the page box cannot be read.
func scanBox(data []byte, page int, figure *figdesc.Figure, temp string) *figscan.Box {
	if len(figure.Box) != 4 {
		return nil
	}
	origin, ok := pageBox(data, page, figure.PageBox, temp)
	if !ok {
		return nil
	}
	left, top, right, bottom := figure.Box[0], figure.Box[1], figure.Box[2], figure.Box[3]
	return &figscan.Box{X0: origin.X0 + left, Y0: origin.Y1 - bottom, X1: origin.X0 + right, Y1: origin.Y1 - top}
}

// pageBox is the named box of a page as pdfinfo reports it (the crop box when unnamed).
func pageBox(data []byte, page int, name, temp string) (figscan.Box, bool) {
	tool := command.Resolve("pdfinfo")
	if tool == "" {
		return figscan.Box{}, false
	}
	directory, err := os.MkdirTemp(temp, "box-")
	if err != nil {
		return figscan.Box{}, false
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, "figure.pdf")
	if os.WriteFile(path, data, 0o600) != nil {
		return figscan.Box{}, false
	}
	number := strconv.Itoa(page)
	output, err := exec.Command(tool, "-box", "-f", number, "-l", number, path).Output()
	if err != nil {
		return figscan.Box{}, false
	}
	return parsePageBox(output, boxLabel(name))
}

// boxLabel is pdfinfo's label for a page box name ("mediabox" is "MediaBox:").
func boxLabel(name string) string {
	labels := map[string]string{"mediabox": "MediaBox:", "cropbox": "CropBox:", "bleedbox": "BleedBox:",
		"trimbox": "TrimBox:", "artbox": "ArtBox:"}
	if label, ok := labels[strings.ToLower(name)]; ok {
		return label
	}
	return "CropBox:"
}

// parsePageBox finds "Page N Label: x0 y0 x1 y1" in pdfinfo's output.
func parsePageBox(output []byte, label string) (figscan.Box, bool) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 7 || fields[0] != "Page" || fields[2] != label {
			continue
		}
		values := [4]float64{}
		for index := range values {
			value, err := strconv.ParseFloat(fields[3+index], 64)
			if err != nil {
				return figscan.Box{}, false
			}
			values[index] = value
		}
		return figscan.Box{X0: values[0], Y0: values[1], X1: values[2], Y1: values[3]}, true
	}
	return figscan.Box{}, false
}

// geometry is the figure's geometry, scanned once per identity and scanner version in a run.
func (d *Drafter) geometry(document rec.Map, figure *figdesc.Figure) Geometry {
	key := figure.Identity() + "|" + figscan.Version + "|" + strconv.Itoa(figure.Turns)
	if found, ok := d.scans[key]; ok {
		return found
	}
	found := Geometry{RestedOn: RestedImage}
	if scanned, err := ScanFigure(document, figure, d.temp); err == nil && scanned != nil {
		switch scanned.Source {
		case figscan.SourceIpe, figscan.SourceTrace, figscan.SourceBitmap, figscan.SourceOCR:
			found = geometryOf(scanned)
		}
	}
	if d.scans == nil {
		d.scans = map[string]Geometry{}
	}
	d.scans[key] = found
	return found
}
