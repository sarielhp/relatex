package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/bundle"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/mathexport"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// projections is ProductInspection::PROJECTIONS.
var projections = []string{"all", "html", "markdown", "tex", "typst"}

type selection struct {
	node, asset, cell, table, word string
	projection                     string
}

// inspectSpec is ProductCLI#inspect_products: read-only inspection of a bundle (or of
// standalone MathML); no TeX, browser or renderer runs.
func inspectSpec() commandSpec {
	chosen := &selection{projection: "all"}
	var attempt, products string
	var raw, standalone bool
	selected, optionsGiven := map[string]bool{}, map[string]bool{}
	pick := func(key, spec, description string, target *string) clihelp.Option {
		return valueFlag(spec, description, func(value string) error { *target, selected[key] = value, true; return nil })
	}
	options := []clihelp.Option{
		pick("node", "--node <id>", "Show this node of the document", &chosen.node),
		pick("asset", "--asset <sha>", "Show this asset (by its SHA-256)", &chosen.asset),
		pick("cell", "--cell <id>", "Show this table cell", &chosen.cell),
		pick("table", "--table <id>", "With --cell: the table the cell is in", &chosen.table),
		pick("word", "--word <id>", "Show this word", &chosen.word),
		choiceFlag("--projection <format>", "The form to show a formula in: "+strings.Join(projections, ", "), projections,
			func(value string) { chosen.projection, selected["projection"] = value, true }),
		valueFlag("--attempt <dir>", "An attempt directory whose products are inspected with the bundle", func(value string) error {
			attempt, optionsGiven["attempt"] = value, true
			return nil
		}),
		valueFlag("--products <dir>", "The attempt's products directory (with --attempt)", func(value string) error {
			products, optionsGiven["products"] = value, true
			return nil
		}),
		switchFlag("--raw", "Include embedded asset/source data", func() { raw, optionsGiven["raw"] = true, true }),
		switchFlag("--mathml", "Inspect standalone retained MathML rather than a bundle", func() { standalone = true }),
	}
	return commandSpec{name: "inspect", description: "Inspect a saved document bundle, or a MathML file, read-only",
		usage: Name + " inspect BUNDLE [--node ID | --asset SHA | --cell ID [--table ID] | --word ID] [--attempt DIR]", options: options,
		args: clihelp.ExactArgs(1), human: inspectHuman, run: func(rest []string) ojson.Object {
			if standalone {
				return inspectMathML(rest[0], *chosen, selected, optionsGiven)
			}
			if optionsGiven["products"] && !optionsGiven["attempt"] {
				fail("--products requires --attempt")
			}
			return newInspection(rest[0], attempt, products, raw).report(*chosen, selected)
		}}
}

func inspectMathML(path string, chosen selection, selected, options map[string]bool) ojson.Object {
	for key := range selected {
		if key != "projection" {
			fail("--mathml accepts only --projection with a MathML file")
		}
	}
	if len(options) > 0 {
		fail("--mathml accepts only --projection with a MathML file")
	}
	info, err := os.Stat(path)
	if err != nil {
		panic(systemFailureAt(err, "rb_file_s_size"))
	}
	if info.Size() > 1_000_000 {
		rec.Fail("MathML size limit")
	}
	return ojson.Object{{"status", "inspected"}, {"qualified", false}, {"result", mathematics(string(readFile(path)), chosen.projection)}}
}

// inspection is ProductInspection.
type inspection struct {
	path     string
	payload  rec.Map
	document rec.Map
	nodes    rec.Map
	ids      []string
	raw      bool
	products *verificationProducts
}

func newInspection(path, attempt, products string, raw bool) *inspection {
	i := &inspection{path: realpath(path), raw: raw}
	i.payload = readBundle(i.path)
	bundle.Validate(i.payload)
	i.document = rec.FetchMap(i.payload, "document")
	i.nodes = rec.FetchMap(i.document, "nodes")
	for id := range i.nodes {
		i.ids = append(i.ids, id)
	}
	sort.Strings(i.ids)
	if attempt != "" {
		i.products = newVerificationProducts(attempt, products)
		if !rec.Equal(i.products.payload, i.payload) {
			rec.Fail("Inspection attempt belongs to another bundle")
		}
	}
	return i
}

