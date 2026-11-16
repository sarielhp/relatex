package alttext

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// pictureAsset is an asset whose displayed form is shown while its original is original: two
// assets sharing an original are one picture the author included twice.
func pictureAsset(original, shown []byte) rec.Map {
	source, display := mediamodel.Encoded(original, "image/png"), mediamodel.Encoded(shown, "image/png")
	return rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": source["sha256"],
		"representations": rec.Map{"original": source, "html": display}}
}

// pictureDocument is a document with three figures, two of them the same picture under two names:
// figs/disk.png and figs/disk-copy.png hold the same original, figs/plot.png other bytes.
func pictureDocument() rec.Map {
	first, second := pictureAsset(pngBytes, pngBytes), pictureAsset(pngBytes, append([]byte{}, append(pngBytes, 'o')...))
	plot := pictureAsset(append([]byte{}, append(pngBytes, '2')...), append([]byte{}, append(pngBytes, '2')...))
	image := func(asset, source string) rec.Map {
		return rec.Map{"type": "image", "asset": asset, "source": source, "width_sp": int64(10), "height_sp": int64(10)}
	}
	return rec.Map{
		"language":    rec.Map{"tag": "en"},
		"assets":      rec.Map{"items": rec.Map{"first": first, "second": second, "plot": plot}},
		"block_order": []any{"p1", "p2", "p3"},
		"nodes": rec.Map{
			"p1": rec.Map{"id": "p1", "type": "paragraph", "children": []any{}, "content": []any{
				image("first", "figures/disk.png")}},
			"p2": rec.Map{"id": "p2", "type": "paragraph", "children": []any{}, "content": []any{
				image("second", "figures/disk-copy.png")}},
			"p3": rec.Map{"id": "p3", "type": "paragraph", "children": []any{}, "content": []any{
				image("plot", "figures/plot.png")}},
		},
	}
}

// The page is a local tool for a book's figures: its pictures load as they are scrolled to, its top
// says how many figures are in each state and jumps to the first, and two names for one picture get
// one decision form instead of a second copy of it.
func TestReviewPageJumpsAndSharesOneFormPerPicture(t *testing.T) {
	figures := figdesc.Figures(pictureDocument())
	if len(figures) != 3 {
		t.Fatalf("figures %+v", figures)
	}
	page := ReviewPage(figures, &figdesc.Sidecar{Entries: map[string]*figdesc.Entry{}})
	if images := strings.Count(page, " loading=\"lazy\" decoding=\"async\">"); images != len(figures) {
		t.Errorf("%d lazy images for %d figures", images, len(figures))
	}
	for _, want := range []string{`<nav class="jump">`, `href="#fig-disk">3 without a description</a>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if forms := strings.Count(page, `<fieldset class="decision"`); forms != 2 {
		t.Errorf("%d decision forms for 3 figures, one a second name for another's picture", forms)
	}
	for _, want := range []string{`<input type="hidden" class="group-key" value="disk-copy">`,
		`Same picture as <a href="#fig-disk">disk</a>`, "covered(f).forEach"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
}

// Two names for one picture keep two forms when they show different texts, or the same text in
// different states (a draft and the author's text: "Tick every draft" and import treat them apart).
func TestReviewPageKeepsFormsForDifferentTextsOrStates(t *testing.T) {
	figures := figdesc.Figures(pictureDocument())
	for name, entries := range map[string]map[string]*figdesc.Entry{
		"different text": {
			"disk":      {Key: "disk", Alt: "A disk.", Provenance: figdesc.Model},
			"disk-copy": {Key: "disk-copy", Alt: "Another disk.", Provenance: figdesc.Model}},
		"different state": {
			"disk":      {Key: "disk", Alt: "A disk.", Provenance: figdesc.Model},
			"disk-copy": {Key: "disk-copy", Alt: "A disk.", Provenance: figdesc.Author}},
	} {
		page := ReviewPage(figures, &figdesc.Sidecar{Entries: entries})
		if forms := strings.Count(page, `<fieldset class="decision"`); forms != 3 {
			t.Errorf("%s: %d decision forms for 3 figures", name, forms)
		}
		for _, unwanted := range []string{"Same picture as", `class="group-key"`} {
			if strings.Contains(page, unwanted) {
				t.Errorf("%s: the page has %q", name, unwanted)
			}
		}
	}
}
