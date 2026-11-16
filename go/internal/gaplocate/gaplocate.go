// Package gaplocate finds where in the author's source a structural accessibility item of an export
// lies (decision 184): a table or a caption is a node of the document, and only the words of the
// conversion's source map carry lines. A caption's first words are looked up in the map; a table is
// placed by the float that holds it (its caption) and its position among that float's tables.
package gaplocate

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/rec"
)

// Place is where an item lies: the source file and line of its float's caption (File "" when the words
// are not in the source map), what the float is and which table of it the item is.
type Place struct {
	File string
	Line int
	// Float is the type of the float holding the item (figure_float, table_float), "" without one.
	Float string
	// Caption is the float caption's text; Number the number it prints ("3.2"); Label a \label whose
	// target lies in the float.
	Caption, Number, Label string
	// Index is the table's position among the float's tables in reading order (1-based, outer tables
	// before the ones in their cells) and Of how many the float holds; 0 for a caption.
	Index, Of int
	// ImageFile and ImageLine are the first \includegraphics inside the table, when the conversion
	// recorded where the source includes it ("" and 0 otherwise).
	ImageFile string
	ImageLine int
}

// Locator is a document and its source map read for lookups; the map's word index is built on the
// first lookup that needs it.
type Locator struct {
	document, sourceMap rec.Map
	nodes               rec.Map
	parents             map[string]string
	owners              map[string]string // table id -> the node whose content holds it
	words               []any
	first               map[string][]int
}

// New reads the document's node tree; sourceMap may be nil.
func New(document, sourceMap rec.Map) *Locator {
	l := &Locator{document: document, sourceMap: sourceMap, nodes: rec.MapOf(document["nodes"]),
		parents: map[string]string{}, owners: map[string]string{}}
	for id, value := range l.nodes {
		for _, child := range rec.SliceOf(rec.MapOf(value)["children"]) {
			if name, ok := child.(string); ok {
				l.parents[name] = id
			}
		}
		l.collectTables(id, rec.SliceOf(rec.MapOf(value)["content"]))
	}
	return l
}

func (l *Locator) collectTables(owner string, content []any) {
	for _, value := range content {
		span := rec.MapOf(value)
		if span["type"] != "table_grid" {
			continue
		}
		if id := rec.StringOf(span["id"]); id != "" {
			l.owners[id] = owner
		}
		for _, cell := range rec.SliceOf(rec.MapOf(span["grid"])["cells"]) {
			l.collectTables(owner, rec.SliceOf(rec.MapOf(cell)["content"]))
		}
	}
}

// floatOf is the nearest float node at or above id, "" for none.
func (l *Locator) floatOf(id string) string {
	for seen := 0; id != "" && seen < 1000; seen++ {
		switch rec.MapOf(l.nodes[id])["type"] {
		case "figure_float", "table_float":
			return id
		}
		id = l.parents[id]
	}
	return ""
}

// Table places the table_grid span with that id.
func (l *Locator) Table(id string) Place {
	owner, ok := l.owners[id]
	if !ok {
		return Place{}
	}
	float := l.floatOf(owner)
	place := l.Float(float)
	if float != "" {
		ids := l.floatTables(float)
		for index, other := range ids {
			if other == id {
				place.Index = index + 1
			}
		}
		place.Of = len(ids)
	}
	place.ImageFile, place.ImageLine = l.tableImage(owner, id)
	if place.File == "" && place.ImageFile != "" {
		place.File, place.Line = place.ImageFile, place.ImageLine
	}
	return place
}

// Caption places a float_caption block by its own words.
func (l *Locator) Caption(id string) Place {
	float := l.floatOf(id)
	place := Place{Float: rec.StringOf(rec.MapOf(l.nodes[float])["type"]), Label: l.label(float)}
	texts := l.captionRuns(id)
	place.Caption = strings.Join(texts, " ")
	if len(texts) > 0 {
		place.Number = captionNumber(texts[0])
	}
	for _, text := range texts {
		if place.File, place.Line = l.Lookup(text); place.File != "" {
			break
		}
	}
	return place
}

// captionRuns are the text runs of a caption node: its own, or its first child's that has text (the
// export holds a caption's text in a paragraph inside it).
func (l *Locator) captionRuns(id string) []string {
	node := rec.MapOf(l.nodes[id])
	if texts := runs(node); len(texts) > 0 {
		return texts
	}
	for _, child := range rec.SliceOf(node["children"]) {
		if texts := runs(rec.MapOf(l.nodes[rec.StringOf(child)])); len(texts) > 0 {
			return texts
		}
	}
	return nil
}

