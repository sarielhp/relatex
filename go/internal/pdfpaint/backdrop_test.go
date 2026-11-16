package pdfpaint

import (
	"reflect"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// backdropTrace: white text on a blue box (a tcolorbox title) and beside it on the paper; white
// text on an image (its colour unknown); a translucent box; and black text, which is never faint.
// test/pdf_backdrops_test.rb reads the same trace.
const backdropTrace = `<document>` +
	`<page number="1" mediabox="0 0 100 100"><fill_path winding="nonzero" colorspace="DeviceRGB" color="0 0 1" transform="1 0 0 1 0 0">` +
	`<moveto x="10" y="10"/><lineto x="60" y="10"/><curveto x1="60" y1="20" x2="60" y2="30" x3="55" y3="30"/><lineto x="10" y="30"/><closepath/></fill_path>` +
	`<fill_text colorspace="DeviceGray" color="1" transform="1 0 0 1 0 0"><span trm="10 0 0 10"><g unicode="T" x="12" y="15" adv=".5"/><g unicode="i" x="17" y="15" adv=".3"/>` +
	`<g unicode=" " x="20" y="15" adv=".3"/><g unicode="H" x="70" y="70" adv=".5"/></span></fill_text>` +
	`<fill_text colorspace="DeviceGray" color="0" transform="1 0 0 1 0 0"><span trm="10 0 0 10"><g unicode="b" x="12" y="15" adv=".5"/></span></fill_text></page>` +
	`<page number="2" mediabox="0 0 100 100"><fill_path winding="nonzero" colorspace="DeviceRGB" color="0 0 1" transform="1 0 0 1 0 0">` +
	`<moveto x="0" y="0"/><lineto x="50" y="0"/><lineto x="50" y="50"/><closepath/></fill_path>` +
	`<fill_image transform="40 0 0 40 0 0"/>` +
	`<fill_path winding="nonzero" colorspace="DeviceRGB" color="1 0 0" alpha=".5" transform="1 0 0 1 0 0"><moveto x="40" y="40"/><lineto x="50" y="50"/></fill_path>` +
	`<fill_text colorspace="DeviceGray" color="1" transform="1 0 0 1 0 0"><span trm="10 0 0 10"><g unicode="x" x="10" y="10" adv=".5"/>` +
	`<g unicode="y" x="42" y="42" adv=".2"/><g unicode="z" x="45" y="2" adv=".2"/></span></fill_text></page></document>`

// The text paint proof records, for each faint ink, its text on each fill painted behind it
// (decision 113): white on the blue box, not on the paper, not on an image or a translucent fill.
func TestTextPaintRecordsTheBackdropsOfFaintText(t *testing.T) {
	_, _, _, canvas, backdrops := inspectTrace([]byte(backdropTrace), false)
	want := []any{rec.Map{"ink": "#ffffff", "fill": "#0000ff", "text": "Tiz"}}
	if canvas != "" || !reflect.DeepEqual(backdrops, want) {
		t.Errorf("canvas %q, backdrops %v", canvas, backdrops)
	}
	// the canvas, a fill the ink is invisible on and one it reads worse on than on paper are none
	reader := &backdropReader{}
	reader.texts = map[[2]string]*strings.Builder{}
	for _, key := range [][2]string{{"#ffffff", "#000000"}, {"#ffffff", "#fefefe"}, {"#ffff00", "#eeeeee"}, {"#ffff00", "#000080"}} {
		reader.texts[key] = &strings.Builder{}
		reader.texts[key].WriteString("a")
	}
	kept := reader.backdrops("#000000")
	if len(kept) != 1 || rec.AsMap(kept[0])["fill"] != "#000080" {
		t.Errorf("kept %v", kept)
	}
}
