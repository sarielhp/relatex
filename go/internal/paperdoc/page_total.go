package paperdoc

import (
	"strconv"

	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/rec"
)

func pageTotalReference(target rec.Map, id string) rec.Map {
	return rec.Map{"type": "reference", "text": rec.Fetch(target, "number"), "label": rec.Fetch(target, "label"), "target": id}
}

func hasReferenceLabel(nodes rec.Map, label any) bool {
	for _, value := range nodes {
		node := rec.AsMap(value)
		content, _ := node["content"].([]any)
		for _, span := range inline.Deep(content) {
			if span["type"] == "reference" && rec.Equal(span["label"], label) {
				return true
			}
		}
	}
	return false
}

func validPageTotalReference(span, node rec.Map, count int64) bool {
	return rec.SameKeys(span, []string{"type", "text", "label", "target"}) && span["type"] == "reference" &&
		span["text"] == strconv.FormatInt(count, 10) && span["label"] == node["source_label"] && span["target"] == node["id"]
}
