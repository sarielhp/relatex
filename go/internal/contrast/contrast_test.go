package contrast

import (
	"math"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func parse(t *testing.T, hex string) RGB {
	t.Helper()
	color, ok := Parse(hex)
	if !ok {
		t.Fatalf("unparsable %s", hex)
	}
	return color
}

// The WCAG ratios of known pairs.
func TestRatioOfKnownPairs(t *testing.T) {
	for _, row := range []struct {
		a, b string
		want float64
	}{{"#000000", "#ffffff", 21}, {"#777777", "#ffffff", 4.48}, {"#0000ff", "#ffffff", 8.59}, {"#ffffff", "#ffffff", 1}} {
		if got := rec.Round(Ratio(parse(t, row.a), parse(t, row.b)), 2); got != row.want {
			t.Errorf("%s on %s: %v, want %v", row.a, row.b, got, row.want)
		}
	}
	if _, ok := Parse("rgb(0, 0, 0)"); ok {
		t.Error("parsed a non-hex color")
	}
}

// An adjusted ink reaches the ratio, keeps its hue, moves the right way, and is the smallest such
// move: a lightness 0.01 closer to the ink fails.
func TestAdjustIsTheSmallestLightnessChange(t *testing.T) {
	cases := []struct {
		ink, background string
		lighten         bool
	}{{"#00ff00", "#faf9f6", false}, {"#ff0000", "#faf9f6", false}, {"#ff00ff", "#eef7ea", false}, {"#00ffff", "#faf9f6", false},
		{"#0000ff", "#1b1d22", true}, {"#000080", "#202836", true}, {"#ff0000", "#1f2a21", true}, {"#000066", "#1b1d22", true}}
	for _, row := range cases {
		ink, background := parse(t, row.ink), parse(t, row.background)
		adjusted := Adjust(ink, []RGB{background}, TextRatio, row.lighten)
		if Ratio(adjusted, background) < TextRatio {
			t.Errorf("%s: %s reads at %.2f", row.ink, adjusted.Hex(), Ratio(adjusted, background))
		}
		lightness, chroma, hue := ink.oklch()
		moved, newChroma, newHue := adjusted.oklch()
		if (moved > lightness) != row.lighten {
			t.Errorf("%s: lightness %.3f to %.3f", row.ink, lightness, moved)
		}
		if newChroma > 0.03 && math.Abs(math.Remainder(newHue-hue, 2*math.Pi)) > 0.05 {
			t.Errorf("%s: hue %.3f to %.3f", row.ink, hue, newHue)
		}
		step := 0.01
		if row.lighten {
			step = -0.01
		}
		closer := fromOKLCH(moved+step, chroma, hue).quantized()
		if Ratio(closer, background) >= TextRatio {
			t.Errorf("%s: %s is not the smallest change (%s passes)", row.ink, adjusted.Hex(), closer.Hex())
		}
	}
}

// An ink that already reads keeps its exact value.
func TestAdjustKeepsAReadableInk(t *testing.T) {
	navy := parse(t, "#000080")
	if Adjust(navy, []RGB{parse(t, "#faf9f6")}, TextRatio, false) != navy {
		t.Error("navy changed on the light page")
	}
	if got := Dark.Shown("#ffffaa", nil); got != "#ffffaa" {
		t.Errorf("light yellow changed to %s in the dark theme", got)
	}
	if got := Dark.Shown("not a color", nil); got != "not a color" {
		t.Errorf("unparsable color changed to %s", got)
	}
}

func colorDocument() rec.Map {
	span := func(color string) rec.Map { return rec.Map{"type": "text", "text": "x", "color": color} }
	return rec.Map{"nodes": rec.Map{
		"d":  rec.Map{"type": "document", "children": []any{"p1", "t"}},
		"p1": rec.Map{"type": "paragraph", "content": []any{span("#00ff00"), span("#000080"), span("#000000"), rec.Map{"type": "rule", "color": "#ffff00"}}},
		"t":  rec.Map{"type": "theorem", "children": []any{"pr"}},
		"pr": rec.Map{"type": "proof", "children": []any{"p2"}},
		"p2": rec.Map{"type": "paragraph", "content": []any{span("#ff0000")}},
	}}
}

// The inks are the text spans' colors with the innermost structure's background; black and rules
// are not inks.
func TestInksAndTheirBackgrounds(t *testing.T) {
	inks := Inks(colorDocument())
	want := map[string][]string{"#00ff00": {"page"}, "#000080": {"page"}, "#ff0000": {"proof"}}
	if !rec.Equal(len(inks), len(want)) {
		t.Fatalf("inks %v", inks)
	}
	for color, kinds := range want {
		if len(inks[color]) != 1 || inks[color][0] != kinds[0] {
			t.Errorf("%s: %v, want %v", color, inks[color], kinds)
		}
	}
}

// The record names each failing ink with its adjustment, and Restore reads the adjusted ink back
// as the PDF's while refusing a record the inks do not give.
func TestAdjustmentsRecordAndRestore(t *testing.T) {
	document := colorDocument()
	record := Adjustments(document)
	items := rec.FetchList(record, "items")
	if len(items) != 2 || rec.FetchString(rec.AsMap(items[0]), "ink") != "#00ff00" || rec.FetchString(rec.AsMap(items[1]), "ink") != "#ff0000" {
		t.Fatalf("record %v", record)
	}
	green := rec.AsMap(items[0])
	if rec.FetchString(green, "background") != "#faf9f6" || rec.FetchString(rec.AsMap(items[1]), "background") != "#eef7ea" {
		t.Errorf("backgrounds %v", items)
	}
	document[Field] = record
	adjusted := rec.FetchString(green, "adjusted")
	inventory := rec.Map{"blocks": []any{rec.Map{"content": []any{rec.Map{"color": adjusted, "text": adjusted}}}},
		"text_paint": []any{rec.Map{"color": cssRGB(adjusted)}}, "tables": []any{rec.Map{"run_colors": []any{cssRGB(adjusted), nil}}}}
	restored := Restore(inventory, document)
	span := rec.AsMap(rec.FetchList(rec.AsMap(rec.FetchList(restored, "blocks")[0]), "content")[0])
	if span["color"] != "#00ff00" || span["text"] != adjusted {
		t.Errorf("restored span %v", span)
	}
	if rec.AsMap(rec.FetchList(restored, "text_paint")[0])["color"] != "rgb(0, 255, 0)" {
		t.Errorf("restored paint %v", restored["text_paint"])
	}
	if colors := rec.FetchList(rec.AsMap(rec.FetchList(restored, "tables")[0]), "run_colors"); colors[0] != "rgb(0, 255, 0)" || colors[1] != nil {
		t.Errorf("restored run colors %v", colors)
	}
	green["adjusted"] = "#000000"
	defer func() {
		if recover() == nil {
			t.Error("a forged adjustment was accepted")
		}
	}()
	Restore(inventory, document)
}
