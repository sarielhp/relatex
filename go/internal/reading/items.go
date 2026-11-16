package reading

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/sarielhp/relatex/internal/rec"
)

// The items of a reading partition: how each flow item becomes a span of its block.

func (e *Evidence) observeItem(item, owner rec.Map, page any) {
	e.validateExternalLink(item, owner)
	itemType := rec.Fetch(item, "type")
	if (truthy(item["note_mark"]) || truthy(item["note_number"])) && itemType != "text" && itemType != "math" {
		rec.Fail("Non-text footnote marker is unsupported")
	}
	if truthy(item["reference"]) {
		reference, ok := e.references.Get(rec.Fetch(item, "reference"))
		if !ok || !less(rec.Fetch(reference, "seq"), rec.Fetch(owner, "seq")) {
			rec.Fail("Dangling or premature reference execution")
		}
		e.retainedRefs = union(e.retainedRefs, rec.Fetch(item, "reference"))
	}
	if itemType == "math" {
		e.observeMath(item, owner, page)
		return
	}
	if e.pending != nil {
		e.insideOpenFormula(item, owner, page)
		return
	}
	if e.observeRule(item, owner, page) {
		return
	}
	block := e.proseBlock(owner, page)
	item = dropMarkerSpace(block, item)
	if e.objectAppended(block, item, owner, page) {
		return
	}
	if item["text"] == "[unmapped]" {
		diagnostic := rec.Map{"code": "unmapped_reading_glyph", "block": rec.Fetch(block, "id")}
		if name, named := item["glyph_name"].(string); named {
			diagnostic["name"] = name
		}
		e.diagnostics = append(e.diagnostics, diagnostic)
	}
	if itemType == "unsupported" {
		diagnostic := rec.Map{"code": "unsupported_reading_node", "block": rec.Fetch(block, "id")}
		if truthy(e.activeTable) {
			diagnostic["table"], diagnostic["owner"], diagnostic["slot"] = e.activeTable, rec.Fetch(owner, "owner"), e.activeSlot
		}
		e.diagnostics = append(e.diagnostics, diagnostic)
	}
	if truthy(item["note_mark"]) || truthy(item["note_number"]) {
		e.appendNoteMarker(block, item)
		return
	}
	e.appendItemText(block, item, spanPaint{color: e.textColor(owner), lang: e.spanLanguage(owner)}, e.fallbackStyle(owner, item))
}

// appendItemText appends a text item to the block's spans.
func (e *Evidence) appendItemText(block, item rec.Map, paint spanPaint, fallback rec.Map) {
	if _, named := item["glyph_name"]; named {
		// A named glyph without a character is a span of its own, so its name labels it alone.
		if space := rec.FetchString(item, "space"); space != "" {
			e.appendText(block, rec.Merge(withoutGlyphName(item), rec.Map{"text": space, "space": ""}), paint, fallback)
		}
		item = rec.Merge(item, rec.Map{"space": ""})
	}
	e.appendText(block, item, paint, fallback)
}

// withoutGlyphName is a text item without its glyph's name.
func withoutGlyphName(item rec.Map) rec.Map {
	result := rec.Map{}
	for key, value := range item {
		if key != "glyph_name" {
			result[key] = value
		}
	}
	return result
}

func (e *Evidence) validateExternalLink(item, owner rec.Map) {
	value, present := item["external_link"]
	if !present {
		return
	}
	id, _ := value.(int64)
	link, ok := e.externalLinks[id]
	if _, isInt := value.(int64); !isInt || !ok {
		rec.Fail("Dangling external link")
	}
	if !less(rec.Fetch(link, "seq"), rec.Fetch(owner, "seq")) {
		rec.Fail("Premature external link use")
	}
	itemType := rec.Fetch(item, "type")
	modeled := itemType == "text" || itemType == "math"
	for _, key := range []string{"reference", "note_mark", "note_number", "citation_link"} {
		if _, has := item[key]; has {
			modeled = false
		}
	}
	if _, unsupported := link["unsupported"]; !unsupported && (modeled || orcidLink(item, link)) {
		return
	}
	// A link around a graphic, inside another link, or to a target no reader can follow is
	// dropped; its content reads without it.
	delete(item, "external_link")
	e.dropExternalLink(link)
}

