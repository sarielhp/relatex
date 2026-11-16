package typstsource

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/xmltree"
)

// Root is the repository root: the preamble (prefix.typ) is lib/relatex/typst_source_preamble.typ.
var Root string

// The layout of the export (decision 063, amendment 2026-11-15): main.typ, references.bib and chapters/ beside
// figs/ and prefix.typ; support/ exists only for a styled bibliography.
const (
	// SupportDirectory holds a styled bibliography: bibliography.typ, bibstyle.typ and vendor/.
	SupportDirectory = "support"
	figuresDirectory = "figs"
	preambleFile     = "prefix.typ"
	// header opens main.typ: how it was made and compiled, then relatex's helpers and rules.
	header = "// Converted from LaTeX by relatex. Compile with:\n//\n//     typst compile --ignore-system-fonts main.typ\n\n" +
		"#import \"" + preambleFile + "\": *\n#show: relatex-setup"
)

var (
	abstractHeading = regexp.MustCompile(`(?i)\A(?:abstract|ABSTRACT)[.:]?\z`)
	wordPattern     = regexp.MustCompile(`[\p{L}\p{N}]+`)
	authorWord      = regexp.MustCompile(`\A(?:[0-9]+|\p{L})\z`)
	tagPattern      = regexp.MustCompile(`\A[ \t\n\v\f\r]*\((.+)\)[ \t\n\v\f\r]*\z`)
)

var structures = []string{"document", "section", "float_caption", "list_item"}

// placement of a label in a pass: on a numbered element, or an anchor only.
const (
	element = "element"
	anchor  = "anchor"
)

type headingKey struct {
	level   int
	segment string
}

type headingFormat struct {
	prefix string
	parts  []formatPart
}

type formatPart struct {
	kind      numeralKind
	separator *string
}

type floatStyle struct {
	kind, supplement, separator string
}

// exporter is one TypstSourceExport.
type exporter struct {
	document  rec.Map
	nodes     rec.Map
	directory string
	math      *mathWriter
	preamble  string

	labels          map[string]string
	labelsByTarget  map[string][]string
	citationCalls   map[string]rec.Map
	citationLabels  map[string]string
	citedEntries    map[string]any
	bibliography    map[string][]rec.Map
	parents         map[string]string
	tags            map[string][]string
	tagLabels       map[string][]string // the labels of the tag paragraphs each display consumed
	consumed        map[string]bool
	abstractBlocks  []string
	frontBlocks     []string
	titleBlocks     []string
	authors         []rec.Map
	authorBlocks    []string
	externalHrefs   map[string]any
	sectioning      *bool
	lastHeadings    map[string]string
	counterRegister map[string]any
	registersReady  bool

	labelNumbers  map[string]string
	labelsMerged  map[string]bool
	placed        map[string]string
	placedBefore  map[string]string
	writtenImages map[string]string
	names         *figures.Names
	output        string
	report        rec.Map

	losses           []any
	frozen           int
	unexpressed      int
	collapsed        int
	footnotes        *series
	footnoteLabels   map[string]string
	footnotesWritten map[string]bool
	equations        *series
	series           map[string]*series
	seriesOrder      []string
	currentSection   *string
	segment          *string
	scopeDepth       int
	noteDepth        int // footnotes being written
	hoisted          []string
	claimedLabels    map[string]bool
	theoremKinds     []*helper
	kindNumbering    map[string]string
	floatStyles      []floatStyle
	sectioned        []string
	headingFormats   map[headingKey]headingFormat
	headingPlan      map[string]headingDirective
	headingInfo      map[string]*headingInfo
	upright          bool
	// longDescriptions are the long descriptions of the images written since the node being
	// rendered began, placed after it (after the whole figure inside a float, floatDepth > 0;
	// decision 110).
	longDescriptions []string
	floatDepth       int
	imagePrefix      string
	activeRules      map[string]string
	// styled is the bibliography Pergamon prints (decision 172), nil for literal entries; bibReport records
	// the choice (nil for an export without the conversion's bibliography inputs).
	styledBib *styledBibliography
	bibReport rec.Map
}

