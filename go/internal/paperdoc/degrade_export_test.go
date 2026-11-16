package paperdoc

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/rec"
)

func degradeBuilder(t *testing.T) *builder {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := diagnostics.LoadProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	return &builder{diagnostics: diagnostics.New(profile, nil, false)}
}

func TestInvisibleMathDegradesToADiagnostic(t *testing.T) {
	b := degradeBuilder(t)
	math := rec.Map{"status": "verified", "diagnostics": []any{rec.Map{"code": "math_without_visible_nodes", "formulas": []any{int64(195), int64(7)}}}}
	if !b.degradeInvisibleMath(math) {
		t.Fatal("formulas without visible nodes must degrade")
	}
	lines := strings.Join(b.diagnostics.Lines("whatever"), "\n")
	if !strings.Contains(lines, "math.nothing_visible") || !strings.Contains(lines, "195, 7") {
		t.Fatalf("diagnostics: %s", lines)
	}
	other := rec.Map{"status": "unresolved", "diagnostics": []any{rec.Map{"code": "missing_mathml", "formulas": []any{int64(1)}}}}
	if b.degradeInvisibleMath(other) {
		t.Fatal("another unresolved math code must still stop the export")
	}
}

func TestUnsupportedGraphicSourceDegrades(t *testing.T) {
	if degradable["unsupported_graphic_source"] != "figure.source_unsupported" {
		t.Fatal("unsupported_graphic_source must degrade")
	}
	b := degradeBuilder(t)
	b.degrade(rec.Map{"code": "unsupported_graphic_source", "reason": "unbound image placement"})
	lines := strings.Join(b.diagnostics.Lines("whatever"), "\n")
	if !strings.Contains(lines, "figure.source_unsupported") || !strings.Contains(lines, "unbound image placement") {
		t.Fatalf("diagnostics: %s", lines)
	}
}

// A label whose only references are typeset inside formulas (a paper's \showmark{m:a} printing
// c_{\ref{m:a}}) is referenced: without an owner it loses visible links, an alert, not a label
// nothing refers to.
func TestLabelReferencedOnlyInsideAFormulaIsReferenced(t *testing.T) {
	b := degradeBuilder(t)
	span := rec.Map{"type": "reference", "id": "r1", "reference": int64(1), "text": "1", "target": "m:a"}
	b.reading = rec.Map{"blocks": []any{}, "notes": rec.Map{"notes": []any{}},
		"math_references": []any{rec.Map{"occurrence": int64(3), "content": []any{span}}}}
	b.degrade(rec.Map{"code": "unresolved_label_owner", "label": "m:a"})
	b.degrade(rec.Map{"code": "unresolved_label_owner", "label": "m:b"})
	lines := strings.Join(b.diagnostics.Lines("whatever"), "\n")
	if !strings.Contains(lines, "label m:a: alert[label.unresolved_owner]") ||
		!strings.Contains(lines, "label m:b: whatever[label.unowned_unreferenced]") {
		t.Fatalf("diagnostics: %s", lines)
	}
}