// dropExternalLink records a link the reading does not keep, once.
func (e *Evidence) dropExternalLink(link rec.Map) {
	if includes(e.droppedLinks, rec.Fetch(link, "link")) {
		return
	}
	e.droppedLinks = append(e.droppedLinks, rec.Fetch(link, "link"))
	reason, has := link["unsupported"]
	if !has {
		reason = "link around a graphic or another link"
	}
	e.diagnostics = append(e.diagnostics, rec.Map{"code": "unsupported_external_link", "href": rec.Fetch(link, "href"), "reason": reason})
}

func (e *Evidence) appendSpace(block, item rec.Map) {
	if space := rec.FetchString(item, "space"); space != "" {
		e.appendText(block, rec.Map{"text": space, "space": ""}, spanPaint{}, nil)
	}
}

// objectAppended: symbols, images and pictures are single spans, not text.
func (e *Evidence) objectAppended(block, item, owner rec.Map, page any) bool {
	switch rec.Fetch(item, "type") {
	case "symbol":
		e.appendSymbol(block, item, owner)
	case "image":
		e.appendSpace(block, item)
		push(block, e.rememberTableSpan(e.graphics.Span(item, owner)))
	case "graphic":
		e.appendRendered(block, item, page)
	default:
		return false
	}
	return true
}

// insideOpenFormula: without a recorded position the picture stays unsupported; its
// diagnostic remains. Text inside an open formula is the formula's own \text{}; its MathML
// already holds it. A picture there (\tikz beside or inside a formula) is no MathML: it
// reads in the block, and so does an included image (a QED image ending a display, an
// adjustbox valign=c graphic in a formula), which the formula's MathML leaves blank.
func (e *Evidence) insideOpenFormula(item, owner rec.Map, page any) {
	switch rec.Fetch(item, "type") {
	case "graphic":
		e.appendRendered(e.pending.block, item, page)
	case "image":
		e.objectAppended(e.pending.block, item, owner, page)
	}
}

func (e *Evidence) appendRendered(block, item rec.Map, page any) {
	span, _ := e.rendered.Span(item, page)
	if span == nil {
		return
	}
	e.appendSpace(block, item)
	e.orcidSpan(span, item)
	push(block, e.rememberTableSpan(span))
}

func (e *Evidence) appendSymbol(block, item, owner rec.Map) {
	id := rec.Fetch(item, "symbol")
	definition, ok := e.symbols.Get(id)
	if !ok || !less(rec.Fetch(definition, "seq"), rec.Fetch(owner, "seq")) {
		rec.Fail("Dangling or premature symbol execution")
	}
	shape := []any{rec.Fetch(item, "width"), rec.Fetch(item, "height"), rec.Fetch(item, "depth"), rec.Fetch(item, "rules")}
	depth, _ := rec.Number(shape[2])
	// The pinned open square is drawn from four rules; an author's QED mark (decision 176) is whatever its
	// one box holds: glyphs, an image or rules.
	mark := rec.Equal(rec.Fetch(definition, "name"), "qed")
	if !(positive(shape[0]) && positive(shape[1]) && depth >= 0 && (mark || rec.Equal(shape[3], int64(4)))) {
		rec.Fail("Incomplete or unsupported QED box")
	}
	if known, seen := e.symbolShapes.Get(id); seen && !rec.Equal(known, shape) {
		rec.Fail("Copied symbol geometry changed")
	}
	e.symbolShapes.Set(id, shape)
	e.symbolCount++
	e.appendSpace(block, item)
	symbol := rec.Map{"type": "symbol", "id": fmt.Sprintf("s%d", e.symbolCount), "symbol": id, "name": rec.Fetch(definition, "name")}
	if mark {
		e.qedMark(symbol, item, owner)
	}
	push(block, e.rememberTableSpan(symbol))
}