// Write is TypstSourceExport.new(document, directory:).write! and its losses: main.typ, its
// images in figs/ and relatex's Typst in prefix.typ.
func Write(document rec.Map, directory string) (report rec.Map, losses []any) {
	return WriteWith(document, directory, nil)
}

// WriteWith is Write with the conversion's bibliography inputs: a styled bibliography when they make
// one (decision 172); the report's "bibliography" records the choice.
func WriteWith(document rec.Map, directory string, bibliography *Bibliography) (report rec.Map, losses []any) {
	paperdoc.Validate(document)
	preamble, err := os.ReadFile(filepath.Join(Root, "lib", "relatex", "typst_source_preamble.typ"))
	if err != nil {
		rec.Fail("%v", err)
	}
	e := &exporter{document: document, nodes: rec.FetchMap(document, "nodes"), directory: directory,
		math: &mathWriter{}, preamble: string(preamble)}
	e.index()
	if mkdirErr := os.MkdirAll(directory, 0o755); mkdirErr != nil {
		rec.Fail("%v", mkdirErr)
	}
	e.styleBibliography(bibliography)
	if e.hasChapters() {
		main, files := e.renderBook()
		writeFile(filepath.Join(directory, "main.typ"), main)
		for _, file := range files {
			writeFile(filepath.Join(directory, file.path), file.content)
		}
	} else {
		main := e.render()
		writeFile(filepath.Join(directory, "main.typ"), main)
	}
	writeFile(filepath.Join(directory, preambleFile), e.preamble)
	if e.bibReport != nil {
		e.report["bibliography"] = e.bibReport
	}
	return e.report, e.losses
}

func writeFile(path, text string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		rec.Fail("%v", err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		rec.Fail("%v", err)
	}
}

// render is TypstSourceExport#render: main.typ as a string; images are written into the
// directory as they are met. The first pass learns what references need (where each label
// landed, the numbers they print).
func (e *exporter) render() string {
	if err := os.MkdirAll(e.directory, 0o755); err != nil {
		rec.Fail("%v", err)
	}
	e.labelNumbers = map[string]string{}
	e.labelsMerged = map[string]bool{}
	e.placed = map[string]string{}
	e.writtenImages = map[string]string{}
	for range 2 {
		e.placedBefore = e.placed
		e.placed = map[string]string{}
		e.output = e.renderDocument()
	}
	e.report = e.buildReport()
	return e.output
}

func (e *exporter) buildReport() rec.Map {
	return rec.Map{"labels": int64(len(e.labels)), "frozen_references": int64(e.frozen),
		"unexpressed_citations": int64(e.unexpressed), "losses": int64(len(e.losses)),
		"collapsed_displays": int64(e.collapsed), "anchor_labels": e.anchorLabels()}
}

