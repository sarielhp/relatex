package paperdoc

import (
	"regexp"
	"slices"
	"unicode"

	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/schema"
)

// bibliographyLabel is CitationEvidence::BIBLIOGRAPHY_LABEL: a number, or an alphabetic label
// (amsalpha's ADM25, BLT+25, Szé20; biblatex's G E97).
var bibliographyLabel = regexp.MustCompile(`\A(?:[1-9][0-9]*[a-z]?|\p{L}[\p{L}0-9+.-]*(?: [\p{L}0-9+.-]*[0-9][\p{L}0-9+.-]*)?|\p{L}[^\\{}$^_~#%]*[0-9]{4}[a-z]?)\z`)

// projectCitations is PaperCitations.project: the typed graph of observed citations.
func projectCitations(evidence, nodes rec.Map, order []any) rec.Map {
	if evidence == nil {
		return ValidateCitations(nil, nodes, order)
	}
	if rec.Fetch(evidence, "status") != "verified" {
		rec.Fail("Unverified citation graph input")
	}
	for _, id := range order {
		reattachClosingPunctuation(rec.FetchList(node(nodes, id), "content"))
	}
	entries := []any{}
	for _, value := range rec.FetchList(evidence, "entries") {
		entry := rec.AsMap(value)
		blocks := []any{}
		for _, id := range order {
			for _, span := range rec.FetchList(node(nodes, id), "content") {
				if rec.Equal(rec.AsMap(span)["bibliography_entry"], rec.Fetch(entry, "id")) {
					blocks = append(blocks, id)
					break
				}
			}
		}
		entries = append(entries, rec.Merge(rec.Pick(entry, "id", "key", "number"), rec.Map{"blocks": blocks}))
	}
	calls := []any{}
	for _, call := range rec.FetchList(evidence, "calls") {
		calls = append(calls, rec.Pick(rec.AsMap(call), "id", "keys", "targets"))
	}
	graph := rec.Map{"status": "projected", "qualified": false, "entries": entries, "calls": calls,
		"links": rec.Fetch(evidence, "links")}
	return ValidateCitations(graph, nodes, order)
}

func node(nodes rec.Map, id any) rec.Map {
	found, ok := nodes[rec.AsString(id)].(rec.Map)
	if !ok {
		panic(rec.Failure{Message: "key not found: " + toS(id), Lookup: true})
	}
	return found
}

// punctuation is /\A[[:punct:][:space:]]*\z/.
func punctuation(text string) bool {
	for _, character := range text {
		if !unicode.IsPunct(character) && !unicode.IsSpace(character) && !rubyPunct(character) {
			return false
		}
	}
	return true
}

// rubyPunct: Onigmo's [[:punct:]] also holds the ASCII symbols Unicode files as symbols.
func rubyPunct(character rune) bool {
	return character < 0x80 && unicode.IsSymbol(character)
}

// reattachClosingPunctuation: REVTeX closes an entry with its period when the next \bibitem
// runs; punctuation that ends an entry's block belongs to that entry.
func reattachClosingPunctuation(content []any) {
	if len(content) == 0 {
		return
	}
	owner := rec.AsMap(content[0])["bibliography_entry"]
	if !truthy(owner) {
		return
	}
	for index := len(content) - 1; index >= 0; index-- {
		span := rec.AsMap(content[index])
		entry := span["bibliography_entry"]
		if rec.Equal(entry, owner) {
			break
		}
		text, _ := span["text"].(string)
		if !truthy(entry) || rec.Fetch(span, "type") != "text" || !punctuation(text) {
			break
		}
		span["bibliography_entry"] = owner
	}
}

// citationGraph is PaperCitations' validation of a graph against the document.
type citationGraph struct {
	graph   rec.Map
	nodes   rec.Map
	order   []any
	spans   []rec.Map
	entries map[any]rec.Map
	calls   map[any]rec.Map
	links   map[any]rec.Map
	ids     map[string][]any
	cited   map[any]any
}

// ValidateCitations is PaperCitations.validate!(graph, nodes, order).
func ValidateCitations(graph, nodes rec.Map, order []any) (result rec.Map) {
	defer incomplete("Incomplete paper citation graph: ")
	content := []any{}
	for _, id := range order {
		content = append(content, rec.FetchList(node(nodes, id), "content")...)
	}
	// Citations in a modeled table sit in its cells.
	g := &citationGraph{graph: graph, nodes: nodes, order: order, spans: inline.Deep(content), ids: map[string][]any{}}
	if graph == nil {
		for _, span := range g.spans {
			for _, field := range citationFields {
				if _, ok := span[field]; ok {
					rec.Fail("Citation spans without a graph")
				}
			}
		}
		return nil
	}
	schema.Validate(graph, contract("paper-citations.schema.json"), "$")
	g.entries, g.calls, g.links = g.catalog("entries"), g.catalog("calls"), g.catalog("links")
	g.validateEntries()
	g.validateCalls()
	g.validateLinks()
	g.validateSpans()
	return graph
}

func (g *citationGraph) catalog(field string) map[any]rec.Map {
	rows := rec.FetchList(g.graph, field)
	index := map[any]rec.Map{}
	for _, value := range rows {
		row := rec.AsMap(value)
		id := rec.Fetch(row, "id")
		if _, seen := index[id]; !seen {
			g.ids[field] = append(g.ids[field], id)
		}
		index[id] = row
	}
	if len(index) != len(rows) {
		rec.Fail("Duplicate paper citation identity")
	}
	return index
}