// qedMark keeps what the author's QED box draws (decision 176): its glyphs' text, or its one image as an image
// span the mark shows without alt text; a box of rules is a drawn mark and keeps neither.
func (e *Evidence) qedMark(symbol, item, owner rec.Map) {
	if text, ok := item["text"].(string); ok && text != "" {
		symbol["text"] = text
	}
	if image, ok := item["image"].(rec.Map); ok {
		symbol["image"] = e.graphics.Span(image, owner)
	}
}

// ruleSize: a rule outside a table is a painted bar (\rule, a tree or frame line): its
// width, height, depth, fill color and overlay margins. Shipout records a running size (its
// box's), a color other than black and the margins of a rule painted under what follows; a
// copy shipped twice differently stays unsupported. Table rules stay the table's borders.
func (e *Evidence) ruleSize(item, owner rec.Map) []any {
	if _, has := item["width"]; rec.Fetch(item, "type") != "unsupported" || truthy(e.activeTable) || !has {
		return nil
	}
	shipped := [][]any{}
	for _, paint := range e.rulePaint[ruleKey{rec.Fetch(owner, "owner"), e.activeSlot}] {
		if !slices.ContainsFunc(shipped, func(seen []any) bool { return reflect.DeepEqual(seen, paint) }) {
			shipped = append(shipped, paint)
		}
	}
	if len(shipped) > 1 {
		return nil
	}
	size := []any{rec.Fetch(item, "width"), rec.Fetch(item, "height"), rec.Fetch(item, "depth")}
	if len(shipped) == 1 {
		size = shipped[0]
	}
	if includes(size, int64(runningSize)) {
		return nil
	}
	return size
}

func paints(size []any) bool {
	height, _ := rec.Number(size[1])
	depth, _ := rec.Number(size[2])
	return positive(size[0]) && height+depth > 0
}

func (e *Evidence) observeRule(item, owner rec.Map, page any) bool {
	size := e.ruleSize(item, owner)
	if size == nil {
		return false
	}
	if !paints(size) {
		return true
	}
	e.appendRule(e.proseBlock(owner, page), item, size)
	return true
}

func (e *Evidence) appendRule(block, item rec.Map, size []any) {
	// As in TeX, a rule without positive width and height plus depth paints nothing.
	if !paints(size) {
		return
	}
	if itemMarkerTaken(block, item, size) {
		return
	}
	e.appendSpace(block, item)
	span := rec.Map{"type": "rule", "width_sp": size[0], "height_sp": size[1], "depth_sp": size[2]}
	if len(size) > 3 && truthy(size[3]) {
		span["color"] = size[3]
	}
	if len(size) > 4 && truthy(size[4]) {
		span["margin_left_sp"], span["margin_right_sp"] = size[4], size[5]
	}
	push(block, span)
}

func (e *Evidence) proseBlock(owner rec.Map, page any) rec.Map {
	if host := e.fragmentHost(owner); host != nil {
		host["pages"] = union(rec.FetchList(host, "pages"), page)
		return host
	}
	part := e.readingPartition(owner)
	last := e.lastBlocks[part]
	id := rec.Fetch(owner, "owner")
	key := blockKey{id, e.ownerUses[id]}
	if previous, ok := e.lastKeys[part]; last == nil || !ok || !rec.Equal(previous.owner, key.owner) || previous.uses != key.uses ||
		rec.Fetch(last, "type") == "display_math" {
		if headingContinues(last, owner) {
			push(last, lineBreak())
			e.lastKeys[part] = key
			last["pages"] = union(rec.FetchList(last, "pages"), page)
			return last
		}
		previous := last
		last = e.newBlock(rec.FetchString(owner, "role"), owner, page)
		if previous != nil {
			adoptOverlayRules(previous, last)
		}
	}
	e.lastKeys[part] = key
	e.lastBlocks[part] = last
	last["pages"] = union(rec.FetchList(last, "pages"), page)
	return last
}

