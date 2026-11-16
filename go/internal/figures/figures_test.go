package figures

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// The expectations are FigureNames' (test/figure_names_test.rb): the LaTeX source name,
// numbered in reading order when the paper draws the figure itself, unique by content.
func TestSourceStemSlugsTheLaTeXFileName(t *testing.T) {
	cases := map[string]string{
		"figs/plot.pdf":                           "plot",
		"figs/My Plot (final).pdf":                "My-Plot-final",
		".hidden.png":                             "hidden",
		"figs/ccbar_HEPMC_SND_HGTD_floorOnly.pdf": "ccbar_HEPMC_SND",
		"x/experiment-plots-final-v3.pdf":         "experiment-plots",
		"abcdefghijklmnopqrstuvwxyz.pdf":          "abcdefghijklmnop",
	}
	for origin, want := range cases {
		if got, ok := SourceStem(map[string]any{"source": origin}); !ok || got != want {
			t.Errorf("SourceStem(%q) = %q, %v, want %q", origin, got, ok, want)
		}
	}
	if _, ok := SourceStem(map[string]any{"source": "rendered/" + repeat("0", 64) + ".pdf"}); ok {
		t.Error("a content name is not a source name")
	}
}

func TestStemNumbersFiguresAndSkipsTakenStems(t *testing.T) {
	names := New()
	if got := names.Stem("rendered", map[string]any{"source": "rendered/" + repeat("0", 64) + ".pdf"}); got != "fig-1" {
		t.Errorf("numbered stem = %q, want fig-1", got)
	}
	if got := names.Stem("copy", map[string]any{"source": "figs/fig-1.pdf"}); got != "fig-1" {
		t.Errorf("taken number = %q, want fig-1", got)
	}
	if got := names.Stem("second", nil); got != "fig-2" {
		t.Errorf("next number = %q, want fig-2", got)
	}
	if got := names.Stem("rendered", nil); got != "fig-1" {
		t.Errorf("a stem is assigned once per figure, got %q", got)
	}
}

func TestClaimReusesContentAndNumbersOtherContent(t *testing.T) {
	names := New()
	first := names.Claim("first", "plot.svg")
	if second := names.Claim("first", "plot.svg"); second != first {
		t.Errorf("same content took %q after %q", second, first)
	}
	if got := names.Claim("second", "plot.svg"); got != "plot-2.svg" {
		t.Errorf("second content = %q, want plot-2.svg", got)
	}
	if got := names.Claim("third", "plot.svg"); got != "plot-3.svg" {
		t.Errorf("third content = %q, want plot-3.svg", got)
	}
}

func TestClaimTriesAlternativesBeforeANumber(t *testing.T) {
	names := New()
	if got := names.Claim("shown", "fig-1.png", "fig-2.png"); got != "fig-1.png" {
		t.Errorf("free name = %q, want fig-1.png", got)
	}
	if got := names.Claim("other", "fig-1.png", "fig-2.png"); got != "fig-2.png" {
		t.Errorf("alternative = %q, want fig-2.png", got)
	}
	if got := names.Claim("third", "fig-1.png"); got != "fig-1-2.png" {
		t.Errorf("numbered = %q, want fig-1-2.png", got)
	}
}

// Two author files whose names differ only in case would overwrite each other in figs/ on
// Windows and macOS: the export refuses them on every platform, naming both (decision 129,
// amendment 2).
func TestStemRefusesAuthorNamesThatDifferOnlyInCase(t *testing.T) {
	names := New()
	names.Stem("upper", map[string]any{"source": "figs/Plot.pdf"})
	if got := names.Stem("again", map[string]any{"source": "other/Plot.pdf"}); got != "Plot" {
		t.Errorf("same-case stem = %q, want Plot (Claim numbers it)", got)
	}
	var err error
	func() {
		defer rec.Recover(&err)
		names.Stem("lower", map[string]any{"source": "img/plot.png"})
	}()
	if err == nil {
		t.Fatal("plot.png after Plot.pdf was accepted")
	}
	for _, want := range []string{"input.unsafe", `"figs/Plot.pdf"`, `"img/plot.png"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %s", err, want)
		}
	}
}

// Claim and the numbering compare names without case, so our own names never collide in case
// with an author's.
func TestClaimAndNumbersIgnoreCase(t *testing.T) {
	names := New()
	names.Stem("drawn", nil)
	if got := names.Claim("drawn", "fig-1.svg"); got != "fig-1.svg" {
		t.Fatalf("numbered = %q", got)
	}
	if got := names.Stem("author", map[string]any{"source": "Fig-2.pdf"}); got != "Fig-2" {
		t.Fatalf("author stem = %q", got)
	}
	if got := names.Stem("next", nil); got != "fig-3" {
		t.Errorf("number after Fig-2 = %q, want fig-3", got)
	}
	if got := names.Claim("other", "FIG-1.svg"); got != "FIG-1-2.svg" {
		t.Errorf("claim of FIG-1.svg beside fig-1.svg = %q, want FIG-1-2.svg", got)
	}
}

func repeat(text string, count int) string {
	result := ""
	for range count {
		result += text
	}
	return result
}
