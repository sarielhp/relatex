package htmlkit

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// captionOf is a float caption holding one text run, with the document nodes it needs.
func captionOf(text string) (rec.Map, rec.Map) {
	nodes := rec.Map{"b": rec.Map{"id": "b", "type": "paragraph",
		"content": []any{rec.Map{"type": "text", "text": text}}}}
	return rec.Map{"id": "c", "type": "float_caption", "children": []any{"b"}}, nodes
}

func TestCaptionLabelIsTheTypesetLabel(t *testing.T) {
	for _, test := range []struct{ caption, want string }{
		{"Figure 3: A plot of the error.", "Figure 3"},
		{"Fig. 5: Accuracy of our neural net.", "Fig. 5"},
		{"Figure 2. Intersection of an oblique line.", "Figure 2"},
		{"Table 12: Super learner libraries.", "Table 12"},
		{"TABLE I: Detailed overview.", "TABLE I"},
		{"Algorithm 2: Greedy matching.", "Algorithm 2"},
		{"A caption with no label.", ""},
		{"Overview of the 3 methods.", ""},
		{"Plot of mild growth.", ""},
		{"Figure did not converge.", ""},
	} {
		caption, nodes := captionOf(test.caption)
		if got := CaptionLabel(caption, nodes); got != test.want {
			t.Errorf("CaptionLabel(%q) = %q, want %q", test.caption, got, test.want)
		}
	}
	if got := CaptionLabel(nil, rec.Map{}); got != "" {
		t.Errorf("CaptionLabel outside a float = %q, want empty", got)
	}
}

// TestImageAlternative: a description wins; an undescribed image in a captioned float takes the
// caption's label; anywhere else it says so, as the biography photos outside a float do.
func TestImageAlternative(t *testing.T) {
	described := rec.Map{"type": "image", "alt": "A plot of the error."}
	if Undescribed(described) || ImageAlt(described, "Figure 3") != "A plot of the error." {
		t.Errorf("a described image lost its description")
	}
	decorative := rec.Map{"type": "image", "decorative": true}
	if Undescribed(decorative) {
		t.Errorf("a decorative image is not undescribed")
	}
	captioned := rec.Map{"type": "image"}
	if !Undescribed(captioned) || ImageAlt(captioned, "Figure 3") != "Figure 3" {
		t.Errorf("an undescribed image in a captioned float takes the caption's label")
	}
	outside := rec.Map{"type": "image"}
	if !Undescribed(outside) || ImageAlt(outside, "") != "Undescribed image" {
		t.Errorf("an undescribed image outside a float says so")
	}
}
