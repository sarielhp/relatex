package bookindex

import (
	"regexp"
	"slices"

	"github.com/sarielhp/relatex/internal/rec"
)

var anchorID = regexp.MustCompile(`\Aidx-[1-9][0-9]*\z`)

// Validate checks a document's index graph: every anchor names a block of the document once,
// and every placed page number is the text of its span at its offset and links only anchors
// the graph holds.
func Validate(document rec.Map) {
	value, present := document["index"]
	if !present {
		return
	}
	graph, ok := value.(rec.Map)
	if !ok || !rec.SameKeys(graph, []string{"anchors", "indexes", "schema_version"}) || graph["schema_version"] != Version {
		rec.Fail("Index graph is malformed")
	}
	nodes := rec.FetchMap(document, "nodes")
	ids := map[string]bool{}
	for _, value := range rec.FetchList(graph, "anchors") {
		a := rec.AsMap(value)
		id := rec.FetchString(a, "id")
		if !rec.SameKeys(a, []string{"block", "id", "key", "page", "span"}) || !anchorID.MatchString(id) || ids[id] || !isTextSpan(nodes, a["block"], a["span"]) {
			rec.Fail("Malformed index anchor")
		}
		ids[id] = true
	}
	for _, value := range rec.FetchList(graph, "indexes") {
		for _, entry := range rec.FetchList(rec.AsMap(value), "entries") {
			for _, locator := range rec.FetchList(rec.AsMap(entry), "locators") {
				validateLocator(nodes, rec.AsMap(locator), ids)
			}
		}
	}
}

func isBlock(nodes rec.Map, id any) bool {
	name, ok := id.(string)
	node, found := nodes[name].(rec.Map)
	return ok && found && slices.Contains([]any{"heading", "paragraph", "display_math"}, node["type"])
}

// isTextSpan: span is the index of a text span of block.
func isTextSpan(nodes rec.Map, block, span any) bool {
	index, ok := span.(int64)
	if !isBlock(nodes, block) || !ok {
		return false
	}
	content := rec.FetchList(rec.AsMap(nodes[block.(string)]), "content")
	return index >= 0 && index < int64(len(content)) && rec.AsMap(content[index])["type"] == "text"
}

func validateLocator(nodes rec.Map, locator rec.Map, ids map[string]bool) {
	if locator["kind"] != "page" {
		return
	}
	for _, id := range rec.FetchList(locator, "anchors") {
		if name, ok := id.(string); !ok || !ids[name] {
			rec.Fail("Index link to an unknown anchor")
		}
	}
	if _, placed := locator["block"]; !placed {
		return
	}
	if !isBlock(nodes, locator["block"]) {
		rec.Fail("Index page number outside a block")
	}
	content := rec.FetchList(rec.AsMap(nodes[rec.FetchString(locator, "block")]), "content")
	span, start := int(rec.FetchInt(locator, "span")), int(rec.FetchInt(locator, "start"))
	if span < 0 || span >= len(content) || rec.AsMap(content[span])["type"] != "text" {
		rec.Fail("Index page number outside a text span")
	}
	text, page := []rune(rec.ToS(rec.AsMap(content[span])["text"])), []rune(rec.FetchString(locator, "text"))
	if start < 0 || start+len(page) > len(text) || string(text[start:start+len(page)]) != string(page) {
		rec.Fail("Index page number differs from its text")
	}
}

// Links are the placed page numbers of a document's index with anchors: for each block node, its
// spans' links (span index, rune offset, printed text, first anchor).
func Links(document rec.Map) map[string][]rec.Map {
	result := map[string][]rec.Map{}
	graph, _ := document["index"].(rec.Map)
	indexes, _ := graph["indexes"].([]any)
	for _, value := range indexes {
		for _, entry := range rec.FetchList(rec.AsMap(value), "entries") {
			for _, item := range rec.FetchList(rec.AsMap(entry), "locators") {
				locator := rec.AsMap(item)
				anchors, _ := locator["anchors"].([]any)
				block, placed := locator["block"].(string)
				if !placed || len(anchors) == 0 {
					continue
				}
				result[block] = append(result[block], rec.Map{"span": locator["span"], "start": locator["start"],
					"text": locator["text"], "anchor": anchors[0]})
			}
		}
	}
	return result
}

// Anchors are the index anchors of a document by block node, in issue order.
func Anchors(document rec.Map) map[string][]rec.Map {
	result := map[string][]rec.Map{}
	graph, _ := document["index"].(rec.Map)
	anchors, _ := graph["anchors"].([]any)
	for _, value := range anchors {
		a := rec.AsMap(value)
		block := rec.FetchString(a, "block")
		result[block] = append(result[block], a)
	}
	return result
}
