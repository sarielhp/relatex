package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/pdffonts"
	"github.com/sarielhp/relatex/internal/pdfua"
)

func reportWith(codes ...string) ojson.Object {
	found := []pdfua.Diagnostic{}
	for _, code := range codes {
		found = append(found, pdfua.Diagnostic{Code: code, Message: code + " happened"})
	}
	return ojson.Object{{"status", "tagged"}, {"diagnostics", pdfua.DiagnosticList(found)}}
}

// --pdf-ua accepts auto and route2, both route 2 (route 1 was removed); nothing else.
func TestPDFUARoutes(t *testing.T) {
	for _, route := range []string{"auto", "route2"} {
		if !slices.Contains(PDFUARoutes, route) {
			t.Errorf("missing route %q", route)
		}
	}
	if len(PDFUARoutes) != 2 {
		t.Errorf("routes %v", PDFUARoutes)
	}
}

// The unwrap's diagnostic names the figure and the face the tagged PDF embeds; without a known
// figure (a font the layout does not attribute) it says an included figure's font.
func TestUnwrappedMessageNamesTheFigureAndItsFace(t *testing.T) {
	unwrapped := pdffonts.Unwrapped{Font: "DCWGQU+CambriaMath", Face: 1, Name: "Cambria Math", Faces: 2,
		Before: 1792664, After: 1574876}
	want := "figure Figures/framework.pdf: font DCWGQU+CambriaMath was embedded as a whole TrueType " +
		"collection (2 faces, 1 792 664 bytes); the tagged PDF embeds its face \"Cambria Math\" alone " +
		"(1 574 876 bytes, glyphs unchanged); re-export the figure with the font embedded as a single font"
	if got := unwrappedMessage(unwrapped, "Figures/framework.pdf"); got != want {
		t.Errorf("message %q, want %q", got, want)
	}
	if got := unwrappedMessage(unwrapped, ""); !strings.HasPrefix(got, "an included figure's font DCWGQU+CambriaMath was") {
		t.Errorf("message without a figure %q", got)
	}
}
