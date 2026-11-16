package structure

import (
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

func context(seq, entity, parent int64, role string) rec.Map {
	return rec.Map{"seq": seq, "kind": "context", "entity": entity, "parent": parent, "role": role}
}

func block(id string, entity int64) rec.Map { return rec.Map{"id": id, "entity": entity} }

func TestAsideIsDisplaced(t *testing.T) {
	records := []rec.Map{{"kind": "header", "protocol": "0.6"}, context(2, 1, 0, "section"), context(3, 2, 0, "section"),
		context(4, 3, 1, "paragraph"), context(5, 4, 2, "paragraph")}
	got, err := report(records, []rec.Map{block("b1", 3), block("b2", 4), block("b3", 3)})
	if err != nil {
		t.Fatal(err)
	}
	bytes, _ := canonjson.Marshal(got)
	want := `{"discarded_entities":[2],"displaced":["b2"],"nodes":[{"content":[{"structure":1}],"id":0,"role":"document"},` +
		`{"content":[{"block":"b1"},{"block":"b2"},{"block":"b3"}],"id":1,"role":"section"}],"status":"verified"}`
	if string(bytes) != want {
		t.Errorf("report = %s", bytes)
	}
}

// A document-level block printed between a section's blocks and a later subsection must not
// reopen the nodes it left behind; the later blocks join the open container as displaced.
func TestDocumentBlockBetweenSectionsDoesNotInterleave(t *testing.T) {
	records := []rec.Map{{"kind": "header", "protocol": "0.6"}, context(2, 1, 0, "section"), context(3, 2, 1, "section"),
		context(4, 3, 1, "section")}
	got, err := report(records, []rec.Map{block("b1", 2), block("b2", 0), block("b3", 3)})
	if err != nil {
		t.Fatal(err)
	}
	if got["status"] != "verified" {
		t.Errorf("report = %v", got)
	}
}

func TestFailures(t *testing.T) {
	header := rec.Map{"kind": "header", "protocol": "0.6"}
	cases := map[string][]rec.Map{
		"Duplicate structural entity":                 {header, context(2, 1, 0, "section"), context(3, 1, 0, "section")},
		"Invalid structural parent":                   {header, context(2, 1, 0, "paragraph"), context(3, 2, 1, "section")},
		"List context without external list protocol": {header, context(2, 1, 0, "ordered_list")},
	}
	for want, records := range cases {
		if _, err := report(records, nil); err == nil || err.Error() != want {
			t.Errorf("got %v, want %q", err, want)
		}
	}
	if _, err := report([]rec.Map{header}, []rec.Map{block("b1", 9)}); err == nil || err.Error() != "Unknown block context" {
		t.Errorf("unknown context: %v", err)
	}
	if got := Report([]rec.Map{{"protocol": "0.5"}}, nil); got["status"] != "unavailable" {
		t.Errorf("old protocol: %v", got)
	}
}

// A theorem set in a float's body (float_member) keeps its own node, printed with the float in
// the container open where the float is printed (here a later section).
func TestFloatMemberJoinsTheFloatsContainer(t *testing.T) {
	records := []rec.Map{{"kind": "header", "protocol": "0.6"}, context(2, 1, 0, "section"), context(3, 2, 1, "paragraph"),
		context(4, 3, 1, "paragraph"), {"kind": "diagnostic", "code": "float_context", "entity": int64(3), "parent": int64(1)},
		context(6, 4, 1, "theorem"), {"kind": "diagnostic", "code": "float_member", "entity": int64(4), "float": int64(3)},
		context(8, 5, 0, "section"), context(9, 6, 5, "paragraph")}
	got, err := report(records, []rec.Map{block("a", 2), block("b", 6), block("figure", 3), block("step", 4), block("c", 6)})
	if err != nil {
		t.Fatal(err)
	}
	bytes, _ := canonjson.Marshal(got["nodes"])
	want := `[{"content":[{"structure":1},{"structure":5}],"id":0,"role":"document"},{"content":[{"block":"a"}],"id":1,"role":"section"},` +
		`{"content":[{"block":"b"},{"block":"figure"},{"structure":4},{"block":"c"}],"id":5,"role":"section"},` +
		`{"content":[{"block":"step"}],"id":4,"role":"theorem"}]`
	if string(bytes) != want {
		t.Errorf("nodes = %s", bytes)
	}
}
