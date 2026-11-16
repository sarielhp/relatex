package sourcealt

import (
	"testing"

	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/rec"
)

// macroEvidence: figure 1 has \relatexalt and \Description, figure 2 \Description only, figure 3 a
// \relatexalt with details but no short description, figure 4 one whose alt is a placeholder.
func macroEvidence() *Evidence {
	return Read([]rec.Map{
		{"code": "graphic_alternative", "use": int64(1), "figure": int64(1)},
		{"code": "graphic_alternative", "use": int64(2), "figure": int64(1), "alt": "The key wins"},
		{"code": "graphic_alternative", "use": int64(3), "figure": int64(2)},
		{"code": "graphic_alternative", "use": int64(4), "figure": int64(3)},
		{"code": "graphic_alternative", "use": int64(5), "figure": int64(4)},
		{"code": "figure_description", "figure": int64(1), "description": "Not this one."},
		{"code": "figure_alt", "figure": int64(1), "alt": "A first  alt", "short": "A short text.", "details": "Step 1."},
		{"code": "figure_alt", "figure": int64(1), "alt": "A later  alt", "short": "Its short\ntext.", "details": "Step 2."},
		{"code": "figure_description", "figure": int64(2), "description": "Two squares."},
		{"code": "figure_alt", "figure": int64(3), "alt": "Three.", "details": "Extends nothing."},
		{"code": "figure_alt", "figure": int64(4), "alt": "TODO: describe"},
		{"code": "figure_description", "figure": int64(4), "description": "The fallback."},
		// Pictures: bound to their figure alone (picture_figure), or with keys of their own.
		{"code": "picture_alternative", "graphic": int64(10), "figure": int64(1)},
		{"code": "picture_alternative", "graphic": int64(11), "figure": int64(2)},
		{"code": "picture_alternative", "graphic": int64(12), "figure": int64(2), "actualtext": ""},
		{"code": "picture_alternative", "graphic": int64(13), "alt": "A drawn alt", "figure": int64(1)},
		{"code": "picture_alternative", "graphic": int64(13), "figure": int64(1)},
	})
}

// TestRelatexAltIsTheFiguresDescription: a graphic's own keys win, then its figure's last
// \relatexalt (alt, short as the long description, details beside it; details without a short one
// are dropped), then its \Description; a placeholder alt describes nothing.
func TestRelatexAltIsTheFiguresDescription(t *testing.T) {
	cases := []struct {
		use  int64
		want Description
	}{
		{1, Description{Alt: "A later alt", Long: "Its short text.", Details: "Step 2.", Macro: true}},
		{2, Description{Alt: "The key wins"}},
		{3, Description{Alt: "Two squares."}},
		{4, Description{Alt: "Three.", Macro: true}},
		{5, Description{Alt: "The fallback."}},
	}
	evidence := macroEvidence()
	for _, test := range cases {
		if got, ok := evidence.For(test.use); !ok || got != test.want {
			t.Errorf("use %d: %+v %v, want %+v", test.use, got, ok, test.want)
		}
	}
}

// TestRelatexAltDescribesADrawnPicture: a picture bound to its figure takes the figure's
// \relatexalt; its \Description only when the picture has keys of its own (as before the binding),
// and its own alt still wins.
func TestRelatexAltDescribesADrawnPicture(t *testing.T) {
	evidence := macroEvidence()
	if got, ok := evidence.ForPicture(int64(10)); !ok || got.Alt != "A later alt" || !got.Macro {
		t.Errorf("bound picture: %+v %v", got, ok)
	}
	if got, ok := evidence.ForPicture(int64(11)); ok {
		t.Errorf("a picture without keys took the figure's \\Description: %+v", got)
	}
	if got, ok := evidence.ForPicture(int64(12)); !ok || got.Alt != "Two squares." {
		t.Errorf("keyed picture: %+v %v", got, ok)
	}
	if got, ok := evidence.ForPicture(int64(13)); !ok || got != (Description{Alt: "A drawn alt"}) {
		t.Errorf("picture with its own alt: %+v %v", got, ok)
	}
}

// TestApplyMarksTheMacrosProvenance: a \relatexalt description reaches the span with provenance
// source_macro and its details, which the sidecar may replace (FromSource is false).
func TestApplyMarksTheMacrosProvenance(t *testing.T) {
	span := rec.Map{"type": "image"}
	Apply(span, Description{Alt: "A", Long: "A short text.", Details: "Step 1.", Macro: true})
	if span["alt_provenance"] != figdesc.SourceMacro || span[figdesc.DetailsField] != "Step 1." || figdesc.FromSource(span) {
		t.Errorf("macro span: %v", span)
	}
	plain := rec.Map{"type": "image"}
	Apply(plain, Description{Alt: "A", Details: "Ignored without a long description."})
	if _, has := plain[figdesc.DetailsField]; has {
		t.Errorf("details without a long description: %v", plain)
	}
}
