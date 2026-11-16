package layout

import (
	"slices"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/rec"
)

// A whole-formula /ActualText (decision 190): an equation tagged with its TeX source as ActualText over
// many glyphs (papers6/0571, 52 to 67 glyphs). MuPDF's structured text spreads that text over the glyphs,
// one character at each glyph's origin and the rest after the last one (135 characters), so no painted
// glyph's own Unicode matches any of it. The ActualText stands for the whole formula: it is one span's
// text, carried by the formula's first glyph (actual_text), and every glyph keeps its own Unicode, as a
// reader of the tagged PDF reads it (decision 148 drops the source's ActualText). The structured text's
// spread characters are replaced by those glyphs' own, so no glyph carries a string longer than its font
// gives it.

// formula is one multi-glyph /ActualText: its text, and each painted glyph with the characters MuPDF's
// structured text spreads onto it (an unanchored companion mark's character joins the glyph before it).
type formula struct {
	text   string
	paints []rec.Map
	spread []string
}

// wholeFormula is the formula an /ActualText over several glyphs makes, when its text has a character
// for each glyph of the trace; the last glyph takes the rest.
func wholeFormula(content *nativetrace.Node, recordOf map[*nativetrace.Node]rec.Map) (formula, bool) {
	if kind, _ := content.Attr("type"); kind != "actualtext" {
		return formula{}, false
	}
	replacement, _ := content.Attr("txt")
	runes := []rune(replacement)
	glyphs := content.Descendants("g")
	if len(glyphs) < 2 || len(runes) < len(glyphs) {
		return formula{}, false
	}
	found := formula{text: replacement}
	for index, glyph := range glyphs {
		characters := string(runes[index : index+1])
		if index == len(glyphs)-1 {
			characters = string(runes[index:])
		}
		if record, anchored := recordOf[glyph]; anchored {
			found.paints, found.spread = append(found.paints, record), append(found.spread, characters)
		} else if len(found.spread) > 0 {
			found.spread[len(found.spread)-1] += characters
		}
	}
	return found, len(found.paints) > 0
}

// respell replaces the structured text's spread characters of every formula by its glyphs' own, and
// returns the records, and their flags, without the characters left over. A formula the structured text does not show
// spread, glyph by glyph, is left as it is.
func (u *unicodeGlyphs) respell(records []rec.Map, flags []int64) ([]rec.Map, []int64) {
	dropped := map[int]bool{}
	for _, found := range u.formulas {
		found.apply(records, flags, dropped)
	}
	if len(dropped) == 0 || len(flags) != len(records) {
		return records, flags
	}
	kept, keptFlags := make([]rec.Map, 0, len(records)-len(dropped)), make([]int64, 0, len(records)-len(dropped))
	for index, record := range records {
		if !dropped[index] {
			kept, keptFlags = append(kept, record), append(keptFlags, flags[index])
		}
	}
	return kept, keptFlags
}

// ownText is the text a painted glyph draws: its Unicode, not an ActualText replacement.
func ownText(paint rec.Map) string {
	if drawn, replaced := paint["drawn_text"]; replaced {
		return rec.ToS(drawn)
	}
	return rec.FetchString(paint, "text")
}

// apply changes the formula's records when every glyph finds its spread characters in the structured
// text, and at least one glyph's own text differs from them. The structured text decides how the
// leftover characters lie: after the last glyph's own, or after the one before it with the final
// character alone on the last glyph (papers6/1253).
func (f formula) apply(records []rec.Map, flags []int64, dropped map[int]bool) {
	if f.place(records, flags, dropped, f.spread) {
		return
	}
	last := len(f.spread) - 1
	rest := []rune(f.spread[last])
	if last == 0 || len(rest) < 2 {
		return
	}
	early := slices.Clone(f.spread)
	early[last-1] += string(rest[:len(rest)-1])
	early[last] = string(rest[len(rest)-1:])
	f.place(records, flags, dropped, early)
}

// place gives the formula's records their glyphs' own text where the structured text spells spread.
func (f formula) place(records []rec.Map, flags []int64, dropped map[int]bool, spread []string) bool {
	chains := make([]chain, len(f.paints))
	taken := map[int]bool{}
	differs := false
	for index, paint := range f.paints {
		found, ok := chainOf(records, flags, paint, spread[index], taken)
		if !ok {
			return false
		}
		chains[index] = found
		for _, position := range append(slices.Clone(found.spelled), found.spaces...) {
			taken[position] = true
		}
		own := ownText(paint)
		differs = differs || own != spread[index]
		if utf8.RuneCountInString(own) > len(found.spelled) {
			return false
		}
	}
	if !differs {
		return false
	}
	for index, paint := range f.paints {
		own := []rune(ownText(paint))
		for offset, position := range chains[index].spelled {
			if offset < len(own) {
				records[position]["text"] = string(own[offset])
			} else {
				dropped[position] = true
			}
		}
		for _, position := range chains[index].spaces {
			dropped[position] = true
		}
	}
	first := chains[0].spelled[0]
	records[first]["actual_text"] = f.text
	return true
}

// chain is the structured records one painted glyph's spread characters take: the ones that spell
// them, and the synthetic spaces MuPDF's structured text put between them.
type chain struct {
	spelled, spaces []int
}

// chainOf is the first untaken structured record at the paint's origin, in its font, that starts the
// characters the structured text spreads onto it, with the records after it that spell the rest.
func chainOf(records []rec.Map, flags []int64, paint rec.Map, characters string, taken map[int]bool) (chain, bool) {
	font := paintFont(paint["font"])
	for index, record := range records {
		if taken[index] || rec.ToS(record["font"]) != font ||
			firstCharacter(rec.FetchString(record, "text")) != firstCharacter(characters) ||
			!within(rec.FetchList(record, "baseline"), rec.FetchList(paint, "baseline")) {
			continue
		}
		if found, ok := spells(records, flags, index, characters, taken); ok {
			return found, true
		}
	}
	return chain{}, false
}

// spells: the records from index, none taken, spell characters one rune each, a synthetic space
// between them left out.
func spells(records []rec.Map, flags []int64, index int, characters string, taken map[int]bool) (chain, bool) {
	found := chain{}
	position := index
	for _, character := range characters {
		for position < len(records) && !taken[position] && string(character) != " " && rec.FetchString(records[position], "text") == " " &&
			position < len(flags) && flags[position]&syntheticFlag != 0 {
			found.spaces = append(found.spaces, position)
			position++
		}
		if position >= len(records) || taken[position] || rec.FetchString(records[position], "text") != string(character) {
			return chain{}, false
		}
		found.spelled = append(found.spelled, position)
		position++
	}
	return found, true
}
