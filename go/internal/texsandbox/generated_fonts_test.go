package texsandbox

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
)

func TestMissingMetricFontsParsesWrappedAndIgnoresNonTFM(t *testing.T) {
	logText := strings.Join([]string{
		"! Font \\T2A/cmr/m/n/10=larm1000 at 10.0pt not loadable: metric data not found or",
		" bad.",
		"<to be read again>",
		"                   relax",
		"l.3 \\begin{document}",
		"! Font \\TU/lmr/m/n/10=[lmroman10-regular]:+tlig; at 10pt not loadable: metric ",
		"data not found or bad.",
		"! Font \\OT1/foo/m/n/12=larm1200 scaled 1095 not loadable: Metric (TFM) file no",
		"t found.",
		"! Font \\OT1/bad/m/n/10=../escape at 10pt not loadable: metric data not found or bad.",
		"! Font \\T2A/cmr/m/n/10=larm1000 at 10.0pt not loadable: metric data not found or bad.",
	}, "\n")
	want := []string{"larm1000", "larm1200"}
	if got := MissingMetricFonts(logText); !slices.Equal(got, want) {
		t.Fatalf("MissingMetricFonts = %v, want %v", got, want)
	}
}

func TestFontGeneratorRecoverBoundsAndNeverLoops(t *testing.T) {
	calls := []string{}
	generator := &FontGenerator{
		Limit: 2,
		Ensure: func(name string, _ time.Duration) (created, available bool) {
			calls = append(calls, name)
			return name != "failingfont", name != "failingfont"
		},
	}
	firstLog := "! Font \\a=fonta at 10pt not loadable: metric data not found or bad.\n" +
		"! Font \\b=failingfont at 10pt not loadable: metric data not found or bad.\n" +
		"! Font \\c=fontc at 10pt not loadable: metric data not found or bad.\n"
	if !generator.Recover(firstLog, time.Now().Add(time.Minute)) {
		t.Fatal("expected Recover to request a rerun after fonta succeeded")
	}
	if !slices.Equal(calls, []string{"fonta", "failingfont"}) {
		t.Fatalf("attempted calls = %v, want [fonta failingfont]", calls)
	}
	if !slices.Equal(generator.Generated, []string{"fonta"}) {
		t.Fatalf("Generated = %v, want [fonta]", generator.Generated)
	}
	// Repeat on the same log (plus fontc): neither repeats nor crosses Limit=2.
	if generator.Recover(firstLog, time.Now().Add(time.Minute)) {
		t.Fatal("expected Recover not to retry already-attempted or over-limit fonts")
	}
	if len(calls) != 2 {
		t.Fatalf("unexpected extra calls: %v", calls)
	}
}

