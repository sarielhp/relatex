package mathexport

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestAnnotationTeXPrefersTheAuthor(t *testing.T) {
	formula := rec.Map{"text": "x & =1", "environment": "align*"}
	node := rec.Map{"formula_tex": formula}
	if got := AnnotationTeX(node); got != "\\begin{align*}\nx & =1\n\\end{align*}" {
		t.Errorf("AnnotationTeX = %q", got)
	}
	node["tex"] = rec.Map{"status": "verified", "text": "\\myx=1"}
	if got := AnnotationTeX(node); got != "\\myx=1" {
		t.Errorf("AnnotationTeX with author TeX = %q", got)
	}
	node["tex"] = rec.Map{"status": "failed", "text": "\\myx=1"}
	if got := AnnotationTeX(node); !strings.HasPrefix(got, "\\begin{align*}") {
		t.Errorf("AnnotationTeX with failed author TeX = %q", got)
	}
	if got := AnnotationTeX(rec.Map{}); got != "" {
		t.Errorf("AnnotationTeX without TeX = %q", got)
	}
}

func TestProjectFormulaAnnotatesFormulaTeX(t *testing.T) {
	root := ParsePaper(`<math xmlns="http://www.w3.org/1998/Math/MathML"><mi>𝑥</mi></math>`)
	html := ProjectFormula(root, rec.Map{"formula_tex": rec.Map{"text": "x"}}, nil)
	if !strings.Contains(html, `<annotation encoding='application/x-tex'>x</annotation>`) {
		t.Errorf("ProjectFormula = %s", html)
	}
}
