package accessibility

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

const attestedCaption = "Figure 1: measured (solid blue) and reconstructed (dashed red) traces."

// attestFixture is a figure float with a label whose caption names colours.
func attestFixture(attestations rec.Map) rec.Map {
	document := rec.Map{
		"block_order": []any{"c1"},
		"labels":      rec.Map{"fig:traces": rec.Map{"target": "f1", "number": "1"}},
		"nodes": rec.Map{
			"root": rec.Map{"id": "root", "type": "document", "children": []any{"f1"}},
			"f1":   rec.Map{"id": "f1", "type": "figure_float", "children": []any{"k1"}},
			"k1":   rec.Map{"id": "k1", "type": "float_caption", "children": []any{"c1"}},
			"c1": rec.Map{"id": "c1", "type": "paragraph", "children": []any{},
				"content": []any{rec.Map{"type": "text", "text": attestedCaption}}},
		},
	}
	if attestations != nil {
		document["colour_captions"] = attestations
	}
	return document
}

// A caption attested by its float's label, caption number or caption name, with the sha of its text,
// is attested: listed with its key, never a gap; the row says attested and the claim can follow.
func TestAttestedColourCaptionIsNotAGap(t *testing.T) {
	sha := CaptionSHA(attestedCaption)
	for _, key := range []string{"fig:traces", "1", "Figure 1"} {
		document := attestFixture(rec.Map{key: sha})
		row := colourRow(document, newWalk(document, nil))
		if row["status"] != "attested" || row["count"] != int64(0) || len(rec.SliceOf(row["items"])) != 0 {
			t.Fatalf("key %q: row = %v", key, row)
		}
		attested := rec.SliceOf(row["attested"])
		if len(attested) != 1 || rec.MapOf(attested[0])["key"] != key {
			t.Errorf("key %q: attested = %v", key, attested)
		}
	}
}

// Without an attestation, with another float's key or after an edit, the caption stays a gap; an
// edited one says its attestation is void. Nothing is attested without the author's key.
func TestEditedOrMissingAttestationIsAGap(t *testing.T) {
	stale := CaptionSHA("Figure 1: measured (solid blue) traces.")
	cases := map[string]rec.Map{"none": nil, "other key": {"fig:other": CaptionSHA(attestedCaption)}, "edited": {"fig:traces": stale}}
	for name, attestations := range cases {
		document := attestFixture(attestations)
		row := colourRow(document, newWalk(document, nil))
		if row["status"] != "gap" || row["count"] != int64(1) {
			t.Fatalf("%s: row = %v", name, row)
		}
		if _, has := row["attested"]; has {
			t.Errorf("%s: attested = %v", name, row["attested"])
		}
		item := rec.MapOf(rec.SliceOf(row["items"])[0])
		if item["sha256"] != CaptionSHA(attestedCaption) {
			t.Errorf("%s: item = %v", name, item)
		}
		if _, voided := item["attestation"]; voided != (name == "edited") {
			t.Errorf("%s: item = %v", name, item)
		}
	}
}

// An attested row adds no open item, so the claim depends on the other checks only.
func TestAttestedColourRowRaisesNoOpenItem(t *testing.T) {
	document := attestFixture(rec.Map{"fig:traces": CaptionSHA(attestedCaption)})
	row := colourRow(document, newWalk(document, nil))
	if items := openItems(nil, []any{row}); len(items) != 0 {
		t.Errorf("open items = %v", items)
	}
}
