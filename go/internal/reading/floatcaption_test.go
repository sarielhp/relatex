package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

func captionContext(entity, float, seq int64) rec.Map {
	return rec.Map{"kind": "diagnostic", "code": "caption_context", "entity": entity, "float": float, "numbered": true, "seq": seq}
}

func TestFloatCaptionEntity(t *testing.T) {
	e := &Evidence{records: []rec.Map{captionContext(2, 1, 12), captionContext(4, 3, 35)}}
	subfigure := func(float, target, seq int64, counter string) rec.Map {
		return rec.Map{"counter": counter, "entity": float, "target_entity": target, "seq": seq}
	}
	cases := map[string]struct {
		label rec.Map
		want  int64
	}{
		"a subfigure label before its caption names its own float's caption": {subfigure(3, 2, 21, "subfigure"), 4},
		"a subfigure label after its caption keeps it":                       {subfigure(3, 4, 40, "subfigure"), 4},
		"a figure label keeps its target":                                    {subfigure(3, 2, 21, "figure"), 2},
		"no caption follows in the float":                                    {subfigure(5, 2, 50, "subfigure"), 2},
	}
	for name, test := range cases {
		if got := e.floatCaptionEntity(test.label, test.label["target_entity"]); !rec.Equal(got, test.want) {
			t.Errorf("%s: entity %v, want %d", name, got, test.want)
		}
	}
}

func TestBoxedCaptionLabelNamesTheCaptionBlock(t *testing.T) {
	picture := textBlock("paragraph", "paragraph", int64(5), "")
	caption := textBlock("paragraph", "paragraph", int64(6), "Figure 3. A wrapped figure.")
	e := &Evidence{blocks: []rec.Map{picture, caption}, records: []rec.Map{captionContext(6, 5, 12)}}
	label := rec.Map{"name": "fig:wrap", "number": "3", "entity": int64(5), "target_entity": int64(6), "seq": int64(13)}
	got := e.boxedCaptionBlocks(label)
	if len(got) != 1 || !rec.Equal(rec.Fetch(got[0], "entity"), int64(6)) {
		t.Errorf("caption blocks %v, want the block of entity 6", got)
	}
}

func TestSubfigureLabelSkipsTheFloatsOwnCaption(t *testing.T) {
	caption := textBlock("b1", "paragraph", int64(15), "Figure 1.", " projection of copper.")
	e := &Evidence{blocks: []rec.Map{caption}, records: []rec.Map{rec.Merge(captionContext(15, 14, 520), rec.Map{"category": "figure"})}}
	label := rec.Map{"counter": "subfigure", "number": "1(a)", "entity": int64(14), "seq": int64(493)}
	if got := e.followingCaptionBlocks(label); len(got) != 0 {
		t.Errorf("subfigure label named the float's own caption: %v", got)
	}
	label["counter"] = "figure"
	if got := e.followingCaptionBlocks(label); len(got) != 1 {
		t.Errorf("figure label lost the following caption: %v", got)
	}
}

func TestInlineMathInAnHboxJoinsTheHboxBlock(t *testing.T) {
	host := rec.Map{"owner": int64(184), "group": "hbox", "entity": int64(15), "items": []any{1, 2, 3}}
	math := rec.Map{"owner": int64(183), "group": "math", "entity": int64(15)}
	caption := textBlock("b1", "paragraph", int64(15), "Figure 1.")
	e := &Evidence{owners: omap.New[any, rec.Map](), ownerProgress: omap.New[any, int64](), scopes: newScopes(nil),
		lastBlocks: map[partition]rec.Map{{}: caption}, lastKeys: map[partition]blockKey{{}: {int64(184), 0}}}
	e.owners.Set(int64(184), host)
	e.ownerProgress.Set(int64(184), 1)
	if got := e.inlineMathBlock(math, int64(1)); rec.FetchString(got, "id") != "b1" {
		t.Errorf("math block %v, want the hbox block b1", got)
	}
}

func TestPictureOnlyTargetFallsToBoxedCaption(t *testing.T) {
	picture := rec.Map{"entity": int64(6), "type": "paragraph", "content": []any{rec.Map{"type": "image"}}}
	caption := textBlock("paragraph", "paragraph", int64(5), "Figure 3. A wrapped figure.")
	e := &Evidence{blocks: []rec.Map{picture, caption}, records: []rec.Map{captionContext(6, 5, 12)}}
	label := rec.Map{"name": "fig:wrap", "number": "3", "counter": "figure", "entity": int64(5), "target_entity": int64(6), "seq": int64(13)}
	got := e.targetBlocks(label)
	if len(got) != 1 || !rec.Equal(rec.Fetch(got[0], "entity"), int64(5)) {
		t.Errorf("target blocks %v, want the caption block of entity 5", got)
	}
}
