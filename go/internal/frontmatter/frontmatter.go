// Package frontmatter is the Go port of Relatex::PaperFrontmatter and PaperTexMetadata: the
// paper's title, authors, abstract, page decorations and source links, read from literal
// source declarations and bound to what the reading observed. It is a model package (decision
// 102): the document's validation re-reads the abstract with the same TeX metadata reader, and
// the core supplies the collector's style catalog (Catalog) the page decorations need.
package frontmatter

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/extlinks"
	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

const abstractBlocks = 6

var (
	decorationSources = []any{"acmart-copyright", "latex-outputpage"}
	nodeID            = regexp.MustCompile(`\An-[0-9a-f]{64}\z`)
	email             = regexp.MustCompile(`\A[A-Za-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\z`)
	fields            = []string{"status", "title", "authors", "abstract", "language", "source", "decorations", "links"}
	sourcePath        = regexp.MustCompile(`\A[^/][^\x00]*\z`)
	slots             = map[rune]string{'': `(?:\[[^\[\]]*\])?`, '': `[\p{L}\p{N}.()\-–]{1,12}`,
		'': `(?:[a-z]+://[^ \t\n\v\f\r]*?)?`}
	decorationSpace = regexp.MustCompile(`[ \t\n\v\f\r\x{00ad}]`)
)

// languageAssertion is PaperFrontmatter::LANGUAGE.
func languageAssertion() rec.Map {
	return rec.Map{"status": "unavailable", "reason": "No supported literal language declaration collected"}
}

// Styles is the collector's text style catalog as the page decorations read it: whether a font
// has a declared style, and styled items joined into text runs (textstyle.Catalog, the core's).
type Styles interface {
	Has(font int64) bool
	Runs(items []any) []rec.Map
}

// Catalog reads the style catalog of the records (textstyle.NewCatalog); the projection asks
// for it only when a decoration glyph carries a style.
type Catalog func(records []rec.Map) Styles

// Project is PaperFrontmatter.project: text is the source with the paper's own included
// files read in place; source binds the main file. Nil when the source declares nothing.
func Project(catalog Catalog, records []rec.Map, source, text, path string, identity, document rec.Map) rec.Map {
	return ProjectWith(NewTexMetadata(text), catalog, records, source, path, identity, document)
}

// ProjectWith is Project with a prepared reader (one given the paper's own style files).
func ProjectWith(parser *TexMetadata, catalog Catalog, records []rec.Map, source, path string, identity, document rec.Map) rec.Map {
	metadata := parser.Project()
	if metadata == nil {
		return nil
	}
	if _, ok := metadata["abstract_source"]; ok {
		projectAbstract(metadata, parser, document)
	}
	sum := sha256.Sum256([]byte(source))
	sha := hex.EncodeToString(sum[:])
	if sha != rec.Fetch(identity, "input_sha256") {
		rec.Fail("Metadata source differs from document input")
	}
	kept, unowned := decorationRows(records)
	front := rec.Map{}
	for key, value := range metadata {
		front[key] = value
	}
	front["status"], front["language"] = "source_bound", languageAssertion()
	front["source"] = rec.Map{"path": path, "sha256": sha}
	front["decorations"] = decorations(records, identity, kept, catalog)
	front["links"] = projectLinks(parser, metadata, document, identity, !truthy(document["external_links"]))
	if len(unowned) > 0 {
		front["decorations_unowned"] = int64(len(unowned))
	}
	return front
}

// PageDecorations is the page headers, footers and publisher notices of the records on their own,
// for a paper whose front matter cannot be typed (they are not part of the title, authors and
// abstract). Rows are validated like the front matter's; it fails when they do not validate.
func PageDecorations(catalog Catalog, records []rec.Map, identity, document rec.Map) []any {
	kept, _ := decorationRows(records)
	rows := decorations(records, identity, kept, catalog)
	validateDecorations(rows, document)
	return rows
}

// Decorations is the document's page decorations: the front matter's when it is typed, else the
// ones kept on their own (page_decorations), else none. Every reader goes through it.
func Decorations(document rec.Map) []any {
	if front, ok := document["frontmatter"].(rec.Map); ok {
		rows, _ := front["decorations"].([]any)
		return rows
	}
	rows, _ := document["page_decorations"].([]any)
	return rows
}

func truthy(value any) bool { return value != nil && value != false }

