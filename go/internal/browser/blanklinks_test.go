package browser

import (
	"testing"

	"github.com/sarielhp/relatex/internal/extlinks"
	"github.com/sarielhp/relatex/internal/rec"
)

// books/0003 (decision 135): "arXiv: 1701.00002 [cs.CG]" links the formula, the space after it
// and "[cs.CG]". The page writes the blank space as plain text, so the check expects no anchor
// for it; an anchor that does show it still fails, as a missing named one does.

func blankLinkDocument() rec.Map {
	linked := func(text string) rec.Map {
		return rec.Map{"type": "text", "text": text, extlinks.Field: "n-link"}
	}
	return rec.Map{
		"schema_version": "paper-0.2",
		"block_order":    []any{"n-block"},
		"nodes": rec.Map{"n-block": rec.Map{"content": []any{
			rec.Map{"type": "text", "text": "arXiv:"}, linked(" "), linked("[cs.CG]")}}},
		"external_links": rec.Map{"items": rec.Map{"n-link": rec.Map{"href": "https://arxiv.org/abs/1701.00002"}}},
	}
}

// pageLinks is the inventory's links for the document's descriptors at the given spans.
func pageLinks(document rec.Map, spans ...int64) []any {
	shown := []any{}
	for _, row := range extlinks.Descriptors(document, true) {
		for _, span := range spans {
			if rec.Equal(row["span"], span) {
				shown = append(shown, rec.Merge(row, rec.Map{"rel": "noopener noreferrer", "visible": true}))
			}
		}
	}
	return shown
}

func frontmatterInventory(links []any) rec.Map {
	return rec.Map{"frontmatter": rec.Map{"metadata": nil, "decorations": []any{}, "links": links}}
}

func verifyFails(inventory, document rec.Map) (failed bool) {
	defer func() { failed = recover() != nil }()
	verifyFrontmatter(inventory, document)
	return false
}

func TestABlankTextLinkIsNoAnchorOnThePage(t *testing.T) {
	document := blankLinkDocument()
	if verifyFails(frontmatterInventory(pageLinks(document, 2)), document) {
		t.Error("the page without an anchor for the blank link failed")
	}
	if !verifyFails(frontmatterInventory(pageLinks(document, 1, 2)), document) {
		t.Error("an anchor showing the blank link passed")
	}
	if !verifyFails(frontmatterInventory(pageLinks(document)), document) {
		t.Error("a page without the named link passed")
	}
}

func TestABlankTextLinkIsNoBrokenLink(t *testing.T) {
	document := blankLinkDocument()
	inventory := rec.Map{"unsafeNodes": int64(0), "brokenLinks": int64(1), "resources": []any{}}
	if got := safeInventory(inventory, document, 0)["brokenLinks"]; !rec.Equal(got, int64(0)) {
		t.Errorf("brokenLinks = %v, want 0 for the one anchored link", got)
	}
}
