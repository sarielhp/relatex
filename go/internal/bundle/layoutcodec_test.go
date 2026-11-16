package bundle

import (
	"bytes"
	"encoding/base64"
	"maps"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/svgoutline"
)

const codecSVG = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">` +
	`<defs><path id="font_1_5" d="M0 0C0 2 2 2 2 0Z"/></defs>` +
	`<g transform="translate(10 20)"><use data-text="f" xlink:href="#font_1_5" transform="scale(10 -10)"/></g></svg>`

// codecLayout is a one-page layout with a measured glyph, bound as svgoutline.Bind binds it, and
// a synthetic space.
func codecLayout(t *testing.T) rec.Map {
	t.Helper()
	svg := []byte(codecSVG)
	native := svgoutline.For(svg).Uses[0]
	binding := maps.Clone(native)
	binding["paint"] = rec.Map{"font": "ABCDEF+Example", "text": "f", "index": int64(0), "glyph": "f",
		"baseline": []any{10.0, 20.0}, "advance": []any{8.0, 0.0}, "matrix": []any{10.0, 0.0, 0.0, -10.0, 10.0, 20.0},
		"operation": "fill_text"}
	binding["origin_binding"] = "paint_origin"
	baseline := []any{10.0, 20.0}
	glyph := rec.Map{"id": "p1-g1", "text": "f", "baseline": baseline, "outline_binding": binding,
		"ink_quad": svgoutline.Quad(native["bounds"], baseline)}
	space := rec.Map{"id": "p1-g2", "text": " ", "baseline": []any{30.0, 20.0},
		"outline_binding": rec.Map{"status": "synthetic_space", "bounds": nil}, "ink_quad": []any{30.0, 20.0, 30.0, 20.0, 30.0, 20.0, 30.0, 20.0}}
	page := rec.Map{"svg": base64.StdEncoding.EncodeToString(svg), "glyphs": []any{glyph, space}}
	return rec.Map{"schema_version": "layout-1.2", "pages": []any{page}}
}

func canonical(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := canonjson.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func firstGlyphs(layout any) []any {
	return rec.AsMap(rec.AsList(rec.AsMap(layout)["pages"])[0])["glyphs"].([]any)
}

func TestCompactLayoutLeavesOutWhatTheSVGHoldsAndReadingRestoresIt(t *testing.T) {
	layout := codecLayout(t)
	before := canonical(t, layout)
	stored := compactLayout(layout)
	if !bytes.Equal(canonical(t, layout), before) {
		t.Fatal("compaction changed the layout it was given")
	}
	glyph := rec.AsMap(firstGlyphs(stored)[0])
	if _, kept := glyph["ink_quad"]; kept {
		t.Fatal("derivable ink quad kept")
	}
	if got := rec.AsMap(glyph["outline_binding"]); !rec.SameKeys(got, []string{"use", "paint", "origin_binding"}) {
		t.Fatalf("binding keys %v", got)
	}
	if len(canonical(t, stored)) >= len(before) {
		t.Fatal("stored layout is not smaller")
	}
	// Through the bytes, as Read decodes them.
	decoded, err := canonjson.Decode(strings.NewReader(string(canonical(t, stored))))
	if err != nil {
		t.Fatal(err)
	}
	if restored := canonical(t, expandLayout(decoded)); !bytes.Equal(restored, before) {
		t.Fatalf("restored layout differs:\n%s\n%s", restored, before)
	}
}

func TestCompactLayoutKeepsFieldsTheSVGDoesNotGiveBack(t *testing.T) {
	layout := codecLayout(t)
	glyph := rec.AsMap(firstGlyphs(layout)[0])
	glyph["ink_quad"] = []any{0.0, 0.0, 1.0, 0.0, 0.0, 1.0, 1.0, 1.0}
	rec.AsMap(glyph["outline_binding"])["text"] = "x"
	stored := rec.AsMap(firstGlyphs(compactLayout(layout))[0])
	if !rec.Equal(stored["ink_quad"], glyph["ink_quad"]) || !rec.Equal(stored["outline_binding"], glyph["outline_binding"]) {
		t.Fatal("a field the SVG does not reproduce was left out")
	}
	if !bytes.Equal(canonical(t, expandLayout(compactLayout(layout))), canonical(t, layout)) {
		t.Fatal("a stored-whole glyph changed on reading")
	}
}

func TestPagesShareTheirGlyphOutlineDefinitions(t *testing.T) {
	definition := `<path id="font_1_5" d="M0 0C0 2 2 2 2 0Z"/>`
	svg := []byte(strings.Join([]string{`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">`,
		`<defs>`, definition, `</defs>`,
		`<g transform="translate(10 20)"><use data-text="f" xlink:href="#font_1_5" transform="scale(10 -10)"/></g>`, `</svg>`, ``}, "\n"))
	layout := codecLayout(t)
	page := rec.AsMap(rec.AsList(layout["pages"])[0])
	page["svg"] = base64.StdEncoding.EncodeToString(svg)
	layout["pages"] = []any{page, maps.Clone(page)}
	before := canonical(t, layout)
	stored := rec.AsMap(compactLayout(layout))
	if shared := rec.AsList(stored[sharedPathsKey]); len(shared) != 1 || shared[0] != definition {
		t.Fatalf("shared outlines %v", shared)
	}
	for _, value := range rec.AsList(stored["pages"]) {
		text, isText := rec.AsMap(value)[svgTextKey].(string)
		if !isText || strings.Contains(text, "M0 0C0") {
			t.Fatalf("page SVG not shared: %v", value)
		}
		if _, ink := rec.AsMap(rec.AsList(rec.AsMap(value)["glyphs"])[0])["ink_quad"]; ink {
			t.Fatal("glyph not compacted before sharing")
		}
	}
	decoded, err := canonjson.Decode(strings.NewReader(string(canonical(t, stored))))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical(t, expandLayout(decoded)), before) {
		t.Fatal("restored layout differs")
	}
}

// compactLayout is the whole layout compacted as the bundle's parts store it (layoutparts.go), for the
// round trips; the layout itself is not changed.
func compactLayout(layout any) any {
	return shareLayoutSVG(mapLayoutPages(layout, compactGlyph))
}
