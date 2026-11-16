package paperdoc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/sarielhp/relatex/internal/bookindex"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/extlinks"
	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/frontmatter"
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/mathml"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/paintmodel"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/schema"
	"github.com/sarielhp/relatex/internal/stylemodel"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

var (
	blockTypes = []string{"heading", "paragraph", "display_math"}
	// structures is PaperDocumentValidation::STRUCTURES.
	structures = []string{"document", "section", "theorem", "proof", "note", "unordered_list", "ordered_list", "list_item", "code",
		"figure_float", "table_float", "float_caption"}
	nodeFields  = []string{"id", "type", "children", "provenance"}
	extraFields = map[string][]string{"math": {"mathml", "tex"}, "math_row": {"index", "tag", "mathml_path"},
		"note": {"number", "source_parent", "labels", "callsites"}}
	inlineFields = map[string][]string{"text": {"type", "text"}, "math": {"type", "node"}, "symbol": {"type", "name"},
		"reference": {"type", "text", "label", "target"}, "note_reference": {"type", "text", "target", "callsite"}}
	imageFields = []string{"type", "id", "asset", "width_sp", "height_sp"}
	nodeID      = regexp.MustCompile(`\An-[0-9a-f]{64}\z`)
	ruleColor   = regexp.MustCompile(`\A#[0-9a-f]{6}\z`)
)

// validMath holds the formulas already found valid: a document is validated at every
// boundary, and parsing its MathML is most of the cost.
var validMath sync.Map

type validation struct {
	data      rec.Map
	nodes     rec.Map
	seen      []string
	seenSet   map[string]bool
	blocks    []any
	callsites []rec.Map
}

// Validate is PaperDocument.validate!.
func Validate(data rec.Map) {
	schema.Validate(data, contract("paper-document.schema.json"), "$")
	expected := map[any]string{"paper-0.1": "paper-reading-diagnostic-v1", "paper-0.2": "paper-reading-diagnostic-v2"}
	profile, ok := expected[rec.Fetch(data, "schema_version")]
	if !ok {
		panic(rec.Failure{Message: fmt.Sprintf("key not found: %q", data["schema_version"]), Lookup: true})
	}
	if rec.Fetch(data, "profile") != profile {
		rec.Fail("Paper version and profile differ")
	}
	v := &validation{data: data, nodes: rec.FetchMap(data, "nodes"), seenSet: map[string]bool{}}
	v.validate()
}

func (v *validation) node(id any) rec.Map { return rec.Node(v.nodes, id) }

