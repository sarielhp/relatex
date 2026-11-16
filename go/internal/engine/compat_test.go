package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func shimsOf(requests []rec.Map) []string {
	shims := []string{}
	for _, request := range requests {
		shims = append(shims, request["shim"].(string))
	}
	return shims
}

func TestCompatibilityRequestsReadTheUndefinedName(t *testing.T) {
	log := "! Undefined control sequence.\nl.1 \\pdfoutput\n               =1\n" +
		"! Undefined control sequence.\n\\xP@literal #1->\\pdfsave\n          \\pdfliteral {#1}\n" +
		"! Undefined control sequence.\nl.38     \\setsansfont\n                     {CMU Sans Serif}\n" +
		"! Undefined control sequence.\nl.9 \\XeTeXlinebreaklocale\n"
	requests := compatibilityRequests(log)
	if got := shimsOf(requests); len(got) != 3 || got[0] != "pdftex" || got[1] != "fontspec" || got[2] != "xetex" {
		t.Fatalf("shims %v", got)
	}
	if requests[0]["cause"] != `undefined \pdfoutput` {
		t.Fatalf("cause %v", requests[0]["cause"])
	}
}

func TestCompatibilityRequestsIgnoreOtherNamesAndTheEngineIdentity(t *testing.T) {
	log := "! Undefined control sequence.\nl.4 \\foo\n! Undefined control sequence.\nl.5 \\XeTeXrevision\n"
	if got := compatibilityRequests(log); len(got) != 0 {
		t.Fatalf("requests %v", got)
	}
}

func TestCompatibilityRequestsReadPackageRefusals(t *testing.T) {
	log := "! Package xypdf Error: pdfTeX version 1.40.0 or higher is needed for the xypdf\n" +
		"! LaTeX Error: *** this package currently works only with XeTeX ***\n"
	if got := shimsOf(compatibilityRequests(log)); len(got) != 2 || got[0] != "pdftex" || got[1] != "xetex" {
		t.Fatalf("shims %v", got)
	}
}

func TestCompatibilityRequestsReadHyperrefsDVIDriverRefusal(t *testing.T) {
	log := "! Package hyperref Error: Wrong DVI mode driver option `dvipdfmx',\n" +
		"(hyperref)                because pdfTeX or LuaTeX is running in PDF mode.\n"
	requests := compatibilityRequests(log)
	if got := shimsOf(requests); len(got) != 1 || got[0] != "hyperref" {
		t.Fatalf("shims %v", got)
	}
	if requests[0]["cause"] != "Package hyperref Error: Wrong DVI mode driver option `dvipdfmx'" {
		t.Fatalf("cause %v", requests[0]["cause"])
	}
}

func TestCompatibilityLineOrdersTheShims(t *testing.T) {
	p := &Probe{compatibility: []rec.Map{{"shim": "fontspec"}, {"shim": "pdftex"}}}
	if got := p.compatibilityLine(); got != "\\RequirePackage[pdftex,fontspec]{relatex-compat}\n" {
		t.Fatalf("line %q", got)
	}
	if got := (&Probe{}).compatibilityLine(); got != "" {
		t.Fatalf("plain line %q", got)
	}
}

func TestSettlesOnlyAFailedPassThatWroteTheAuxWithoutAFatalStop(t *testing.T) {
	output := t.TempDir()
	b := build{output: output, job: "main"}
	log := filepath.Join(output, "pass-1.log")
	failed := rec.Map{"exit": int64(1), "timeout": false}
	write := func(path, text string) {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(log, "! LaTeX Error: Counter too large.\n")
	if settles(failed, b, log) {
		t.Fatal("settled without an aux file")
	}
	write(filepath.Join(output, "main.aux"), "\\relax\n")
	if !settles(failed, b, log) {
		t.Fatal("did not settle")
	}
	if settles(rec.Map{"exit": int64(0), "timeout": false}, b, log) {
		t.Fatal("a clean pass settles")
	}
	write(log, "! Emergency stop.\n")
	if settles(failed, b, log) {
		t.Fatal("settled a fatal stop")
	}
}

// Decision 082, amendment 1: an 8-bit text family with no TU shapes (times' ptm) falls back to Latin
// Modern regular with only a warning; the first pass asks for t1text whether or not it compiled. A
// substitution within Latin Modern itself asks for nothing.
func TestWarningRequestsReadATextFamilyThatFellBack(t *testing.T) {
	log := "LaTeX Font Warning: Font shape `TU/ptm/m/n' undefined\n(Font)              using `TU/lmr/m/n' instead on input line 3.\n"
	if got := shimsOf(warningRequests(log)); len(got) != 1 || got[0] != "t1text" {
		t.Fatalf("warning requests %v", got)
	}
	if got := shimsOf(compatibilityRequests(log)); len(got) != 1 || got[0] != "t1text" {
		t.Fatalf("failed-pass requests %v", got)
	}
	if got := warningRequests("LaTeX Font Warning: Font shape `TU/lmr/bx/sc' undefined\n"); len(got) != 0 {
		t.Fatalf("a Latin Modern substitution asked for %v", got)
	}
}
