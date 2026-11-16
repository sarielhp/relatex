package browser

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/mathexport"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// clipScript finds a formula's box in page coordinates, captured at twice its resolution.
const clipScript = `(() => {
  const r = document.getElementById(%s).getBoundingClientRect();
  return {x:r.x+window.scrollX,y:r.y+window.scrollY,width:r.width,height:r.height,scale:2};
})()
`

// Fallback is MathImageFallback: formulas the writers cannot convert, rendered from the
// sanitized retained MathML in sandboxed Chromium as PNG images beside the export.
type Fallback struct {
	document  rec.Map
	directory string
	render    func(rec.Map) string
	captured  map[string]rec.Map
}

// NewFallback is MathImageFallback.new(document, directory). render writes the reading page the
// formulas are captured from (the html output's, supplied by the export writer).
func NewFallback(document rec.Map, directory string, render func(rec.Map) string) *Fallback {
	return &Fallback{document: document, directory: directory, render: render, captured: map[string]rec.Map{}}
}

// Capture is MathImageFallback#capture: the formula id's image, rendered once.
func (f *Fallback) Capture(id string) rec.Map {
	if captured, ok := f.captured[id]; ok {
		return captured
	}
	root := filepath.Join(f.directory, "fallback-source")
	if err := os.MkdirAll(root, 0o755); err != nil {
		rec.Fail("%v", err)
	}
	page := filepath.Join(root, "index.html")
	if err := os.WriteFile(page, []byte(f.render(f.document)), 0o644); err != nil {
		rec.Fail("%v", err)
	}
	// The page links the accent font as a file beside it, as the reading page does.
	mathexport.WriteAccentFont(f.document, root)
	sum := sha256.Sum256([]byte(id))
	filename := "math-" + hex.EncodeToString(sum[:]) + ".png"
	path := filepath.Join(f.directory, filename)
	image := capture(page, id)
	if err := os.WriteFile(path, image, 0o644); err != nil {
		rec.Fail("%v", err)
	}
	digest := sha256.Sum256(image)
	f.captured[id] = rec.Map{"path": filename, "sha256": hex.EncodeToString(digest[:]),
		"source": "sanitized retained MathML in sandboxed Chromium", "resolution_scale": int64(2)}
	return f.captured[id]
}

// capture renders the page on a fresh page of the configured browser and screenshots the
// formula's box; a browser unavailable on every attempt fails the export.
func capture(page, id string) []byte {
	image, _, err := withPage(filepath.Dir(page), func(browser *Browser) []byte {
		return screenshot(browser, filepath.Base(page), id)
	})
	if err != nil {
		rec.Fail("%v", err)
	}
	return image
}

func screenshot(browser *Browser, name, id string) []byte {
	browser.GoTo(pageURL(name))
	browser.Page("Runtime.evaluate", rec.Map{"expression": "document.fonts.ready.then(() => true)", "awaitPromise": true})
	quoted, err := canonjson.Marshal(id)
	if err != nil {
		rec.Fail("%v", err)
	}
	clip, ok := ojson.Plain(browser.Evaluate(strings.Replace(clipScript, "%s", string(quoted), 1))).(map[string]any)
	if !ok || !positiveNumber(clip["width"]) || !positiveNumber(clip["height"]) {
		rec.Fail("Fallback formula has no rendered geometry")
	}
	result := rec.AsMap(ojson.Plain(browser.Page("Page.captureScreenshot",
		rec.Map{"format": "png", "clip": clip, "captureBeyondViewport": true})))
	data, err := base64.StdEncoding.DecodeString(rec.FetchString(result, "data"))
	if err != nil {
		rec.Fail("%v", err)
	}
	return data
}

func positiveNumber(value any) bool {
	number, ok := rec.Number(value)
	return ok && number > 0
}
