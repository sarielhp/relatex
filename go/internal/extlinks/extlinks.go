// Package extlinks is the Go port of Relatex::PaperExternalLinks' paper side: external links
// bound to executed nodes, independently of front matter (the record catalog is package
// links).
//
// It is in the model layer (decision 102): paperdoc projects and validates the link graph with
// it, and the outputs read the links (Descriptors, Resolve) here.
package extlinks

import (
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/links"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
	"github.com/sarielhp/relatex/internal/xmltree"
)

// Version is PaperExternalLinks::VERSION.
const Version = "external-links-1"

// Field is PaperExternalLinks::FIELD.
const Field = "external_link"

var nodeID = regexp.MustCompile(`\An-[0-9a-f]{64}\z`)

// Project is PaperExternalLinks.project: the external-link graph of a paper document, nil
// when the paper collected no external links.
func Project(inspection, data rec.Map, uid func(scope string, key any) string) rec.Map {
	rows := []rec.Map{}
	for _, value := range rec.FetchList(inspection, "diagnostics") {
		row := rec.AsMap(value)
		if row["code"] == "external_link" {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 && inspection["external_link_collection"] != "hyperref-execution-1" {
		return nil
	}
	definitions := links.Definitions(rows)
	ids := make([]int64, 0, len(definitions))
	for id := range definitions {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	items := rec.Map{}
	for _, id := range ids {
		row := definitions[id]
		if _, unsupported := row["unsupported"]; unsupported {
			continue
		}
		items[uid("external_link", id)] = rec.Map{"href": rec.Fetch(row, "href"), "native": id}
	}
	used := []any{}
	for _, entry := range Locations(data, false) {
		value := rec.FetchMap(entry, "value")[Field]
		if truthy(value) && !slices.Contains(used, value) {
			used = append(used, value)
		}
	}
	selected := rec.Map{}
	for _, id := range used {
		name, _ := id.(string)
		item, ok := items[name]
		if !ok {
			rec.Fail("Undeclared retained external link")
		}
		selected[name] = item
	}
	return rec.Map{"schema_version": Version, "items": selected}
}

func truthy(value any) bool { return value != nil && value != false }

// Locations is PaperExternalLinks.locations: every inline of the paper's blocks with its
// place (block, span; for a rich table cell's inline also table_span and cell).
func Locations(document rec.Map, presentation bool) []rec.Map {
	result := []rec.Map{}
	nodes := rec.FetchMap(document, "nodes")
	for _, block := range rec.FetchList(document, "block_order") {
		node := rec.AsMap(fetchKey(nodes, block))
		for index, value := range rec.FetchList(node, "content") {
			span := rec.AsMap(value)
			if span["type"] != "table_grid" {
				result = append(result, rec.Map{"block": block, "span": int64(index), "value": span})
				continue
			}
			grid, _ := span["grid"].(rec.Map)
			if grid["schema_version"] != tablemodel.Version {
				continue
			}
			for _, cell := range inline.Cells(span, presentation) {
				for position, item := range rec.FetchList(cell, "content") {
					result = append(result, rec.Map{"block": block, "table_span": int64(index), "cell": rec.Fetch(cell, "id"),
						"span": int64(position), "value": item})
				}
			}
		}
	}
	return result
}

func fetchKey(m rec.Map, key any) any {
	name, _ := key.(string)
	value, ok := m[name]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + inspect(key), Lookup: true})
	}
	return value
}

func inspect(value any) string {
	bytes, err := canonjson.Marshal(value)
	if err != nil {
		return "nil"
	}
	return string(bytes)
}

// Validate is PaperExternalLinks.validate!.
func Validate(document rec.Map) {
	graph, present := document["external_links"]
	used := []rec.Map{}
	for _, entry := range Locations(document, false) {
		if _, ok := rec.FetchMap(entry, "value")[Field]; ok {
			used = append(used, entry)
		}
	}
	if (!present || graph == nil) && len(used) == 0 {
		return
	}
	items := validGraph(graph)
	if document["schema_version"] != "paper-0.2" {
		rec.Fail("Executed links require paper-0.2")
	}
	ids := []string{}
	for _, entry := range used {
		id, _ := rec.Fetch(rec.FetchMap(entry, "value"), Field).(string)
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, ids) {
		rec.Fail("External link graph coverage differs")
	}
	for _, key := range keys {
		item, isMap := items[key].(rec.Map)
		native, isInt := item["native"].(int64)
		if !nodeID.MatchString(key) || !isMap || !rec.SameKeys(item, []string{"href", "native"}) || !isInt || native <= 0 {
			rec.Fail("Malformed external link identity")
		}
		SafeHref(rec.Fetch(item, "href"))
	}
	for _, entry := range used {
		span := rec.FetchMap(entry, "value")
		kind := rec.Fetch(span, "type")
		if _, cited := span["citation_link"]; (kind != "text" && kind != "math" && kind != "image") || cited {
			rec.Fail("Unsupported nested external/internal link")
		}
		if kind == "image" && !OrcidImage(document, span) {
			rec.Fail("A linked image is not the ORCID logo")
		}
	}
}