// adoptOverlayRules moves the fill rules a block sits on from the end of the block before it
// into it (a heading, or a beamer block title or body, set on a colorbox): beamer sets a frame
// title on a beamercolorbox, a rule the box ships under the text with margin_right = -width
// (overlay), and the title text, owned by its section, is cut into a block of its own after it. Left behind, the rule paints a stray bar at the end of the previous
// paragraph and the title loses its fill. A paragraph that would be left empty keeps them.
func adoptOverlayRules(from, to rec.Map) {
	content := rec.FetchList(from, "content")
	keep := len(content)
	for keep > 0 && overlayFill(rec.AsMap(content[keep-1])) {
		keep--
	}
	if keep == 0 || keep == len(content) {
		return
	}
	to["content"] = append(slices.Clone(content[keep:]), rec.FetchList(to, "content")...)
	from["content"] = slices.Clone(content[:keep])
}

// overlayFill: a rule painted under what follows (margin_right = -width, no left margin), a
// background: a coloured fill, or a bar at least as wide as it is tall. A rule in the text colour
// that is taller than wide is a stroke kerned back over, the edge of a drawing (chessboard's frame
// at the end of a beamer column, slides/0013 and 0015; decision 144): it stays where TeX set it and
// no list item or listing after it opens with it.
func overlayFill(span rec.Map) bool {
	if span["type"] != "rule" {
		return false
	}
	left, _ := rec.Number(span["margin_left_sp"])
	right, hasRight := rec.Number(span["margin_right_sp"])
	width, _ := rec.Number(span["width_sp"])
	if !hasRight || left != 0 || width <= 0 || right != -width {
		return false
	}
	height, _ := rec.Number(span["height_sp"])
	depth, _ := rec.Number(span["depth_sp"])
	return truthy(span["color"]) || width >= height+depth
}

func (e *Evidence) newBlock(kind string, owner rec.Map, page any) rec.Map {
	if kind == "unordered_list" || kind == "ordered_list" || kind == "list_item" || kind == "code" {
		kind = "paragraph"
	}
	block := rec.Map{"id": fmt.Sprintf("b%d", len(e.blocks)+1), "type": kind, "owner": rec.Fetch(owner, "owner"),
		"entity": rec.Fetch(owner, "entity"), "pages": []any{page}, "content": []any{}}
	e.blocks = append(e.blocks, block)
	e.lastBlocks[e.readingPartition(owner)] = block
	return block
}

func lastSpan(block rec.Map) rec.Map {
	items := rec.FetchList(block, "content")
	if len(items) == 0 {
		return nil
	}
	return rec.AsMap(items[len(items)-1])
}

// styleValue is a style as a JSON value: no style is nil.
func styleValue(style rec.Map) any {
	if style == nil {
		return nil
	}
	return style
}

func (e *Evidence) appendNoteMarker(block, item rec.Map) {
	if e.notes == nil {
		rec.Fail("Footnote marker without notes extension")
	}
	if truthy(item["note_mark"]) && truthy(item["note_number"]) {
		rec.Fail("Conflicting footnote marker roles")
	}
	field, kind, source := "note", "note_number", "note_number"
	if truthy(item["note_mark"]) {
		field, kind, source = "mark", "note_reference", "note_mark"
	}
	id := rec.Fetch(item, source)
	last := lastSpan(block)
	text := rec.FetchString(item, "space") + rec.FetchString(item, "text")
	style := styleValue(e.textStyles.ItemStyle(item, "Undeclared text font"))
	if last != nil && rec.Fetch(last, "type") == kind && rec.Equal(rec.Fetch(last, field), id) && rec.Equal(last["style"], style) &&
		e.sameTableOrigin(last) {
		last["text"] = rec.FetchString(last, "text") + text
		return
	}
	span := rec.Map{"type": kind, field: id, "text": text}
	if style != nil {
		span["style"] = style
	}
	push(block, e.rememberTableSpan(span))
}
