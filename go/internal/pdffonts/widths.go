package pdffonts

import (
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
)

// The Widths entry against the font's own advances (8.4.5.6): a viewer advances by the Widths
// entry, the validator compares it with the embedded program's advance (a Type 3 glyph's d0 or d1
// width), and the two must agree.

// widthTolerance is how far a font dictionary's width may stand from its program's before veraPDF
// calls them inconsistent (8.4.5.6: 1.4 units fail, LuaTeX's TFM widths within one unit pass). A
// width a later glyph is placed after is changed, and the glyph moved back, only beyond it: each
// kern adds a viewer's rounding to the positions after it.
const widthTolerance = 1.0

// advanceOf is a font's own advance of a code, in its glyph units, and whether it is known.
type advanceOf func(code int) (float64, bool)

// correctWidths sets the Widths of the codes shown to the font's own advance where they disagree
// (8.4.5.6). A viewer advances by the Widths entry, so without compensation (use.Compensated) only
// a code that is never followed by a glyph placed from where it ended may change.
func (x *Fixer) correctWidths(f *font, use Use, names map[int]string) {
	widths, _ := x.store.Resolve(f.dict["/Widths"]).([]any)
	advance, scale := x.advances(f, names)
	if advance == nil || len(widths) == 0 {
		return
	}
	if use.Compensated {
		x.compensatedWidths(f, use, advance, scale, widths)
		return
	}
	first := int(Number(f.dict["/FirstChar"]))
	corrected := slices.Clone(widths)
	count := 0
	for code := range use.Codes {
		index := code - first
		own, known := advance(code)
		if index < 0 || index >= len(widths) || !known || use.Advanced[code] {
			continue
		}
		if math.Abs(Number(widths[index])-own) > 0.5 {
			corrected[index] = round(own)
			count++
		}
	}
	if count > 0 {
		f.dict["/Widths"] = corrected
		f.changed = true
		x.Report.Widths += count
	}
}

// compensatedWidths sets the Widths of the codes shown to the font's own advance where they
// disagree: a code no later glyph is placed after by more than half a unit, as correctWidths does;
// one a later glyph is placed after beyond widthTolerance, the caller moving that glyph back by the
// change (Moved, in a TJ kern's thousandths of the font size: scale per glyph unit), so that
// nothing on the page moves. The Widths grow to cover every code shown: a code outside
// FirstChar..LastChar advanced by the original MissingWidth (Word's fonts give one width, for code
// 0, and place every glyph themselves). A width changes by whole units, keeping its fraction
// (444.3 becomes 446.3 for a program's 446, within half a unit of it): MuPDF reads Widths as
// integers, and a whole change undone by an equal kern leaves its positions as they were too.
func (x *Fixer) compensatedWidths(f *font, use Use, advance advanceOf, scale float64, widths []any) {
	first := int(Number(f.dict["/FirstChar"]))
	old := func(code int) float64 {
		if code < first || code-first >= len(widths) {
			return f.missing
		}
		return Number(widths[code-first])
	}
	changed, moved, stretched := map[int]float64{}, map[int]float64{}, map[int]Stretch{}
	for code := range use.Codes {
		own, known := advance(code)
		difference := math.Abs(old(code) - own)
		if !known || difference <= 0.5 || (use.Advanced[code] && difference <= widthTolerance) {
			continue
		}
		changed[code] = math.Round(own - old(code)) // whole units: see compensatedWidths
		if f.substitute && own > 0 && old(code) > 0 {
			stretched[code] = Stretch{Factor: old(code) / own, Old: old(code), New: old(code) + changed[code], Own: own}
		}
		if use.Advanced[code] {
			moved[code] = changed[code] * scale
		}
	}
	if len(changed) == 0 {
		return
	}
	codes := slices.Sorted(maps.Keys(use.Codes))
	low, high := min(first, codes[0]), max(first+len(widths)-1, codes[len(codes)-1])
	corrected := make([]any, 0, high-low+1)
	for code := low; code <= high; code++ {
		change, ok := changed[code]
		switch {
		case ok:
			corrected = append(corrected, round(old(code)+change))
		case code >= first && code-first < len(widths):
			corrected = append(corrected, widths[code-first]) // as written: not even rounded
		default:
			corrected = append(corrected, round(f.missing))
		}
	}
	f.dict["/FirstChar"], f.dict["/LastChar"], f.dict["/Widths"] = low, high, corrected
	f.changed = true
	x.Report.Widths += len(changed)
	x.stretch(f.ref, stretched)
	if len(moved) == 0 {
		return
	}
	if x.Moved == nil {
		x.Moved = map[string]map[int]float64{}
	}
	x.Moved[f.ref] = moved
}

// advances is the font's own advance of each code and the factor from its glyph units to a TJ
// kern's thousandths of the font size: a Type 1 program's widths (thousandths already), or a Type
// 3 font's glyph widths (each glyph procedure's d0 or d1) through its FontMatrix (Word draws Times
// New Roman as Type 3 glyphs 2048 units wide with Widths in thousandths: 479 for a glyph of 983).
// Nil when the font has neither.
func (x *Fixer) advances(f *font, names map[int]string) (advanceOf, float64) {
	if f.program != nil {
		return func(code int) (float64, bool) {
			width, known := f.program.Widths[names[code]]
			return width, known
		}, 1
	}
	if f.dict["/Subtype"] != "/Type3" {
		return nil, 0
	}
	matrix, _ := x.store.Resolve(f.dict["/FontMatrix"]).([]any)
	procedures, _ := x.store.Resolve(f.dict["/CharProcs"]).(map[string]any)
	if len(matrix) != 6 || Number(matrix[0]) <= 0 || procedures == nil {
		return nil, 0
	}
	return func(code int) (float64, bool) {
		ref, ok := procedures["/"+names[code]].(string)
		if !ok {
			return 0, false
		}
		data, err := x.store.StreamData(ref)
		if err != nil {
			return 0, false
		}
		return procedureWidth(data)
	}, Number(matrix[0]) * 1000
}

// procedureWidth is a Type 3 glyph procedure's width: the first operand of its d0 or d1, which begins it;
// 0 for an empty procedure.
func procedureWidth(procedure []byte) (float64, bool) {
	fields := strings.Fields(string(procedure[:min(len(procedure), 200)]))
	if len(fields) == 0 {
		return 0, true // an empty procedure (Word's space) advances by nothing, as validators read it
	}
	for index, field := range fields {
		if field != "d0" && field != "d1" {
			continue
		}
		operands := map[string]int{"d0": 2, "d1": 6}[field]
		if index < operands {
			return 0, false
		}
		width, err := strconv.ParseFloat(fields[index-operands], 64)
		return width, err == nil
	}
	return 0, false
}

// Stretch is how a viewer drew one code of a font the ordinary PDF did not embed and the tagged
// PDF embeds as the program the viewer drew it with (embedStandard): MuPDF stretches a substituted
// font's glyph horizontally from the program's advance (Own) to its Widths entry (Old), by Factor.
// Embedded, the glyph is drawn at its own advance, so the tagged PDF's content scales it by Factor
// (decision 148); New is the Widths entry the font has now.
type Stretch struct {
	Factor, Old, New, Own float64
}

// stretch records the codes of a font whose glyphs a viewer stretched.
func (x *Fixer) stretch(ref string, stretched map[int]Stretch) {
	if len(stretched) == 0 {
		return
	}
	if x.Stretched == nil {
		x.Stretched = map[string]map[int]Stretch{}
	}
	x.Stretched[ref] = stretched
	x.Report.Stretched += len(stretched)
}
