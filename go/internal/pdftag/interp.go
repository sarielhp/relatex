package pdftag

import (
	"encoding/json"
	"fmt"
	"github.com/sarielhp/relatex/internal/affine"
	"math"
	"strings"
)

// itemKind is what an operation does, as far as marking content is concerned.
type itemKind int

const (
	kindState     itemKind = iota // changes state only; written where it stands
	kindTextBegin                 // BT
	kindTextEnd                   // ET
	kindShow                      // a text-showing operator
	kindPathPart                  // path construction or clipping, part of the next painting operator's object
	kindPaint                     // a path-painting operator ending a path object
	kindObject                    // Do, sh or an inline image: a whole graphics object
	kindMarked                    // the source's own marked-content operator, dropped
)

var pathParts = map[string]bool{"m": true, "l": true, "c": true, "v": true, "y": true, "h": true, "re": true,
	"W": true, "W*": true}

var pathPaints = map[string]bool{"S": true, "s": true, "f": true, "F": true, "f*": true, "B": true, "B*": true,
	"b": true, "b*": true, "n": true}

var markedOperators = map[string]bool{"BMC": true, "BDC": true, "EMC": true, "MP": true, "DP": true}

// element is one element of a show operator's strings: a string's glyph codes, or a kern.
type element struct {
	codes [][]byte // one entry per glyph; nil for a kern
	raw   []byte   // the kern's number
}

// item is an operation of a page's content with what the tagger needs to know of it.
type item struct {
	op       operation
	kind     itemKind
	elements []element // show operators: their strings and kerns
	first    int       // ordinal of the first glyph it paints (show, or a form XObject's)
	count    int       // glyphs it paints
	origin   [2]float64
	placed   bool       // origin is known
	xobject  string     // Do: Image or Form
	ref      string     // Do: the object's reference
	size     [2]float64 // Do: the object's width and height on the page
	painted  int        // glyphs painted on the page before this item
	mcid     int        // the MCID of the innermost marked content around it, -1 without one
	// actual is the innermost open /ActualText around it (actualtext.go); its sequence 0 without one.
	actual actualText
	text   textState // show operators: the text state they show with
	font   string    // show operators: the font's object reference, "" for a direct dictionary
	box    bounds    // paints and objects: the device box they paint in (pictures.go)
	boxed  bool      // box is known
	clip   bool      // show operators: the text rendering mode adds to the clipping path (Tr 4 to 7)
}

// matrix is a PDF transformation [a b c d e f].
type matrix [6]float64

var identity = matrix{1, 0, 0, 1, 0, 0}

// times is m × n (m applied first).
func (m matrix) times(n matrix) matrix {
	return matrix(affine.Matrix(m).Then(affine.Matrix(n)))
}

// graphicsState is the part of the graphics state the interpreter tracks.
type graphicsState struct {
	ctm                  matrix
	font                 string
	size, leading, rise  float64
	scale                float64
	charSpace, wordSpace float64 // Tc and Tw
	render               int     // the text rendering mode (Tr)
	clip                 bounds  // the clipping paths' device box, when clipped
	clipped              bool
}

// interpreter walks a content stream as the viewer would, counting glyphs.
type interpreter struct {
	file      *pdfFile
	state     graphicsState
	stack     []graphicsState
	tm, tlm   matrix
	tmKnown   bool
	glyphs    int
	resources map[string]any
	depth     int
	marked    []int                    // the open marked-content sequences' MCIDs (-1 without one)
	actuals   []actualText             // their /ActualText, sequence 0 for none
	sequences int                      // the /ActualText sequences opened so far
	codes     map[string]map[int]bool  // the one-byte codes shown, by font reference
	advanced  map[string]map[int]bool  // those whose advance places a later glyph, by font reference
	pending   glyphCode                // the last glyph shown, until the text position is set anew
	trace     tracePaints              // each glyph's paint index in MuPDF's trace (paints.go)
	strokes   int                      // stroke paints the trace adds when the shown text is flushed
	buffered  int                      // the rendering mode of the text not flushed yet
	path      bounds                   // the device box of the path being built
	clipping  bool                     // the path being built is a clip (W, W*)
	forms     map[formKey]*interpreter // the form XObjects interpreted, by reference, CTM and rendering mode (formGlyphs)
}

// glyphCode is one simple font's code; font "" for none.
type glyphCode struct {
	font string
	code int
}