// sortedIDs is the nodes in the order a parsed document holds them (canonical files sort keys).
func (v *validation) sortedIDs() []string {
	ids := make([]string, 0, len(v.nodes))
	for id := range v.nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (v *validation) validate() {
	v.validateDocument()
	if !rec.Equal(v.descendantBlocks(rec.FetchString(v.data, "root"), map[string]bool{}), rec.FetchList(v.data, "main_block_order")) {
		rec.Fail("Main paper block order mismatch")
	}
	v.validateCallsites()
	v.validateImages()
	v.validateTableGrids()
	v.validateLists()
	validateFloats(v.nodes, v.sortedIDs())
	paintmodel.ValidateTables(v.data)
	paintmodel.ValidateText(v.data)
	fontmodel.Validate(v.data)
	frontmatter.Validate(v.data)
	extlinks.Validate(v.data)
	bookindex.Validate(v.data)
	citations, _ := v.data["citations"].(rec.Map)
	ValidateCitations(citations, v.nodes, rec.FetchList(v.data, "block_order"))
}

// validateDocument is DocumentValidation#validate!.
func (v *validation) validateDocument() {
	defer incomplete("Invalid document: ")
	if rec.Fetch(v.node(rec.Fetch(v.data, "root")), "type") != "document" {
		rec.Fail("Unexpected root type")
	}
	v.validateProfile()
	roots := append([]any{rec.Fetch(v.data, "root")}, rec.FetchList(v.data, "note_order")...)
	for _, root := range roots {
		v.visit(rec.AsString(root), 0)
	}
	seen := slices.Clone(v.seen)
	sort.Strings(seen)
	if !slices.Equal(seen, v.sortedIDs()) {
		rec.Fail("Unowned document nodes")
	}
	if !rec.Equal(v.blocks, rec.FetchList(v.data, "block_order")) {
		rec.Fail("Document block order mismatch")
	}
	labels := rec.FetchMap(v.data, "labels")
	names := make([]string, 0, len(labels))
	for name := range labels {
		names = append(names, name)
	}
	sort.Strings(names)
	targets := append(slices.Clone(blockTypes), "math_row", "document_page_total")
	for _, name := range names {
		label := rec.AsMap(labels[name])
		if rec.Fetch(label, "status") != "verified" {
			rec.Fail("Unverified label")
		}
		if !slices.Contains(targets, rec.FetchString(v.node(rec.Fetch(label, "target")), "type")) {
			rec.Fail("Invalid label target")
		}
	}
}

func (v *validation) validateProfile() {
	for _, value := range rec.FetchMap(v.data, "labels") {
		label, ok := value.(rec.Map)
		valid := ok && rec.SameKeys(label, []string{"number", "status", "target"})
		for _, field := range label {
			_, isString := field.(string)
			valid = valid && isString
		}
		if !valid {
			rec.Fail("Malformed paper label")
		}
	}
	if !sha256Hex.MatchString(rec.FetchString(rec.FetchMap(v.data, "identity"), "input_sha256")) {
		rec.Fail("Invalid paper digest")
	}
	if !rec.Equal(rec.Fetch(v.data, "unverified_requirements"), Gaps) {
		rec.Fail("Missing paper verification gaps")
	}
	notes := rec.FetchList(v.data, "note_order")
	seen := map[any]bool{}
	for _, id := range notes {
		if seen[id] || rec.Fetch(v.node(id), "type") != "note" {
			rec.Fail("Invalid note root inventory")
		}
		seen[id] = true
	}
}

func (v *validation) visit(identifier string, depth int) {
	if v.seenSet[identifier] || depth > 64 {
		rec.Fail("Document cycle, repeated owner, or excessive depth")
	}
	node := v.node(identifier)
	schema.Validate(node, contract("paper-document-node.schema.json"), "$")
	if !nodeID.MatchString(identifier) || rec.Fetch(node, "id") != identifier {
		rec.Fail("Invalid document ID")
	}
	v.seen = append(v.seen, identifier)
	v.seenSet[identifier] = true
	v.validateNode(node)
	for _, child := range rec.FetchList(node, "children") {
		v.visit(rec.AsString(child), depth+1)
	}
}

func (v *validation) validateNode(node rec.Map) {
	if rec.Fetch(node, "type") == "document_page_total" {
		v.validatePageTotal(node)
		return
	}
	children := []any{}
	for _, child := range rec.FetchList(node, "children") {
		if rec.Fetch(v.node(child), "type") != "document_page_total" {
			children = append(children, child)
		}
	}
	v.validateDocumentNode(rec.Merge(node, rec.Map{"children": children}))
	if !slices.Contains(structures, rec.FetchString(node, "type")) {
		return
	}
	for _, child := range rec.FetchList(node, "children") {
		if rec.Fetch(v.node(child), "type") == "note" {
			rec.Fail("Notes must be separate reading roots")
		}
	}
}

// validateDocumentNode is DocumentValidation#validate_node!.
func (v *validation) validateDocumentNode(node rec.Map) {
	kind := rec.FetchString(node, "type")
	block := slices.Contains(blockTypes, kind)
	if !slices.Contains(structures, kind) && !block && kind != "math" && kind != "math_row" {
		rec.Fail("Unsupported document node: %s", kind)
	}
	fields := extraFields[kind]
	if kind == "math" {
		// A formula may print references of its own; a formula without them carries no content.
		if _, ok := node["content"]; ok {
			fields = append(slices.Clone(fields), "content")
		}
		// A formula whose round trip proved its TeX (decision 141) carries it.
		if _, ok := node["formula_tex"]; ok {
			fields = append(slices.Clone(fields), "formula_tex")
		}
	}
	if block {
		fields = []string{"content"}
	}
	own := rec.Map{}
	for key, value := range node {
		if key != "semantics" && key != "align" && key != "marker" {
			own[key] = value
		}
	}
	if !rec.SameKeys(own, append(slices.Clone(nodeFields), fields...)) {
		rec.Fail("Unknown or missing node fields")
	}
	if _, aligned := node["align"]; aligned && kind != "paragraph" {
		rec.Fail("Only a paragraph has an alignment")
	}
	if marker, marked := node["marker"]; marked {
		validateItemMarker(kind, marker)
	}
	switch {
	case slices.Contains(structures, kind):
		allowed := append(slices.Clone(structures[1:]), blockTypes...)
		for _, child := range rec.FetchList(node, "children") {
			if !slices.Contains(allowed, rec.FetchString(v.node(child), "type")) {
				rec.Fail("Invalid structural child")
			}
		}
	case block:
		v.blocks = append(v.blocks, rec.Fetch(node, "id"))
		v.validateBlock(node)
	case kind == "math":
		v.validateMathReferences(node)
		v.validateMath(node)
	case kind == "math_row":
		if len(rec.FetchList(node, "children")) > 0 {
			rec.Fail("Math row cannot own children")
		}
	}
}

func spanMaps(values []any) []rec.Map {
	result := make([]rec.Map, len(values))
	for index, value := range values {
		result[index] = rec.AsMap(value)
	}
	return result
}

// validateBlock is PaperDocumentValidation#validate_block!.
func (v *validation) validateBlock(node rec.Map) {
	content := spanMaps(rec.FetchList(node, "content"))
	grids, rest := []rec.Map{}, []rec.Map{}
	for _, span := range content {
		if rec.Fetch(span, "type") == "table_grid" {
			grids = append(grids, span)
		} else {
			rest = append(rest, span)
		}
	}
	if len(grids) > 0 {
		v.validateTableBlock(node, grids)
	}
	for _, span := range rest {
		v.validateSpan(span)
	}
	// Legacy grids carry their text in the grid; rich grids own their cells' spans.
	canonical := []any{}
	for _, span := range inline.Deep(rec.FetchList(node, "content")) {
		kind := rec.Fetch(span, "type")
		if kind == "image" || kind == "rule" || kind == "table_grid" {
			continue
		}
		stripped := rec.Map{}
		for key, value := range span {
			if kind == "symbol" && key == "image" {
				continue
			}
			if key != "style" && key != "color" && key != "lang" && key != "glyph_name" && key != "decoration" && key != "line_break" && key != "external_link" && !slices.Contains(citationFields, key) &&
				!slices.Contains(mathLinkFields, key) {
				stripped[key] = value
			}
		}
		canonical = append(canonical, stripped)
	}
	v.validateInline(rec.Merge(node, rec.Map{"content": canonical}))
}

// validateSymbolMark: an author's QED mark (decision 176) shows its glyphs' text or one image named "End of proof", never
// both; the pinned open square shows neither.
func (v *validation) validateSymbolMark(span rec.Map) {
	_, text := span["text"]
	mark, image := span["image"].(rec.Map)
	_, other := span["image"]
	if other && !image || span["name"] != "qed" && (text || other) || text && other {
		rec.Fail("Invalid QED mark")
	}
	if image {
		if !namedMark(mark) && !legacyMark(mark) {
			rec.Fail("A QED mark's image is named End of proof")
		}
		v.validateImageSpan(mark)
	}
}

// namedMark is a QED mark's image as relatex writes it since decision 176 amendment 1.
func namedMark(mark rec.Map) bool {
	return mark["decorative"] != true && mark["alt"] == EndOfProof && mark["alt_provenance"] == Inferred
}

// legacyMark is a QED mark's image as a document converted before amendment 1 holds it (decorative),
// still read so lint and alt work on an existing export until it is converted again.
func legacyMark(mark rec.Map) bool {
	_, alt := mark["alt"]
	return mark["decorative"] == true && !alt && mark["alt_provenance"] == Inferred
}

func (v *validation) validateSpan(span rec.Map) {
	kind := rec.Fetch(span, "type")
	switch kind {
	case "image":
		v.validateImageSpan(span)
		return
	case "rule":
		validateRuleSpan(span)
	case "symbol":
		v.validateSymbolMark(span)
	}
	validateSpanColor(span, kind)
	validateSpanLanguage(span, kind)
	validateSpanGlyphName(span, kind)
	validateLineBreak(span, kind)
	v.validateMathLink(span, kind)
	style, styled := span["style"]
	if !styled {
		return
	}
	if kind != "text" && kind != "reference" && kind != "note_reference" {
		rec.Fail("Style on non-text content")
	}
	stylemodel.Validate(style)
}

// mathLinkFields: a formula set wholly inside a \hyperref text carries that link's label and its
// target, the label's own.
var mathLinkFields = []string{"link_label", "link_target"}

func (v *validation) validateMathLink(span rec.Map, kind any) {
	label, labelled := span["link_label"]
	target, targeted := span["link_target"]
	if !labelled && !targeted {
		return
	}
	name, isName := label.(string)
	if kind != "math" || !labelled || !targeted || !isName {
		rec.Fail("Invalid formula link")
	}
	entry, found := rec.FetchMap(v.data, "labels")[name].(rec.Map)
	if !found || rec.Fetch(entry, "target") != target {
		rec.Fail("Formula link target mismatch")
	}
}

// textColor is a text span's optional fill color: #rrggbb, never plain black (black is absent).
var textColor = regexp.MustCompile(`\A#[0-9a-f]{6}\z`)

func validateSpanColor(span rec.Map, kind any) {
	color, colored := span["color"]
	if !colored || kind == "rule" {
		return
	}
	text, isString := color.(string)
	if kind != "text" && kind != "reference" || !isString || !textColor.MatchString(text) || text == "#000000" {
		rec.Fail("Invalid text color")
	}
}

func gridOccurrence(span rec.Map) bool {
	id, ok := span["id"].(string)
	return rec.SameKeys(span, []string{"grid", "id", "type"}) && ok && nodeID.MatchString(id)
}

func (v *validation) validateTableBlock(node rec.Map, grids []rec.Map) {
	rich := true
	for _, span := range grids {
		grid, _ := span["grid"].(rec.Map)
		rich = rich && grid["schema_version"] == tablemodel.Version
	}
	if rich {
		if v.data["schema_version"] != "paper-0.2" {
			rec.Fail("Rich table requires paper-0.2")
		}
		for _, span := range grids {
			if !gridOccurrence(span) {
				rec.Fail("Invalid rich table occurrence")
			}
			tablemodel.NewWith(rec.FetchMap(span, "grid"), v.nodes)
		}
		return
	}
	if len(grids) != 1 || len(rec.FetchList(node, "content")) != 1 || rec.Fetch(node, "type") != "paragraph" {
		rec.Fail("Grid requires an exclusive paragraph block")
	}
	if !gridOccurrence(grids[0]) {
		rec.Fail("Invalid table grid occurrence")
	}
	tablemodel.ValidateShape(rec.FetchMap(grids[0], "grid"))
}

// validateInline is DocumentValidation#validate_block!.
func (v *validation) validateInline(node rec.Map) {
	math := []any{}
	for _, value := range rec.FetchList(node, "content") {
		span := rec.AsMap(value)
		kind := rec.FetchString(span, "type")
		expected, ok := inlineFields[kind]
		if !ok {
			rec.Fail("Unsupported inline node")
		}
		if _, text := span["text"]; kind == "symbol" && text {
			expected = []string{"type", "name", "text"}
		}
		strings := rec.SameKeys(span, expected)
		for _, field := range span {
			_, isString := field.(string)
			strings = strings && isString
		}
		if !strings {
			rec.Fail("Unknown or missing inline fields")
		}
		switch kind {
		case "symbol":
			if span["name"] != "qed_open_square" && span["name"] != "qed" {
				rec.Fail("Unknown symbol")
			}
		case "math":
			if rec.Fetch(v.node(span["node"]), "type") != "math" {
				rec.Fail("Invalid math use")
			}
			math = append(math, span["node"])
		case "reference":
			label, found := rec.FetchMap(v.data, "labels")[rec.FetchString(span, "label")].(rec.Map)
			if !found {
				panic(rec.Failure{Message: fmt.Sprintf("key not found: %q", span["label"]), Lookup: true})
			}
			if rec.Fetch(label, "target") != span["target"] {
				rec.Fail("Reference target mismatch")
			}
		}
	}
	if !rec.Equal(math, rec.FetchList(node, "children")) {
		rec.Fail("Math ownership mismatch")
	}
}

// validateMathReferences: a reference printed inside a formula is an ordinary reference
// span on the formula node; its printed text and target are the label's, and its number is
// the formula's MathML, not a second text run.
func (v *validation) validateMathReferences(node rec.Map) {
	content, ok := node["content"].([]any)
	if !ok {
		return
	}
	for _, value := range content {
		span := rec.AsMap(value)
		if rec.Fetch(span, "type") != "reference" || !rec.SameKeys(span, inlineFields["reference"]) {
			rec.Fail("Invalid formula reference content")
		}
		for _, field := range span {
			if _, isString := field.(string); !isString {
				rec.Fail("Unknown or missing formula reference fields")
			}
		}
		name := rec.FetchString(span, "label")
		label, found := rec.FetchMap(v.data, "labels")[name].(rec.Map)
		if !found {
			panic(rec.Failure{Message: fmt.Sprintf("key not found: %q", span["label"]), Lookup: true})
		}
		if rec.Fetch(label, "target") != span["target"] {
			rec.Fail("Reference target mismatch")
		}
	}
}

// validateMath: a formula and the rows it is checked against, once valid, are valid again.
func (v *validation) validateMath(node rec.Map) {
	rows := []any{}
	for _, id := range rec.FetchList(node, "children") {
		rows = append(rows, v.node(id))
	}
	bytes, err := canonjson.Marshal([]any{"Relatex::PaperMathML", rec.Fetch(node, "mathml"), rows})
	if err != nil {
		rec.Fail("%v", err)
	}
	sum := sha256.Sum256(bytes)
	key := hex.EncodeToString(sum[:])
	if _, ok := validMath.Load(key); ok {
		return
	}
	v.checkMath(node)
	validMath.Store(key, true)
}

func (v *validation) checkMath(node rec.Map) {
	math := mathml.ParsePaper(rec.FetchString(node, "mathml"))
	for index, id := range rec.FetchList(node, "children") {
		row := v.node(id)
		path := fmt.Sprintf("/math/mtable/mtr[%d]", index+1)
		if row["type"] != "math_row" || !rec.Equal(row["index"], int64(index+1)) || row["mathml_path"] != path {
			rec.Fail("Invalid mathematical row")
		}
		element := math.Row(index + 1)
		if element == nil {
			rec.Fail("Missing mathematical row")
		}
		cells := element.Elements()
		if len(cells) == 0 {
			rec.Fail("undefined method 'attributes' for nil")
		}
		var tag any
		if cells[0].Attr("intent") == ":equation-label" {
			tag = mathml.LabelText(cells[0])
		}
		if rec.Fetch(row, "tag") != tag {
			rec.Fail("Mathematical tag mismatch")
		}
	}
}

func (v *validation) validatePageTotal(node rec.Map) {
	count, isInt := node["count"].(int64)
	anchor, isAnchor := node["anchor"].(string)
	digest, isDigest := node["pdf_sha256"].(string)
	fields := append(slices.Clone(nodeFields), "count", "source_label", "anchor", "pdf_sha256")
	content, hasContent := node["content"]
	if hasContent {
		fields = append(fields, "content")
	}
	_ = anchor
	if !rec.SameKeys(node, fields) ||
		len(rec.FetchList(node, "children")) > 0 || node["source_label"] != "TotPages" || !isInt || count < 1 || count > 100_000 ||
		!isAnchor || !isDigest || !sha256Hex.MatchString(digest) {
		rec.Fail("Invalid document page-total node")
	}
	if hasContent {
		spans := spanMaps(rec.FetchList(rec.Map{"content": content}, "content"))
		if len(spans) != 1 || !validPageTotalReference(spans[0], node, count) {
			rec.Fail("Invalid document page-total reference")
		}
		v.validateSpan(spans[0])
	} else if !hasReferenceLabel(v.nodes, "TotPages") {
		rec.Fail("Missing document page-total reference")
	}
	root := v.node(rec.Fetch(v.data, "root"))
	label, ok := rec.FetchMap(v.data, "labels")["TotPages"].(rec.Map)
	if !ok {
		panic(rec.Failure{Message: `key not found: "TotPages"`, Lookup: true})
	}
	metadata := rec.Map{"status": "partial", "source_page_total": rec.Fetch(node, "id")}
	if !slices.Contains(rec.FetchList(root, "children"), rec.Fetch(node, "id")) || rec.Fetch(label, "target") != rec.Fetch(node, "id") ||
		rec.Fetch(label, "number") != toS(count) || !rec.Equal(rec.Fetch(v.data, "metadata"), metadata) {
		rec.Fail("Document page-total ownership mismatch")
	}
}

func validateRuleSpan(span rec.Map) {
	sizes := []any{span["width_sp"], span["height_sp"], span["depth_sp"]}
	if truthy(span["margin_left_sp"]) || truthy(span["margin_right_sp"]) {
		sizes = append(sizes, span["margin_left_sp"], span["margin_right_sp"])
	}
	own := rec.Map{}
	for key, value := range span {
		if !slices.Contains(ruleOptions, key) {
			own[key] = value
		}
	}
	valid := rec.SameKeys(own, ruleFields)
	numbers := []int64{}
	for _, size := range sizes {
		value, isInt := size.(int64)
		valid = valid && isInt && value > -1_073_741_824 && value < 1_073_741_824
		numbers = append(numbers, value)
	}
	valid = valid && numbers[0] > 0 && numbers[1]+numbers[2] > 0
	if color, ok := span["color"]; valid && (!ok || ruleColor.MatchString(toS(color))) {
		return
	}
	rec.Fail("Invalid painted rule")
}

// describedFields are fields plus an image's description fields, checked: a text alternative
// (relatex/alt.toml, decision 086, or the source, decision 096) with where it came from and an
// optional long description; or a decorative mark from the source or the author's sidecar.
func describedFields(span rec.Map, fields []string) []string {
	_, described := span["alt"]
	_, decorative := span["decorative"]
	_, long := span["long_description"]
	provenance, _ := span["alt_provenance"].(string)
	switch {
	case described && !decorative:
		text, isText := span["alt"].(string)
		if !isText || strings.TrimSpace(text) == "" || !slices.Contains([]string{"author", "model", "reviewed", "source", "source_macro", Inferred}, provenance) {
			rec.Fail("Invalid figure description")
		}
		fields = append(slices.Clone(fields), "alt", "alt_provenance")
		if long {
			if text, isText := span["long_description"].(string); !isText || strings.TrimSpace(text) == "" {
				rec.Fail("Invalid figure description")
			}
			fields = append(fields, "long_description")
			fields = detailsField(span, fields)
		}
	case decorative && !described && !long:
		if span["decorative"] != true || !slices.Contains([]string{"author", "source", Inferred}, provenance) {
			rec.Fail("Invalid figure description")
		}
		fields = append(slices.Clone(fields), "decorative", "alt_provenance")
	case decorative || long:
		rec.Fail("Invalid figure description")
	}
	return fields
}

func (v *validation) validateImageSpan(span rec.Map) {
	fields := imageFields
	if _, has := span["source"]; has {
		fields = append(slices.Clone(fields), "source")
	}
	if _, has := span["source_file"]; has {
		fields = append(slices.Clone(fields), "source_file", "source_line")
	}
	// Only the ORCID logo is a linked image (its link's target and size are checked with the links).
	if _, linked := span["external_link"]; linked {
		fields = append(slices.Clone(fields), "external_link")
	}
	fields = describedFields(span, fields)
	if placement, ok := span["placement"]; ok {
		if v.data["schema_version"] != "paper-0.2" {
			rec.Fail("Placement requires paper-0.2")
		}
		fields = append(slices.Clone(fields), "placement", "depth_sp")
		geometry := rec.AsMap(placement)
		mediamodel.ValidatePlacement(geometry)
		if !rec.Equal(rec.Pick(span, "width_sp", "height_sp", "depth_sp"), rec.Pick(geometry, "width_sp", "height_sp", "depth_sp")) {
			rec.Fail("Media span differs from placement geometry")
		}
	}
	asset, isAsset := span["asset"].(string)
	id, isID := span["id"].(string)
	valid := rec.SameKeys(span, fields) && isAsset && sha256Hex.MatchString(asset) && isID && nodeID.MatchString(id)
	if origin, has := span["source"]; has {
		valid = valid && mediamodel.SafeSourcePath(origin)
	}
	if file, has := span["source_file"]; has {
		line, isInt := span["source_line"].(int64)
		valid = valid && mediamodel.SafeSourcePath(file) && isInt && line >= 1 && span["source"] != nil
	}
	for _, key := range []string{"width_sp", "height_sp"} {
		value, isInt := span[key].(int64)
		valid = valid && isInt && value >= 1 && value <= 1_073_741_823
	}
	if !valid {
		rec.Fail("Invalid paper image occurrence")
	}
}

func (v *validation) contentSpans() []rec.Map {
	all := []any{}
	for _, id := range v.sortedIDs() {
		content, _ := rec.AsMap(v.nodes[id])["content"].([]any)
		all = append(all, content...)
	}
	return inline.Deep(all)
}

func (v *validation) validateTableGrids() {
	ids, images := []any{}, []any{}
	for _, span := range v.contentSpans() {
		switch span["type"] {
		case "table_grid":
			ids = append(ids, rec.Fetch(span, "id"))
		case "image":
			images = append(images, rec.Fetch(span, "id"))
		}
	}
	seen := map[any]bool{}
	for _, id := range ids {
		_, isNode := v.nodes[toS(id)]
		if seen[id] || isNode || includes(images, id) {
			rec.Fail("Duplicate or conflicting grid identity")
		}
		seen[id] = true
	}
}

func (v *validation) validateImages() {
	images := []rec.Map{}
	seen := map[any]bool{}
	for _, span := range v.contentSpans() {
		if rec.Fetch(span, "type") != "image" {
			continue
		}
		id := rec.Fetch(span, "id")
		if _, isNode := v.nodes[toS(id)]; seen[id] || isNode {
			rec.Fail("Duplicate or conflicting paper image identity")
		}
		seen[id] = true
		images = append(images, span)
	}
	assetData := rec.FetchMap(v.data, "assets")
	mediamodel.ValidatePNGAssets(assetData, images)
	for _, image := range images {
		asset, ok := rec.FetchMap(assetData, "items")[rec.FetchString(image, "asset")].(rec.Map)
		if !ok {
			panic(rec.Failure{Message: fmt.Sprintf("key not found: %q", image["asset"]), Lookup: true})
		}
		placements, _ := asset["placements"].(rec.Map)
		placementList, _ := asset["placements"].([]any)
		_, placed := image["placement"]
		if (len(placements) > 0 || len(placementList) > 0) && !placed {
			rec.Fail("Placement-aware media occurrence lost its placement")
		}
		if placed {
			mediamodel.NewMediaAsset(asset).Representation("html", rec.FetchMap(image, "placement"))
		}
	}
}

func (v *validation) validateLists() {
	for _, id := range v.sortedIDs() {
		node := rec.AsMap(v.nodes[id])
		kind := node["type"]
		list := kind == "unordered_list" || kind == "ordered_list"
		for _, child := range rec.FetchList(node, "children") {
			childType := rec.Fetch(v.node(child), "type")
			if list && childType != "list_item" {
				rec.Fail("List contains a child outside an item")
			}
			if !list && childType == "list_item" {
				rec.Fail("List item has a non-list parent")
			}
		}
	}
}

func (v *validation) validateCallsites() {
	v.callsites = []rec.Map{}
	for _, id := range rec.FetchList(v.data, "note_order") {
		v.validateNote(v.node(id))
	}
	actual := []rec.Map{}
	for _, id := range v.sortedIDs() {
		content, _ := rec.AsMap(v.nodes[id])["content"].([]any)
		for _, span := range spanMaps(content) {
			if rec.Fetch(span, "type") == "note_reference" {
				actual = append(actual, span)
			}
		}
	}
	expected := map[any]rec.Map{}
	for _, entry := range v.callsites {
		if _, repeated := expected[entry["callsite"]]; repeated {
			rec.Fail("Repeated declared callsite ID")
		}
		expected[entry["callsite"]] = rec.Map{"target": entry["target"], "block": entry["block"]}
	}
	observed := map[any]rec.Map{}
	for _, link := range v.actualNoteLinks() {
		observed[link["callsite"]] = rec.Map{"target": link["target"], "block": link["block"]}
	}
	if !sameLinks(observed, expected) {
		rec.Fail("Note callsite ownership mismatch")
	}
	seen := map[any]bool{}
	for _, span := range actual {
		if seen[rec.Fetch(span, "callsite")] {
			rec.Fail("Repeated note callsite ID")
		}
		seen[span["callsite"]] = true
	}
	for _, span := range actual {
		id := rec.FetchString(span, "callsite")
		if _, isNode := v.nodes[id]; !nodeID.MatchString(id) || isNode {
			rec.Fail("Invalid or colliding callsite anchor")
		}
	}
}

func sameLinks(left, right map[any]rec.Map) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		other, ok := right[key]
		if !ok || !rec.Equal(value, other) {
			return false
		}
	}
	return true
}