// Float describes a float node: its caption's text, number and source line, and a label of it.
func (l *Locator) Float(float string) Place {
	if float == "" {
		return Place{}
	}
	node := rec.MapOf(l.nodes[float])
	place := Place{Float: rec.StringOf(node["type"])}
	for _, child := range rec.SliceOf(node["children"]) {
		name, _ := child.(string)
		if rec.MapOf(l.nodes[name])["type"] != "float_caption" {
			continue
		}
		caption := l.Caption(name)
		place.Caption, place.Number, place.File, place.Line = caption.Caption, caption.Number, caption.File, caption.Line
		break
	}
	place.Label = l.label(float)
	return place
}

// label is a \label whose target lies in the float, the first by name.
func (l *Locator) label(float string) string {
	best := ""
	if float == "" {
		return best
	}
	for label, value := range rec.MapOf(l.document["labels"]) {
		target, _ := rec.MapOf(value)["target"].(string)
		if target != "" && l.floatOf(target) == float && (best == "" || label < best) {
			best = label
		}
	}
	return best
}

// floatTables are the ids of the tables inside the float, in reading order.
func (l *Locator) floatTables(float string) []string {
	ids := []string{}
	var visit func(id string)
	visit = func(id string) {
		node := rec.MapOf(l.nodes[id])
		ids = append(ids, tableIDs(rec.SliceOf(node["content"]))...)
		for _, child := range rec.SliceOf(node["children"]) {
			if name, ok := child.(string); ok {
				visit(name)
			}
		}
	}
	visit(float)
	return ids
}

func tableIDs(content []any) []string {
	ids := []string{}
	for _, value := range content {
		span := rec.MapOf(value)
		if span["type"] != "table_grid" {
			continue
		}
		ids = append(ids, rec.StringOf(span["id"]))
		for _, cell := range rec.SliceOf(rec.MapOf(span["grid"])["cells"]) {
			ids = append(ids, tableIDs(rec.SliceOf(rec.MapOf(cell)["content"]))...)
		}
	}
	return ids
}

// Grid is the grid of the table_grid span with that id, nil when the document has none.
func (l *Locator) Grid(id string) rec.Map {
	owner, ok := l.owners[id]
	if !ok {
		return nil
	}
	return findGrid(rec.SliceOf(rec.MapOf(l.nodes[owner])["content"]), id)
}

func findGrid(content []any, id string) rec.Map {
	for _, value := range content {
		span := rec.MapOf(value)
		if span["type"] != "table_grid" {
			continue
		}
		if span["id"] == id {
			return rec.MapOf(span["grid"])
		}
		for _, cell := range rec.SliceOf(rec.MapOf(span["grid"])["cells"]) {
			if grid := findGrid(rec.SliceOf(rec.MapOf(cell)["content"]), id); grid != nil {
				return grid
			}
		}
	}
	return nil
}

// tableImage is the first source file and line a figure inside the table records.
func (l *Locator) tableImage(owner, id string) (string, int) {
	for _, value := range rec.SliceOf(rec.MapOf(l.nodes[owner])["content"]) {
		if span := rec.MapOf(value); span["id"] == id {
			return firstImage(span)
		}
	}
	return "", 0
}

// firstImage is the first span at or inside span that records a source file and line.
func firstImage(span rec.Map) (string, int) {
	if file := rec.StringOf(span["source_file"]); file != "" && rec.IntOf(span["source_line"]) > 0 {
		return file, int(rec.IntOf(span["source_line"]))
	}
	for _, cell := range rec.SliceOf(rec.MapOf(span["grid"])["cells"]) {
		for _, inner := range rec.SliceOf(rec.MapOf(cell)["content"]) {
			if file, line := firstImage(rec.MapOf(inner)); file != "" {
				return file, line
			}
		}
	}
	return "", 0
}

// leadWords is how many words of a text the lookup tries to find in a row; a text with fewer than
// minWords is not looked up (a single word occurs everywhere).
const (
	leadWords = 5
	minWords  = 2
)

// Lookup finds the line where the text's first words stand in the source map: the longest run of its
// first leadWords words (down to minWords) found in a row among the map's words. ("", 0) when there
// is none or no map.
func (l *Locator) Lookup(text string) (string, int) {
	tokens := strings.Fields(captionBody.ReplaceAllString(text, ""))
	if len(tokens) < minWords || l.sourceMap == nil {
		return "", 0
	}
	l.index()
	for count := min(leadWords, len(tokens)); count >= minWords; count-- {
		for _, start := range l.first[tokens[0]] {
			if l.matches(start, tokens[:count]) {
				return l.origin(start)
			}
		}
	}
	return "", 0
}