// anchorLabels: the labels placed as link targets only (their LaTeX number is not the one their
// element prints), sorted. The Typst acceptance probes every other placed label's number
// (decision 154); these print LaTeX's number through their references instead.
func (e *exporter) anchorLabels() []any {
	names := []string{}
	for name, placement := range e.placed {
		if placement == anchor {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	result := make([]any, len(names))
	for index, name := range names {
		result[index] = name
	}
	return result
}

func (e *exporter) node(id any) rec.Map { return rec.Node(e.nodes, id) }

func (e *exporter) sortedIDs() []string {
	ids := make([]string, 0, len(e.nodes))
	for id := range e.nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func children(node rec.Map) []string {
	ids := []string{}
	for _, child := range rec.FetchList(node, "children") {
		ids = append(ids, rec.AsString(child))
	}
	return ids
}

func content(node rec.Map) []rec.Map { return maps(node["content"]) }

// ---- indexes built once ----

func (e *exporter) index() {
	e.labels = map[string]string{}
	e.labelsByTarget = map[string][]string{}
	used := map[string]bool{}
	labels, _ := e.document["labels"].(rec.Map)
	names := []string{}
	for name := range labels {
		// Package-internal labels (thm-restate's thmt@@name) are no author's.
		if !strings.Contains(name, "@@") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		e.labels[name] = unique(labelName(name), used)
		target := toS(rec.Fetch(rec.AsMap(labels[name]), "target"))
		e.labelsByTarget[target] = append(e.labelsByTarget[target], name)
	}
	e.indexCitations(used)
	e.parents = map[string]string{}
	for _, id := range e.sortedIDs() {
		for _, child := range children(e.node(id)) {
			e.parents[child] = id
		}
	}
	e.indexTags()
	e.indexFrontmatter()
}

// indexCitations: a merged REVTeX item holds several keys: the first labels the item, the
// others are anchors.
func (e *exporter) indexCitations(used map[string]bool) {
	citations, _ := e.document["citations"].(rec.Map)
	e.citationCalls = map[string]rec.Map{}
	for _, call := range maps(citations["calls"]) {
		e.citationCalls[toS(rec.Fetch(call, "id"))] = call
	}
	e.citationLabels = map[string]string{}
	e.bibliography = map[string][]rec.Map{}
	entries := maps(citations["entries"])
	// A key naming several items (a repeated or empty \bibitem key) labels only the last, the
	// item LaTeX numbers its citations with; the others print without a label.
	e.citedEntries = map[string]any{}
	for _, entry := range entries {
		e.citationLabels[toS(rec.Fetch(entry, "key"))] = toS(rec.Fetch(entry, "number"))
		e.citedEntries[toS(rec.Fetch(entry, "key"))] = rec.Fetch(entry, "id")
	}
	for _, entry := range entries {
		blocks := rec.FetchList(entry, "blocks")
		if len(blocks) == 0 || blocks[0] == nil {
			continue
		}
		block := toS(blocks[0])
		e.bibliography[block] = append(e.bibliography[block], entry)
		if e.cited(entry) {
			key := toS(rec.Fetch(entry, "key"))
			e.labels["cite:"+key] = unique(labelName(key), used)
		}
	}
}

// cited: the entry is the item its key names (the last item of a repeated key).
func (e *exporter) cited(entry rec.Map) bool {
	return rec.Equal(e.citedEntries[toS(rec.Fetch(entry, "key"))], rec.Fetch(entry, "id"))
}

func unique(name string, used map[string]bool) string {
	candidate := name
	for suffix := 2; used[candidate]; suffix++ {
		candidate = fmt.Sprintf("%s-%d", name, suffix)
	}
	used[candidate] = true
	return candidate
}

// indexTags: a display's printed tag may be a paragraph of its own beside it: same equation
// entity.
func (e *exporter) indexTags() {
	e.tags = map[string][]string{}
	e.tagLabels = map[string][]string{}
	e.consumed = map[string]bool{}
	for _, id := range e.sortedIDs() {
		groups := map[string][]rec.Map{}
		order := []string{}
		entities := map[string]any{}
		for _, child := range children(e.node(id)) {
			node := e.node(child)
			entity := dig(node, "semantics", "entity")
			key := toS(entity) + "\x00" + fmt.Sprintf("%T", entity)
			if _, seen := groups[key]; !seen {
				order = append(order, key)
				entities[key] = entity
			}
			groups[key] = append(groups[key], node)
		}
		for _, key := range order {
			e.tagGroup(entities[key], groups[key])
		}
	}
}

func (e *exporter) tagGroup(entity any, group []rec.Map) {
	if !truthy(entity) {
		return
	}
	index := slices.IndexFunc(group, func(child rec.Map) bool { return child["type"] == "display_math" })
	if index < 0 {
		return
	}
	display := toS(group[index]["id"])
	for _, child := range group {
		if child["type"] != "paragraph" {
			continue
		}
		if tag, ok := e.tagText(child); ok {
			e.tags[display] = append(e.tags[display], tag)
			e.consumed[toS(child["id"])] = true
			e.tagLabels[display] = append(e.tagLabels[display], e.labelsByTarget[toS(child["id"])]...)
		}
	}
}

func (e *exporter) tagText(node rec.Map) (string, bool) {
	spans := content(node)
	if len(spans) == 0 || slices.ContainsFunc(spans, func(span rec.Map) bool { return span["type"] != "text" && span["type"] != "math" }) {
		return "", false
	}
	var builder strings.Builder
	for _, span := range spans {
		if span["type"] == "text" {
			builder.WriteString(rec.FetchString(span, "text"))
		} else {
			builder.WriteString(e.mathText(span))
		}
	}
	text := rec.Strip(builder.String())
	return text, tagPattern.MatchString(text)
}

// mathText: a formula as the characters it prints: \tag{$\ast$} prints (∗).
func (e *exporter) mathText(span rec.Map) string {
	mathml := toS(e.node(rec.Fetch(span, "node"))["mathml"])
	if mathml == "" {
		return ""
	}
	root, err := xmltree.Parse(mathml)
	if err != nil {
		rec.Fail("undefined method 'xpath' for nil")
	}
	var builder strings.Builder
	spacedText(root, &builder)
	return rec.Strip(norm.NFKC.String(builder.String()))
}

// spacedText is the node's text with each mspace read as a space.
func spacedText(node *xmltree.Node, builder *strings.Builder) {
	if node.Name == "" {
		builder.WriteString(node.Text)
		return
	}
	if node.Name == "mspace" {
		builder.WriteByte(' ')
		return
	}
	for _, child := range node.Children {
		spacedText(child, builder)
	}
}

// ---- one pass ----

func (e *exporter) renderDocument() string {
	e.resetState()
	parts := []string{}
	for _, id := range children(e.node(rec.Fetch(e.document, "root"))) {
		if output := e.renderNode(id); output != "" {
			parts = append(parts, output)
		}
	}
	body := strings.Join(parts, "\n\n")
	settings := e.settings()
	front := e.frontMatter()
	sections := []string{}
	for _, part := range []string{header, settings, e.bibliographySetup(), front, body} {
		if part != "" {
			sections = append(sections, part)
		}
	}
	return strings.Join(sections, "\n\n") + "\n"
}

func (e *exporter) resetState() {
	e.losses = []any{}
	e.frozen, e.unexpressed, e.collapsed = 0, 0, 0
	e.footnotes = newSeries("%s")
	e.footnoteLabels = map[string]string{}
	e.footnotesWritten = map[string]bool{}
	e.equations = newSeries("(%s)")
	e.series = map[string]*series{}
	e.seriesOrder = nil
	e.currentSection, e.segment = nil, nil
	e.scopeDepth = 0
	e.hoisted = nil
	e.claimedLabels = map[string]bool{}
	e.theoremKinds = nil
	e.kindNumbering = map[string]string{}
	e.floatStyles = nil
	e.sectioned = nil
	e.headingFormats = map[headingKey]headingFormat{}
	e.headingPlan = e.planHeadings()
}

// seriesFor is @series[kind]: a series made on first use, starting in the current section.
func (e *exporter) seriesFor(kind string) *series {
	if found, ok := e.series[kind]; ok {
		return found
	}
	created := newSeries("%s")
	created.setSection(e.currentSection)
	e.series[kind] = created
	e.seriesOrder = append(e.seriesOrder, kind)
	return created
}

// settings: global settings learned in the first pass: float supplements, references, resets.
func (e *exporter) settings() string {
	lines := []string{"#show <ltd-nonumber>: set math.equation(numbering: none)"}
	// The document's declared language (decision 080); Typst's own default is English.
	if declared, ok := e.document["language"].(rec.Map); ok && declared["source"] != "default" {
		lines = append([]string{"#set text(" + typstLanguage(toS(declared["tag"])) + ")"}, lines...)
	}
	for _, style := range e.floatStyles {
		lines = append(lines, fmt.Sprintf("#show figure.where(kind: %s): set figure(supplement: [%s])", style.kind, escape(style.supplement)),
			fmt.Sprintf("#show figure.where(kind: %s): set figure.caption(separator: [%s])", style.kind, escape(style.separator)))
	}
	lines = append(lines, e.headingReferences(), probe)
	if len(e.sectioned) > 0 {
		updates := make([]string, len(e.sectioned))
		for index, counter := range e.sectioned {
			updates[index] = counter + ".update(0)"
		}
		lines = append(lines, "#show heading.where(level: 1): it => { "+strings.Join(updates, "; ")+"; it }")
	}
	return strings.Join(lines, "\n")
}

// ---- structure ----

// renderNode is a node's Typst followed by the long descriptions of the images it holds that no
// node inside it placed (decision 110); inside a float they wait for the figure's end.
func (e *exporter) renderNode(id string) string {
	if e.floatDepth > 0 {
		return e.renderNodeOnly(id)
	}
	outer := e.longDescriptions
	e.longDescriptions = nil
	output := joinPresent("\n\n", e.renderNodeOnly(id), strings.Join(e.longDescriptions, "\n\n"))
	e.longDescriptions = outer
	return output
}

func (e *exporter) renderNodeOnly(id string) string {
	if e.consumed[id] {
		return ""
	}
	node := e.node(id)
	kind := toS(node["type"])
	switch {
	case slices.Contains(structures, kind):
		return e.children(node)
	case kind == "heading" || kind == "paragraph":
		if _, ok := e.headingInfo[id]; ok {
			return e.heading(node)
		}
		return e.paragraph(node)
	case kind == "display_math":
		return e.display(node)
	case kind == "theorem":
		return e.theorem(node)
	case kind == "proof":
		return e.proof(node)
	case kind == "code":
		return e.code(node)
	case kind == "ordered_list" || kind == "unordered_list":
		return e.list(node)
	case kind == "figure_float" || kind == "table_float":
		return e.float(node)
	}
	return ""
}

func (e *exporter) children(node rec.Map) string {
	return e.renderNodeSlice(children(node))
}

func (e *exporter) renderNodeSlice(ids []string) string {
	outputs := []string{}
	for _, id := range ids {
		output := e.renderNode(id)
		if e.scopeDepth == 0 && len(e.hoisted) > 0 {
			hoisted := strings.Join(e.hoisted, "\n")
			e.hoisted = nil
			output = joinPresent("\n", hoisted, output)
		}
		if output != "" {
			outputs = append(outputs, output)
		}
	}
	return strings.Join(outputs, "\n\n")
}

func joinPresent(separator string, parts ...string) string {
	present := []string{}
	for _, part := range parts {
		if part != "" {
			present = append(present, part)
		}
	}
	return strings.Join(present, separator)
}

// scoped: #set and #let are scoped to their content block: inside one they move before the
// top-level block that contains it. Counter updates work anywhere and stay.
func (e *exporter) scoped(line string) string {
	e.trackRule(line)
	if e.scopeDepth == 0 {
		return line
	}
	e.hoisted = append(e.hoisted, line)
	return ""
}

func (e *exporter) nested(render func() string) string {
	e.scopeDepth++
	defer func() { e.scopeDepth-- }()
	return render()
}

// ---- labels ----

// anchors: the node's labels not placed yet, as link targets (#metadata) where they are written.
func (e *exporter) anchors(node rec.Map) string {
	return e.anchorsOf(e.labelsByTarget[toS(node["id"])], "")
}

// anchorsOf: the names not placed yet, other than skip (the element's own label, placed after
// its body), as link targets: the second label of a heading or display, a label on a theorem's
// head paragraph. Their references print LaTeX's number through a link.
func (e *exporter) anchorsOf(names []string, skip string) string {
	parts := []string{}
	for _, name := range names {
		if e.claimedLabels[name] || name == skip {
			continue
		}
		if _, ok := e.placed[name]; ok {
			continue
		}
		e.placed[name] = anchor
		parts = append(parts, "#metadata(none) <"+e.labels[name]+">")
	}
	return strings.Join(parts, " ")
}

func (e *exporter) ownLabel(node rec.Map) string {
	for _, name := range e.labelsByTarget[toS(node["id"])] {
		if !e.claimedLabels[name] {
			return name
		}
	}
	return ""
}

func (e *exporter) labelNumber(name string) any { return dig(e.document, "labels", name, "number") }

// labelSuffix: a label on an element: @label can refer to it when it is numbered, #link
// otherwise. A label LaTeX numbered otherwise than this element prints is no reference to it:
// a link target only, so references keep the number LaTeX printed.
func (e *exporter) labelSuffix(name string, numbered bool, printed *string) string {
	if name == "" {
		return ""
	}
	if _, ok := e.placed[name]; ok {
		return ""
	}
	if printed != nil && comparableNumber(e.labelNumber(name)) != comparableNumber(*printed) {
		numbered = false
	}
	e.placed[name] = anchor
	if numbered {
		e.placed[name] = element
	}
	return " <" + e.labels[name] + ">"
}

// ---- paragraphs ----

func (e *exporter) paragraph(node rec.Map) string {
	id := toS(node["id"])
	if e.styledBib != nil && e.styledBib.blocks[id] {
		return e.styledItem(id)
	}
	if len(e.bibliography[id]) > 0 {
		return e.bibliographyItem(node)
	}
	text := e.abstractText(node, protectStart(rec.Strip(e.inline(content(node), node, 0, false))))
	if text == "" {
		return ""
	}
	if anchors := e.anchors(node); anchors != "" {
		text = anchors + " " + text
	}
	if align, ok := node["align"].(string); ok {
		return "#align(" + align + ")[" + text + "]"
	}
	// Title-page lines are centered; a long paragraph before the first section is body text.
	if slices.Contains(e.frontBlocks, id) && !slices.Contains(e.abstractBlocks, id) && len(fields(plain(node))) <= 40 {
		return "#align(center)[" + text + "]"
	}
	return text
}

func (e *exporter) abstractText(node rec.Map, text string) string {
	id := toS(node["id"])
	if !slices.Contains(e.abstractBlocks, id) {
		return text
	}
	if e.abstractBlocks[0] == id {
		text = "#abstract[" + dropAbstractWord(text)
	}
	if e.abstractBlocks[len(e.abstractBlocks)-1] == id {
		text += "]"
	}
	return text
}

// dropAbstractWord is sub(/\A(?:\\?\s)*(?:Abstract|ABSTRACT)[.:—–]?\s*/, ”).
func dropAbstractWord(text string) string {
	runes := []rune(text)
	at := 0
	for {
		if at < len(runes) && isSpace(runes[at]) {
			at++
		} else if at+1 < len(runes) && runes[at] == '\\' && isSpace(runes[at+1]) {
			at += 2
		} else {
			break
		}
	}
	rest := string(runes[at:])
	word := ""
	for _, candidate := range []string{"Abstract", "ABSTRACT"} {
		if strings.HasPrefix(rest, candidate) {
			word = candidate
			break
		}
	}
	if word == "" {
		return text
	}
	after := []rune(rest[len(word):])
	index := 0
	if index < len(after) && strings.ContainsRune(".:—–", after[index]) {
		index++
	}
	for index < len(after) && isSpace(after[index]) {
		index++
	}
	return string(after[index:])
}

// bibliographyItem: an entry printed with its label ([12] ...), or an author-year entry,
// which has none.
func (e *exporter) bibliographyItem(node rec.Map) string {
	entries := e.bibliography[toS(node["id"])]
	entry, merged := entries[0], entries[1:]
	printed := toS(rec.Fetch(entry, "number"))
	pattern := regexp.MustCompile(`\A[ \t\n\v\f\r]*\[?` + regexp.QuoteMeta(printed) + `\]?[ \t\n\v\f\r]*`)
	text := marked(node)
	location := pattern.FindStringIndex(text)
	skip := 0
	numbered := location != nil
	if numbered {
		skip = runeOffset(text, location[1])
	}
	body := rec.Strip(e.inline(content(node), node, skip, false))
	var anchors strings.Builder
	for _, other := range merged {
		if !e.cited(other) {
			continue
		}
		key := toS(rec.Fetch(other, "key"))
		e.placed["cite:"+key] = anchor
		e.labelsMerged[key] = true
		anchors.WriteString(" #metadata(none) <" + e.labels["cite:"+key] + ">")
	}
	// A \label in the entry (\label{LastBibItem}) is a link target there.
	if own := e.anchors(node); own != "" {
		anchors.WriteString(" " + own)
	}
	label := "none"
	if numbered {
		label = typstString(printed)
	}
	item := "#bibitem(" + label + ")[" + body + anchors.String() + "]"
	if !e.cited(entry) {
		return item
	}
	name := "cite:" + toS(rec.Fetch(entry, "key"))
	e.placed[name] = anchor
	if numbered {
		e.placed[name] = element
	}
	return item + " <" + e.labels[name] + ">"
}