// decorationRows: a header or footer glyph without a reading owner (a URL typeset in a
// running foot) is not front matter: it is set aside and reported, the rest is typed.
func decorationRows(records []rec.Map) (kept, unowned []rec.Map) {
	for _, row := range records {
		_, char := row["char"]
		_, text := row["text"]
		if row["code"] != "excluded_page_decoration" || (!char && !text) {
			continue
		}
		_, candidate := row["candidate"]
		_, position := row["position"]
		if candidate && position && text {
			kept = append(kept, row)
		} else {
			unowned = append(unowned, row)
		}
	}
	return kept, unowned
}

func nodes(document rec.Map) rec.Map { return rec.FetchMap(document, "nodes") }

func node(document rec.Map, id any) rec.Map {
	name, _ := id.(string)
	value, ok := nodes(document)[name]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + inspect(id), Lookup: true})
	}
	return rec.AsMap(value)
}

func inspect(value any) string {
	bytes, err := canonjson.Marshal(value)
	if err != nil {
		return "nil"
	}
	return string(bytes)
}

func projectAbstract(metadata rec.Map, parser *TexMetadata, document rec.Map) {
	sourceProse := abstractPattern(parser.AbstractProse(rec.FetchString(metadata, "abstract_source")))
	order := rec.FetchList(document, "block_order")
	prose := make([]string, len(order))
	for index, id := range order {
		prose[index] = blockProse(document, id)
	}
	// One block, or a few consecutive ones for an abstract of several paragraphs. Spacing
	// around removed math differs between source and typeset text; every other character
	// must match.
	matching := [][]any{}
	for _, indices := range abstractRuns(order, document) {
		run := make([]string, len(indices))
		for position, index := range indices {
			run[position] = prose[index]
		}
		// AMS classes run the heading into the abstract; article sets it as a separate line.
		if parser.AMS() && !amsHeading.MatchString(run[0]) {
			continue
		}
		// A run starts and ends with prose: a figure or formula block, or a heading line of its
		// own ("Abstract:"), beside the abstract adds nothing.
		if compactProse(normalizeAbstract(run[0])) == "" || compactProse(normalizeAbstract(run[len(run)-1])) == "" {
			continue
		}
		if sourceProse.MatchString(compactProse(normalizeAbstract(strings.Join(run, " ")))) {
			matching = append(matching, valuesAt(order, indices))
		}
	}
	if len(matching) != 1 {
		rec.Fail("Abstract source prose has no unique complete observed block")
	}
	metadata["abstract_blocks"] = matching[0]
	metadata["abstract"] = abstractSummary(document, matching[0])
}

var amsHeading = rx(`\A\s*Abstract[.:]`)

func valuesAt(order []any, indices []int) []any {
	result := make([]any, len(indices))
	for position, index := range indices {
		result[position] = order[index]
	}
	return result
}

// abstractRuns: consecutive blocks, or paragraphs a page interrupts with lines of its own
// (AASTeX's affiliations at the foot of the abstract's page), read as headings between
// its parts.
func abstractRuns(order []any, document rec.Map) [][]int {
	types := make([]any, len(order))
	for index, id := range order {
		types[index] = rec.Fetch(node(document, id), "type")
	}
	runs := [][]int{}
	for size := 1; size <= abstractBlocks; size++ {
		for first := range order {
			if first+size-1 < len(order) {
				run := make([]int, size)
				for offset := range run {
					run[offset] = first + offset
				}
				runs = append(runs, run)
			}
		}
	}
	for first := range order {
		if types[first] == "paragraph" {
			runs = append(runs, interruptedRuns(types, first)...)
		}
	}
	return runs
}

func interruptedRuns(types []any, first int) [][]int {
	runs := [][]int{}
	indices := []int{first}
	index := skipHeadings(types, first+1)
	for index < len(types) && types[index] == "paragraph" && index > indices[len(indices)-1]+1 && len(indices) < abstractBlocks {
		indices = append(indices, index)
		runs = append(runs, slices.Clone(indices))
		index = skipHeadings(types, index+1)
	}
	return runs
}

func skipHeadings(types []any, index int) int {
	for index < len(types) && types[index] == "heading" {
		index++
	}
	return index
}

// blockProse is a block's prose and the references it prints; citation calls are not the
// abstract's own prose.
func blockProse(document rec.Map, id any) string {
	spans, _ := node(document, id)["content"].([]any)
	if _, has := node(document, id)["content"]; !has {
		spans = []any{}
	}
	var builder strings.Builder
	for _, value := range spans {
		span := rec.AsMap(value)
		kind := span["type"]
		if _, cited := span["citation"]; (kind == "text" || kind == "reference") && !cited {
			builder.WriteString(rec.FetchString(span, "text"))
		}
	}
	return builder.String()
}

