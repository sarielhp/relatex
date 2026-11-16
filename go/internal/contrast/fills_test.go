package contrast

import (
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func fillRule(color string, width int64) rec.Map {
	return rec.Map{"type": "rule", "color": color, "width_sp": width, "height_sp": int64(435812), "depth_sp": int64(186779),
		"margin_left_sp": int64(0), "margin_right_sp": -width}
}

func colored(color string) rec.Map { return rec.Map{"type": "text", "text": "x", "color": color} }

// A fill is a coloured rule that takes back its width and is tall enough to stand behind text:
// a thin line, a rule that kerns further back or one in the text colour is none (paper 73's bars).
func TestFillOf(t *testing.T) {
	if FillOf(fillRule("#f2f2eb", 1000)) != "#f2f2eb" {
		t.Error("a listings background is a fill")
	}
	thin := fillRule("#828085", 1000)
	thin["height_sp"], thin["depth_sp"] = int64(65536), int64(0)
	back := fillRule("#bfbfbf", 1000)
	back["margin_right_sp"] = int64(-5000)
	plain := fillRule("#bfbfbf", 1000)
	delete(plain, "color")
	for _, span := range []rec.Map{thin, back, plain, colored("#f2f2eb")} {
		if FillOf(span) != "" {
			t.Errorf("not a fill: %v", span)
		}
	}
	behind := Behind([]any{colored("#808080"), fillRule("#f2f2eb", 5144052), colored("#808080"), fillRule("#ffffff", 5144052), colored("#ff00ff")})
	if !slices.Equal(behind, []string{"", "", "#f2f2eb", "#f2f2eb", "#ffffff"}) {
		t.Errorf("behind = %v", behind)
	}
	// Paper 1021: a section number's 16 pt box behind a twelve-character title at 12 pt; paper 344:
	// a 14 pt box whose frame rules move past it before the text.
	title := rec.Map{"type": "text", "text": "7 Conclusion", "style": rec.Map{"size_sp": int64(786432)}}
	frame := rec.Map{"type": "rule", "width_sp": int64(1006620), "height_sp": int64(31452)}
	if past := Behind([]any{fillRule("#fcc712", 1085263), title, fillRule("#808080", 943716), frame, colored("#000000")}); !slices.Equal(past, []string{"", "", "", "#808080", ""}) {
		t.Errorf("past the fill = %v", past)
	}
}

// Paper 389: grey line numbers on a listing's light fill read 3.4:1. The ink keeps its page
// adjustment where it is on the page and gets its own, against the fill, where it is on the fill;
// white on a beamer box's orange (slides 20) is painted, not hidden, and darkens to read.
func TestInksOnFillsAreAdjustedAgainstTheFill(t *testing.T) {
	document := rec.Map{"nodes": rec.Map{
		"listing": rec.Map{"type": "paragraph", "content": []any{fillRule("#f2f2eb", 5144052), colored("#808080")}},
		"page":    rec.Map{"type": "paragraph", "content": []any{colored("#808080")}},
		"box":     rec.Map{"type": "paragraph", "content": []any{fillRule("#f58a4d", 5144052), colored("#ffffff")}},
		"dark":    rec.Map{"type": "paragraph", "content": []any{fillRule("#23373b", 5144052), colored("#606060")}},
	}}
	inks := Inks(document)
	if !slices.Contains(inks["#808080"], "page") || !slices.Contains(inks["#808080"], "fill:#f2f2eb") || len(inks["#ffffff"]) != 1 {
		t.Fatalf("inks = %v", inks)
	}
	if Hidden(document)["#ffffff"] {
		t.Fatal("white on a fill is hidden")
	}
	recorded := Recorded(rec.Map{Field: Adjustments(document)})
	for context, wantLighter := range map[string]bool{"#808080": false, "#808080 on #f2f2eb": false, "#ffffff on #f58a4d": false, "#606060 on #23373b": true} {
		adjusted, ok := recorded[context]
		if !ok {
			t.Fatalf("%s not adjusted: %v", context, recorded)
		}
		color, _ := Parse(adjusted)
		fill := "#faf9f6"
		if _, on, found := strings.Cut(context, " on "); found {
			fill = on
		}
		background, _ := Parse(fill)
		if Ratio(color, background) < TextRatio || LightensOn(background) != wantLighter {
			t.Errorf("%s as %s reads %.2f", context, adjusted, Ratio(color, background))
		}
	}
	if recorded["#808080"] == recorded["#808080 on #f2f2eb"] {
		t.Error("two adjustments share a colour")
	}
}

// White used both hidden on the page and on a fill: the page use stays hidden and unadjusted,
// the fill use is adjusted.
func TestHiddenInkOnAFillIsAdjustedThere(t *testing.T) {
	document := rec.Map{"nodes": rec.Map{
		"hidden": rec.Map{"type": "paragraph", "content": []any{colored("#ffffff")}},
		"box":    rec.Map{"type": "paragraph", "content": []any{fillRule("#f58a4d", 5144052), colored("#ffffff")}},
	}}
	if !Hidden(document)["#ffffff"] {
		t.Fatal("white on the page is no longer hidden")
	}
	recorded := Recorded(rec.Map{Field: Adjustments(document)})
	if _, page := recorded["#ffffff"]; page || recorded["#ffffff on #f58a4d"] == "" {
		t.Fatalf("recorded = %v", recorded)
	}
	if fills := Fills(document); !slices.Equal(fills, []string{"#f58a4d"}) {
		t.Errorf("fills = %v", fills)
	}
}

// The dark theme paints a light fill as a dark box of its hue and a dark fill as itself; an ink
// on it reads on that box and on the dark page alike (paper 14's listing lines wrap off the box).
func TestDarkFillsAndTheirInks(t *testing.T) {
	page, _ := Parse(Dark.Backgrounds["page"])
	for _, hex := range []string{"#f6f6f6", "#f2f2eb", "#f58a4d", "#808080", "#fcc712", "#23373b"} {
		fill, _ := Parse(hex)
		dark := DarkFill(fill)
		if LightensOn(fill) != (dark == fill) || !LightensOn(dark) {
			t.Errorf("%s as %s", hex, dark.Hex())
		}
		for _, ink := range []string{"#e9e6df", "#404040", "#808080", "#ff00ff", "#ffffff"} {
			color, _ := Parse(ink)
			shown := OnDarkFill(color, fill)
			if Ratio(shown, dark) < TextRatio || Ratio(shown, page) < TextRatio {
				t.Errorf("%s on %s as %s", ink, hex, shown.Hex())
			}
		}
	}
}