func newInterpreter(file *pdfFile, resources map[string]any) *interpreter {
	return &interpreter{file: file, resources: resources, state: graphicsState{ctm: identity, scale: 1},
		codes: map[string]map[int]bool{}, advanced: map[string]map[int]bool{}, path: emptyBounds,
		forms: map[formKey]*interpreter{}}
}

// use records the codes a font shows, and which of a simple font's codes a later glyph is placed
// after: a viewer advances by the font's Widths entry, so those widths must not change (8.4.5.6).
// A composite font's two-byte codes are recorded for its ToUnicode map, not for its widths.
func (in *interpreter) use(font string, codes [][]byte) {
	if font != "" && in.codes[font] == nil {
		in.codes[font] = map[int]bool{}
	}
	for _, code := range codes {
		if in.pending.font != "" {
			addCode(in.advanced, in.pending.font, in.pending.code)
		}
		in.pending = glyphCode{}
		switch {
		case font == "":
		case len(code) == 2:
			addCode(in.codes, font, int(code[0])<<8|int(code[1]))
		case len(code) == 1:
			addCode(in.codes, font, int(code[0]))
			in.pending = glyphCode{font: font, code: int(code[0])}
		}
	}
}

// addCode adds a font's code to a set by font.
func addCode(set map[string]map[int]bool, font string, code int) {
	if set[font] == nil {
		set[font] = map[int]bool{}
	}
	set[font][code] = true
}

// run interprets a page's operations into items.
func (in *interpreter) run(operations []operation) ([]item, error) {
	items := make([]item, 0, len(operations))
	for _, op := range operations {
		entry := item{op: op, painted: in.glyphs, mcid: in.currentMCID(), actual: in.currentActual()}
		if err := in.step(&entry); err != nil {
			return nil, fmt.Errorf("%s: %w", op.operator, err)
		}
		items = append(items, entry)
	}
	in.flush()
	return items, nil
}

func (in *interpreter) step(entry *item) error {
	op := entry.op
	switch {
	case markedOperators[op.operator]:
		entry.kind = kindMarked
		in.mark(op)
	case pathParts[op.operator]:
		entry.kind = kindPathPart
		in.extendPath(op)
	case pathPaints[op.operator]:
		entry.kind = kindPaint
		entry.box, entry.boxed = in.endPath()
	case op.operator == "BT":
		entry.kind, in.tm, in.tlm, in.tmKnown = kindTextBegin, identity, identity, true
		in.pending = glyphCode{}
	case op.operator == "ET":
		entry.kind = kindTextEnd
		in.flush()
	case op.operator == "Do":
		return in.xobject(entry)
	case op.operator == "sh" || op.operator == "BI":
		entry.kind = kindObject
		in.objectBox(entry, nil)
	case op.operator == "Tj" || op.operator == "TJ" || op.operator == "'" || op.operator == "\"":
		return in.show(entry)
	default:
		return in.state1(op)
	}
	return nil
}

