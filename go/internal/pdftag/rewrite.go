package pdftag

import (
	"bytes"
	"fmt"

	"github.com/sarielhp/relatex/internal/pdffonts"
)

// The rewritten content stream: every graphics object inside a marked-content sequence, either
// a structure leaf's (with an MCID) or an Artifact's (decision 105 item 2).

const artifactKey = "\x00artifact"

// pageTags says, for one page, which leaf each painted thing belongs to.
type pageTags struct {
	number    int
	leafOf    func(ordinal int) string
	spans     []span         // formulas and labels: paths painted inside them are theirs
	objects   map[int]string // item index → leaf, for images and forms
	roleOf    func(leaf string) string
	moved     map[string]map[int]float64          // width changes to undo, by font (internal/pdffonts Moved)
	drawn     map[string]bool                     // drawing fonts, by reference (internal/pdffonts Drawn)
	owns      func(leaf string) bool              // whether the paths among a leaf's glyphs are its own
	pictures  []picture                           // drawn pictures: what is painted in their boxes is theirs
	spelled   func(ordinal int) string            // TeX's character of a glyph the font spells otherwise, "" for none
	stretched map[string]map[int]pdffonts.Stretch // codes to scale to a viewer's stretch, by font
}

// drawingText is the marked content around a drawing font's glyphs: they read as no text (their
// ToUnicode value is U+FFFD, which PDF/UA-2 needs some value for), as a path does.
const drawingText = "/Span <</ActualText ()>> BDC\n"

// span is a leaf's glyph range on a page.
type span struct {
	leaf        string
	first, last int
}

// emitter writes the new content stream.
type emitter struct {
	out     bytes.Buffer
	open    string
	depth   int // q nesting
	opened  int // the q nesting the open sequence began at
	mcid    int
	marks   []mark
	pending []byte
	tags    *pageTags
	items   []item
	fixed   bool   // a clip-mode text object is written inside one sequence begun before its BT
	source  []bool // the source's open marked-content sequences: whether each is optional content
}

// mark records an MCID's leaf.
type mark struct {
	leaf  string
	mcid  int
	first int
}

func (e *emitter) close() {
	if e.open != "" {
		e.out.WriteString("EMC\n")
		e.open = ""
	}
}

// begin opens the marked-content sequence of key unless it is open.
func (e *emitter) begin(key string, first int) {
	if key == e.open || e.fixed {
		return
	}
	e.close()
	e.open, e.opened = key, e.depth
	if key == artifactKey {
		e.out.WriteString("/Artifact BMC\n")
		return
	}
	fmt.Fprintf(&e.out, "/%s <</MCID %d>> BDC\n", e.tags.roleOf(key), e.mcid)
	e.marks = append(e.marks, mark{leaf: key, mcid: e.mcid, first: first})
	e.mcid++
}

func (e *emitter) write(raw []byte) {
	e.out.Write(raw)
	e.out.WriteByte('\n')
}

// keyOf is a glyph's marked-content key. A drawing font's glyph is drawing, as a path is: the
// formula's, label's or figure's it is bound to or painted within, else an Artifact.
func (t *pageTags) keyOf(font string, ordinal int) string {
	leaf := t.leafOf(ordinal)
	if t.drawn[font] && (leaf == "" || !t.owns(leaf)) {
		return t.pathKey(ordinal)
	}
	if leaf != "" {
		return leaf
	}
	return artifactKey
}

// writeShow writes a text-showing operator, a drawing font's inside its drawingText.
func (e *emitter) writeShow(font string, raw []byte) {
	if !e.tags.drawn[font] {
		e.write(raw)
		return
	}
	e.out.WriteString(drawingText)
	e.write(raw)
	e.out.WriteString("EMC\n")
}

// pathKey is the key of a path or shading painted after painted glyphs.
func (t *pageTags) pathKey(painted int) string {
	for _, s := range t.spans {
		if s.first < painted && painted <= s.last {
			return s.leaf
		}
	}
	return artifactKey
}

