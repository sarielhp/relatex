package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// Decision 194: an .ipe graphic is the PDF or EPS Ipe wrote under another name; anything else becomes a placeholder
// box, so that the paper builds instead of stopping with "Unknown graphics extension: .ipe".
func TestIpeFiguresConvertByContent(t *testing.T) {
	source := t.TempDir()
	files := map[string]string{
		"main.tex":    `\includegraphics{a.ipe}\includegraphics{b.ipe}\includegraphics{c.ipe}\input{d.ipe}`,
		"a.ipe":       "%PDF-1.4\n",
		"b.ipe":       "%!PS-Adobe-2.0 EPSF-1.2\n",
		"c.ipe":       "<?xml version=\"1.0\"?><ipe>",
		"d.ipe":       "%!PS-Adobe-2.0 EPSF-1.2\n",
		"unnamed.ipe": "%!PS-Adobe-2.0 EPSF-1.2\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(source, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(tool string, arguments []string, _ string) rec.Map {
		switch tool {
		case "epstopdf":
			_ = os.WriteFile(strings.TrimPrefix(arguments[0], "--outfile="), []byte("%PDF-1.5"), 0o644)
		case "qpdf":
			_ = os.WriteFile(arguments[len(arguments)-1], []byte("%PDF-1.5 normalized"), 0o644)
		}
		return rec.Map{"exit": int64(0), "timeout": false}
	}
	got := map[string]string{}
	for _, entry := range ipeRecord(prepareEPSAssets(source, run)) {
		got[rec.ToS(rec.AsMap(entry)["path"])] = rec.ToS(rec.AsMap(entry)["how"])
	}
	want := map[string]string{"a.ipe": "PDF", "b.ipe": "EPS", "c.ipe": "placeholder", "d.ipe": "EPS"}
	if len(got) != len(want) {
		t.Errorf("ipe record = %v, want %v", got, want)
	}
	for name, how := range want {
		if got[name] != how {
			t.Errorf("%s: %q, want %q", name, got[name], how)
		}
	}
	for _, name := range []string{"a-ipe-converted-to.pdf", "a.pdf", "c-ipe-converted-to.pdf", "c.pdf"} {
		if !isFile(filepath.Join(source, name)) {
			t.Errorf("%s was not made", name)
		}
	}
	if isFile(filepath.Join(source, "unnamed-ipe-converted-to.pdf")) {
		t.Error("an .ipe no source names was converted")
	}
	placeholder := placeholderPDF()
	if !bytes.HasPrefix(placeholder, []byte("%PDF-1.4")) || !bytes.HasSuffix(placeholder, []byte("%%EOF\n")) {
		t.Errorf("placeholder is no PDF: %q", placeholder)
	}
}

func TestEveryBuildDeclaresTheIpeGraphicsRule(t *testing.T) {
	p := &Probe{main: "paper.tex"}
	if got := p.wrapper("control"); !strings.Contains(got, `\epstopdfDeclareGraphicsRule{.ipe}{pdf}{.pdf}{\ETE@epstopdf{#1}}`) {
		t.Errorf("wrapper = %q", got)
	}
}

// Decision 194: a LaTeX 2.09 package TeX Live lacks asks for its substitute shim.
func TestMissingOldPackagesRequestTheirShims(t *testing.T) {
	for shim, name := range map[string]string{"psfig": "psfig.sty", "ltexpprt": "ltexpprt.sty"} {
		log := "! LaTeX Error: File `" + name + "' not found.\n\nType X to quit or <RETURN> to proceed,\n"
		requests := compatibilityRequests(log)
		if len(requests) != 1 || requests[0]["shim"] != shim {
			t.Errorf("%s: requests = %v", name, requests)
		}
		if got := (&Probe{compatibility: requests}).compatibilityLine(); got != "\\RequirePackage["+shim+"]{relatex-compat}\n" {
			t.Errorf("%s: line = %q", name, got)
		}
	}
	if requests := compatibilityRequests("! LaTeX Error: File `other.sty' not found.\n"); len(requests) != 0 {
		t.Errorf("other.sty requested %v", requests)
	}
}
