package pdftag

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/confusables"
)

// Which glyphs get the document's character (decision 148). A text leaf whose glyphs, read in
// order, spell the model's text except for symbols the font names otherwise (cmsy's bardbl, which
// the kernel's table reads ∥ where the document writes ‖, papers3 0629; a typewriter font's en
// dash painted with its hyphen glyph, papers4 0995) gets the model's character for each of those
// glyphs. The leaf's glyphs are aligned with the model's text character by character: equal
// characters match (a small capital in either case, as the order check reads it; U+FFFD, which
// the check reads as any character of a font the tagging spelled), a hyphen may be left out (the
// check drops hyphens), and a glyph of one character that is no letter and no digit may stand for
// a model character that is no letter and no digit only when Unicode calls the two the same
// character: their UTS #39 skeletons (internal/confusables, Unicode's confusables.txt) are equal,
// as ‖ and ∥ (both "ll") and – and - are. A + glyph never stands for a −, nor < for ≤: such a leaf
// fails the order check as before. Only an alignment that uses every character of both sides is
// taken; letters and digits must agree as they are, so a word bound to the wrong leaf is never
// respelled.

// maxSpellCells bounds the alignment's table (characters times hyphens).
const maxSpellCells = 5_000_000

// readRune is a character of a leaf's glyphs as the check reads them.
type readRune struct {
	r      rune
	ref    glyphRef
	single bool // the glyph reads as this one character
}

// spellLeaves records the document's character of every glyph of a text leaf that the font
// spells otherwise.
func (t *tagger) spellLeaves() {
	byLeaf := map[string][]glyphRef{}
	for ref, leaf := range t.binding.leafOf {
		host := linkHost(leaf)
		byLeaf[host] = append(byLeaf[host], ref)
	}
	t.spelled = map[glyphRef]string{}
	for _, leaf := range slices.Sorted(maps.Keys(byLeaf)) {
		want, ok := t.leafText[leaf]
		if !ok || !textRoles[t.roleOf(leaf)] {
			continue
		}
		refs := byLeaf[leaf]
		slices.SortFunc(refs, func(a, b glyphRef) int { return cmp.Or(cmp.Compare(a.page, b.page), cmp.Compare(a.index, b.index)) })
		maps.Copy(t.spelled, spellLeaf(t.painted, refs, want))
	}
	t.report.ActualText = len(t.spelled)
}

// linkHost is the text leaf a Link's leaf (linkLeaf) belongs to; any other leaf is its own.
func linkHost(leaf string) string {
	at := strings.LastIndex(leaf, "#a")
	if at < 0 || at+2 == len(leaf) || strings.Trim(leaf[at+2:], "0123456789") != "" {
		return leaf
	}
	return leaf[:at]
}

// spellLeaf aligns one leaf's glyphs with its model text; the glyphs to respell, with the model's
// character.
func spellLeaf(painted map[int][]paintedGlyph, refs []glyphRef, want expectedText) map[glyphRef]string {
	var read []readRune
	for _, ref := range refs {
		page := painted[ref.page]
		if ref.index >= len(page) {
			continue
		}
		text := normalized(page[ref.index].text)
		for _, r := range text {
			read = append(read, readRune{r: r, ref: ref, single: len(text) == 1})
		}
	}
	model := []rune(want.text)
	if string(readPlain(read)) == want.text {
		return nil
	}
	return align(read, model, want.caps)
}

// readPlain is the read characters without hyphens, as the check compares them.
func readPlain(read []readRune) []rune {
	out := make([]rune, 0, len(read))
	for _, c := range read {
		if !isHyphen(c.r) {
			out = append(out, c.r)
		}
	}
	return out
}

// Alignment steps.
const (
	stepNone byte = iota
	stepMatch
	stepRespell
	stepDrop
)

// align is the alignment of spellLeaf: a table over model position i and the hyphens dropped so
// far d (the read position is i+d), each cell the fewest respellings that reach it.
func align(read []readRune, model []rune, caps []bool) map[glyphRef]string {
	n, drops := len(model), len(read)-len(model)
	if drops < 0 || drops > len(read)-len(readPlain(read)) || (n+1)*(drops+1) > maxSpellCells {
		return nil
	}
	width := drops + 1
	cost := make([]int, (n+1)*width)
	step := make([]byte, (n+1)*width)
	for index := range cost {
		cost[index] = -1
	}
	cost[0] = 0
	relax := func(cell, value int, how byte) {
		if cost[cell] < 0 || value < cost[cell] {
			cost[cell], step[cell] = value, how
		}
	}
	for i := 0; i <= n; i++ {
		for d := 0; d < width; d++ {
			here := cost[i*width+d]
			if here < 0 || i+d >= len(read) {
				continue
			}
			c := read[i+d]
			if d+1 < width && isHyphen(c.r) {
				relax(i*width+d+1, here, stepDrop)
			}
			if i == n {
				continue
			}
			switch {
			case sameRune(c.r, model[i], caps, i):
				relax((i+1)*width+d, here, stepMatch)
			case respellable(c, model[i]):
				relax((i+1)*width+d, here+1, stepRespell)
			}
		}
	}
	if cost[n*width+drops] <= 0 {
		return nil
	}
	return respellings(read, model, step, width, n, drops)
}

// respellings walks the alignment back from its end and collects the respelled glyphs.
func respellings(read []readRune, model []rune, step []byte, width, i, d int) map[glyphRef]string {
	out := map[glyphRef]string{}
	for i > 0 || d > 0 {
		switch step[i*width+d] {
		case stepDrop:
			d--
		case stepRespell:
			i--
			out[read[i+d].ref] = string(model[i])
		case stepMatch:
			i--
		default:
			return nil
		}
	}
	return out
}

// sameRune: a read character the check takes for the model's (no hyphen: the check drops those).
func sameRune(read, want rune, caps []bool, at int) bool {
	switch {
	case isHyphen(read):
		return false
	case read == want, read == unicode.ReplacementChar:
		return true
	default:
		return caps != nil && caps[at] && unicode.ToLower(read) == unicode.ToLower(want)
	}
}

// respellable: a glyph of one character that is no letter and no digit, standing for a model
// character that is no letter and no digit either and that Unicode's confusables make the same
// character (equal UTS #39 skeletons).
func respellable(read readRune, want rune) bool {
	return read.single && read.r != want && read.r != unicode.ReplacementChar && symbol(read.r) && symbol(want) &&
		confusables.Confusable(string(read.r), string(want))
}

func symbol(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r) }