func (i *inspection) report(chosen selection, selected map[string]bool) ojson.Object {
	count := 0
	for _, key := range []string{"node", "asset", "cell", "word"} {
		if selected[key] {
			count++
		}
	}
	if count > 1 {
		rec.Fail("Select only one node, asset, cell or word")
	}
	if selected["table"] && !selected["cell"] {
		rec.Fail("--table requires a cell selector")
	}
	if chosen.projection != "all" && (!selected["node"] || rec.Fetch(rec.Node(i.nodes, chosen.node), "type") != "math") {
		rec.Fail("--projection requires a math node")
	}
	var result any
	switch {
	case selected["node"]:
		result = i.inspectNode(chosen.node, chosen.projection)
	case selected["asset"]:
		result = i.inspectAsset(chosen.asset)
	case selected["cell"]:
		result = i.inspectCell(chosen.cell, chosen.table, selected["table"])
	case selected["word"]:
		result = i.inspectWord(chosen.word)
	default:
		result = i.summary()
	}
	return ojson.Object{{"status", "inspected"}, {"qualified", false}, {"bundle", i.path},
		{"bundle_sha256", command.FileDigest(i.path)}, {"result", result}}
}

func (i *inspection) summary() ojson.Object {
	types := ojson.Object{}
	for _, id := range i.ids {
		kind := rec.FetchString(rec.AsMap(i.nodes[id]), "type")
		count, _ := types.Get(kind).(int64)
		types = types.Set(kind, count+1)
	}
	items, _ := rec.FetchMap(i.document, "assets")["items"].(rec.Map)
	assetIDs := []string{}
	for id := range items {
		assetIDs = append(assetIDs, id)
	}
	sort.Strings(assetIDs)
	layout, _ := i.payload["layout"].(rec.Map)
	sourceMap, _ := i.payload["source_map"].(rec.Map)
	var pages, wordCount any
	if layout != nil {
		pages = int64(bundle.LayoutPageCount(layout))
	}
	if sourceMap != nil {
		wordCount = int64(len(rec.FetchList(sourceMap, "words")))
	}
	losses, ok := i.payload["losses"]
	if !ok {
		losses = []any{}
	}
	return ojson.Object{
		{"schemas", ojson.Object{{"document", rec.Fetch(i.document, "schema_version")}, {"layout", layout["schema_version"]},
			{"source_map", sourceMap["schema_version"]}}},
		{"identity", rec.Fetch(i.document, "identity")}, {"node_types", types}, {"nodes", words(i.ids...)},
		{"assets", words(assetIDs...)}, {"pages", pages}, {"words", wordCount}, {"losses", losses}}
}

func (i *inspection) inspectNode(id, projection string) ojson.Object {
	node, ok := i.nodes[id].(rec.Map)
	if !ok {
		rec.Fail("Unknown node: %s", id)
	}
	result := ojson.Object{{"node", node}, {"owners", i.nodeOwners(id)}}
	if node["type"] == "math" {
		result = append(result, ojson.Pair{"mathematics", mathematics(rec.FetchString(node, "mathml"), projection)})
		if i.products != nil {
			result = append(result, ojson.Pair{"native_evidence", i.products.mathEvidence(node)})
		}
	}
	return result
}

func (i *inspection) inspectAsset(id string) ojson.Object {
	items, _ := rec.FetchMap(i.document, "assets")["items"].(rec.Map)
	asset, ok := items[id].(rec.Map)
	if !ok {
		rec.Fail("Unknown asset: %s", id)
	}
	data := mediamodel.NewMediaAsset(asset).Data
	if !i.raw {
		data = withoutDataURIs(data)
	}
	uses := []any{}
	for _, use := range i.inlineUses() {
		if span, _ := use.Get("span").(rec.Map); span["asset"] == id {
			uses = append(uses, use)
		}
	}
	return ojson.Object{{"asset", data}, {"uses", uses}}
}

// withoutDataURIs is data.merge('representations' => ... except('data_uri')).
func withoutDataURIs(data rec.Map) rec.Map {
	copied := rec.Map{}
	for key, value := range data {
		copied[key] = value
	}
	representations := rec.Map{}
	for name, item := range rec.FetchMap(data, "representations") {
		entry := rec.Map{}
		for key, value := range rec.AsMap(item) {
			if key != "data_uri" {
				entry[key] = value
			}
		}
		representations[name] = entry
	}
	copied["representations"] = representations
	return copied
}

func (i *inspection) inspectCell(id, table string, byTable bool) ojson.Object {
	matches := []ojson.Object{}
	for _, use := range i.inlineUses() {
		span, _ := use.Get("span").(rec.Map)
		if span["type"] != "table_grid" || (byTable && span["id"] != table) {
			continue
		}
		model := tablemodel.NewWith(rec.FetchMap(span, "grid"), i.nodes)
		data, occupancy := model.Data, model.Occupancy
		for _, value := range rec.FetchList(data, "cells") {
			cell := rec.AsMap(value)
			if rec.Fetch(cell, "id") != id {
				continue
			}
			fragments, ok := data["native_fragments"]
			if !ok {
				fragments = []any{}
			}
			matches = append(matches, ojson.Object{{"owner", use.Get("owner")}, {"table", rec.Fetch(span, "id")}, {"cell", cell},
				{"occupancy", grid(occupancy)}, {"native_fragments", fragments}})
			break
		}
	}
	if len(matches) != 1 {
		rec.Fail("Cell is missing or ambiguous; use --table TABLE_ID with an exact cell ID")
	}
	return matches[0]
}