// paintKey is the key of a path or object: the drawn picture it is painted in, else as pathKey says.
func (t *pageTags) paintKey(entry item) string {
	if entry.boxed {
		if leaf := pictureOf(t.pictures, entry.box, pictureMargin); leaf != "" {
			return leaf
		}
	}
	return t.pathKey(entry.painted)
}

// rewrite writes a page's items with marked content.
func rewrite(items []item, tags *pageTags) ([]byte, []mark) {
	e := &emitter{tags: tags, items: items}
	for index := 0; index < len(items); index++ {
		if items[index].kind == kindTextBegin {
			if last, ok := e.clipObjects(index); ok {
				index = last
				continue
			}
		}
		e.item(index, items[index])
	}
	if len(e.pending) > 0 {
		e.begin(artifactKey, 0)
		e.write(e.pending)
	}
	e.close()
	return e.out.Bytes(), e.marks
}

func (e *emitter) item(index int, entry item) {
	if len(e.pending) > 0 && entry.kind != kindPaint && entry.kind != kindMarked {
		e.pending = append(append(e.pending, entry.op.raw...), '\n')
		return
	}
	switch entry.kind {
	case kindMarked:
		e.marked(entry.op)
	case kindPathPart:
		e.pending = append(append(e.pending, entry.op.raw...), '\n')
	case kindPaint:
		e.begin(e.tags.paintKey(entry), entry.painted)
		e.out.Write(e.pending)
		e.write(entry.op.raw)
		e.pending = nil
	case kindTextBegin:
		e.textBegin(index, entry)
	case kindTextEnd:
		if !e.fixed {
			e.close()
		}
		e.fixed = false
		e.write(entry.op.raw)
	case kindObject:
		e.object(index, entry)
	case kindShow:
		e.show(entry)
	default:
		e.state(entry.op)
	}
}

// state writes a state operator; a sequence begun inside a q ends before its Q, so marked
// content and saved graphics states nest.
func (e *emitter) state(op operation) {
	switch op.operator {
	case "q":
		e.depth++
	case "Q":
		if e.open != "" && e.opened >= e.depth {
			e.close()
		}
		e.depth--
	}
	e.write(op.raw)
}

// object marks a Do, sh or inline image.
func (e *emitter) object(index int, entry item) {
	key, ok := e.tags.objects[index]
	if !ok {
		key = e.tags.paintKey(entry)
	}
	if key == "" {
		key = artifactKey
	}
	e.begin(key, entry.first)
	e.write(entry.op.raw)
}

// show marks a text-showing operator, split where its glyphs change leaf.
func (e *emitter) show(entry item) {
	if entry.count == 0 {
		e.write(entry.op.raw)
		return
	}
	if e.uniform(entry) && !movesGlyphs(entry, e.tags.moved[entry.font]) && !e.anyLone(entry) {
		e.begin(e.tags.keyOf(entry.font, entry.first), entry.first)
		e.writeShow(entry.font, entry.op.raw)
		return
	}
	e.split(entry)
}

func (e *emitter) uniform(entry item) bool {
	key := e.tags.keyOf(entry.font, entry.first)
	for ordinal := entry.first + 1; ordinal < entry.first+entry.count; ordinal++ {
		if e.tags.keyOf(entry.font, ordinal) != key {
			return false
		}
	}
	return true
}

