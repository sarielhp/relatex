package figscan

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// traceCases are real figures without Ipe source (decision 123, step 4): books/0003's cairo,
// Asymptote, Ghostscript and dvips figures and a pdfTeX bitmap wrapper, and paper figures
// from matplotlib, gnuplot, Inkscape, Mathematica and a relatex-cut TeX drawing.
var traceCases = []struct {
	file, source string
}{
	{"cairo-smiley", SourceTrace}, {"asymptote-fr_curves_s", SourceTrace}, {"ghostscript-quadratic", SourceTrace},
	{"dvips-draw_ball", SourceTrace}, {"bitmap-fd_single", SourceBitmap}, {"cairo-normalization", SourceTrace},
	{"matplotlib-surv_slice", SourceTrace}, {"gnuplot-Fig3", SourceTrace}, {"inkscape-frey12", SourceTrace},
	{"mathematica-fg_nonAbelian", SourceTrace}, {"tex-fig-1", SourceTrace},
}

// TestTraceGolden compares the trace fallback's text and JSON forms with the checked outputs
// (checked by eye against pdftocairo renders). FIGSCAN_UPDATE=1 rewrites them.
func TestTraceGolden(t *testing.T) {
	for _, c := range traceCases {
		figure := scanCase(t, c.file, 1)
		if figure.Source != c.source {
			t.Fatalf("%s: source %q, want %q", c.file, figure.Source, c.source)
		}
		base := filepath.Join("testdata", "golden", c.file+"-1")
		compareGolden(t, base+".txt", []byte(figure.Text()))
		compareGolden(t, base+".json", figure.JSON())
	}
}

// TestTraceDeterministic scans each traced figure twice: the same bytes both times.
func TestTraceDeterministic(t *testing.T) {
	for _, c := range traceCases {
		first, second := scanCase(t, c.file, 1), scanCase(t, c.file, 1)
		if !bytes.Equal(first.JSON(), second.JSON()) || first.Text() != second.Text() {
			t.Fatalf("%s: two scans differ", c.file)
		}
	}
}

// TestTracePastEnd: mutool clamps a page past the end to the last one; Scan reports it instead.
func TestTracePastEnd(t *testing.T) {
	if _, err := Scan(mustRead(t, "gnuplot-Fig3"), 2, nil); err == nil {
		t.Fatal("page 2 of a one-page PDF scanned")
	}
}

// TestShowFile prints the text form of $FIGSCAN_FILE page 1 (a survey aid, skipped without it).
func TestShowFile(t *testing.T) {
	path := os.Getenv("FIGSCAN_FILE")
	if path == "" {
		t.Skip("FIGSCAN_FILE not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	figure, err := Scan(data, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + figure.Text())
}

// TestTracePageUsesRunScratch verifies mutool trace writes its temporary PDF under RELATEX_RUNS_DIR
// via command.Scratch and removes it on return (decision 138 E).
func TestTracePageUsesRunScratch(t *testing.T) {
	base, bin := t.TempDir(), t.TempDir()
	t.Setenv("RELATEX_RUNS_DIR", base)
	seen := filepath.Join(t.TempDir(), "seen")
	stubMutool(t, bin, seen)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := tracePage([]byte("%PDF-1.4\n"), 1); err != nil {
		t.Fatalf("tracePage: %v", err)
	}
	recorded, err := os.ReadFile(seen)
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimSpace(string(recorded))
	resolved, _ := filepath.EvalSymlinks(base)
	if !strings.HasPrefix(path, base) && !strings.HasPrefix(path, resolved) {
		t.Fatalf("tracePage wrote %s outside RELATEX_RUNS_DIR %s", path, base)
	}
	if left, _ := filepath.Glob(filepath.Join(base, "figscan-*")); len(left) != 0 {
		t.Fatalf("tracePage left temporary directories: %v", left)
	}
}

func stubMutool(t *testing.T, bin, seen string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(bin, "mutool.bat")
		body := "@echo off\r\nif \"%~1\"==\"show\" (\r\n  echo %~2> \"" + seen + "\"\r\n  echo 1\r\n  exit /b 0\r\n)\r\necho ^<document^>^<page/^>^</document^>\r\n"
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return
	}
	path := filepath.Join(bin, "mutool")
	body := "#!/bin/sh\nif [ \"$1\" = \"show\" ]; then\n  printf '%s\\n' \"$2\" > \"" + seen + "\"\n  echo 1\n  exit 0\nfi\necho '<document><page/></document>'\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
