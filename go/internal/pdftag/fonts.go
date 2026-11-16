package pdftag

import (
	"bytes"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/glyphnames"
	"github.com/sarielhp/relatex/internal/pdffonts"
)

// The fonts the pages show, made to conform by internal/pdffonts (decisions 104, 105): standard
// fonts of included figures embedded as their URW equivalents (8.4.5.5.1), every code shown given
// a usable ToUnicode value (8.4.5.8, 8.4.3), Widths that disagree with the embedded program
// corrected where no glyph moves (8.4.5.6).

// FontReport says what the font repair did.
type FontReport struct {
	pdffonts.Report
	FormsCompensated int `json:"forms_compensated,omitempty"` // form XObjects whose kerns undo a width change
}

// fontStore is the PDF as pdffonts sees it.
type fontStore struct{ file *pdfFile }

func (s fontStore) Resolve(value any) any                       { return s.file.resolve(value) }
func (s fontStore) StreamData(ref string) ([]byte, error)       { return s.file.streamData(ref) }
func (s fontStore) Set(ref string, value map[string]any)        { s.file.set(ref, value) }
func (s fontStore) Add(value map[string]any) string             { return s.file.add(value) }
func (s fontStore) AddStream(d map[string]any, b []byte) string { return s.file.addStream(d, b) }
func (s fontStore) SetStream(ref string, d map[string]any, b []byte) {
	s.file.setStream(ref, d, b)
}

// repairFonts repairs every font the pages show; the width changes to compensate are kept in
// t.moved.
func (t *tagger) repairFonts(used map[string]pdffonts.Use, table glyphnames.Table) FontReport {
	fixer := pdffonts.NewFixer(fontStore{t.file}, table)
	fixer.Locator = pdffonts.SystemFonts()
	fixer.Prepare(slices.Sorted(maps.Keys(used)))
	for _, ref := range slices.Sorted(maps.Keys(used)) {
		fixer.Fix(ref, used[ref])
	}
	t.moved, t.drawn, t.stretched = fixer.Moved, fixer.Drawn, fixer.Stretched
	return FontReport{Report: fixer.Report, FormsCompensated: t.compensateForms()}
}

// compensateForms rewrites the show operators of every form XObject the pages reach (and of the
// forms in them) that shows a glyph whose width changed, each followed by a kern undoing the
// change, and a glyph a viewer stretched scaled to its stretch (decision 148): the figure's text
// stays where it was and as wide. It returns the number of forms rewritten.
func (t *tagger) compensateForms() int {
	if len(t.moved) == 0 && len(t.stretched) == 0 {
		return 0
	}
	seen, count := map[string]bool{}, 0
	for _, p := range t.file.pages() {
		count += t.compensateFormsIn(p.resources, seen, 0)
	}
	return count
}

func (t *tagger) compensateFormsIn(resources map[string]any, seen map[string]bool, depth int) int {
	xobjects := t.file.dict(resources["/XObject"])
	count := 0
	for _, name := range slices.Sorted(maps.Keys(xobjects)) {
		ref, _ := xobjects[name].(string)
		dict := t.file.dict(ref)
		if ref == "" || seen[ref] || dict["/Subtype"] != "/Form" || depth > 12 {
			continue
		}
		seen[ref] = true
		inner := t.file.dict(dict["/Resources"])
		if inner == nil {
			inner = resources
		}
		count += t.compensateForm(ref, dict, inner)
		count += t.compensateFormsIn(inner, seen, depth+1)
	}
	return count
}