// compactProse: spacing ignored and double quotes folded (a source " prints as a curly quote).
func compactProse(text string) string {
	return strings.NewReplacer("“", `"`, "”", `"`).Replace(squeezeSpace(text, ""))
}

// abstractPattern: source prose, spacing ignored, where each citation call matches one
// printed bracketed label, a reference its printed number and a URL itself or nothing.
func abstractPattern(prose string) *regexp.Regexp {
	var builder strings.Builder
	builder.WriteString(`\A`)
	for _, character := range compactProse(prose) {
		if slot, ok := slots[character]; ok {
			builder.WriteString(slot)
		} else {
			builder.WriteString(regexp.QuoteMeta(string(character)))
		}
	}
	builder.WriteString(`\z`)
	return regexp.MustCompile(builder.String())
}

// normalizeAbstract drops a run-in heading: "Abstract.", "Abstract:" or IEEE's "Abstract—",
// or a bare "Abstract" (svjour) or "ABSTRACT" (IEEE) before the capital that opens the
// first sentence ("Abstract algebra" stays prose).
func normalizeAbstract(text string) string {
	text = withoutAbstractHeading(text)
	return nfc(strip(squeezeSpace(text, " ")))
}

func withoutAbstractHeading(text string) string {
	start := skipSpace(text, 0)
	rest := text[start:]
	if strings.HasPrefix(rest, "Abstract") {
		after := rest[len("Abstract"):]
		for _, mark := range []string{".", ":", "—", "–"} {
			if strings.HasPrefix(after, mark) {
				return after[skipSpace(after, len(mark)):]
			}
		}
	}
	for _, word := range []string{"Abstract", "ABSTRACT"} {
		if !strings.HasPrefix(rest, word) {
			continue
		}
		after := rest[skipSpace(rest, len(word)):]
		if after == "" || upperStart.MatchString(after) {
			return after
		}
	}
	return text
}

var upperStart = regexp.MustCompile(`\A\p{Lu}`)

func abstractSummary(document rec.Map, blocks []any) string {
	parts := make([]string, len(blocks))
	for index, id := range blocks {
		var builder strings.Builder
		for _, value := range rec.FetchList(node(document, id), "content") {
			span := rec.AsMap(value)
			switch span["type"] {
			case "text", "reference":
				builder.WriteString(rec.FetchString(span, "text"))
			case "math":
				builder.WriteString(nfkc(extlinks.MathText(rec.FetchString(node(document, rec.Fetch(span, "node")), "mathml"))))
			default:
				rec.Fail("Unsupported rich abstract content")
			}
		}
		parts[index] = builder.String()
	}
	return normalizeAbstract(strings.Join(parts, " "))
}

// identifier is 'n-' + SHA-256 of JSON.generate([input_sha256, scope, key]).
func identifier(identity rec.Map, scope string, key any) string {
	bytes, err := canonjson.Marshal([]any{rec.Fetch(identity, "input_sha256"), scope, key})
	if err != nil {
		rec.Fail("%v", err)
	}
	sum := sha256.Sum256(bytes)
	return "n-" + hex.EncodeToString(sum[:])
}

func decorations(records []rec.Map, identity rec.Map, rows []rec.Map, catalog Catalog) []any {
	candidates := map[any][]rec.Map{}
	for _, row := range records {
		if row["kind"] == "candidate" {
			id := rec.Fetch(row, "candidate")
			candidates[id] = append(candidates[id], row)
		}
	}
	styles := decorationStyles(records, rows, catalog)
	groups := omap.New[string, []rec.Map]()
	keys := map[string][]any{}
	for _, row := range rows {
		key := []any{row["page"], row["source"], row["candidate"]}
		name := inspect(key)
		observed, _ := groups.Get(name)
		groups.Set(name, append(observed, row))
		keys[name] = key
	}
	result := []any{}
	groups.Each(func(name string, observed []rec.Map) {
		result = append(result, decoration(identity, keys[name], observed, candidates, styles))
	})
	return result
}

