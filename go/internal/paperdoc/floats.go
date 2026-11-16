package paperdoc

import (
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// floatTypes is PaperFloats::TYPES.
var floatTypes = []string{"figure_float", "table_float", "float_caption"}

// floatBodyTypes are the blocks a float body holds: paragraphs, and displays (a TikZ picture
// set in \[ \] inside a subfigure; third cohort 817), and the lists and theorems set in it (an
// algorithm's steps; the Geometric Approximation Algorithms book).
var floatBodyTypes = []any{"paragraph", "display_math", "ordered_list", "unordered_list", "theorem", "proof", "code"}

var captionSources = []any{"caption-package-public-hooks", "latex-native-caption-hooks-1"}

// floats is PaperFloats: observed float members grouped without moving or rewriting retained
// blocks.
type floats struct {
	b        *builder
	blocks   []rec.Map
	byEntity map[any][]rec.Map
	frames   []rec.Map
	captions []rec.Map
	// displays are the display environments set inside a float (their equation_context names it).
	displays []rec.Map
	// members are the lists and theorems set in a float body (their float_member names it).
	members []rec.Map
}

func projectFloats(inspection rec.Map, b *builder) {
	f := &floats{b: b, blocks: b.allBlocks(), byEntity: map[any][]rec.Map{}}
	for _, block := range f.blocks {
		entity := rec.Fetch(block, "entity")
		f.byEntity[entity] = append(f.byEntity[entity], block)
	}
	for _, value := range rec.FetchList(inspection, "diagnostics") {
		row := rec.AsMap(value)
		switch row["code"] {
		case "float_context":
			f.frames = append(f.frames, row)
		case "caption_context":
			f.captions = append(f.captions, row)
		case "float_member":
			f.members = append(f.members, row)
		case "equation_context":
			if number, _ := row["float"].(int64); number > 0 {
				f.displays = append(f.displays, row)
			}
		}
	}
	f.project()
}

func (f *floats) project() {
	frames := map[any]rec.Map{}
	for _, row := range f.frames {
		frames[rec.Fetch(row, "entity")] = row
	}
	if len(frames) != len(f.frames) {
		rec.Fail("Duplicate native float identity")
	}
	for _, caption := range f.captions {
		if len(f.byEntity[rec.Fetch(caption, "entity")]) == 0 || rec.Equal(rec.Fetch(caption, "float"), int64(0)) {
			continue
		}
		if _, ok := frames[rec.Fetch(caption, "float")]; !ok {
			rec.Fail("Retained caption without native float")
		}
	}
	for _, frame := range f.frames {
		f.projectFrame(frame)
	}
	validateFloats(f.b.nodes, f.b.order)
}

func (f *floats) projectFrame(frame rec.Map) {
	if frame["source"] != "latex-float-environment-hooks" {
		rec.Fail("Unsupported native float source")
	}
	captions := []rec.Map{}
	for _, row := range f.captions {
		if rec.Equal(rec.Fetch(row, "float"), rec.Fetch(frame, "entity")) {
			validateCaption(frame, row)
			captions = append(captions, row)
		}
	}
	// Subfigure captions (subfig, subcaption) come first; the float's own caption is the last
	// one. The others stay in the body as ordinary paragraphs.
	var main rec.Map
	for index := len(captions) - 1; index >= 0; index-- {
		if _, ok := f.byEntity[rec.Fetch(captions[index], "entity")]; ok {
			main = captions[index]
			break
		}
	}
	captionBlocks := []rec.Map{}
	if main != nil {
		captionBlocks = f.byEntity[rec.Fetch(main, "entity")]
	}
	named := f.namedCaptions(captions)
	body := f.byEntity[rec.Fetch(frame, "entity")]
	if len(captionBlocks) == 0 && len(body) == 0 {
		return
	}
	// A float whose body the reading did not retain (a PostScript figure, a display array in a
	// table; second cohort 297, 317) keeps its caption and says what it lost.
	structures := f.memberStructures(frame)
	if !slices.ContainsFunc(body, retained) && len(structures) == 0 {
		f.b.diagnostics.Add("float.body_missing", "a float keeps its caption but its body was not retained", nil)
	}
	entities := f.memberEntities(frame, captions)
	children := []any{}
	seen := map[int64]bool{}
	for _, block := range f.blocks {
		if includes(entities, rec.Fetch(block, "entity")) {
			children = append(children, f.b.uid("block", rec.Fetch(block, "id")))
		} else if member, ok := structures[toS(rec.Fetch(block, "id"))]; ok && !seen[member] {
			seen[member] = true
			children = append(children, f.b.uidOf("main", member))
		}
	}
	parent, first := f.memberParent(children)
	if parent == nil {
		// A frame with allowframebreaks splits its content across pages anywhere, a figure too: its
		// image ends one page and its caption opens the next. The pieces stay where they were set.
		f.b.diagnostics.Add("float.split", "a float broken across pages is kept as its parts, its caption not joined to its body", nil)
		return
	}
	grouped := f.captioned(children, named, main, captionBlocks)
	id := f.b.uid("float", rec.Fetch(frame, "entity"))
	// Other float types (algorithm, listing) are figures with their own caption counter.
	kind := "figure_float"
	if frame["category"] == "table" {
		kind = "table_float"
	}
	f.addNode(id, kind, grouped, "float_context:"+toS(rec.Fetch(frame, "seq")))
	own := rec.FetchList(parent, "children")
	parent["children"] = append(append(slices.Clone(own[:first]), id), own[first+len(children):]...)
}

// captioned is the float's members with its caption blocks grouped: each named caption of a
// float with several, else its own (main) caption.
func (f *floats) captioned(children []any, named []rec.Map, main rec.Map, captionBlocks []rec.Map) []any {
	grouped := slices.Clone(children)
	if len(named) > 1 {
		for _, caption := range named {
			grouped = f.addCaption(grouped, caption, f.byEntity[rec.Fetch(caption, "entity")])
		}
	} else if len(captionBlocks) > 0 {
		grouped = f.addCaption(grouped, main, captionBlocks)
	}
	return grouped
}

// memberStructures maps each block of a list or theorem set in the float's body (float_member)
// to that member's structure entity: the member joins the float whole.
func (f *floats) memberStructures(frame rec.Map) map[string]int64 {
	result := map[string]int64{}
	if len(f.members) == 0 {
		return result
	}
	groups := map[int64]rec.Map{}
	for _, value := range rec.FetchList(rec.FetchMap(f.b.reading, "structure"), "nodes") {
		node := rec.AsMap(value)
		groups[rec.FetchInt(node, "id")] = node
	}
	var collect func(member, id int64)
	collect = func(member, id int64) {
		node, ok := groups[id]
		if !ok {
			return
		}
		for _, value := range rec.FetchList(node, "content") {
			item := rec.AsMap(value)
			if block, ok := item["block"]; ok {
				result[toS(block)] = member
			} else {
				collect(member, rec.FetchInt(item, "structure"))
			}
		}
	}
	for _, row := range f.members {
		if rec.Equal(rec.Fetch(row, "float"), rec.Fetch(frame, "entity")) {
			collect(rec.FetchInt(row, "entity"), rec.FetchInt(row, "entity"))
		}
	}
	return result
}

// memberEntities are the float's own entity, its captions' and the display environments set
// inside it (amsldoc's table of display examples).
func (f *floats) memberEntities(frame rec.Map, captions []rec.Map) []any {
	entities := []any{rec.Fetch(frame, "entity")}
	for _, row := range captions {
		entities = append(entities, rec.Fetch(row, "entity"))
	}
	for _, row := range f.displays {
		if rec.Equal(rec.Fetch(row, "float"), rec.Fetch(frame, "entity")) {
			entities = append(entities, rec.Fetch(row, "entity"))
		}
	}
	return entities
}

// captionOpening is how a float's own caption begins: its kind and number ("Figure 2:",
// "Fig. 3.", "TABLE IV"). A subfigure's caption begins with its letter instead.
var captionOpening = regexp.MustCompile(`\A\p{L}+\.?\s*(?:[0-9]+|[IVXLCivxlc]+)\b`)

// namedCaptions are the numbered captions of a float that read as its own: a float with several
// of them (a second \caption) keeps every one as a caption of its own; a subfigure's caption
// stays a body paragraph.
func (f *floats) namedCaptions(captions []rec.Map) []rec.Map {
	named := []rec.Map{}
	for _, caption := range captions {
		blocks := f.byEntity[rec.Fetch(caption, "entity")]
		if len(blocks) > 0 && caption["numbered"] == true && captionOpening.MatchString(strings.TrimSpace(blockText(blocks[0]))) {
			named = append(named, caption)
		}
	}
	return named
}

func blockText(block rec.Map) string {
	var text strings.Builder
	for _, value := range rec.FetchList(block, "content") {
		if span, ok := value.(rec.Map); ok {
			text.WriteString(toS(span["text"]))
		}
	}
	return text.String()
}

func validateCaption(frame, caption rec.Map) {
	if frame["source"] != "latex-float-environment-hooks" || !slices.Contains(captionSources, caption["source"]) ||
		!rec.Equal(rec.Fetch(frame, "parent"), rec.Fetch(caption, "parent")) ||
		!less(rec.Fetch(frame, "seq"), rec.Fetch(caption, "seq")) {
		rec.Fail("Inconsistent native float/caption association")
	}
}

func less(left, right any) bool {
	a, _ := rec.Number(left)
	b, _ := rec.Number(right)
	return a < b
}

func containsAll(list []any, values []any) bool {
	for _, value := range values {
		if !slices.Contains(list, value) {
			return false
		}
	}
	return true
}

// memberParent is the one structure holding all the float's members, and the index of the first;
// nil when no structure holds them all (a float split across pages).
func (f *floats) memberParent(children []any) (rec.Map, int) {
	parents := []rec.Map{}
	for _, id := range f.b.order {
		node := rec.AsMap(f.b.nodes[id])
		if containsAll(rec.FetchList(node, "children"), children) {
			parents = append(parents, node)
		}
	}
	if len(parents) == 0 {
		return nil, 0
	}
	if len(parents) != 1 {
		rec.Fail("Float members have different structural parents")
	}
	parent := parents[0]
	own := rec.FetchList(parent, "children")
	first := slices.Index(own, children[0])
	if first+len(children) > len(own) || !slices.Equal(own[first:first+len(children)], children) {
		rec.Fail("Nonconsecutive native float blocks")
	}
	return parent, first
}

func (f *floats) addCaption(children []any, caption rec.Map, blocks []rec.Map) []any {
	members := []any{}
	for _, block := range blocks {
		members = append(members, f.b.uid("block", rec.Fetch(block, "id")))
	}
	first := slices.Index(children, members[0])
	if first < 0 || first+len(members) > len(children) || !slices.Equal(children[first:first+len(members)], members) {
		rec.Fail("Nonconsecutive native caption blocks")
	}
	id := f.b.uid("float_caption", rec.Fetch(caption, "entity"))
	f.addNode(id, "float_caption", members, "caption_context:"+toS(rec.Fetch(caption, "seq")))
	return append(append(slices.Clone(children[:first]), id), children[first+len(members):]...)
}

func (f *floats) addNode(id, kind string, children []any, source string) {
	if _, ok := f.b.nodes[id]; ok {
		rec.Fail("Duplicate paper float node")
	}
	f.b.nodes[id] = rec.Map{"id": id, "type": kind, "children": children,
		"provenance": rec.Map{"inspection_key": source, "source_hint": "unavailable"}}
	f.b.order = append(f.b.order, id)
}

// validateFloats is PaperFloats.validate!; order is the nodes' insertion order.
func validateFloats(nodes rec.Map, order []string) {
	for _, id := range order {
		node := rec.AsMap(nodes[id])
		kind := rec.FetchString(node, "type")
		if !slices.Contains(floatTypes, kind) {
			continue
		}
		children := childNodes(nodes, node)
		if kind == "float_caption" {
			validateFloatCaption(nodes, order, node, children)
			continue
		}
		captions, body := 0, []rec.Map{}
		for _, child := range children {
			if child["type"] == "float_caption" {
				captions++
			} else {
				body = append(body, child)
			}
		}
		paragraphs := true
		for _, child := range body {
			paragraphs = paragraphs && slices.Contains(floatBodyTypes, child["type"])
		}
		if !paragraphs {
			rec.Fail("Invalid float body or caption ownership")
		}
		// A float's category is the environment its author chose, not what it holds: it only
		// needs content of its own.
		if slices.ContainsFunc(body, retained) && !hasContent(body) {
			rec.Fail("Float category differs from body content")
		}
	}
}

// retained reports whether a body block holds anything: a paragraph left empty (its only content
// a picture the reading dropped) is a body not retained, so the float keeps its caption and says
// so (float.body_missing) instead of failing as a float of another category.
func retained(block rec.Map) bool {
	content, _ := block["content"].([]any)
	children, _ := block["children"].([]any)
	return block["type"] != "paragraph" || len(content) > 0 || len(children) > 0
}

// A structure member (a theorem or list set in a float: book 0003's ntheorem algorithms) has
// children and no content of its own.
func hasContent(body []rec.Map) bool {
	for _, child := range body {
		content, _ := child["content"].([]any)
		if len(content) > 0 || len(rec.FetchList(child, "children")) > 0 && !slices.Contains([]any{"paragraph", "display_math"}, child["type"]) {
			return true
		}
	}
	return false
}

func childNodes(nodes rec.Map, node rec.Map) []rec.Map {
	result := []rec.Map{}
	for _, id := range rec.FetchList(node, "children") {
		child, ok := nodes[rec.AsString(id)].(rec.Map)
		if !ok {
			panic(rec.Failure{Message: "key not found: " + toS(id), Lookup: true})
		}
		result = append(result, child)
	}
	return result
}

func validateFloatCaption(nodes rec.Map, order []string, node rec.Map, children []rec.Map) {
	valid := len(children) > 0
	for _, child := range children {
		valid = valid && child["type"] == "paragraph"
	}
	if !valid {
		rec.Fail("Invalid float caption content")
	}
	parentTypes := []any{}
	for _, id := range order {
		parent := rec.AsMap(nodes[id])
		if slices.Contains(rec.FetchList(parent, "children"), any(node["id"])) {
			parentTypes = append(parentTypes, parent["type"])
		}
	}
	if len(parentTypes) != 1 || (parentTypes[0] != "figure_float" && parentTypes[0] != "table_float") {
		rec.Fail("Caption has no float parent")
	}
}
