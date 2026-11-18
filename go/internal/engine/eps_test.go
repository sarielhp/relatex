package engine

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A profile of 176 bytes with one text tag (offset 156, 16 bytes: "ab", its terminator and
// leftover memory) and one curve tag whose zero-free bytes stay.
func TestClearProfilePadding(t *testing.T) {
	header := make([]byte, 132)
	binary.BigEndian.PutUint32(header, 176)
	copy(header[36:], "acsp")
	binary.BigEndian.PutUint32(header[128:], 2)
	tags := []byte{}
	for _, entry := range []struct {
		signature      string
		offset, length uint32
	}{{"cprt", 156, 16}, {"rTRC", 172, 4}} {
		tags = append(tags, entry.signature...)
		tags = binary.BigEndian.AppendUint32(tags, entry.offset)
		tags = binary.BigEndian.AppendUint32(tags, entry.length)
	}
	prefix := append([]byte("1 0 obj\nstream\n"), append(header, tags...)...)
	written := append(bytes.Clone(prefix), "text\x00\x00\x00\x00ab\x00XYZW\x00XYZW\nendstream\n"...)
	path := filepath.Join(t.TempDir(), "expanded.pdf")
	if err := os.WriteFile(path, written, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := clearProfilePadding(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := append(bytes.Clone(prefix), "text\x00\x00\x00\x00ab\x00\x00\x00\x00\x00\x00XYZW\nendstream\n"...)
	if !bytes.Equal(got, want) {
		t.Errorf("clearProfilePadding:\n got %q\nwant %q", got, want)
	}
}

// xfig's X.pstex is EPS under another name: it converts like an .eps, to X-pstex-converted-to.pdf, and
// the plain X.pdf the graphics rule of pstexRule reads is made beside it. Its text file X.pstex_t is no figure.
func TestPstexFiguresConvertLikeEPS(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "main.tex"), []byte(`\includegraphics{one.ps}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dudv.pstex", "dudv.pstex_t", "plot.eps", "one.ps", "two.ps", "two.pdf"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte("%!PS-Adobe-3.0 EPSF-3.0\n"), 0o644); err != nil {
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
	prepareEPSAssets(source, run)
	for _, want := range []string{"dudv-pstex-converted-to.pdf", "dudv.pdf", "plot-eps-converted-to.pdf", "plot.pdf", "one-ps-converted-to.pdf", "one.pdf"} {
		if !isFile(filepath.Join(source, want)) {
			t.Errorf("%s was not made", want)
		}
	}
	if isFile(filepath.Join(source, "two-ps-converted-to.pdf")) {
		t.Error("a .ps no source names was converted")
	}
	if isFile(filepath.Join(source, "dudv.pstex_t-pstex-converted-to.pdf")) {
		t.Error("the .pstex_t text file was converted")
	}
	if got := epsPDFName("a/b.PSTEX"); got != "a/b-pstex-converted-to.pdf" {
		t.Errorf("epsPDFName = %q", got)
	}
}

// Both builds carry the .pstex graphics rule, the control build too: without it stock LuaLaTeX stops with
// "Unknown graphics extension: .pstex" before the paper is read (enet, a paper made for dvips).
func TestEveryBuildDeclaresThePstexGraphicsRule(t *testing.T) {
	p := &Probe{main: "paper.tex"}
	for _, strategy := range []string{"control", "external"} {
		got := p.wrapper(strategy)
		if !strings.Contains(got, `\epstopdfDeclareGraphicsRule{.pstex}{pdf}{.pdf}{\ETE@epstopdf{#1}}`) || !strings.HasSuffix(got, "\\input{paper.tex}\n") {
			t.Errorf("%s wrapper = %q", strategy, got)
		}
	}
}
