package mathexport

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

const leftRight = `<math xmlns="http://www.w3.org/1998/Math/MathML"><mi>G</mi><mrow>` +
	`<mo fence="true" lspace="0" rspace="0" symmetric="true">(</mo>%s` +
	`<mo fence="true" lspace="0" rspace="0" symmetric="true">)</mo></mrow></math>`

func TestOrdinaryFencesAreNotStretchy(t *testing.T) {
	html := ProjectPage(ParsePaper(strings.Replace(leftRight, "%s", "<mi>m</mi><msup><mi>x</mi><mn>2</mn></msup>", 1)), rec.Map{}, nil)
	if got := strings.Count(html, "stretchy='false'"); got != 2 {
		t.Errorf("want both fences not stretchy, got %d in %s", got, html)
	}
}

func TestTallFencesStayStretchy(t *testing.T) {
	for _, content := range []string{"<mfrac><mi>m</mi><mi>n</mi></mfrac>", "<mo>∑</mo><mi>m</mi>"} {
		html := ProjectPage(ParsePaper(strings.Replace(leftRight, "%s", content, 1)), rec.Map{}, nil)
		if strings.Contains(html, "stretchy") {
			t.Errorf("tall content %s lost its stretchy fences: %s", content, html)
		}
	}
}
