package pdfpaint

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/paintmodel"
	"github.com/sarielhp/relatex/internal/rec"
)

func TestInkHexRoundsAsTheCollectorDoes(t *testing.T) {
	cases := []struct {
		space  string
		values []string
		hex    string
	}{
		{"DeviceGray", []string{"0"}, ""},
		{"DeviceRGB", []string{"0", "0", "0"}, ""},
		{"DeviceGray", []string{".25"}, "#404040"},
		{"DeviceRGB", []string{"1", "0", "0"}, "#ff0000"},
		{"DeviceRGB", []string{"0", "0", ".5"}, "#000080"},
		{"DeviceGray", []string{"1"}, "#ffffff"},
		{"DeviceCMYK", []string{"0", "0", "0", "1"}, ""},
		{"DeviceCMYK", []string{"0", "1", "1", "0"}, "#ff0000"},
		{"Separation(DeviceCMYK,Cyan)", []string{"1"}, "#00ffff"},
	}
	for _, c := range cases {
		got, ok := InkHex(c.space, c.values)
		if !ok || got != c.hex {
			t.Errorf("InkHex(%s, %v) = %q, %v, want %q", c.space, c.values, got, ok, c.hex)
		}
	}
	for _, c := range []struct {
		space  string
		values []string
	}{{"DeviceCMYK", []string{"0", "0", "1"}}, {"Separation(DeviceCMYK,PANTONE 300)", []string{"1"}}, {"Separation(DeviceCMYK,Cyan)", []string{".5"}}, {"DeviceGray", []string{"0", "0"}}, {"DeviceRGB", []string{"x", "0", "0"}}, {"Indexed", []string{"1"}}} {
		if _, ok := InkHex(c.space, c.values); ok {
			t.Errorf("InkHex(%s, %v) was accepted", c.space, c.values)
		}
	}
}

func fillTrace(attributes string) []byte {
	return []byte(`<document><page number="1" mediabox="0 0 10 10"><group bbox="0 0 10 10" isolated="1" knockout="0" blendmode="Normal" alpha="1">` +
		`<fill_text ` + attributes + `><span trm="1 0 0 1"><g unicode="a" x="1" y="1" adv="1"/></span></fill_text></group></page></document>`)
}

// Invisible text (render mode 3) paints nothing: it is neither ink nor a counted glyph.
func TestInspectTraceIgnoresInvisibleText(t *testing.T) {
	trace := strings.Replace(string(fillTrace(`colorspace="DeviceGray" color="0"`)), "</group>",
		`<ignore_text><span trm="1 0 0 1"><g unicode="x" x="1" y="1" adv="1"/></span></ignore_text></group>`, 1)
	pages, glyphs, inks, _, _ := inspectTrace([]byte(trace), false)
	if pages != 1 || glyphs != 1 || len(inks) != 0 {
		t.Errorf("pages %d glyphs %d inks %v", pages, glyphs, inks)
	}
}

func TestInspectTraceCollectsOpaqueColoredInk(t *testing.T) {
	pages, glyphs, inks, _, _ := inspectTrace(fillTrace(`colorspace="DeviceRGB" color="1 0 0"`), false)
	if pages != 1 || glyphs != 1 || len(inks) != 1 || inks[0] != "#ff0000" {
		t.Errorf("pages %d glyphs %d inks %v", pages, glyphs, inks)
	}
	if _, _, inks, _, _ := inspectTrace(fillTrace(`colorspace="DeviceGray" color="0"`), false); len(inks) != 0 {
		t.Errorf("black recorded as ink: %v", inks)
	}
}

func TestInspectTraceRejectsTranslucentOrUnsupportedInk(t *testing.T) {
	for _, attributes := range []string{`colorspace="DeviceRGB" color="1 0 0" alpha=".5"`, `colorspace="Lab" color="0 0 0"`, `colorspace="Separation(DeviceCMYK,Spot)" color="1"`} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s was accepted", attributes)
				}
			}()
			inspectTrace(fillTrace(attributes), false)
		}()
	}
}

func coloredDocument(color string, inks any) rec.Map {
	proof := rec.Map{"status": "native_text_paint_verified", "pages": int64(1), "glyphs": int64(1), "color": paintmodel.Color,
		"pdf_sha256": sha64("a"), "helper_sha256": sha64("b"), "trace_sha256": sha64("c")}
	if inks != nil {
		proof["inks"] = inks
	}
	span := rec.Map{"type": "text", "text": "x"}
	if color != "" {
		span["color"] = color
	}
	return rec.Map{"text_paint": proof, "nodes": rec.Map{"n": rec.Map{"content": []any{span}}}}
}

func sha64(letter string) string { return strings.Repeat(letter, 64) }

func TestValidateTextRequiresEverySpanColorToBePaintedNatively(t *testing.T) {
	paintmodel.ValidateText(coloredDocument("", nil))
	paintmodel.ValidateText(coloredDocument("#ff0000", []any{"#00ff00", "#ff0000"}))
	for name, document := range map[string]rec.Map{
		"colored span without inks": coloredDocument("#ff0000", nil),
		"span color not among inks": coloredDocument("#ff0000", []any{"#00ff00"}),
		"unsorted inks":             coloredDocument("#ff0000", []any{"#ff0000", "#00ff00"}),
		"black among the inks":      coloredDocument("", []any{"#000000"}),
		"malformed ink":             coloredDocument("", []any{"red"}),
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s was accepted", name)
				}
			}()
			paintmodel.ValidateText(document)
		}()
	}
}

// A figure drawn at alpha .8 has its own translucent group: figure paint, not text. Text painted
// through such a group stays rejected.
func TestTranslucentFigureGroupIsAcceptedOnlyWithoutTextInside(t *testing.T) {
	page := `<group bbox="0 0 10 10" isolated="1" knockout="0" blendmode="Normal" alpha="1">`
	figure := `<group bbox="1 1 5 5" isolated="0" knockout="0" blendmode="Normal" alpha=".8">`
	text := `<fill_text colorspace="DeviceGray" color="0"><span trm="1 0 0 1"><g unicode="a" x="1" y="1" adv="1"/></span></fill_text>`
	trace := func(body string) []byte {
		return []byte(`<document><page number="1" mediabox="0 0 10 10">` + page + body + `</group></page></document>`)
	}
	if pages, glyphs, _, _, _ := inspectTrace(trace(figure+`</group>`+text), true); pages != 1 || glyphs != 1 {
		t.Errorf("a translucent figure beside the text: pages %d glyphs %d", pages, glyphs)
	}
	for name, body := range map[string]string{
		"text inside": figure + text + `</group>`,
		"clear group": strings.Replace(figure, ".8", "0", 1) + `</group>` + text,
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s was accepted", name)
				}
			}()
			inspectTrace(trace(body), true)
		}()
	}
}
