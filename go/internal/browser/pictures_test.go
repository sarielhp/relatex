package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// labelledPicture is a picture as decision 137 leaves it: MuPDF's drawing (a nested svg with its
// own namespace declaration, an embedded image whose data URI breaks over lines, a glyph outline in
// defs, a clip path) with its labels as SVG text.
const labelledPicture = `<svg height='20pt' viewBox='0 0 30 20' width='30pt' xmlns:xlink='http://www.w3.org/1999/xlink' xmlns='http://www.w3.org/2000/svg'>` +
	`<g transform='matrix(1 0 0 1 0 0)'><svg height='20' xmlns:inkscape='http://www.inkscape.org/namespaces/inkscape' preserveAspectRatio='none' viewBox='0 0 30 20' width='30'>
<image width='1' height='1' xlink:href='data:image/png;base64,
iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==&#10;'/>
<defs><path d='M0 0H1V1Z' id='font_1_1'/><clipPath id='clip_1'><path d='M0 0H30V20H0Z'/></clipPath></defs>
<g clip-path='url(#clip_1)'><path d='M1 1H29' stroke='#000000'/><use xlink:href='#font_1_1' fill='#000000'/></g>
<text font-family='"LM Roman 10", serif' font-size='10' xml:space='preserve' y='15'><tspan x='2'>a</tspan><tspan x='7'> </tspan><tspan x='9'>&amp;</tspan></text>
</svg></g></svg>`

// pictureDocument is a paragraph holding one picture TeX drew, shown by svg.
func pictureDocument(svg string) rec.Map {
	original := mediamodel.Encoded([]byte("%PDF crop"), "application/pdf")
	shown := mediamodel.Encoded([]byte(svg), "image/svg+xml")
	asset := rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": original["sha256"],
		"representations": rec.Map{"original": original, "html": shown}}
	span := rec.Map{"id": "n-0-image", "type": "image", "asset": asset["sha256"], "source": "rendered/crop.pdf",
		"width_sp": int64(1_966_080), "height_sp": int64(1_310_720), "alt": "A segment labelled a and an ampersand.",
		"alt_provenance": "author"}
	return rec.Map{"block_order": []any{"n-0"},
		"nodes":  rec.Map{"n-0": rec.Map{"id": "n-0", "type": "paragraph", "content": []any{span}, "children": []any{}}},
		"assets": rec.Map{"items": rec.Map{rec.FetchString(asset, "sha256"): asset}}}
}

// pictureInventory is the images inventory of the page the HTML writer makes of document, with
// edit applied to its markup.
func pictureInventory(t *testing.T, document rec.Map, edit func(string) string) rec.Map {
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
	span := rec.AsMap(rec.FetchList(rec.FetchMap(rec.FetchMap(document, "nodes"), "n-0"), "content")[0])
	picture, inline := htmlkit.InlinePicture(document, span)
	if !inline {
		t.Fatal("the picture is not inline")
	}
	// The markup html.Paper.imageSpan writes (html's TestPictureWithLabelsIsInlineSVG).
	figure := `<svg` + picture.PictureAttributes() + ` id="n-0-image" data-kind="image" data-asset="` + rec.ToS(span["asset"]) +
		`" data-width-sp="1966080" data-height-sp="1310720" data-src="` + htmlkit.ImageSource(document, span) + `" style="width:39.850560px;height:26.567040px" ` +
		`role="img" aria-label="A segment labelled a and an ampersand.">` + picture.Content + `</svg>`
	page := `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>Fixture</title></head><body><main><p>` +
		edit(figure) + `</p></main></body></html>`
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	inventory, _, err := withPage(directory, func(browser *Browser) rec.Map {
		browser.GoTo(pageURL("index.html"))
		return rec.Map{"images": ojson.Plain((&Checker{browser: browser}).evaluateJSON(imagesScript))}
	})
	if err != nil {
		t.Fatalf("browser: %v", err)
	}
	return inventory
}

// TestInlinePictureReadsBackAsItsFile: the page the writer makes shows the picture as inline SVG
// (decision 146), and the browser reads back exactly the file's markup, ids prefixed: its ids are
// the image's own and the prefixed ones, and no img is shown.
func TestInlinePictureReadsBackAsItsFile(t *testing.T) {
	document := pictureDocument(labelledPicture)
	inventory := pictureInventory(t, document, func(page string) string { return page })
	ids, shown := validateImages(inventory, document)
	want := []string{"n-0-image-font_1_1", "n-0-image-clip_1", "n-0-image"}
	if strings.Join(ids, " ") != strings.Join(want, " ") || shown != 0 {
		t.Errorf("ids %v, shown as img %d", ids, shown)
	}
}

// TestInlinePictureCatchesChangedMarkup: a label, a reference or the size the page changed fails.
func TestInlinePictureCatchesChangedMarkup(t *testing.T) {
	document := pictureDocument(labelledPicture)
	for edit, problem := range map[[2]string]string{
		{`<tspan x="2">a</tspan>`, `<tspan x="2">b</tspan>`}:                   "Browser changed inline figure markup",
		{`url(#n-0-image-clip_1)`, `url(#clip_1)`}:                             "Browser changed inline figure markup",
		{`viewBox="0 0 30 20" xmlns:xlink`, `viewBox="0 0 30 21" xmlns:xlink`}: "Browser changed inline figure root viewBox",
		{`role="img" `, ``}: "Unexpected paper image attributes",
	} {
		inventory := pictureInventory(t, document, func(page string) string {
			if !strings.Contains(page, edit[0]) {
				t.Fatalf("the page lacks %s:\n%s", edit[0], page)
			}
			return strings.Replace(page, edit[0], edit[1], 1)
		})
		func() {
			defer func() {
				failure, ok := recover().(rec.Failure)
				if !ok || failure.Message != problem {
					t.Errorf("%s: %v, want %s", edit[1], failure, problem)
				}
			}()
			validateImages(inventory, document)
		}()
	}
}
