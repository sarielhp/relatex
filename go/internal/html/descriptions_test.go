package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/rec"
)

// imageHTML is the page's markup for the first figure of document.
func imageHTML(document rec.Map) string {
	displayed, _ := figures.Plan(document)
	p := &Paper{document: document, figures: displayed}
	node := rec.AsMap(rec.FetchMap(document, "nodes")["n-0"])
	return p.imageSpan(rec.AsMap(rec.FetchList(node, "content")[0]))
}

// paragraphHTML is the page's markup for the first figure's block, as a paragraph.
func paragraphHTML(document rec.Map) string {
	rec.AsMap(rec.FetchMap(document, "nodes")["n-0"])["type"] = "paragraph"
	displayed, _ := figures.Plan(document)
	return (&Paper{document: document, nodes: rec.FetchMap(document, "nodes"), figures: displayed}).RenderNode("n-0")
}

// TestImageSpanWritesDecorativeAndLongDescriptions: a decorative image (decision 096) has an
// empty alternative with role="presentation"; a long description (decision 110) is a disclosure
// after the image's block, which the image names with aria-details, its text with aria-describedby.
func TestImageSpanWritesDecorativeAndLongDescriptions(t *testing.T) {
	plain := imageHTML(figurePaper(figure("figs/plot.pdf", "10")))
	if !strings.HasSuffix(plain, ` alt="Undescribed image" data-undescribed="true"/>`) || strings.Contains(plain, "role=") {
		t.Errorf("plain image %s", plain)
	}
	decorative := figure("figs/rule.pdf", "10")
	decorative.span["decorative"], decorative.span["alt_provenance"] = true, "source"
	if got := imageHTML(figurePaper(decorative)); !strings.HasSuffix(got, ` alt="" role="presentation"/>`) {
		t.Errorf("decorative image %s", got)
	}
	described := figure("figs/plot.pdf", "10")
	described.span["alt"], described.span["alt_provenance"] = "A plot.", "source"
	described.span["long_description"] = "The error falls <fast>."
	document := figurePaper(described)
	got := imageHTML(document)
	want := `alt="A plot." aria-describedby="n-0-image-description" aria-details="n-0-image-details"/>`
	if !strings.HasPrefix(got, `<img id="n-0-image"`) || !strings.HasSuffix(got, want) {
		t.Errorf("described image %s", got)
	}
	disclosure := `</p><details id="n-0-image-details" data-kind="long-description" class="long-description">` +
		`<summary>Description of the figure</summary><p id="n-0-image-description">The error falls &lt;fast&gt;.</p></details>`
	if page := paragraphHTML(document); !strings.HasSuffix(page, disclosure) {
		t.Errorf("described paragraph %s", page)
	}
	document["language"] = rec.Map{"tag": "de"}
	if german := paragraphHTML(document); !strings.Contains(german, `<summary lang="en">`) {
		t.Errorf("a German page's disclosure label %s", german)
	}
}

// TestImageDetailsAreADisclosureInsideTheDescription: a figure's details (decision 125) are their own
// disclosure, "More about this figure", inside the description's after the short text; the image
// names the short text only, and a page in another language marks both labels English.
func TestImageDetailsAreADisclosureInsideTheDescription(t *testing.T) {
	described := figure("figs/plot.pdf", "10")
	described.span["alt"], described.span["alt_provenance"] = "A plot.", "reviewed"
	described.span["long_description"] = "The error falls."
	described.span["description_details"] = "Step 1 halves it; step 2 <again>."
	document := figurePaper(described)
	want := `</p><details id="n-0-image-details" data-kind="long-description" class="long-description">` +
		`<summary>Description of the figure</summary><p id="n-0-image-description">The error falls.</p>` +
		`<details class="figure-details"><summary>More about this figure</summary><p>Step 1 halves it; step 2 &lt;again&gt;.</p></details></details>`
	if page := paragraphHTML(document); !strings.HasSuffix(page, want) {
		t.Errorf("paragraph with details %s", page)
	}
	if got := imageHTML(document); !strings.Contains(got, `aria-describedby="n-0-image-description" aria-details="n-0-image-details"`) {
		t.Errorf("image %s", got)
	}
	document["language"] = rec.Map{"tag": "de"}
	if german := paragraphHTML(document); strings.Count(german, `<summary lang="en">`) != 2 {
		t.Errorf("a German page's labels %s", german)
	}
}

// TestRelatexAltWritesTheSameDisclosures: a description from \relatexalt (provenance source_macro,
// decision 125 amendment 2) is written exactly as the sidecar's: the alt, the short description's
// disclosure and the details nested in it.
func TestRelatexAltWritesTheSameDisclosures(t *testing.T) {
	markup := func(provenance string) (string, string) {
		described := figure("figs/plot.pdf", "10")
		described.span["alt"], described.span["alt_provenance"] = "A plot.", provenance
		described.span["long_description"] = "The error falls."
		described.span["description_details"] = "Step 1 halves it."
		document := figurePaper(described)
		return imageHTML(document), paragraphHTML(document)
	}
	image, page := markup("source_macro")
	sidecarImage, sidecarPage := markup("reviewed")
	if image != sidecarImage || page != sidecarPage {
		t.Errorf("source_macro differs from the sidecar's markup:\n%s\n%s", page, sidecarPage)
	}
	if !strings.Contains(page, `<details class="figure-details"><summary>More about this figure</summary><p>Step 1 halves it.</p></details></details>`) {
		t.Errorf("details %s", page)
	}
}

// TestImageInACaptionedFloatIsNamedByItsCaption: an undescribed image inside a captioned float
// takes the caption's label as its alt and takes it as its label.
func TestImageInACaptionedFloatIsNamedByItsCaption(t *testing.T) {
	document := figurePaper(figure("figs/plot.pdf", "10"))
	nodes := rec.FetchMap(document, "nodes")
	rec.AsMap(nodes["n-0"])["type"] = "paragraph"
	caption := rec.Map{"id": "cap", "type": "float_caption", "children": []any{"cap-text"}}
	nodes["cap"] = caption
	nodes["cap-text"] = rec.Map{"id": "cap-text", "type": "paragraph", "children": []any{},
		"content": []any{rec.Map{"type": "text", "text": "Figure 3: A plot."}}}
	nodes["f"] = rec.Map{"id": "f", "type": "figure_float", "children": []any{"cap", "n-0"}}
	displayed, _ := figures.Plan(document)
	page := (&Paper{document: document, nodes: nodes, figures: displayed}).RenderNode("f")
	for _, want := range []string{`alt="Figure 3"`, ` data-undescribed="true"`,
		`<figcaption id="cap" data-structure="float_caption">`} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q in %s", want, page)
		}
	}
	// The figcaption names the figure natively: nothing points at the caption again (decision 183).
	for _, unwanted := range []string{"aria-describedby", "aria-labelledby"} {
		if strings.Contains(page, unwanted) {
			t.Errorf("%s repeats the caption: %s", unwanted, page)
		}
	}
}