func (v *validation) validateNote(note rec.Map) {
	blocks := v.descendantBlocks(rec.FetchString(note, "id"), map[string]bool{})
	for _, value := range rec.FetchList(note, "labels") {
		label := rec.AsMap(value)
		if !includes(blocks, rec.Fetch(label, "block")) {
			rec.Fail("Unowned note label")
		}
		if style, ok := label["style"]; ok {
			stylemodel.Validate(style)
		}
	}
	for _, value := range rec.FetchList(note, "callsites") {
		callsite := rec.AsMap(value)
		v.callsites = append(v.callsites, rec.Map{"target": rec.Fetch(note, "id"), "callsite": rec.Fetch(callsite, "id"),
			"block": rec.Fetch(callsite, "block")})
	}
}

func (v *validation) descendantBlocks(identifier string, seen map[string]bool) []any {
	if seen[identifier] {
		rec.Fail("Repeated note descendant")
	}
	seen[identifier] = true
	node := v.node(identifier)
	if slices.Contains(blockTypes, rec.FetchString(node, "type")) {
		return []any{identifier}
	}
	result := []any{}
	for _, child := range rec.FetchList(node, "children") {
		result = append(result, v.descendantBlocks(rec.AsString(child), seen)...)
	}
	return result
}

// actualNoteLinks: a call can sit in a table cell of its block (an author's \thanks in a
// title tabular).
func (v *validation) actualNoteLinks() []rec.Map {
	links := []rec.Map{}
	for _, block := range rec.FetchList(v.data, "block_order") {
		for _, span := range inline.Deep(rec.FetchList(v.node(block), "content")) {
			if rec.Fetch(span, "type") == "note_reference" {
				links = append(links, rec.Map{"target": span["target"], "callsite": span["callsite"], "block": block})
			}
		}
	}
	return links
}

// Structures is PaperDocumentValidation::STRUCTURES.
var Structures = structures
