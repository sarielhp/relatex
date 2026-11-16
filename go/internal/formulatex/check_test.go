package formulatex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/authormath"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
)

// TestCheckRoundTripsAndCaches typesets three formulas with LuaLaTeX: the two the serializer
// writes faithfully get their TeX (a bold italic letter through bm), the third does not, and a
// second run answers them all from the cache.
func TestCheckRoundTripsAndCaches(t *testing.T) {
	engine := command.TeXLiveBin("lualatex")
	if _, err := os.Stat(engine); err != nil {
		t.Skip("LuaLaTeX not available")
	}
	authormath.Root, _ = filepath.Abs("../../..")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	good := inline(`<msub><mi>𝑥</mi><mi>𝑖</mi></msub><mo lspace="0.222em" rspace="0.222em">+</mo><mn>1</mn>`)
	// A bold italic letter: amsmath's own \boldsymbol sets it in an mpadded, bm's does not.
	bold := inline(`<mi>𝑨</mi><mo lspace="0.222em" rspace="0.222em">+</mo><mi>𝝀</mi>`)
	// An operator marked bold: TeX sets no mathvariant on an operator, so the round trip fails.
	tight := inline(`<mi>𝑥</mi><mo mathvariant="bold" lspace="0.222em" rspace="0.222em">+</mo><mi>𝑦</mi>`)
	nodes := []rec.Map{{"type": "math", "mathml": good}, {"type": "math", "mathml": tight}, {"type": "math", "mathml": good},
		{"type": "math", "mathml": bold}}
	summary := Check(nodes, t.TempDir(), engine)
	if summary.Formulas != 4 || summary.Verified != 3 || summary.Typeset != 3 {
		t.Fatalf("summary = %+v", summary)
	}
	tex, ok := nodes[0]["formula_tex"].(rec.Map)
	if !ok || tex["text"] != "x_i+1" {
		t.Errorf("formula_tex = %v", nodes[0]["formula_tex"])
	}
	if tex, ok := nodes[3]["formula_tex"].(rec.Map); !ok || tex["text"] != `\boldsymbol{A}+\boldsymbol{\lambda}` {
		t.Errorf("bold italic formula_tex = %v", nodes[3]["formula_tex"])
	}
	if _, ok := nodes[1]["formula_tex"]; ok {
		t.Errorf("unverified formula has TeX")
	}
	again := []rec.Map{{"type": "math", "mathml": good}, {"type": "math", "mathml": tight}, {"type": "math", "mathml": bold}}
	if summary := Check(again, t.TempDir(), engine); summary.Typeset != 0 || summary.Verified != 2 {
		t.Errorf("cached summary = %+v", summary)
	}
}
