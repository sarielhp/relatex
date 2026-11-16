package pdfua

import (
	"strings"
	"testing"
)

// veraPDFReport is the 0802 verdict, cut to the rule and context pdfua.figure_font reads.
const veraPDFReport = `{"report":{"jobs":[{"validationResult":{"compliant":false,"profileName":"PDF/UA-2",
	"details":{"ruleSummaries":[{"clause":"8.4.5.5.1","testNumber":1,"status":"failed","failedChecks":1,
	"description":"The font program is not embedded","checks":[{"status":"failed",
	"context":"root/document[0]/pages[2](10 0 obj PDPage)/contentStream[0](667 0 obj PDSemanticContentStream)/operators[12]/xObject[0]/contentStream[0](1736 0 obj PDSemanticContentStream)/operators[740]/font[0](DCWGQU+CambriaMath)/DescendantFonts[0](DCWGQU+CambriaMath)",
	"errorMessage":"The font program is not embedded"}]}]}}}]}}`

// frameworkFont is the font of papers3/0802's Figures/framework.pdf.
var frameworkFont = FigureFont{Source: "Figures/framework.pdf", Font: "DCWGQU+CambriaMath", Creator: "WPS 演示", Collection: true}

func TestReadValidationKeepsFailedCheckContexts(t *testing.T) {
	validation := readValidation([]byte(veraPDFReport))
	if validation.Status != "failed" || len(validation.Checks) != 1 {
		t.Fatalf("%+v", validation)
	}
	check := validation.Checks[0]
	if check.Clause != "8.4.5.5.1" || check.Test != 1 || !strings.Contains(check.Context, "font[0](DCWGQU+CambriaMath)") ||
		check.Message != "The font program is not embedded" {
		t.Fatalf("check %+v", check)
	}
}

func TestFigureFontDiagnosticsNamesFigureFontRuleAndCreator(t *testing.T) {
	validation := readValidation([]byte(veraPDFReport))
	found := FigureFontDiagnostics(validation, []FigureFont{frameworkFont})
	if len(found) != 1 {
		t.Fatalf("diagnostics %+v", found)
	}
	diagnostic := found[0]
	if diagnostic.Code != "pdfua.figure_font" || diagnostic.Magnitude != 1 {
		t.Fatalf("diagnostic %+v", diagnostic)
	}
	for _, want := range []string{"Figures/framework.pdf", "DCWGQU+CambriaMath", "8.4.5.5.1-1", "WPS 演示",
		"The font program is not embedded", "TrueType collection", "re-export the figure"} {
		if !strings.Contains(diagnostic.Message, want) {
			t.Fatalf("message lacks %q: %s", want, diagnostic.Message)
		}
	}
}

func TestFigureFontDiagnosticsIgnoresTeXFontsAndOtherChecks(t *testing.T) {
	validation := readValidation([]byte(veraPDFReport))
	other := FigureFont{Source: "Figures/framework.pdf", Font: "NimbusRomNo9L-Regu", Creator: "TeX"}
	if found := FigureFontDiagnostics(validation, []FigureFont{other}); len(found) != 0 {
		t.Fatalf("a TeX font got a figure diagnostic: %+v", found)
	}
	noFont := readValidation([]byte(`{"report":{"jobs":[{"validationResult":{"compliant":false,"profileName":"PDF/UA-2",
		"details":{"ruleSummaries":[{"clause":"5","testNumber":1,"status":"failed","failedChecks":1,
		"description":"identification","checks":[{"status":"failed","context":"root/document[0]","errorMessage":"no PDF/UA identification"}]}]}}}]}}`))
	if found := FigureFontDiagnostics(noFont, []FigureFont{frameworkFont}); len(found) != 0 {
		t.Fatalf("a check without a font got a figure diagnostic: %+v", found)
	}
}