func fetchID(index map[any]rec.Map, id any) rec.Map {
	row, ok := index[id]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + toS(id), Lookup: true})
	}
	return row
}

func (g *citationGraph) validateEntries() {
	// A key may name several items (a repeated or empty \bibitem key): LaTeX numbers its
	// citations with the last of them, so that item is the key's only target.
	g.cited = map[any]any{}
	for _, id := range g.ids["entries"] {
		g.cited[rec.Fetch(g.entries[id], "key")] = id
	}
	for _, id := range g.ids["entries"] {
		entry := g.entries[id]
		if !bibliographyLabel.MatchString(rec.FetchString(entry, "number")) {
			rec.Fail("Invalid paper bibliography number")
		}
		expected := []any{}
		for _, block := range g.order {
			if g.ownsEntry(block, id, entry) {
				expected = append(expected, block)
			}
		}
		if len(expected) == 0 || !rec.Equal(expected, rec.FetchList(entry, "blocks")) {
			rec.Fail("Bibliography target coverage differs")
		}
	}
}

// ownsEntry: entries merged under one number (REVTeX \cite{a,*b}) share their item's block.
func (g *citationGraph) ownsEntry(block, id any, entry rec.Map) bool {
	content := rec.FetchList(node(g.nodes, block), "content")
	found := false
	owners := []any{}
	for _, value := range content {
		owner := rec.AsMap(value)["bibliography_entry"]
		found = found || rec.Equal(owner, id)
		if !includes(owners, owner) {
			owners = append(owners, owner)
		}
	}
	if !found {
		return false
	}
	for _, owner := range owners {
		// Content with no entry of its own (a \line standing for a repeated author) is the item's.
		if !truthy(owner) {
			continue
		}
		if !rec.Equal(rec.Fetch(fetchID(g.entries, owner), "number"), rec.Fetch(entry, "number")) {
			rec.Fail("Mixed bibliography block ownership")
		}
	}
	return true
}

func (g *citationGraph) validateCalls() {
	for _, id := range g.ids["calls"] {
		call := g.calls[id]
		keys := rec.FetchList(call, "keys")
		actual := []any{}
		for _, target := range rec.FetchList(call, "targets") {
			key := rec.Fetch(fetchID(g.entries, target), "key")
			if !rec.Equal(g.cited[key], target) {
				rec.Fail("Citation targets an item its key no longer names")
			}
			actual = append(actual, key)
		}
		// An author may repeat a key within one \cite; each still resolves to its own entry.
		if len(keys) == 0 || !rec.Equal(actual, keys) {
			rec.Fail("Citation cluster lost keys or targets")
		}
	}
}

func (g *citationGraph) validateLinks() {
	for _, id := range g.ids["links"] {
		link := g.links[id]
		call := fetchID(g.calls, rec.Fetch(link, "citation"))
		entry := fetchID(g.entries, rec.Fetch(link, "entry"))
		if !includes(rec.FetchList(call, "keys"), rec.Fetch(link, "key")) || !rec.Equal(rec.Fetch(entry, "key"), rec.Fetch(link, "key")) ||
			!rec.Equal(rec.Fetch(entry, "number"), rec.Fetch(link, "number")) || !rec.Equal(g.cited[rec.Fetch(link, "key")], rec.Fetch(link, "entry")) {
			rec.Fail("Citation hyperlink target mismatch")
		}
	}
}

func (g *citationGraph) validateSpans() {
	for _, span := range g.spans {
		g.validateSpan(span)
	}
	for field, index := range map[string]map[any]rec.Map{"citation": g.calls, "citation_link": g.links, "bibliography_entry": g.entries} {
		actual := []any{}
		for _, span := range g.spans {
			if value := span[field]; truthy(value) && !includes(actual, value) {
				actual = append(actual, value)
			}
		}
		if !sameIDs(actual, index) {
			rec.Fail("Paper citation graph coverage differs")
		}
	}
}

func sameIDs(actual []any, index map[any]rec.Map) bool {
	if len(actual) != len(index) {
		return false
	}
	for _, id := range actual {
		if _, ok := index[id]; !ok {
			return false
		}
	}
	return true
}

func (g *citationGraph) validateSpan(span rec.Map) {
	fields := rec.Pick(span, citationFields...)
	if len(fields) == 0 {
		return
	}
	if !slices.Contains([]any{"text", "reference", "math"}, rec.Fetch(span, "type")) {
		rec.Fail("Citation identity on unsupported content")
	}
	for _, value := range fields {
		if !rec.Positive(value) {
			rec.Fail("Invalid paper citation span identity")
		}
	}
	if citation, ok := span["citation"]; ok {
		fetchID(g.calls, citation)
	}
	if entry, ok := span["bibliography_entry"]; ok {
		fetchID(g.entries, entry)
	}
	link, ok := span["citation_link"]
	if !ok {
		return
	}
	if span["type"] == "reference" {
		rec.Fail("Nested citation hyperlink")
	}
	if !rec.Equal(rec.Fetch(fetchID(g.links, link), "citation"), span["citation"]) {
		rec.Fail("Citation span belongs to a different cluster")
	}
}
