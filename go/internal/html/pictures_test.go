package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// labelledCrop is a picture TeX drew whose labels are SVG text (decision 137).
const labelledCrop = `<svg height='10pt' viewBox='0 0 10 10' width='10pt' xmlns:xlink='http://www.w3.org/1999/xlink' ` +
	`xmlns='http://www.w3.org/2000/svg'><defs><path d='M0 0H1Z' id='font_1_1'/></defs><use xlink:href='#font_1_1'/>` +
	`<text font-family='serif' font-size='5' xml:space='preserve' y='8'><tspan x='1'>x</tspan></text></svg>`

// TestPictureWithLabelsIsInlineSVG: a picture TeX drew whose labels are text is shown as inline SVG
// (decision 146), an image named by its alternative, ids prefixed by its own, the file it shows
// named by data-src; with its labels as outlines it stays an img.
func TestPictureWithLabelsIsInlineSVG(t *testing.T) {
	crop := figure("", "10")
	shown := mediamodel.Encoded([]byte(labelledCrop), "image/svg+xml")
	representations := rec.FetchMap(crop.asset, "representations")
	representations["html"], representations["markdown"], representations["typst"] = shown, shown, shown
	crop.span["source"] = "rendered/" + rec.ToS(crop.asset["sha256"]) + ".pdf"
	got := imageHTML(figurePaper(crop))
	want := `<svg viewBox="0 0 10 10" xmlns:xlink="http://www.w3.org/1999/xlink" xmlns="http://www.w3.org/2000/svg" id="n-0-image" ` +
		`data-kind="image" data-asset="` + rec.ToS(crop.asset["sha256"]) + `" data-width-sp="655360" data-height-sp="655360" ` +
		`data-src="figs/fig-1.svg" style="width:13.283520px;height:13.283520px" role="img" aria-label="Undescribed image" ` +
		`data-undescribed="true"><defs><path d="M0 0H1Z" id="n-0-image-font_1_1"/></defs><use xlink:href="#n-0-image-font_1_1"/>` +
		`<text font-family="serif" font-size="5" xml:space="preserve" y="8"><tspan x="1">x</tspan></text></svg>`
	if got != want {
		t.Errorf("inline picture\n%s\nwant\n%s", got, want)
	}
	outlines := figure("", "10")
	outlines.span["source"] = "rendered/" + rec.ToS(outlines.asset["sha256"]) + ".pdf"
	if got := imageHTML(figurePaper(outlines)); !strings.HasPrefix(got, "<img ") {
		t.Errorf("a picture without labels %s", got)
	}
}
