package sourcealt

import (
	"os"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/authorhints"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/rec"
)

// hintContract reads the hints schema the parser validates relatex/hints.json against.
func hintContract(t *testing.T) rec.Map {
	t.Helper()
	data, err := os.ReadFile("../../../schemas/relatex-hints.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	value, err := canonjson.Decode(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return rec.AsMap(value)
}

// hintDocument holds two figures: fig:disk names the first, whose images one the source describes
// and one it marks decorative; fig:plain names the second.
func hintDocument() rec.Map {
	nodes := rec.Map{
		"n-root":    rec.Map{"type": "document", "children": []any{"n-figure", "n-figure2"}},
		"n-figure":  rec.Map{"type": "figure_float", "children": []any{"n-body", "n-caption"}},
		"n-body":    rec.Map{"type": "paragraph", "content": []any{rec.Map{"type": "image", "id": "n-image", "alt": "source text", "alt_provenance": "source"}}},
		"n-caption": rec.Map{"type": "float_caption", "children": []any{"n-text"}},
		"n-text":    rec.Map{"type": "paragraph", "content": []any{}},
		"n-figure2": rec.Map{"type": "figure_float", "children": []any{"n-body2", "n-caption2"}},
		"n-body2": rec.Map{"type": "paragraph", "content": []any{
			rec.Map{"type": "image", "id": "n-image2", "alt": "keep me", "alt_provenance": "source"},
			rec.Map{"type": "image", "id": "n-mark", "decorative": true, "alt_provenance": "source"},
		}},
		"n-caption2": rec.Map{"type": "float_caption", "children": []any{"n-text2"}},
		"n-text2":    rec.Map{"type": "paragraph", "content": []any{}},
	}
	return rec.Map{"root": "n-root", "nodes": nodes, "labels": rec.Map{
		"fig:disk":  rec.Map{"target": "n-text", "number": "1", "status": "verified"},
		"fig:plain": rec.Map{"target": "n-text2", "number": "2", "status": "verified"},
	}}
}

// hintImage finds an image span by its id.
func hintImage(t *testing.T, document rec.Map, id string) rec.Map {
	t.Helper()
	for _, value := range rec.FetchMap(document, "nodes") {
		content, _ := rec.AsMap(value)["content"].([]any)
		for _, item := range content {
			if image, ok := item.(rec.Map); ok && image["id"] == id {
				return image
			}
		}
	}
	t.Fatalf("no image %s", id)
	return nil
}

// TestApplyHintsByLabel: an alt hint keyed by a figure's label describes every image of that
// figure and beats the source's own description, as a source description no sidecar entry or model
// draft replaces; another figure keeps its source description.
func TestApplyHintsByLabel(t *testing.T) {
	document := hintDocument()
	hints := &authorhints.Hints{Sections: map[string]map[string]authorhints.Hint{
		"alt": {"fig:disk": {Value: "A red disk"}}}}
	if got := ApplyHints(document, hints); got != 1 {
		t.Errorf("described %d images, want 1", got)
	}
	image := hintImage(t, document, "n-image")
	if image["alt"] != "A red disk" || !figdesc.FromSource(image) {
		t.Errorf("image = %v", image)
	}
	if other := hintImage(t, document, "n-image2"); other["alt"] != "keep me" {
		t.Errorf("an unhinted figure changed: %v", other)
	}
}

// TestApplyHintsReplacesDecorativeMark: a hinted image the source called decorative becomes described.
func TestApplyHintsReplacesDecorativeMark(t *testing.T) {
	document := hintDocument()
	hints := &authorhints.Hints{Sections: map[string]map[string]authorhints.Hint{
		"alt": {"fig:plain": {Value: "Two arrows"}}}}
	ApplyHints(document, hints)
	for _, id := range []string{"n-image2", "n-mark"} {
		image := hintImage(t, document, id)
		if image["alt"] != "Two arrows" || !figdesc.FromSource(image) {
			t.Errorf("%s = %v", id, image)
		}
		if _, decorative := image["decorative"]; decorative {
			t.Errorf("%s is still decorative", id)
		}
	}
}

// TestApplyHintsUnknownLabel: a hint for a label the document does not have changes nothing.
func TestApplyHintsUnknownLabel(t *testing.T) {
	document := hintDocument()
	hints := &authorhints.Hints{Sections: map[string]map[string]authorhints.Hint{
		"alt": {"fig:missing": {Value: "Nowhere"}}}}
	if got := ApplyHints(document, hints); got != 0 {
		t.Errorf("described %d images, want 0", got)
	}
	if image := hintImage(t, document, "n-image"); image["alt"] != "source text" {
		t.Errorf("image = %v", image)
	}
}

// TestApplyHintsJSONWinsOverComment: relatex/hints.json's alt hint overrides one written in a comment.
func TestApplyHintsJSONWinsOverComment(t *testing.T) {
	hints, err := authorhints.Parse([]authorhints.Source{{Path: "main.tex",
		Text: "%% relatex alt: fig:disk = \"from comment\"\n"}}, []byte(`{"alt": {"fig:disk": "from json"}}`), hintContract(t))
	if err != nil {
		t.Fatal(err)
	}
	document := hintDocument()
	ApplyHints(document, hints)
	if image := hintImage(t, document, "n-image"); image["alt"] != "from json" {
		t.Errorf("image = %v", image)
	}
}