func validGraph(value any) rec.Map {
	graph, ok := value.(rec.Map)
	if !ok || !rec.SameKeys(graph, []string{"items", "schema_version"}) || graph["schema_version"] != Version {
		rec.Fail("External link graph is missing or malformed")
	}
	items, ok := graph["items"].(rec.Map)
	if !ok {
		rec.Fail("undefined method 'each' for %T", graph["items"])
	}
	return items
}

// SafeHref is PaperFrontmatter.safe_href!: the link a reader can follow, else a Failure
// with the reason.
func SafeHref(value any) string {
	href, ok := value.(string)
	if !ok {
		rec.Fail("Unsafe external link characters")
	}
	reason, err := links.SafeHref(href)
	if err != nil {
		panic(err)
	}
	if reason != "" {
		rec.Fail("%s", reason)
	}
	return href
}

// NativeDescriptors is PaperExternalLinks.native_descriptors: each executed link use, as
// the views place it.
func NativeDescriptors(document rec.Map, presentation bool) []rec.Map {
	graph, ok := document["external_links"].(rec.Map)
	if !ok {
		return []rec.Map{}
	}
	result := []rec.Map{}
	for _, entry := range Locations(document, presentation) {
		span := rec.FetchMap(entry, "value")
		if _, linked := span[Field]; !linked {
			continue
		}
		id := rec.Fetch(span, Field)
		href := rec.Fetch(rec.AsMap(fetchKey(rec.FetchMap(graph, "items"), id)), "href")
		key := rec.Map{}
		for name, value := range entry {
			if name != "value" {
				key[name] = value
			}
		}
		digest, err := canonjson.Digest(rec.Map{"link": id, "location": key})
		if err != nil {
			rec.Fail("%v", err)
		}
		linkID := "n-" + digest
		if span["type"] == "math" {
			node := rec.AsMap(fetchKey(rec.FetchMap(document, "nodes"), rec.Fetch(span, "node")))
			result = append(result, rec.Map{"id": linkID, "href": href, "text": MathText(rec.FetchString(node, "mathml")),
				"math_node": rec.Fetch(span, "node")})
			continue
		}
		// The ORCID logo's link holds only the logo: no text.
		text := any("")
		if span["type"] != "image" {
			text = rec.Fetch(span, "text")
		}
		descriptor := rec.Map{"id": linkID, "href": href, "text": text, "start": int64(0)}
		for name, value := range key {
			if name != "table_span" {
				descriptor[name] = value
			}
		}
		result = append(result, descriptor)
	}
	return result
}

// Descriptors is PaperExternalLinks.descriptors: front matter's source links, then the
// executed ones.
func Descriptors(document rec.Map, presentation bool) []rec.Map {
	result := []rec.Map{}
	if front, ok := document["frontmatter"].(rec.Map); ok {
		listed, _ := front["links"].([]any)
		for _, link := range listed {
			result = append(result, rec.AsMap(link))
		}
	}
	return append(result, NativeDescriptors(document, presentation)...)
}

// BlankText reports whether a text link's text is only spaces, which names nothing (WCAG 2.4.4/4.1.2); the
// page writes it as plain text, not as a nameless anchor (decision 135).
func BlankText(text string) bool { return strings.TrimSpace(text) == "" }

// Anchored is PaperExternalLinks.anchored: the descriptors the page writes as anchors, every
// one but a text link whose text is blank (decision 135). A math or image link is always one.
func Anchored(document rec.Map, descriptors []rec.Map) []rec.Map {
	result := []rec.Map{}
	for _, link := range descriptors {
		if _, inBlock := link["block"]; inBlock && Resolve(document, link)["type"] == "text" && BlankText(rec.ToS(link["text"])) {
			continue
		}
		result = append(result, link)
	}
	return result
}

// Resolve is PaperExternalLinks.resolve: the inline a text link names.
func Resolve(document, link rec.Map) rec.Map {
	content := rec.FetchList(rec.AsMap(fetchKey(rec.FetchMap(document, "nodes"), rec.Fetch(link, "block"))), "content")
	position := int(rec.FetchInt(link, "span"))
	if _, inCell := link["cell"]; !inCell {
		return rec.AsMap(rec.Index(content, position))
	}
	for _, value := range content {
		span := rec.AsMap(value)
		if span["type"] != "table_grid" {
			continue
		}
		for _, cell := range rec.FetchList(rec.FetchMap(span, "grid"), "cells") {
			if item := rec.AsMap(cell); rec.Equal(rec.Fetch(item, "id"), rec.Fetch(link, "cell")) {
				return rec.AsMap(rec.Index(rec.FetchList(item, "content"), position))
			}
		}
	}
	rec.Fail("External link has no owned table cell")
	return nil
}

// MathText is PaperFrontmatter.math_text: the text of a formula's MathML, a square root
// printing its radical sign before its radicand.
func MathText(mathml string) string {
	root, err := xmltree.Parse(mathml)
	if err != nil {
		rec.Fail("%v", err)
	}
	var builder strings.Builder
	mathTextOf(root, &builder)
	return builder.String()
}

func mathTextOf(node *xmltree.Node, builder *strings.Builder) {
	if node.Name == "" {
		builder.WriteString(node.Text)
		return
	}
	if node.Name == "msqrt" {
		builder.WriteString("√")
	}
	for _, child := range node.Children {
		mathTextOf(child, builder)
	}
}
