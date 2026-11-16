package browser

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// longPage evaluates the scripts on a page whose body is a 300,000 px spacer followed by the
// given content: a book's depth, where Chromium's single-float client rects round to 1/32 px.
func longPage(t *testing.T, content string, scripts ...string) []any {
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
	useRepositoryRoot(t)
	directory := t.TempDir()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="15" height="15"><rect width="15" height="15"/></svg>`
	page := `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>Fixture</title></head>
<body><main><div style="height:300000.3px"></div>` + content + `<div style="height:2000px"></div></main></body></html>`
	for name, text := range map[string]string{"index.html": page, "duck.svg": svg} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	results, _, err := withPage(directory, func(browser *Browser) []any {
		browser.GoTo(pageURL("index.html"))
		values := []any{}
		for _, script := range scripts {
			values = append(values, ojson.Plain((&Checker{browser: browser}).evaluateJSON(script)))
		}
		return values
	})
	if err != nil {
		t.Fatalf("browser: %v", err)
	}
	return results
}

// TestImageGeometryIsExactFarDownALongPage: the 838-page book failed "Browser changed image
// display geometry" for images past 2^18 px, whose client-rect height had rounded to 1/32 px.
// Measured with its top in the viewport, the box is the one the style sets.
func TestImageGeometryIsExactFarDownALongPage(t *testing.T) {
	image := `<div style="height:13.7px"></div><img data-kind="image" src="duck.svg" style="display:block;width:14.628006px;height:15.241956px" alt="Image"/>`
	results := longPage(t, strings.Repeat(image, 16), imagesScript, "[window.scrollX, window.scrollY]")
	images := rec.AsList(results[0])
	if len(images) != 16 {
		t.Fatalf("images %v", images)
	}
	for _, image := range images {
		measured := rec.AsMap(image)
		for dimension, expected := range map[string]float64{"width": 14.628006, "height": 15.241956} {
			observed, _ := rec.Number(measured[dimension])
			if math.Abs(observed-expected) >= 1.0/64 {
				t.Errorf("%s %v, style %v", dimension, observed, expected)
			}
		}
	}
	if !rec.Equal(results[1], []any{int64(0), int64(0)}) {
		t.Errorf("scroll not restored: %v", results[1])
	}
}

// TestTableInventoryMeasuresFarDownALongPage: a table cell's box past 2^18 px is measured as exact
// as one at the top of the page.
func TestTableInventoryMeasuresFarDownALongPage(t *testing.T) {
	useRepositoryRoot(t)
	table := `<div style="height:13.7px"></div><table style="border-spacing:0"><tbody><tr><td style="padding:0">` +
		`<div style="height:15.241956px"></div></td></tr></tbody></table>`
	for _, table := range rec.AsList(longPage(t, strings.Repeat(table, 16), script("table_inventory.js"))[0]) {
		cell := rec.AsMap(rec.AsList(rec.AsMap(rec.AsList(rec.AsMap(table)["rows"])[0])["cells"])[0])
		if observed, _ := rec.Number(cell["height"]); math.Abs(observed-15.241956) >= 1.0/64 {
			t.Errorf("cell height %v", observed)
		}
	}
}

// useRepositoryRoot points Root at the repository, where lib/relatex holds the page scripts.
func useRepositoryRoot(t *testing.T) {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	Root = root
}