func grid(rows [][]any) []any {
	result := make([]any, len(rows))
	for index, row := range rows {
		result[index] = row
	}
	return result
}

// glyphSourceSchema is GlyphSourceMap::SCHEMA.
const glyphSourceSchema = "source-map-2.0"

func (i *inspection) inspectWord(id string) ojson.Object {
	sourceMap, ok := i.payload["source_map"].(rec.Map)
	if !ok {
		rec.Fail("Bundle has no source map")
	}
	candidates := append(append([]any{}, rec.FetchList(sourceMap, "words")...), listOr(sourceMap, "logical_words")...)
	var word rec.Map
	for _, value := range candidates {
		if candidate := rec.AsMap(value); rec.Fetch(candidate, "id") == id {
			word = candidate
			break
		}
	}
	if word == nil {
		rec.Fail("Unknown word: %s", id)
	}
	sources := ojson.Object{}
	catalog, _ := sourceMap["sources"].(rec.Map)
	for _, value := range rec.FetchList(word, "origins") {
		file := rec.FetchString(rec.AsMap(value), "file")
		data, _ := catalog[file].(rec.Map)
		var entry any
		if data != nil {
			entry = data
			if !i.raw {
				entry = without(data, "text")
			}
		}
		sources = sources.Set(file, entry)
	}
	members := listOr(word, "word_ids")
	if _, has := word["word_ids"]; !has {
		members = []any{id}
	}
	fragments := []any{}
	for _, value := range rec.FetchList(sourceMap, "words") {
		if slices.Contains(members, rec.Fetch(rec.AsMap(value), "id")) {
			fragments = append(fragments, value)
		}
	}
	return ojson.Object{{"word", word}, {"fragments", fragments}, {"glyphs", i.wordGlyphs(sourceMap, fragments)}, {"sources", sources},
		{"coordinates", rec.Fetch(sourceMap, "coordinates")}, {"pdf_sha256", rec.Fetch(sourceMap, "pdf_sha256")}}
}

// wordGlyphs: source map 2 words come from TeX's glyph map, not from the PDF layout's glyphs.
func (i *inspection) wordGlyphs(sourceMap rec.Map, fragments []any) []any {
	glyphs := []any{}
	if sourceMap["schema_version"] == glyphSourceSchema {
		return glyphs
	}
	ids := []any{}
	for _, fragment := range fragments {
		for _, row := range rec.FetchList(rec.AsMap(fragment), "characters_to_glyphs") {
			ids = append(ids, rec.Fetch(rec.AsMap(row), "glyph"))
		}
	}
	// A layout stored in part files is searched part by part (decision 087).
	bundle.EachLayoutPages(rec.FetchMap(i.payload, "layout"), func(_ int, pages []any) {
		for _, page := range pages {
			for _, glyph := range rec.FetchList(rec.AsMap(page), "glyphs") {
				if slices.Contains(ids, rec.Fetch(rec.AsMap(glyph), "id")) {
					glyphs = append(glyphs, glyph)
				}
			}
		}
	})
	return glyphs
}

func listOr(m rec.Map, key string) []any {
	list, _ := m[key].([]any)
	return list
}

func without(m rec.Map, key string) rec.Map {
	result := rec.Map{}
	for name, value := range m {
		if name != key {
			result[name] = value
		}
	}
	return result
}

// inlineUses is every inline span with its owner, the cells of a table grid too.
func (i *inspection) inlineUses() []ojson.Object {
	uses := []ojson.Object{}
	for _, id := range i.ids {
		owner := rec.AsMap(i.nodes[id])
		for _, value := range listOr(owner, "content") {
			span := rec.AsMap(value)
			uses = append(uses, ojson.Object{{"owner", rec.Fetch(owner, "id")}, {"span", span}})
			if span["type"] != "table_grid" {
				continue
			}
			data := tablemodel.NewWith(rec.FetchMap(span, "grid"), i.nodes).Data
			for _, cellValue := range rec.FetchList(data, "cells") {
				cell := rec.AsMap(cellValue)
				for _, inline := range rec.FetchList(cell, "content") {
					uses = append(uses, ojson.Object{{"owner", rec.Fetch(owner, "id")}, {"table", rec.Fetch(span, "id")},
						{"cell", rec.Fetch(cell, "id")}, {"span", inline}})
				}
			}
		}
	}
	return uses
}

