package cli

import (
	"slices"
	"testing"
)

// formatsOf is the formats a conversion makes for options as the command line left them.
func formatsOf(t *testing.T, options map[string]any) []string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	paper := t.TempDir()
	options["output"] = t.TempDir()
	applyUserConfig(options, paper)
	return textList(options["formats"].([]any))
}

// A format flag selects the formats: --typst makes Typst alone, not Markdown and HTML as well; with
// none the configured formats apply; --format and flags together make exactly their union.
func TestFormatFlagsSelectTheFormats(t *testing.T) {
	configured := formatsOf(t, map[string]any{})
	if !slices.Contains(configured, "markdown") || !slices.Contains(configured, "html") {
		t.Fatalf("the configured formats are %v", configured)
	}
	for _, c := range []struct {
		name    string
		options map[string]any
		want    []string
	}{
		{"typst", map[string]any{"extra_formats": []any{"typst-source"}}, []string{"typst-source"}},
		{"html and epub", map[string]any{"extra_formats": []any{"html", "epub"}}, []string{"html", "epub"}},
		{"pdf-ua", map[string]any{"extra_formats": []any{"pdf-ua"}}, []string{"pdf-ua"}},
		{"format list", map[string]any{"formats": []any{"json", "epub"}}, []string{"json", "epub"}},
		{"format list and flag", map[string]any{"formats": []any{"json"}, "extra_formats": []any{"html"}}, []string{"json", "html"}},
	} {
		if got := formatsOf(t, c.options); !slices.Equal(got, c.want) {
			t.Errorf("%s: formats %v, want %v", c.name, got, c.want)
		}
	}
}
