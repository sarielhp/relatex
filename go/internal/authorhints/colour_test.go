package authorhints

import "testing"

// A colour caption attestation is read from a comment and from hints.json, which wins (decision 188).
func TestColourCaptionHints(t *testing.T) {
	const old = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const current = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	hints := parse(t, []Source{{Path: "main.tex", Text: "%% relatex colour_captions: fig:a = \"" + old + "\"\n"}},
		`{"colour_captions": {"fig:a": "`+current+`", "16.9": "`+old+`"}}`)
	if hint, ok := hints.Get(ColourCaptionsSection, "fig:a"); !ok || hint.Value != current {
		t.Fatalf("fig:a = %+v %v", hint, ok)
	}
	if hint, ok := hints.Get(ColourCaptionsSection, "16.9"); !ok || hint.Value != old {
		t.Fatalf("16.9 = %+v %v", hint, ok)
	}
	if len(hints.Conflicts) != 1 || len(hints.Unknown) != 0 {
		t.Fatalf("conflicts %+v unknown %+v", hints.Conflicts, hints.Unknown)
	}
}
