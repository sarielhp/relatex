package frontmatter

import (
	"reflect"
	"slices"

	"github.com/sarielhp/relatex/internal/extlinks"
	"github.com/sarielhp/relatex/internal/rec"
)

// projectLinks is PaperFrontmatter.links: the author emails and (without executed links)
// the source's own link declarations, each bound to the inline text or formula printing it.
func projectLinks(parser *TexMetadata, metadata, document, identity rec.Map, sourceLinks bool) []any {
	declarations := []rec.Map{}
	for _, value := range rec.FetchList(metadata, "authors") {
		for _, address := range rec.FetchList(rec.AsMap(value), "emails") {
			text, _ := address.(string)
			if !email.MatchString(text) {
				rec.Fail("Unsupported author email")
			}
			declarations = append(declarations, rec.Map{"href": "mailto:" + text, "text": text})
		}
	}
	if sourceLinks {
		declarations = append(declarations, parser.Links()...)
	}
	projected := []rec.Map{}
	for _, declaration := range uniqueMaps(declarations) {
		extlinks.SafeHref(rec.Fetch(declaration, "href"))
		matches := append(mathLinks(declaration, document, identity), textLinks(declaration, document, identity)...)
		if len(matches) == 0 {
			rec.Fail("Source link has no exact observed inline payload")
		}
		projected = append(projected, matches...)
	}
	if !sourceLinks {
		native := extlinks.NativeDescriptors(document, false)
		// A native link may include adjacent spacing; any projected range it already covers
		// would render twice.
		projected = slices.DeleteFunc(projected, func(link rec.Map) bool {
			return slices.ContainsFunc(native, func(observed rec.Map) bool { return covers(observed, link) })
		})
	}
	locations := make(map[uintptr][]int, len(projected))
	for _, link := range projected {
		locations[rec.Identity(link)] = linkLocation(link, document)
	}
	slices.SortStableFunc(projected, func(a, b rec.Map) int {
		return slices.Compare(locations[rec.Identity(a)], locations[rec.Identity(b)])
	})
	result := make([]any, len(projected))
	for index, link := range projected {
		result[index] = link
	}
	return result
}

func uniqueMaps(values []rec.Map) []rec.Map {
	result := []rec.Map{}
	for _, value := range values {
		if !slices.ContainsFunc(result, func(seen rec.Map) bool { return reflect.DeepEqual(seen, value) }) {
			result = append(result, value)
		}
	}
	return result
}

func covers(native, link rec.Map) bool {
	for _, key := range []string{"href", "block", "span", "cell"} {
		if !rec.Equal(native[key], link[key]) {
			return false
		}
	}
	_, nativeStart := native["start"]
	_, linkStart := link["start"]
	if !nativeStart || !linkStart {
		return true
	}
	start, nativeBegin := rec.FetchInt(link, "start"), rec.FetchInt(native, "start")
	return nativeBegin <= start &&
		start+int64(len([]rune(rec.FetchString(link, "text")))) <= nativeBegin+int64(len([]rune(rec.FetchString(native, "text"))))
}

func mathLinks(declaration, document, identity rec.Map) []rec.Map {
	result := []rec.Map{}
	for _, id := range sortedKeys(nodes(document)) {
		item := rec.AsMap(nodes(document)[id])
		if item["type"] != "math" || extlinks.MathText(rec.FetchString(item, "mathml")) != rec.Fetch(declaration, "text") {
			continue
		}
		key := []any{rec.Fetch(declaration, "href"), rec.Fetch(item, "id")}
		link := clone(declaration)
		link["id"], link["math_node"] = identifier(identity, "external-link", key), rec.Fetch(item, "id")
		result = append(result, link)
	}
	return result
}

func sortedKeys(m rec.Map) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func clone(m rec.Map) rec.Map {
	result := rec.Map{}
	for key, value := range m {
		result[key] = value
	}
	return result
}

// textLinks: a link is found in a block's text or in the text of a cell of its table (an
// author table), named by block, cell, span and start.
func textLinks(declaration, document, identity rec.Map) []rec.Map {
	if _, ok := document["block_order"]; !ok {
		return []rec.Map{}
	}
	result := []rec.Map{}
	label := []rune(rec.FetchString(declaration, "text"))
	for _, location := range extlinks.Locations(document, false) {
		span := rec.FetchMap(location, "value")
		if span["type"] != "text" {
			continue
		}
		for _, start := range occurrences([]rune(rec.FetchString(span, "text")), label) {
			if _, cited := span["citation_link"]; cited {
				rec.Fail("External link overlaps an internal citation link")
			}
			link := clone(declaration)
			key := []any{rec.Fetch(declaration, "href"), location["block"]}
			link["block"] = location["block"]
			if cell, ok := location["cell"]; ok {
				link["cell"] = cell
				if cell != nil {
					key = append(key, cell)
				}
			}
			link["span"], link["start"] = location["span"], int64(start)
			key = append(key, location["span"], int64(start))
			link["id"] = identifier(identity, "external-link", key)
			result = append(result, link)
		}
	}
	return result
}

// occurrences are the character offsets of label in text, without overlap.
func occurrences(text, label []rune) []int {
	if len(label) == 0 {
		rec.Fail("Source link has an empty label")
	}
	result := []int{}
	for cursor := 0; ; {
		start := runeIndex(text, label, cursor)
		if start < 0 {
			return result
		}
		result = append(result, start)
		cursor = start + len(label)
	}
}

func linkLocation(link, document rec.Map) []int {
	order := rec.FetchList(document, "block_order")
	if block, ok := link["block"]; ok {
		return []int{slices.IndexFunc(order, func(id any) bool { return rec.Equal(id, block) }), int(rec.FetchInt(link, "span")),
			int(rec.FetchInt(link, "start"))}
	}
	for index, block := range order {
		for position, value := range rec.FetchList(node(document, block), "content") {
			if rec.Equal(rec.AsMap(value)["node"], rec.Fetch(link, "math_node")) {
				return []int{index, position, 0}
			}
		}
	}
	return []int{0, 0, 0}
}
