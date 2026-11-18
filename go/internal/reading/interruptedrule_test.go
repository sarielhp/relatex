package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

// A box's paragraph that holds one frame rule per line of the listing nested in it (2611.18517: a
// lstlisting with backgroundcolor in a tcolorbox) paints each rule into the listing's line before
// it. The rule is no block of its own between the lines (that interleaved the paragraph's blocks
// with the listing's), and the paragraph reads on after the rules.
func TestARuleOfAnInterruptedParagraphPaintsIntoTheLineBefore(t *testing.T) {
	e := &Evidence{owners: omap.New[any, rec.Map](), ownerProgress: omap.New[any, int64](), ownerUses: map[any]int64{},
		scopes: newScopes(nil), lastOwners: map[partition]any{}, lastBlocks: map[partition]rec.Map{},
		lastKeys: map[partition]blockKey{}, blocks: []rec.Map{}, diagnostics: []rec.Map{}}
	text := rec.Map{"type": "text", "text": "x"}
	rule := rec.Map{"type": "unsupported", "text": "[rule]", "width": int64(100), "height": int64(5), "depth": int64(1), "space": " "}
	e.owners.Set(int64(1), rec.Map{"owner": int64(1), "group": "paragraph", "role": "paragraph", "entity": int64(7),
		"items": []any{text, text, rule, rule, text}})
	e.owners.Set(int64(2), rec.Map{"owner": int64(2), "group": "paragraph", "role": "code", "entity": int64(8), "items": []any{text, text}})
	outer, _ := e.owners.Get(int64(1))
	line, _ := e.owners.Get(int64(2))

	e.trackOwner(outer, 1, 2)
	e.proseBlock(outer, int64(1))
	e.trackOwner(line, 1, 2)
	listing := e.proseBlock(line, int64(1))
	if !e.paintedIntoBlock(outer, 3, 4) {
		t.Fatal("the rules of the interrupted paragraph were not painted into the line before")
	}
	rules := 0
	for _, span := range rec.FetchList(listing, "content") {
		if rec.AsMap(span)["type"] == "rule" {
			rules++
		}
	}
	if rules != 2 {
		t.Errorf("the line holds %d rules, want 2", rules)
	}
	e.trackOwner(outer, 5, 5)
	if last := e.proseBlock(outer, int64(1)); rec.Fetch(last, "id") == rec.Fetch(listing, "id") {
		t.Errorf("the paragraph's rest joined block %v", rec.Fetch(last, "id"))
	}
}

// A paragraph's own rules, read while nothing else was printed inside it, stay in its text.
func TestARuleOfAnUninterruptedParagraphIsNotPainted(t *testing.T) {
	e := &Evidence{owners: omap.New[any, rec.Map](), ownerProgress: omap.New[any, int64](), ownerUses: map[any]int64{},
		scopes: newScopes(nil), lastOwners: map[partition]any{}, lastBlocks: map[partition]rec.Map{},
		lastKeys: map[partition]blockKey{}, blocks: []rec.Map{}, diagnostics: []rec.Map{}}
	text := rec.Map{"type": "text", "text": "x"}
	rule := rec.Map{"type": "unsupported", "text": "[rule]", "width": int64(100), "height": int64(5), "depth": int64(1), "space": " "}
	e.owners.Set(int64(1), rec.Map{"owner": int64(1), "group": "paragraph", "role": "paragraph", "entity": int64(7),
		"items": []any{text, rule, text}})
	outer, _ := e.owners.Get(int64(1))
	e.trackOwner(outer, 1, 1)
	e.proseBlock(outer, int64(1))
	if e.paintedIntoBlock(outer, 2, 2) {
		t.Error("a rule of an uninterrupted paragraph was painted into a block")
	}
}
