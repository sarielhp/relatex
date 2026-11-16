package htmlkit

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// labelledPicture is a picture as decision 137 leaves it: MuPDF's drawing (a nested svg, a glyph
// outline in defs, a clip path) with its labels as SVG text.
const labelledPicture = `<svg height='20pt' viewBox='0 0 30 20' width='30pt' xmlns:xlink='http://www.w3.org/1999/xlink' xmlns='http://www.w3.org/2000/svg'>` +
	`<g transform='matrix(1 0 0 1 0 0)'><svg height='20' preserveAspectRatio='none' viewBox='0 0 30 20' width='30'>
<defs><path d='M0 0H1V1Z' id='font_1_1'/><clipPath id='clip_1'><path d='M0 0H30V20H0Z'/></clipPath></defs>
<g clip-path='url(#clip_1)'><path d='M1 1H29' stroke='#000000'/><use xlink:href='#font_1_1' fill='#000000'/></g>
<text font-family='"LM Roman 10", serif' font-size='10' xml:space='preserve' y='15'><tspan x='2'>a</tspan><tspan x='7'> </tspan><tspan x='9'>&amp;</tspan></text>
</svg></g></svg>`

func TestInlineSVGPrefixesIdsAndReferences(t *testing.T) {
	picture, err := inlineSVG([]byte(labelledPicture), "n-1-")
	if err != nil {
		t.Fatal(err)
	}
	if got := picture.PictureAttributes(); got != ` viewBox="0 0 30 20" xmlns:xlink="http://www.w3.org/1999/xlink" xmlns="http://www.w3.org/2000/svg"` {
		t.Errorf("root attributes %s", got)
	}
	for _, want := range []string{`id="n-1-font_1_1"`, `id="n-1-clip_1"`, `clip-path="url(#n-1-clip_1)"`, `xlink:href="#n-1-font_1_1"`,
		`<path d="M0 0H1V1Z" id="n-1-font_1_1"/>`, `<tspan x="9">&amp;</tspan>`, `xml:space="preserve"`, `font-family="&quot;LM Roman 10&quot;, serif"`} {
		if !strings.Contains(picture.Content, want) {
			t.Errorf("content lacks %s:\n%s", want, picture.Content)
		}
	}
	if strings.Contains(picture.Content, `"#font_1_1"`) || strings.Contains(picture.Content, "url(#clip") {
		t.Errorf("an unprefixed reference is left:\n%s", picture.Content)
	}
}

// TestInlineSVGWritesAttributeValuesAsXMLReadsThem: MuPDF breaks an embedded image's data URI over
// lines; XML reads each line break in a value as a space, an HTML parser would keep it (papers/0191).
func TestInlineSVGWritesAttributeValuesAsXMLReadsThem(t *testing.T) {
	svg := strings.Replace(labelledPicture, "<defs>", "<image xlink:href='data:image/png;base64,\nAB\r\nCD&#10;EF'/><defs>", 1)
	picture, err := inlineSVG([]byte(svg), "n-1-")
	if err != nil {
		t.Fatal(err)
	}
	if want := `<image xlink:href="data:image/png;base64, AB CD&#10;EF"/>`; !strings.Contains(picture.Content, want) {
		t.Errorf("content lacks %s:\n%s", want, picture.Content)
	}
}

func TestInlineSVGNeedsTextAndNoForbiddenElement(t *testing.T) {
	for name, svg := range map[string]string{
		"no text":        strings.NewReplacer("<text", "<desc", "</text>", "</desc>").Replace(labelledPicture),
		"style":          strings.Replace(labelledPicture, "<defs>", "<style>path{fill:red}</style><defs>", 1),
		"script":         strings.Replace(labelledPicture, "<defs>", "<script>alert(1)</script><defs>", 1),
		"foreign":        strings.Replace(labelledPicture, "<defs>", "<foreignObject/><defs>", 1),
		"not svg":        `<html xmlns='http://www.w3.org/2000/svg'><text>a</text></html>`,
		"svg namespace?": `<svg><text>a</text></svg>`,
	} {
		if _, err := inlineSVG([]byte(svg), "p-"); err == nil {
			t.Errorf("%s: inlined", name)
		}
	}
}

// pictureDocument is one image span showing svg, its source a crop (a picture TeX drew) or a file.
func pictureDocument(svg, source string) (rec.Map, rec.Map) {
	original := mediamodel.Encoded([]byte("%PDF crop"), "application/pdf")
	shown := mediamodel.Encoded([]byte(svg), "image/svg+xml")
	asset := rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": original["sha256"],
		"representations": rec.Map{"original": original, "html": shown}}
	span := rec.Map{"id": "n-1", "type": "image", "asset": asset["sha256"], "source": source, "width_sp": int64(1_966_080),
		"height_sp": int64(1_310_720)}
	document := rec.Map{"block_order": []any{"b"}, "nodes": rec.Map{"b": rec.Map{"id": "b", "content": []any{span}}},
		"assets": rec.Map{"items": rec.Map{rec.ToS(asset["sha256"]): asset}}}
	return document, span
}

func TestInlinePictureIsAPictureTeXDrewWithLabels(t *testing.T) {
	document, span := pictureDocument(labelledPicture, "rendered/abc.pdf")
	if _, inline := InlinePicture(document, span); !inline {
		t.Fatal("a cropped picture with labels is not inline")
	}
	for name, change := range map[string]func(rec.Map){
		"author file": func(s rec.Map) { s["source"] = "figs/plot.pdf" },
		"decorative":  func(s rec.Map) { s["decorative"] = true },
		"linked":      func(s rec.Map) { s["external_link"] = "x" },
	} {
		changed := rec.Map{}
		for key, value := range span {
			changed[key] = value
		}
		change(changed)
		if _, inline := InlinePicture(document, changed); inline {
			t.Errorf("%s: inline", name)
		}
	}
	outlines, span := pictureDocument(strings.NewReplacer("<text", "<desc", "</text>", "</desc>").Replace(labelledPicture), "rendered/abc.pdf")
	if _, inline := InlinePicture(outlines, span); inline {
		t.Error("a picture whose labels stayed outlines is inline")
	}
}
