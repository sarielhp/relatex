package reading

import (
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

func strayFragmentEvidence(occurrences [][3]int64) *Evidence {
	e := &Evidence{owners: omap.New[any, rec.Map](), ownerProgress: omap.New[any, int64](), ownerUses: map[any]int64{},
		scopes: newScopes(nil), lastOwners: map[partition]any{}, lastBlocks: map[partition]rec.Map{},
		lastKeys: map[partition]blockKey{}, blocks: []rec.Map{}, diagnostics: []rec.Map{}}
	items := func(n int) []any { return make([]any, n) }
	// Owner 1 is the measured copy (10 items), owner 2 the set copy without its hyphen (9 items).
	e.owners.Set(int64(1), rec.Map{"owner": int64(1), "group": "paragraph", "role": "paragraph", "entity": int64(7), "items": items(10)})
	e.owners.Set(int64(2), rec.Map{"owner": int64(2), "group": "paragraph", "role": "paragraph", "entity": int64(7), "items": items(9)})
	e.owners.Set(int64(3), rec.Map{"owner": int64(3), "group": "paragraph", "role": "paragraph", "entity": int64(7), "items": items(4)})
	for _, occurrence := range occurrences {
		e.records = append(e.records, rec.Map{"kind": "flow_occurrence", "owner": occurrence[0], "first": occurrence[1], "last": occurrence[2]})
	}
	return e
}

func readOccurrences(e *Evidence) []string {
	ids := []string{}
	for _, row := range e.records {
		owner, _ := e.owners.Get(rec.Fetch(row, "owner"))
		e.trackOwner(owner, rec.FetchInt(row, "first"), rec.FetchInt(row, "last"))
		ids = append(ids, rec.FetchString(e.proseBlock(owner, int64(1)), "id"))
	}
	return ids
}

func TestStrayCopyFragmentStaysInItsParagraph(t *testing.T) {
	e := strayFragmentEvidence([][3]int64{{2, 1, 4}, {1, 5, 5}, {2, 5, 7}, {1, 9, 9}, {2, 8, 9}, {3, 1, 4}})
	got := readOccurrences(e)
	want := []string{"b1", "b1", "b1", "b1", "b1", "b2"}
	if !slices.Equal(got, want) {
		t.Errorf("blocks %v, want %v", got, want)
	}
	if len(e.diagnostics) != 0 {
		t.Errorf("diagnostics %v, want none", e.diagnostics)
	}
}

func TestParagraphShippingItsFirstSlotIsNoStrayCopy(t *testing.T) {
	e := strayFragmentEvidence([][3]int64{{2, 1, 4}, {3, 1, 4}, {2, 5, 9}})
	if got, want := readOccurrences(e), []string{"b1", "b2", "b3"}; !slices.Equal(got, want) {
		t.Errorf("blocks %v, want %v", got, want)
	}
}