func (l *Locator) index() {
	if l.first != nil {
		return
	}
	l.words, l.first = rec.SliceOf(l.sourceMap["words"]), map[string][]int{}
	for index, value := range l.words {
		text := rec.StringOf(rec.MapOf(value)["text"])
		l.first[text] = append(l.first[text], index)
	}
}

func (l *Locator) matches(start int, tokens []string) bool {
	if start+len(tokens) > len(l.words) {
		return false
	}
	for offset, token := range tokens {
		if rec.StringOf(rec.MapOf(l.words[start+offset])["text"]) != token {
			return false
		}
	}
	return true
}

// origin is the file and line of the word's first source origin.
func (l *Locator) origin(index int) (string, int) {
	for _, value := range rec.SliceOf(rec.MapOf(l.words[index])["origins"]) {
		origin := rec.MapOf(value)
		if file := rec.StringOf(origin["file"]); file != "" && rec.IntOf(origin["line"]) > 0 {
			return file, int(rec.IntOf(origin["line"]))
		}
	}
	return "", 0
}

var (
	// captionBody removes a caption's generated label ("Figure 3.2:", "Table IV.") from the front.
	captionBody = regexp.MustCompile(`^\s*\p{L}+\.?[\s\x{a0}]+(?:[A-Z]?[0-9]+(?:\.[0-9]+)*|[A-Z]\.[0-9]+(?:\.[0-9]+)*|[IVXLC]+)[:.]?[\s\x{a0}]+`)
	captionPart = regexp.MustCompile(`^\s*\p{L}+\.?[\s\x{a0}]+([A-Z]?[0-9]+(?:\.[0-9]+)*|[A-Z]\.[0-9]+(?:\.[0-9]+)*|[IVXLC]+)(?:[:.\s\x{a0}]|$)`)
)

// captionNumber is the number a caption's text prints after its name, "" without one.
func captionNumber(text string) string {
	if match := captionPart.FindStringSubmatch(text); match != nil {
		return match[1]
	}
	return ""
}

// runs are the runs of a block's text and reference spans, split where a span of another kind (a
// formula, a figure) stands, each trimmed; runs without a letter or digit are left out.
func runs(node rec.Map) []string {
	found, current := []string{}, strings.Builder{}
	flush := func() {
		if text := strings.TrimSpace(current.String()); strings.ContainsFunc(text, unicode.IsLetter) || strings.ContainsFunc(text, unicode.IsDigit) {
			found = append(found, text)
		}
		current.Reset()
	}
	for _, value := range rec.SliceOf(node["content"]) {
		span := rec.MapOf(value)
		if span["type"] == "text" || span["type"] == "reference" {
			current.WriteString(rec.StringOf(span["text"]))
		} else {
			flush()
		}
	}
	flush()
	return found
}

// FloatOf is the nearest float node (figure_float or table_float) at or above id, "" for none.
func (l *Locator) FloatOf(id string) string { return l.floatOf(id) }

// FloatTables are the ids of the tables inside the float, in reading order: the order the table
// hints' position counts (decision 188, "fig:wspd#2") and a report item's table position share.
func (l *Locator) FloatTables(float string) []string { return l.floatTables(float) }

// FloatKeys are the names an author can give the float: the labels whose target lies in it, sorted,
// the number its caption prints ("16.9") and the caption's own words for it ("Figure 16.9"). A float
// without a label or a printed number has none.
func (l *Locator) FloatKeys(float string) []string {
	keys := []string{}
	if float == "" {
		return keys
	}
	for label, value := range rec.MapOf(l.document["labels"]) {
		target, _ := rec.MapOf(value)["target"].(string)
		if target != "" && l.floatOf(target) == float {
			keys = append(keys, label)
		}
	}
	sort.Strings(keys)
	for _, child := range rec.SliceOf(rec.MapOf(l.nodes[float])["children"]) {
		name, _ := child.(string)
		if rec.MapOf(l.nodes[name])["type"] != "float_caption" {
			continue
		}
		texts := l.captionRuns(name)
		if len(texts) == 0 {
			break
		}
		if match := captionPart.FindStringSubmatch(texts[0]); match != nil {
			keys = append(keys, match[1], strings.TrimRight(strings.TrimSpace(texts[0][:strings.Index(texts[0], match[1])+len(match[1])]), ".:"))
		}
		break
	}
	return keys
}
