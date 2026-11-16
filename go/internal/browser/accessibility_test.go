package browser

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/rec"
)

// TestAxeLimitGrowsWithThePage: five minutes cover a page of up to 5000 elements, every 5000 more
// add 30 seconds, and no page gets more than 30 minutes.
func TestAxeLimitGrowsWithThePage(t *testing.T) {
	for _, test := range []struct {
		elements int
		limit    time.Duration
	}{
		{0, 300 * time.Second},
		{5000, 300 * time.Second},
		{5001, 330 * time.Second},
		{10000, 330 * time.Second},
		{10001, 360 * time.Second},
		{1 << 30, 30 * time.Minute},
	} {
		if got := axeLimitFor(test.elements); got != test.limit {
			t.Fatalf("axeLimitFor(%d) = %s, want %s", test.elements, got, test.limit)
		}
	}
}

// axePage runs Accessibility on a page with the given body, on the local Chromium (skipped
// without one, as the browser tests are).
func axePage(t *testing.T, body string) rec.Map {
	t.Helper()
	chromium := os.Getenv("RELATEX_CHROMIUM")
	if chromium == "" {
		chromium = DefaultBrowser()
	}
	if _, err := os.Stat(chromium); err != nil {
		t.Skip("no Chromium")
	}
	t.Setenv(EndpointVariable, "")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	Root = root
	directory := t.TempDir()
	page := `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>Fixture</title>
<style>:root { color: #ddd; background: #111 } :root[data-theme="light"] { color: #222; background: #fff }</style></head>
<body><main><h1>Fixture</h1>` + body + `</main></body></html>`
	if err = os.WriteFile(filepath.Join(directory, "index.html"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		result rec.Map
		err    error
	}
	got, _, err := withPage(directory, func(browser *Browser) outcome {
		browser.GoTo(pageURL("index.html"))
		result, axeErr := (&Checker{browser: browser}).Accessibility()
		return outcome{result, axeErr}
	})
	if err != nil || got.err != nil {
		t.Fatalf("browser: %v, axe: %v", err, got.err)
	}
	if got.result["axe"] != AxeVersion {
		t.Fatalf("axe version %v", got.result["axe"])
	}
	return got.result
}

func ruleIDs(t *testing.T, result rec.Map, theme string) []string {
	t.Helper()
	ids := []string{}
	for _, violation := range rec.FetchList(rec.FetchMap(result, "themes"), theme) {
		ids = append(ids, rec.FetchString(rec.AsMap(violation), "id"))
	}
	return ids
}

// TestAxeReportsAnImageWithoutAlternativeInBothThemes: an <img> without alt is axe's image-alt
// violation, with its selector, in the light and the dark theme.
func TestAxeReportsAnImageWithoutAlternativeInBothThemes(t *testing.T) {
	result := axePage(t, `<p>Text.</p><img id="bare" width="10" height="10" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=">`)
	for _, theme := range []string{"light", "dark"} {
		ids := ruleIDs(t, result, theme)
		if len(ids) != 1 || ids[0] != "image-alt" {
			t.Fatalf("%s theme violations %v, want [image-alt]", theme, ids)
		}
		violation := rec.AsMap(rec.FetchList(rec.FetchMap(result, "themes"), theme)[0])
		node := rec.AsMap(rec.FetchList(violation, "nodes")[0])
		if target := rec.FetchList(node, "target"); len(target) != 1 || target[0] != "#bare" || rec.FetchInt(violation, "count") != 1 {
			t.Fatalf("%s theme node %v", theme, node)
		}
	}
}

// TestAxeFindsNothingOnACleanPage: a page with a language, a title, a heading and text passes in
// both themes.
func TestAxeFindsNothingOnACleanPage(t *testing.T) {
	result := axePage(t, `<p>Text.</p>`)
	for _, theme := range []string{"light", "dark"} {
		if ids := ruleIDs(t, result, theme); len(ids) != 0 {
			t.Fatalf("%s theme violations %v", theme, ids)
		}
	}
}

// TestAxeSeesTheThemesApart: low contrast in the dark theme only is a dark-theme violation.
func TestAxeSeesTheThemesApart(t *testing.T) {
	result := axePage(t, `<style>:root:not([data-theme="light"]) .faint { color: #333 }</style><p class="faint">Faint text.</p>`)
	if ids := ruleIDs(t, result, "light"); len(ids) != 0 {
		t.Fatalf("light theme violations %v", ids)
	}
	if ids := ruleIDs(t, result, "dark"); len(ids) != 1 || ids[0] != "color-contrast" {
		t.Fatalf("dark theme violations %v, want [color-contrast]", ids)
	}
}
