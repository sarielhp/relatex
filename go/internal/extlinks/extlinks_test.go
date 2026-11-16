package extlinks

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestValidateWithoutLinks(_ *testing.T) {
	Validate(rec.Map{"block_order": []any{}, "nodes": rec.Map{}})
}

func TestProjectWithoutCollection(t *testing.T) {
	if Project(rec.Map{"diagnostics": []any{}}, rec.Map{}, nil) != nil {
		t.Error("a paper without external links projects no graph")
	}
}

func TestMathText(t *testing.T) {
	if got := MathText(`<math xmlns="http://www.w3.org/1998/Math/MathML"><mi>x</mi><mo>&lt;</mo><mn>1</mn></math>`); got != "x<1" {
		t.Errorf("math text %q", got)
	}
	// A square root prints its radical sign, as PaperFrontmatter.math_text reads it.
	if got := MathText(`<math><mfrac><msup><mi>z</mi><mi>k</mi></msup><msqrt><mi>k</mi><mo>!</mo></msqrt></mfrac></math>`); got != "zk√k!" {
		t.Errorf("square root math text %q", got)
	}
}
