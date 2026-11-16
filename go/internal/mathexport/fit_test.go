package mathexport

import (
	"strings"
	"testing"
)

const stackedRows = `<math xmlns="http://www.w3.org/1998/Math/MathML"><mpadded width="74pt" height="9pt" depth="2pt"><mrow>` +
	`<mpadded lspace="-109pt" width="0pt" voffset="1pt"><mi>a</mi></mpadded>` +
	`<mpadded lspace="0pt" width="0pt"><mi>b</mi></mpadded></mrow></mpadded></math>`

func TestFitToPageReachesLeftOfStackedRows(t *testing.T) {
	root := ParsePaper(stackedRows)
	FitToPage(root, 12*65_536)
	box := root.Descendants()[0]
	if box.Get("lspace") != "109.000000pt" || box.Get("width") != "183.000000pt" {
		t.Fatalf("stack box: lspace %q width %q", box.Get("lspace"), box.Get("width"))
	}
}

func TestFitToPageScalesPointsToThePageText(t *testing.T) {
	root := ParsePaper(`<math xmlns="http://www.w3.org/1998/Math/MathML"><mpadded lspace="10pt" width="0pt" voffset="5pt" ` +
		`height="1em"><mtext>x</mtext></mpadded><mspace width="9pt"/></math>`)
	FitToPage(root, 10*65_536)
	out := root.String()
	for _, want := range []string{"lspace='12.000000pt'", "voffset='6.000000pt'", "height='1em'", "width='10.800000pt'"} {
		if !strings.Contains(out, want) {
			t.Errorf("%s missing in %s", want, out)
		}
	}
}

func TestFitToPageLeavesTwelvePointDocuments(t *testing.T) {
	root := ParsePaper(`<math xmlns="http://www.w3.org/1998/Math/MathML"><mspace width="9pt"/></math>`)
	before := root.String()
	FitToPage(root, 12*65_536)
	if root.String() != before {
		t.Fatalf("changed: %s", root.String())
	}
}