// numbers reads an operation's numeric operands.
func numbers(op operation, want int) ([]float64, error) {
	if len(op.operands) < want {
		return nil, fmt.Errorf("needs %d operands", want)
	}
	values := make([]float64, want)
	for i := range want {
		value, err := number(op.operands[len(op.operands)-want+i])
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	return values, nil
}

// state1 applies a state operator.
func (in *interpreter) state1(op operation) error {
	switch op.operator {
	case "q":
		in.stack = append(in.stack, in.state)
	case "Q":
		if len(in.stack) > 0 {
			in.state, in.stack = in.stack[len(in.stack)-1], in.stack[:len(in.stack)-1]
		}
	case "cm":
		values, err := numbers(op, 6)
		if err != nil {
			return err
		}
		in.state.ctm = matrix(values).times(in.state.ctm)
	case "Tf":
		values, err := numbers(op, 1)
		if err != nil || len(op.operands) < 2 {
			return fmt.Errorf("bad Tf")
		}
		in.state.font, in.state.size = strings.TrimPrefix(string(op.operands[0].raw), "/"), values[0]
	default:
		return in.textState(op)
	}
	return nil
}

// textState applies text positioning and text state operators.
func (in *interpreter) textState(op operation) error {
	switch op.operator {
	case "Tm":
		values, err := numbers(op, 6)
		if err != nil {
			return err
		}
		in.tm, in.tlm, in.tmKnown = matrix(values), matrix(values), true
		in.pending = glyphCode{}
	case "Td", "TD":
		values, err := numbers(op, 2)
		if err != nil {
			return err
		}
		if op.operator == "TD" {
			in.state.leading = -values[1]
		}
		in.newLine(values[0], values[1])
	case "T*":
		in.newLine(0, -in.state.leading)
	case "TL", "Ts", "Tz", "Tr", "Tc", "Tw":
		values, err := numbers(op, 1)
		if err != nil {
			return err
		}
		in.textParameter(op.operator, values[0])
	}
	return nil
}

func (in *interpreter) textParameter(operator string, value float64) {
	switch operator {
	case "TL":
		in.state.leading = value
	case "Ts":
		in.state.rise = value
	case "Tz":
		in.state.scale = value / 100
	case "Tr":
		in.state.render = int(value)
	case "Tc":
		in.state.charSpace = value
	case "Tw":
		in.state.wordSpace = value
	}
}

func (in *interpreter) newLine(tx, ty float64) {
	in.tlm = matrix{1, 0, 0, 1, tx, ty}.times(in.tlm)
	in.tm, in.tmKnown = in.tlm, true
	in.pending = glyphCode{}
}

// codeLength is the bytes per character code of the current font: two for a Type0 font with an
// Identity encoding, one for a simple font.
func (in *interpreter) codeLength() (int, error) {
	font := in.file.dict(in.file.dict(in.resources["/Font"])["/"+in.state.font])
	if font == nil {
		return 0, fmt.Errorf("font %s not in the resources", in.state.font)
	}
	if font["/Subtype"] != "/Type0" {
		return 1, nil
	}
	if encoding, _ := font["/Encoding"].(string); encoding == "/Identity-H" || encoding == "/Identity-V" {
		return 2, nil
	}
	return 0, fmt.Errorf("font %s has a CMap encoding", in.state.font)
}

// show reads a text-showing operator's glyphs.
func (in *interpreter) show(entry *item) error {
	entry.kind = kindShow
	op := entry.op
	if op.operator == "'" || op.operator == "\"" {
		in.newLine(0, -in.state.leading)
	}
	if op.operator == "\"" && len(op.operands) == 3 {
		in.state.wordSpace, _ = number(op.operands[0])
		in.state.charSpace, _ = number(op.operands[1])
	}
	entry.clip = in.state.render >= 4
	entry.text = textState{size: in.state.size, scale: in.state.scale, charSpace: in.state.charSpace, wordSpace: in.state.wordSpace}
	if len(op.operands) == 0 {
		return fmt.Errorf("no operand")
	}
	length, err := in.codeLength()
	entry.font, _ = in.file.dict(in.resources["/Font"])["/"+in.state.font].(string)
	if err != nil {
		return err
	}
	operand := op.operands[len(op.operands)-1]
	parts := []token{operand}
	if op.operator == "TJ" {
		if parts, err = arrayItems(operand); err != nil {
			return err
		}
	}
	entry.first = in.glyphs
	shift := 0.0
	for _, part := range parts {
		next, err := in.element(entry, part, length, &shift)
		if err != nil {
			return err
		}
		entry.elements = append(entry.elements, next)
	}
	in.tmKnown = false
	return nil
}

// element reads one element of a show operator; the first glyph's origin is recorded.
func (in *interpreter) element(entry *item, part token, length int, shift *float64) (element, error) {
	if part.kind == tokNumber {
		value, err := number(part)
		if entry.count == 0 {
			*shift -= float64(value / 1000 * in.state.size * in.state.scale)
		}
		return element{raw: part.raw}, err
	}
	if part.kind != tokString && part.kind != tokHex {
		return element{}, fmt.Errorf("unexpected %s in a text array", part.raw)
	}
	data := stringBytes(part)
	codes := make([][]byte, 0, len(data)/length)
	for i := 0; i+length <= len(data); i += length {
		codes = append(codes, data[i:i+length])
	}
	if entry.count == 0 && len(codes) > 0 && in.tmKnown {
		at := matrix{1, 0, 0, 1, *shift, in.state.rise}.times(in.tm).times(in.state.ctm)
		entry.origin, entry.placed = [2]float64{at[4], at[5]}, true
	}
	in.use(entry.font, codes)
	entry.count += len(codes)
	in.glyphs += len(codes)
	in.paint(len(codes))
	return element{codes: codes}, nil
}

// xobject reads a Do: an image, or a form whose glyphs are counted.
func (in *interpreter) xobject(entry *item) error {
	entry.kind = kindObject
	if len(entry.op.operands) == 0 {
		return fmt.Errorf("no operand")
	}
	name := string(entry.op.operands[0].raw)
	ref, _ := in.file.dict(in.resources["/XObject"])[name].(string)
	dict := in.file.dict(ref)
	if dict == nil {
		return fmt.Errorf("xobject %s not in the resources", name)
	}
	entry.xobject = strings.TrimPrefix(fmt.Sprint(dict["/Subtype"]), "/")
	entry.ref = ref
	entry.first = in.glyphs
	entry.size = in.objectSize(entry.xobject, dict)
	in.objectBox(entry, dict)
	in.flush()
	if entry.xobject != "Form" || in.depth > 12 {
		return nil
	}
	inner, err := in.formGlyphs(ref, dict)
	if err != nil {
		return err
	}
	entry.count = inner.glyphs
	in.glyphs += inner.glyphs
	in.trace.append(inner.trace)
	return nil
}

// formKey identifies a form XObject's interpretation: its reference, the CTM and the text
// rendering mode it is painted with (which decides its trace's paints, paints.go).
type formKey struct {
	ref    string
	ctm    matrix
	render int
}

// formGlyphs interprets a form XObject: the glyphs it paints and their paints.
func (in *interpreter) formGlyphs(ref string, dict map[string]any) (*interpreter, error) {
	key := formKey{ref: ref, ctm: in.state.ctm, render: in.state.render}
	if inner, ok := in.forms[key]; ok {
		return inner, nil
	}
	data, err := in.file.streamData(ref)
	if err != nil {
		return nil, err
	}
	operations, err := parseContent(data)
	if err != nil {
		return nil, err
	}
	resources := in.file.dict(dict["/Resources"])
	if resources == nil {
		resources = in.resources
	}
	inner := newInterpreter(in.file, resources)
	inner.depth = in.depth + 1
	inner.codes, inner.advanced = in.codes, in.advanced
	inner.state.ctm, inner.state.render, inner.buffered = in.state.ctm, in.state.render, in.state.render
	if _, err := inner.run(operations); err != nil {
		return nil, err
	}
	in.forms[key] = inner
	return inner, nil
}

// mark follows the marked-content operators, so a tagged page's items know their MCID.
func (in *interpreter) mark(op operation) {
	switch op.operator {
	case "BMC":
		in.marked = append(in.marked, -1)
		in.actuals = append(in.actuals, in.currentActual())
	case "BDC":
		in.marked = append(in.marked, mcidOf(op))
		in.actuals = append(in.actuals, in.openActual(op))
	case "EMC":
		if len(in.marked) > 0 {
			in.marked = in.marked[:len(in.marked)-1]
			in.actuals = in.actuals[:len(in.actuals)-1]
		}
	}
}

// currentMCID is the innermost open MCID, -1 without one.
func (in *interpreter) currentMCID() int {
	for i := len(in.marked) - 1; i >= 0; i-- {
		if in.marked[i] >= 0 {
			return in.marked[i]
		}
	}
	return -1
}

// mcidOf reads the MCID of a BDC's property dictionary, -1 without one.
func mcidOf(op operation) int {
	if len(op.operands) < 2 || op.operands[1].kind != tokDict {
		return -1
	}
	l := &lexer{data: op.operands[1].raw[2 : len(op.operands[1].raw)-2]}
	previous := ""
	for {
		entry, err := l.next()
		if err != nil {
			return -1
		}
		if previous == "/MCID" {
			value, err := number(entry)
			if err != nil {
				return -1
			}
			return int(value)
		}
		previous = string(entry.raw)
	}
}

// objectSize is an XObject's size on the page: the unit square for an image, the bounding box
// through the form's matrix for a form.
func (in *interpreter) objectSize(subtype string, dict map[string]any) [2]float64 {
	at := in.state.ctm
	width, height := 1.0, 1.0
	if subtype == "Form" {
		box := in.file.array(dict["/BBox"])
		if len(box) == 4 {
			width, height = numberOf(box[2])-numberOf(box[0]), numberOf(box[3])-numberOf(box[1])
		}
		if form := in.file.array(dict["/Matrix"]); len(form) == 6 {
			var m matrix
			for i := range m {
				m[i] = numberOf(form[i])
			}
			at = m.times(at)
		}
	}
	return [2]float64{width * math.Hypot(at[0], at[1]), height * math.Hypot(at[2], at[3])}
}

// numberOf reads a JSON number of qpdf's output.
func numberOf(value any) float64 {
	if text, ok := value.(json.Number); ok {
		number, _ := text.Float64()
		return number
	}
	return 0
}