// compensateForm rewrites one form's content when it shows a moved glyph; 1 when it did.
func (t *tagger) compensateForm(ref string, dict, resources map[string]any) int {
	data, err := t.file.streamData(ref)
	if err != nil {
		return 0
	}
	operations, err := parseContent(data)
	if err != nil {
		return 0
	}
	items, err := newInterpreter(t.file, resources).run(operations)
	if err != nil {
		return 0
	}
	var out bytes.Buffer
	changed := false
	for _, entry := range items {
		moved, stretched := t.moved[entry.font], t.stretched[entry.font]
		if entry.kind == kindShow && (movesGlyphs(entry, moved) || stretchesGlyphs(entry, stretched)) {
			writeCompensated(&out, entry, moved, stretched)
			changed = true
			continue
		}
		out.Write(entry.op.raw)
		out.WriteByte('\n')
	}
	if !changed {
		return 0
	}
	t.file.setStream(ref, dict, out.Bytes())
	return 1
}

// movesGlyphs: the operator shows a one-byte code whose width changed.
func movesGlyphs(entry item, moved map[int]float64) bool {
	if len(moved) == 0 {
		return false
	}
	for _, part := range entry.elements {
		for _, code := range part.codes {
			if len(code) == 1 && moved[int(code[0])] != 0 {
				return true
			}
		}
	}
	return false
}

// writeCompensated writes a show operator as TJ operators whose glyphs each are followed by the kern
// that undoes their width change; a glyph a viewer stretched is written alone, scaled (writeLone).
func writeCompensated(out *bytes.Buffer, entry item, moved map[int]float64, stretched map[int]pdffonts.Stretch) {
	writeLineMove(out, entry.op)
	var group bytes.Buffer
	var run []byte
	flush := func() {
		group.Write(flushRun(&run))
		if group.Len() > 0 {
			fmt.Fprintf(out, "[%s] TJ\n", group.Bytes())
			group.Reset()
		}
	}
	for _, part := range entry.elements {
		if part.codes == nil {
			group.Write(flushRun(&run))
			group.Write(part.raw)
			group.WriteByte(' ')
			continue
		}
		for _, code := range part.codes {
			if s, ok := stretchOf(stretched, entry, code); ok {
				flush()
				writeLone(out, entry, code, lone{stretch: s, scaled: true}, moved, func(show []byte) {
					out.Write(show)
					out.WriteByte('\n')
				})
				continue
			}
			run = append(run, code...)
			if kern := compensation(moved, code); kern != "" {
				group.Write(flushRun(&run))
				group.WriteString(kern)
			}
		}
	}
	flush()
}

// flushRun is the codes gathered as a hex string and a space, the run emptied; nothing for none.
func flushRun(run *[]byte) []byte {
	if len(*run) == 0 {
		return nil
	}
	written := []byte(hexString(*run) + " ")
	*run = nil
	return written
}

// compensation is the TJ kern that undoes a code's width change ("1.7 "), "" for none: a TJ number
// n moves the next glyph back by n thousandths of the font size, as much as the width grew.
func compensation(moved map[int]float64, code []byte) string {
	if len(code) != 1 {
		return ""
	}
	change := moved[int(code[0])]
	if change == 0 {
		return ""
	}
	return strconv.FormatFloat(math.Round(change*1000)/1000, 'f', -1, 64) + " "
}

// identityGlyphMaps names /Identity as the CIDToGIDMap of every embedded TrueType CIDFont without
// one (an included figure's, as matplotlib writes them). PDF 1.x reads a missing map as Identity;
// PDF 2.0, and so PDF/UA-2 (8.4.5.3.2), wants it said: the glyphs drawn stay the same.
func (t *tagger) identityGlyphMaps() int {
	completed := 0
	for _, key := range slices.Sorted(maps.Keys(t.file.objects)) {
		ref := strings.TrimPrefix(key, "obj:")
		font := t.file.dict(ref)
		if font == nil || font["/Subtype"] != "/CIDFontType2" || font["/CIDToGIDMap"] != nil {
			continue
		}
		if t.file.dict(font["/FontDescriptor"])["/FontFile2"] == nil {
			continue
		}
		changed := copyDict(font)
		changed["/CIDToGIDMap"] = "/Identity"
		t.file.set(ref, changed)
		completed++
	}
	return completed
}
