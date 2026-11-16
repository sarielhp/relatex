package contrast

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// Two inks whose smallest adjustments coincide (grey and a near-white grey both darken to #737373), and an
// adjustment that would land on another text colour, are moved on until each recorded adjustment
// names one ink: the checks read it back (paper 587 of the third cohort).
func TestAdjustmentsAreDistinctColors(t *testing.T) {
	span := func(color string) any { return rec.Map{"type": "text", "text": "x", "color": color} }
	document := rec.Map{"nodes": rec.Map{"p": rec.Map{"type": "paragraph", "content": []any{span("#808080"), span("#eeeeee"), span("#727272")}}}}
	seen := map[string]bool{"#808080": true, "#eeeeee": true, "#727272": true}
	for _, value := range rec.FetchList(Adjustments(document), "items") {
		item := rec.AsMap(value)
		adjusted := rec.FetchString(item, "adjusted")
		if seen[adjusted] || Ratio(parse(t, adjusted), parse(t, "#faf9f6")) < TextRatio {
			t.Errorf("adjustment %v is taken or does not read", item)
		}
		seen[adjusted] = true
	}
}

// An ink lighter than its backgrounds (a light grey that is still visible on the paper) reads worst on the darkest of them once
// darkened: the adjustment must read on every background it is shown on, and the record names the
// one that binds.
func TestAdjustmentReadsOnEveryBackground(t *testing.T) {
	white := rec.Map{"type": "text", "text": "x", "color": "#eeeeee"}
	document := rec.Map{"nodes": rec.Map{
		"d":  rec.Map{"type": "document", "children": []any{"p", "pr"}},
		"p":  rec.Map{"type": "paragraph", "content": []any{white}},
		"pr": rec.Map{"type": "proof", "children": []any{"q"}},
		"q":  rec.Map{"type": "paragraph", "content": []any{white}},
	}}
	item := rec.AsMap(rec.FetchList(Adjustments(document), "items")[0])
	adjusted := parse(t, rec.FetchString(item, "adjusted"))
	for _, background := range []string{"#faf9f6", "#eef7ea"} {
		if Ratio(adjusted, parse(t, background)) < TextRatio {
			t.Errorf("%s reads at %.2f on %s", adjusted.Hex(), Ratio(adjusted, parse(t, background)), background)
		}
	}
	if rec.FetchString(item, "background") != "#eef7ea" {
		t.Errorf("binding background %v", item)
	}
}