// split writes the operator as TJ operators, one per run of glyphs of one leaf; the kerns stay
// where they were, and a glyph whose width changed is followed by the kern that undoes the change,
// so every glyph lands where it did.
func (e *emitter) split(entry item) {
	writeLineMove(&e.out, entry.op)
	var group bytes.Buffer
	key, first := "", entry.first
	flush := func() {
		if group.Len() > 0 {
			e.begin(key, first)
			e.writeShow(entry.font, fmt.Appendf(nil, "[%s] TJ", bytes.TrimSpace(group.Bytes())))
			group.Reset()
		}
	}
	ordinal := entry.first
	moved := e.tags.moved[entry.font]
	for _, part := range entry.elements {
		if part.codes == nil {
			group.Write(part.raw)
			group.WriteByte(' ')
			continue
		}
		var run []byte
		for _, code := range part.codes {
			if next := e.tags.keyOf(entry.font, ordinal); next != key {
				if len(run) > 0 {
					group.WriteString(hexString(run) + " ")
					run = nil
				}
				flush()
				key, first = next, ordinal
			}
			if glyph, ok := e.tags.lone(entry, ordinal, code, key); ok {
				group.Write(flushRun(&run))
				flush()
				e.begin(key, first)
				writeLone(&e.out, entry, code, glyph, moved, func(show []byte) { e.writeShow(entry.font, show) })
				ordinal++
				continue
			}
			run = append(run, code...)
			ordinal++
			if kern := compensation(moved, code); kern != "" {
				group.Write(flushRun(&run))
				group.WriteString(kern)
			}
		}
		if len(run) > 0 {
			group.WriteString(hexString(run) + " ")
		}
	}
	flush()
}

// writeLineMove writes what ' and " do before showing their string.
func writeLineMove(out *bytes.Buffer, op operation) {
	switch op.operator {
	case "'":
		out.WriteString("T*\n")
	case "\"":
		if len(op.operands) == 3 {
			fmt.Fprintf(out, "%s Tw %s Tc T*\n", op.operands[0].raw, op.operands[1].raw)
		}
	}
}

// marked keeps the source's optional-content layers (/OC BDC ... EMC) around what they hold, as the
// source has them, so the tagged page looks as the ordinary one (decision 121, amendment 2); every
// other source marked-content operator is dropped. Our own sequence nests inside a layer: it ends
// before the layer begins and before the layer ends.
func (e *emitter) marked(op operation) {
	switch op.operator {
	case "BMC":
		e.source = append(e.source, false)
	case "BDC":
		optional := len(op.operands) > 0 && string(op.operands[0].raw) == "/OC"
		e.source = append(e.source, optional)
		if optional {
			e.close()
			e.write(op.raw)
		}
	case "EMC":
		if len(e.source) == 0 {
			return
		}
		optional := e.source[len(e.source)-1]
		e.source = e.source[:len(e.source)-1]
		if optional {
			e.close()
			e.write([]byte("EMC"))
		}
	}
}

// textBegin writes a BT. A text object that shows in a clip rendering mode (Tr 4 to 7) is written
// whole inside one sequence begun before its BT: MuPDF renders marked content inside such a text
// object as an empty clip, which hides the text (decision 121, amendment 2). The sequence is the
// object's one leaf's, an Artifact when it has none; with glyphs of several leaves the text object
// keeps its sequences inside.
func (e *emitter) textBegin(index int, entry item) {
	e.close()
	if key, first, ok := e.clipKey(index); ok {
		e.begin(key, first)
		e.fixed = true
	}
	e.write(entry.op.raw)
}

// clipKey is the key of the text object that begins at index, when it shows in a clip mode.
func (e *emitter) clipKey(index int) (string, int, bool) {
	key, first, clip := artifactKey, 0, false
	for _, entry := range e.items[index+1:] {
		if entry.kind == kindTextEnd {
			break
		}
		if entry.kind != kindShow || entry.count == 0 {
			continue
		}
		clip = clip || entry.clip
		for ordinal := entry.first; ordinal < entry.first+entry.count; ordinal++ {
			next := e.tags.keyOf(entry.font, ordinal)
			if next == artifactKey || next == key {
				continue
			}
			if key != artifactKey {
				return "", 0, false
			}
			key, first = next, ordinal
		}
	}
	return key, first, clip
}