func (i *inspection) nodeOwners(id string) []any {
	owners := []any{}
	for _, nodeID := range i.ids {
		node := rec.AsMap(i.nodes[nodeID])
		if slices.Contains(rec.FetchList(node, "children"), any(id)) {
			owners = append(owners, ojson.Object{{"owner", rec.Fetch(node, "id")}, {"via", "children"}})
		}
	}
	for _, use := range i.inlineUses() {
		span, _ := use.Get("span").(rec.Map)
		if span["node"] != id {
			continue
		}
		owner := ojson.Object{}
		for _, pair := range use {
			if pair.Key() != "span" {
				owner = append(owner, pair)
			}
		}
		owners = append(owners, append(owner, ojson.Pair{"via", "inline_math"}))
	}
	return owners
}

// mathematics is ProductInspection.mathematics: the formula's presentation and projections.
func mathematics(xml, projection string) (result ojson.Object) {
	sum := sha256.Sum256([]byte(xml))
	defer func() {
		if message, ok := failureMessage(recover()); ok {
			result = ojson.Object{{"mathml", xml}, {"source_mathml_sha256", hex.EncodeToString(sum[:])}, {"error", message}}
		}
	}()
	presentation := mathexport.NewPresentation(xml)
	targets := []string{"html", "markdown", "typst"}
	if projection != "all" {
		targets = []string{projection}
	}
	projected := ojson.Object{}
	for _, target := range targets {
		projected = append(projected, ojson.Pair{target, projectMath(xml, target)})
	}
	return ojson.Object{{"mathml", xml}, {"source_mathml_sha256", presentation.SHA256},
		{"presentation", presentationData(presentation)}, {"projections", projected}}
}

// failureMessage is a recovered Relatex::Failure's message; anything else panics on.
func failureMessage(recovered any) (string, bool) {
	if recovered == nil {
		return "", false
	}
	if failure, ok := recovered.(rec.Failure); ok && !failure.Lookup {
		return failure.Message, true
	}
	panic(recovered)
}

func projectMath(xml, target string) (result ojson.Object) {
	if target == "html" {
		return nativeMathML(xml, "")
	}
	defer unprojected(xml, target, &result)
	var value string
	switch target {
	case "typst":
		value = mathexport.NewTypst().Convert(xml)
	case "markdown":
		value = mathexport.NewTeX("pandoc").Convert(xml)
	default:
		value = mathexport.NewTeX("tex").Convert(xml)
	}
	return ojson.Object{{"kind", "native_projection"}, {"value", value}, {"rendering", "unverified"}}
}

// unprojected is project_math's rescue: Markdown falls back to native MathML, the others report
// the projection unsupported.
func unprojected(xml, target string, result *ojson.Object) {
	message, ok := failureMessage(recover())
	switch {
	case !ok:
	case target == "markdown":
		*result = nativeMathML(xml, message)
	default:
		*result = ojson.Object{{"kind", "unsupported"}, {"error", message}}
	}
}

func nativeMathML(xml, reason string) (result ojson.Object) {
	defer func() {
		if message, ok := failureMessage(recover()); ok {
			result = ojson.Object{{"kind", "unsupported"}, {"error", message}}
		}
	}()
	result = ojson.Object{{"kind", "native_mathml"}, {"value", mathexport.Project(mathexport.ParsePaper(xml))}, {"rendering", "unverified"}}
	if reason != "" {
		result = append(result, ojson.Pair{"reason", reason})
	}
	return result
}

// inheritedStyle is MathPresentation::INHERITED.
var inheritedStyle = []string{"mathvariant", "displaystyle", "scriptlevel", "mathsize", "mathcolor", "dir"}

// presentationData is MathPresentation#data, its hashes in Ruby's key order.
func presentationData(p *mathexport.Presentation) ojson.Object {
	return ojson.Object{{"schema_version", "math-presentation-1"}, {"source_mathml_sha256", p.SHA256},
		{"root", describe(p, p.Root, []string{"displaystyle", "scriptlevel"})}}
}

// describe is MathPresentation#describe. order is the effective style's keys in the order
// Ruby's merges put them: the root's two, then each ancestor's inherited attributes.
func describe(p *mathexport.Presentation, node *mathexport.Node, order []string) ojson.Object {
	order = slices.Clone(order)
	for _, name := range inheritedStyle {
		if _, ok := node.RawAttribute(name); ok && !slices.Contains(order, name) {
			order = append(order, name)
		}
	}
	attributes := ojson.Object{}
	for _, attribute := range node.Attrs {
		attributes = attributes.Set(attribute.Name, attribute.Raw)
	}
	style := p.Style(node)
	effective := ojson.Object{}
	for _, name := range order {
		if value, ok := style[name]; ok {
			effective = append(effective, ojson.Pair{name, value})
		}
	}
	text := ""
	for _, part := range node.Texts() {
		text += part
	}
	children := []any{}
	for _, child := range node.Elements() {
		children = append(children, describe(p, child, order))
	}
	return ojson.Object{{"element", node.Name}, {"path", p.Path(node)}, {"attributes", attributes}, {"effective_style", effective},
		{"text", text}, {"children", children}}
}
