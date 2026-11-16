package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/pdffonts"
	"github.com/sarielhp/relatex/internal/pdftag"
	"github.com/sarielhp/relatex/internal/pdfua"
	"github.com/sarielhp/relatex/internal/rec"
)

// The tagged PDF's route (decision 105): route 2 tags the ordinary PDF from the model (pdftag). It
// is the only route; route 1, the document rebuilt with LaTeX's tagging, was removed on 2026-11-03
// (decisions 097 and 105; git tag v0.1.55 holds its last version).

// PDFUARoutes are --pdf-ua's choices, both route 2: auto and route2.
var PDFUARoutes = []string{"auto", "route2"}

// tagged is the pdf-ua format's report, with the PDF/UA-2 identification kept in the file only when it
// conforms and the web page reaches its claim (decision 112).
func (c *conversion) tagged(directory string) ojson.Object {
	report, target := c.route2(directory)
	return EarnedClaim(report, target, c.webClaim)
}

// route2 tags the conversion's ordinary PDF into directory from the export written so far
// (document.json.gz, the layout parts and the glyph owners in the export root).
func (c *conversion) route2(directory string) (ojson.Object, string) {
	target := filepath.Join(directory, filepath.Base(c.pdf))
	return TagOrdinary(pdftag.Options{Output: c.exportRoot, PDF: c.pdf, Target: target,
		Glyphs: filepath.Join(c.options.Root, "tex", "relatex-symbol-glyphs.txt")}), target
}

// TagOrdinary tags the ordinary PDF of an export directory by route 2 and validates it with
// veraPDF: the pdf-ua report (conforming, tagged, or failed with the reason).
func TagOrdinary(options pdftag.Options) ojson.Object {
	started := time.Now()
	report, err := pdftag.Tag(options)
	if err != nil {
		return ojson.Object{{"route", "2"}, {"status", "failed"}, {"reason", err.Error()},
			{"diagnostics", pdfua.DiagnosticList([]pdfua.Diagnostic{{Code: "pdfua.failed", Message: "no tagged PDF by route 2: " + err.Error()}})},
			{"losses", []any{}}}
	}
	validation := pdfua.Validate(report.PDF)
	status := "tagged"
	if validation.Status == "passed" && report.Faithful() {
		status = "conforming"
	}
	data, err := json.Marshal(report)
	if err != nil {
		rec.Fail("tagged PDF: %v", err)
	}
	plain, err := ojson.Decode(bytes.NewReader(data))
	if err != nil {
		rec.Fail("tagged PDF: %v", err)
	}
	found := append(validation.Diagnostics(), figureFontDiagnostics(validation, report)...)
	found = append(found, route2Diagnostics(report)...)
	return ojson.Object{{"route", "2"}, {"status", status}, {"file", filepath.Base(report.PDF)}, {"standard", "PDF/UA-2"},
		{"language", report.Language}, {"verapdf", validation.Report()}, {"tagging", plain},
		{"diagnostics", pdfua.DiagnosticList(found)}, {"losses", []any{}},
		{"seconds", math.Round(time.Since(started).Seconds()*10) / 10}}
}

// figureFontDiagnostics are veraPDF's failures on a font an included figure brought in: the author
// can re-export the figure, so the figure and its font are named (pdfua.figure_font).
func figureFontDiagnostics(validation pdfua.Validation, report pdftag.Report) []pdfua.Diagnostic {
	fonts := make([]pdfua.FigureFont, 0, len(report.FigureFonts))
	for _, font := range report.FigureFonts {
		fonts = append(fonts, pdfua.FigureFont{Source: font.Source, Font: font.Font, Creator: font.Creator,
			Collection: font.Collection})
	}
	return pdfua.FigureFontDiagnostics(validation, fonts)
}

