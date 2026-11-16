package paintmodel

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func failure(run func()) (message string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message = recovered.(rec.Failure).Message
		}
	}()
	run()
	return ""
}

func TestParseCMap(t *testing.T) {
	cmap := "begincmap\n2 beginbfchar\n<41> <0041>\n<42> <D835DC00>\nendbfchar\n1 beginbfrange\n<61> <62> <0061>\nendbfrange\nendcmap"
	got := parseCMap([]byte(cmap))
	want := rec.Map{"65": "A", "66": "𝐀", "97": "a", "98": "b"}
	if !rec.Equal(got, want) {
		t.Errorf("parseCMap = %v", got)
	}
	if message := failure(func() { parseCMap([]byte("1 beginbfchar\n<41> <0041> junk\nendbfchar")) }); message != "Unsupported PDF Unicode CMap block" {
		t.Errorf("junk: %q", message)
	}
	if message := failure(func() { parseCMap([]byte("1 begincidchar <41> 3 endcidchar")) }); message != "Inherited or CID CMaps are unsupported" {
		t.Errorf("cid: %q", message)
	}
}

func TestFontName(t *testing.T) {
	if got := FontName("ABCDEF+CMR-10_x y"); got != "cmr10xy" {
		t.Errorf("FontName = %q", got)
	}
}