func decoration(identity rec.Map, key []any, observed []rec.Map, candidates map[any][]rec.Map, styles Styles) rec.Map {
	rows, ok := candidates[key[2]]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + inspect(key[2]), Lookup: true})
	}
	definitions := []rec.Map{}
	for _, row := range rows {
		definition := rec.Map{}
		for _, field := range []string{"text", "glyphs"} {
			if value, has := row[field]; has {
				definition[field] = value
			}
		}
		if !slices.ContainsFunc(definitions, func(seen rec.Map) bool { return reflect.DeepEqual(seen, definition) }) {
			definitions = append(definitions, definition)
		}
	}
	if len(definitions) != 1 {
		rec.Fail("Ambiguous page decoration candidate")
	}
	candidate := definitions[0]
	positions := []any{}
	texts := []string{}
	for _, row := range observed {
		positions = append(positions, rec.Fetch(row, "position"))
		texts = append(texts, rec.FetchString(row, "text"))
	}
	glyphs := rec.FetchInt(candidate, "glyphs")
	expected := []any{}
	for position := int64(1); position <= glyphs; position++ {
		expected = append(expected, position)
	}
	if !rec.Equal(positions, expected) || !slices.Contains(decorationSources, key[1]) {
		rec.Fail("Incomplete page decoration glyph coverage")
	}
	if compactDecoration(strings.Join(texts, "")) != compactDecoration(rec.FetchString(candidate, "text")) {
		rec.Fail("Page decoration text differs from observed glyphs")
	}
	result := rec.Map{"id": identifier(identity, "page-decoration", key), "page": key[0], "source": key[1],
		"candidate": key[2], "glyphs": int64(len(positions)), "text": rec.Fetch(candidate, "text")}
	if styles != nil {
		result["runs"] = decorationRuns(rec.FetchString(candidate, "text"), observed, styles)
	}
	return result
}

func decorationStyles(records []rec.Map, rows []rec.Map, newCatalog Catalog) Styles {
	styled := false
	for _, row := range rows {
		_, has := row["style"]
		styled = styled || has
	}
	if !styled {
		return nil
	}
	catalog := newCatalog(records)
	declarations := map[any]any{}
	for _, row := range records {
		if row["code"] == "external_text_font" {
			declarations[rec.Fetch(row, "font")] = rec.Fetch(row, "seq")
		}
	}
	for _, row := range rows {
		if _, has := row["style"]; !has && isMathGlyph(row) {
			continue
		}
		font, isInt := row["style"].(int64)
		known := catalog.Has(font)
		if !isInt || !known {
			rec.Fail("Incomplete or premature decoration font binding")
		}
		declared, ok := declarations[row["style"]]
		if !ok {
			panic(rec.Failure{Message: "key not found: " + inspect(row["style"]), Lookup: true})
		}
		if rec.AsInt(declared) >= rec.FetchInt(row, "seq") {
			rec.Fail("Incomplete or premature decoration font binding")
		}
	}
	return catalog
}

func decorationRuns(text string, observed []rec.Map, styles Styles) []any {
	characters := []rune(text)
	cursor := 0
	items := []any{}
	runs := []rec.Map{}
	for _, glyph := range observed {
		label := []rune(rec.FetchString(glyph, "text"))
		index := runeIndex(characters, label, cursor)
		if index < 0 {
			rec.Fail("Decoration glyph cannot be bound to literal candidate text")
		}
		gap := string(characters[cursor:index])
		if strip(gap) != "" {
			rec.Fail("Decoration glyph binding skipped non-space text")
		}
		cursor = index + len(label)
		item := rec.Map{"text": glyph["text"], "space": gap}
		if style, has := glyph["style"]; has {
			item["style"] = style
		}
		if _, styled := item["style"]; !styled && isMathGlyph(glyph) {
			// A math glyph is a run of its own, marked, never merged into a text face.
			runs = append(runs, styles.Runs(items)...)
			items = []any{}
			runs = append(runs, rec.Map{"type": "text", "text": gap + rec.FetchString(glyph, "text"), "math": true})
			continue
		}
		items = append(items, item)
	}
	rest := string(characters[cursor:])
	if strip(rest) != "" {
		rec.Fail("Decoration glyph binding left trailing text")
	}
	runs = append(runs, styles.Runs(items)...)
	result := []any{}
	for _, run := range runs {
		delete(run, "type")
		result = append(result, run)
	}
	if rest != "" && len(runs) > 0 {
		last := runs[len(runs)-1]
		last["text"] = rec.FetchString(last, "text") + rest
	}
	return result
}

// compactDecoration is unicode_normalize(:nfkc).gsub(/[\s­]/, ”).
func compactDecoration(text string) string {
	return decorationSpace.ReplaceAllLiteralString(nfkc(text), "")
}

// isMathGlyph is a decoration glyph the collector marks as drawn by a math font (math: true).
func isMathGlyph(glyph rec.Map) bool {
	marked, _ := glyph["math"].(bool)
	return marked
}
