package pdftag

import "testing"

// A heading that sets one word in small capitals (paper 0593's Times bold caps) keeps that run's
// shape: the caps glyphs match in either case, and the rest of the heading still compares case.
func TestHeadingSmallCapsRunMatchesItsOwnLetters(t *testing.T) {
	builder := newBuilder(&document{Nodes: map[string]node{}})
	caps := contentItem{Type: "text", Text: "Waveguide"}
	caps.Style.Shape = "sc"
	items := []contentItem{{Type: "text", Text: "3.5 The "}, caps, {Type: "text", Text: " Plan Space"}}
	want := leafExpectations(builder.itemPieces("heading", items))["heading"]
	if kind := matchKind(readText{plain: "3.5TheWAVEGUIDEPlanSpace", reading: "3.5TheWAVEGUIDEPlanSpace"}, want, false); kind != "small_caps" {
		t.Fatalf("caps word match = %q, want small_caps", kind)
	}
	// The whole heading is not small capitals: uppercase elsewhere is a mismatch.
	if kind := matchKind(readText{plain: "3.5THEWAVEGUIDEPLANSPACE", reading: "3.5THEWAVEGUIDEPLANSPACE"}, want, false); kind != "" {
		t.Fatalf("whole heading match = %q, want no match", kind)
	}
}
