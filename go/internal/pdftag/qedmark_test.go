package pdftag

import "testing"

// An image QED mark (decision 176, amendment 1) is a Figure named "End of proof", not an artifact.
func TestImageQEDMarkIsAFigureNamedEndOfProof(t *testing.T) {
	b := newBuilder(&document{})
	item := markImage(contentItem{Type: "symbol", Image: &contentItem{Type: "image", ID: "duck", Source: "duck.pdf", Alt: "End of proof"}})
	figure := b.figure("p", 0, item)
	if figure == nil || figure.alt != "End of proof" || len(b.undescribe) != 0 {
		t.Fatalf("figure %+v, undescribed %v", figure, b.undescribe)
	}
	if text := markImage(contentItem{Type: "symbol"}); text.Type != "symbol" {
		t.Fatalf("a text mark is no image: %+v", text)
	}
}