// route2Diagnostics are route 2's own findings: undescribed figures and failed checks.
func route2Diagnostics(report pdftag.Report) []pdfua.Diagnostic {
	found := []pdfua.Diagnostic{}
	if missing := report.Undescribed; len(missing) > 0 {
		found = append(found, pdfua.Diagnostic{Code: "pdfua.undescribed", Message: fmt.Sprintf("%d graphics have no description "+
			"from relatex/alt.toml and are tagged with their file name (%s)", len(missing), strings.Join(missing[:min(5, len(missing))], ", ")),
			Magnitude: int64(len(missing))})
	}
	for _, alert := range report.Fonts.Alerts() {
		found = append(found, pdfua.Diagnostic{Code: "pdfua.font_unidentified", Message: alert.Message, Magnitude: int64(alert.Glyphs)})
	}
	for _, unwrapped := range report.Fonts.Unwrapped {
		found = append(found, pdfua.Diagnostic{Code: "pdfua.font_unwrapped", Magnitude: 1,
			Message: unwrappedMessage(unwrapped, figureSource(report.FigureFonts, unwrapped.Font))})
	}
	for _, added := range report.Fonts.CmapsAdded {
		found = append(found, pdfua.Diagnostic{Code: "pdfua.font_cmap_added", Magnitude: 1,
			Message: cmapAddedMessage(added, figureSource(report.FigureFonts, added.Font))})
	}
	if !report.Faithful() {
		order, look := report.Order, report.Appearance
		found = append(found, pdfua.Diagnostic{Code: "pdfua.unfaithful", Message: fmt.Sprintf("%d pages left unbound, %d of %d "+
			"elements read back otherwise than the document, %d of %d pages look different%s", len(report.PageFailures),
			order.TextLeaves-order.TextEqual, order.TextLeaves, look.Pages-look.Identical, look.Pages, checkFailures(order, look)),
			Magnitude: int64(len(report.PageFailures) + order.TextLeaves - order.TextEqual + look.Pages - look.Identical)})
	}
	return found
}

// figureSource is the file of the included figure that brought a font in; "" when none is known
// (a figure the layout does not name, or a font of the document itself).
func figureSource(fonts []pdftag.FigureFont, font string) string {
	for _, figure := range fonts {
		if figure.Font == font {
			return figure.Source
		}
	}
	return ""
}

// unwrappedMessage says what the unwrap did to one font, naming the figure so its author can
// re-export it.
func unwrappedMessage(unwrapped pdffonts.Unwrapped, source string) string {
	subject := "an included figure's font " + unwrapped.Font
	if source != "" {
		subject = "figure " + source + ": font " + unwrapped.Font
	}
	return fmt.Sprintf("%s was embedded as a whole TrueType collection (%d faces, %s bytes); the "+
		"tagged PDF embeds its face \"%s\" alone (%s bytes, glyphs unchanged); re-export the figure "+
		"with the font embedded as a single font", subject, unwrapped.Faces, spaced(unwrapped.Before),
		unwrapped.Name, spaced(unwrapped.After))
}

// cmapAddedMessage says what the cmap repair did to one font, naming the figure so its author can
// re-export it.
func cmapAddedMessage(added pdffonts.CmapAdded, source string) string {
	subject := "an included figure's font " + added.Font
	if source != "" {
		subject = "figure " + source + ": font " + added.Font
	}
	return fmt.Sprintf("%s is a non-symbolic TrueType font whose program kept only the Unicode platform's cmap subtable %s; "+
		"the tagged PDF's copy names that same subtable as Microsoft Unicode (3,1) too (glyphs, widths and outlines "+
		"unchanged); re-export the figure with a Windows or Macintosh Roman cmap", subject, added.From)
}

// spaced is a count with its thousands separated by spaces (1 792 664).
func spaced(count int) string {
	digits := strconv.Itoa(count)
	var out []byte
	for index, digit := range []byte(digits) {
		if index > 0 && (len(digits)-index)%3 == 0 {
			out = append(out, ' ')
		}
		out = append(out, digit)
	}
	return string(out)
}

func checkFailures(order pdftag.OrderCheck, look pdftag.AppearanceCheck) string {
	failures := []string{}
	for _, failure := range []string{order.Failure, look.Failure} {
		if failure != "" {
			failures = append(failures, failure)
		}
	}
	if len(failures) == 0 {
		return ""
	}
	return " (" + strings.Join(failures, "; ") + ")"
}