func TestRunPassGeneratesAllMissingFontsAcrossReruns(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "pass-1.log")
	generator := &FontGenerator{
		Ensure: func(_ string, _ time.Duration) (bool, bool) { return true, true },
	}
	sequence := []string{"fonta", "fontb", "fontc"}
	passes := 0
	retries := 0
	stage := generator.RunPass(logPath, time.Now().Add(time.Minute), func() rec.Map {
		passes++
		if passes <= len(sequence) {
			line := fmt.Sprintf("! Font \\f=%s at 10pt not loadable: metric data not found or bad.\n", sequence[passes-1])
			if err := os.WriteFile(logPath, []byte(line), 0o644); err != nil {
				t.Fatal(err)
			}
			return rec.Map{"exit": int64(1), "timeout": false}
		}
		if err := os.WriteFile(logPath, []byte("Output written on main.pdf.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return rec.Map{"exit": int64(0), "timeout": false}
	}, func() {
		retries++
	})
	if !rec.Equal(stage["exit"], int64(0)) {
		t.Fatalf("final exit = %v, want 0", stage["exit"])
	}
	if passes != 4 || retries != 3 {
		t.Fatalf("passes = %d, retries = %d; want 4, 3", passes, retries)
	}
	if !slices.Equal(generator.Generated, sequence) {
		t.Fatalf("Generated = %v, want %v", generator.Generated, sequence)
	}
}

func TestRunPassDoesNotLoopOnSameFont(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "pass-1.log")
	generator := &FontGenerator{
		Ensure: func(_ string, _ time.Duration) (bool, bool) { return true, true },
	}
	passes := 0
	stage := generator.RunPass(logPath, time.Now().Add(time.Minute), func() rec.Map {
		passes++
		line := "! Font \\f=fonta at 10pt not loadable: metric data not found or bad.\n"
		if err := os.WriteFile(logPath, []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
		return rec.Map{"exit": int64(1), "timeout": false}
	}, nil)
	if !rec.Equal(stage["exit"], int64(1)) || passes != 2 {
		t.Fatalf("stage exit = %v, passes = %d; want 1, 2", stage["exit"], passes)
	}
	if !slices.Equal(generator.Generated, []string{"fonta"}) {
		t.Fatalf("Generated = %v, want [fonta]", generator.Generated)
	}
}

func TestRunPassStopsWhenCapReached(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "pass-1.log")
	generator := &FontGenerator{
		Ensure: func(_ string, _ time.Duration) (bool, bool) { return true, true },
	}
	passes := 0
	stage := generator.RunPass(logPath, time.Now().Add(time.Minute), func() rec.Map {
		passes++
		line := fmt.Sprintf("! Font \\f=font%02d at 10pt not loadable: metric data not found or bad.\n", passes)
		if err := os.WriteFile(logPath, []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
		return rec.Map{"exit": int64(1), "timeout": false}
	}, nil)
	if !rec.Equal(stage["exit"], int64(1)) {
		t.Fatalf("stage exit = %v, want 1", stage["exit"])
	}
	if len(generator.Generated) != MaxGeneratedFonts || passes != MaxGeneratedFonts+1 {
		t.Fatalf("Generated = %d, passes = %d; want %d, %d", len(generator.Generated), passes, MaxGeneratedFonts, MaxGeneratedFonts+1)
	}
}

func TestRunPassDoesNotRerunOnNonFontFailure(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "pass-1.log")
	generator := &FontGenerator{
		Ensure: func(_ string, _ time.Duration) (bool, bool) { return true, true },
	}
	passes := 0
	stage := generator.RunPass(logPath, time.Now().Add(time.Minute), func() rec.Map {
		passes++
		if err := os.WriteFile(logPath, []byte("! Undefined control sequence.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return rec.Map{"exit": int64(1), "timeout": false}
	}, nil)
	if !rec.Equal(stage["exit"], int64(1)) || passes != 1 || len(generator.Generated) != 0 {
		t.Fatalf("exit = %v, passes = %d, generated = %v; want 1, 1, []", stage["exit"], passes, generator.Generated)
	}
}

func TestWithGeneratedFontsAndSandboxMount(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	expectedDir := filepath.Join(cache, "relatex", "texmf-generated")
	env := WithGeneratedFonts(map[string]string{"TEXMFVAR": filepath.Join(cache, "var"), "TEXMFCACHE": filepath.Join(cache, "c")})
	if got := env["TEXMFAUXTREES"]; got != filepath.ToSlash(expectedDir)+"," {
		t.Fatalf("TEXMFAUXTREES = %q, want %q", got, filepath.ToSlash(expectedDir)+",")
	}

	engine := command.Executable("lualatex")
	if command.Executable("bwrap") == "" || engine == "" {
		return
	}
	if err := os.MkdirAll(expectedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	out := t.TempDir()
	wrapped := strings.Join(Current(engine).WrapWith([]string{"true"}, src, out, src, env, nil), " ")
	mount := "--ro-bind " + expectedDir + " " + expectedDir
	if !strings.Contains(wrapped, mount) || strings.Index(wrapped, mount) < strings.Index(wrapped, "--tmpfs /tmp") {
		t.Fatalf("generated-fonts dir not mounted after /tmp: %s", wrapped)
	}
}

func TestGeneratesMissingT2AMetricsAndResolvesViaKpsewhich(t *testing.T) {
	mf := command.Executable("mf")
	engine := command.Executable("lualatex")
	mktextfm := command.Executable("mktextfm")
	kpsewhich := command.Executable("kpsewhich")
	if mf == "" || engine == "" || mktextfm == "" || kpsewhich == "" {
		t.Skip("mf, lualatex, mktextfm, or kpsewhich not installed")
	}
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	work := t.TempDir()
	varDir := filepath.Join(work, "texmf-var")
	texmfCache := filepath.Join(work, "texmf-cache")
	if err := os.MkdirAll(varDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(texmfCache, 0o755); err != nil {
		t.Fatal(err)
	}
	tex := "\\documentclass[10pt]{article}\n\\usepackage[T2A]{fontenc}\n\\begin{document}\\CYRP\\end{document}\n"
	if err := os.WriteFile(filepath.Join(work, "main.tex"), []byte(tex), 0o644); err != nil {
		t.Fatal(err)
	}
	env := WithGeneratedFonts(map[string]string{
		"PATH":       command.ToolPath(engine),
		"TEXMFVAR":   varDir,
		"TEXMFCACHE": texmfCache,
	})
	logPath := filepath.Join(work, "main.log")
	args := []string{engine, "-interaction=nonstopmode", "-halt-on-error", "-no-shell-escape", "-output-directory=" + filepath.ToSlash(work), "main.tex"}
	first := command.Run(args, work, logPath, 60*time.Second, env)
	if rec.Equal(first["exit"], int64(0)) {
		t.Skip("larm1000 is already present in system TeX Live trees")
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	generator := NewFontGenerator(engine)
	if !generator.Recover(string(logBytes), time.Now().Add(time.Minute)) {
		t.Fatalf("expected Recover to generate missing T2A TFM from log:\n%s", string(logBytes))
	}
	if !slices.Contains(generator.Generated, "larm1000") || !hasTFM(GeneratedFontsDir(), "larm1000") {
		t.Fatalf("expected larm1000 generated in %s, got %v", GeneratedFontsDir(), generator.Generated)
	}
	second := command.Run(args, work, logPath, 60*time.Second, env)
	if !rec.Equal(second["exit"], int64(0)) {
		t.Fatalf("rerun failed: %v", second)
	}
	if _, statErr := os.Stat(filepath.Join(work, "main.pdf")); statErr != nil {
		t.Fatalf("expected main.pdf after rerun: %v", statErr)
	}
	cmd := exec.Command(kpsewhich, "-engine=luahbtex", "-progname=lualatex", "-must-exist", "-no-mktex=pk", "-no-mktex=mf", "-no-mktex=tex", "-no-mktex=tfm", "--", "larm1000.tfm")
	cmd.Env = command.Environment(env)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("kpsewhich failed to find larm1000.tfm: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(out.String()), "texmf-generated") {
		t.Fatalf("expected resolved path in texmf-generated, got %q", out.String())
	}
}
