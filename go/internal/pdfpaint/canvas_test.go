package pdfpaint

import (
	"strings"
	"testing"
)

// canvasPage is a page whose first operation fills path (corners in the page's flipped space, as
// beamer's background canvas rule reaches mutool's trace) before its text.
func canvasPage(number, fill, path string) string {
	text := `<fill_text colorspace="DeviceGray" color="1"><span trm="1 0 0 1"><g unicode="a" x="1" y="1" adv="1"/></span></fill_text>`
	return `<page number="` + number + `" mediabox="0 0 362.835 272.126"><set_default_colorspaces gray="DeviceGray"/>` +
		`<fill_path winding="nonzero" ` + fill + ` transform="1 0 0 -1 0 272.126">` + path + `</fill_path>` + text + `</page>`
}

const pageRectangle = `<moveto x="0" y="0"/><lineto x="362.835" y="0"/><lineto x="362.835" y="272.126"/><lineto x="0" y="272.126"/><closepath/>`

// A deck whose every page is first filled black (beamer's background canvas, slides 0007) records
// that canvas; a white canvas, a page without one, a canvas of another colour on one page, a
// smaller rectangle or a translucent fill records none.
func TestTextPaintRecordsTheCanvasEveryPageHas(t *testing.T) {
	black := `colorspace="DeviceRGB" color="0 0 0"`
	for _, test := range []struct {
		name  string
		pages []string
		want  string
	}{
		{"black deck", []string{canvasPage("1", black, pageRectangle), canvasPage("2", black, pageRectangle)}, "#000000"},
		{"white deck", []string{canvasPage("1", `colorspace="DeviceGray" color="1"`, pageRectangle)}, ""},
		{"one page without", []string{canvasPage("1", black, pageRectangle), strings.Replace(canvasPage("2", black, pageRectangle), "<fill_path", "<fill_text colorspace=\"DeviceGray\" color=\"0\"/><fill_path", 1)}, ""},
		{"two colours", []string{canvasPage("1", black, pageRectangle), canvasPage("2", `colorspace="DeviceRGB" color="0 0 1"`, pageRectangle)}, ""},
		{"a box", []string{canvasPage("1", black, strings.ReplaceAll(pageRectangle, "362.835", "100"))}, ""},
		{"translucent", []string{canvasPage("1", black+` alpha=".5"`, pageRectangle)}, ""},
		{"curved", []string{canvasPage("1", black, strings.Replace(pageRectangle, "<closepath/>", `<curveto x1="0" y1="0" x2="0" y2="0" x3="0" y3="0"/>`, 1))}, ""},
	} {
		_, _, _, canvas, _ := inspectTrace([]byte("<document>"+strings.Join(test.pages, "")+"</document>"), false)
		if canvas != test.want {
			t.Errorf("%s: canvas %q, want %q", test.name, canvas, test.want)
		}
	}
}
