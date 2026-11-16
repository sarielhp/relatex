package pdftag

import (
	"math"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// The text check's normalization (decision 105, "What the text check forgives"). Both sides are
// already NFKC without white space or hyphens. The text read back is first compared as the
// glyphs spell it; only when that fails is it compared again in its reading form, where what
// the page shows as typography, not text, is read as Unicode writes it:
//
//   - a leader dot (a contents line's \dotfill) is dropped. A dot is a leader by its geometry:
//     blank of at least leaderBlank of its own advance on both sides, which no full stop, no
//     abbreviation dot and no ellipsis dot has. A full stop the source spaced from its word still
//     looks like one, so the form that keeps the dot is tried too: the model says which it is;
//   - a TeX accent (\accent paints the accent, then its base, over it) follows its base as
//     the combining mark Unicode writes after the letter, and the text is composed again (NFKC).
//     An accent is a lone accent glyph whose box overlaps the next glyph's; TeX's dotless \i
//     and \j under an accent are the i and j of the composed letter (í);
//   - a letter set in small capitals (the model's style says shape sc) matches the model's
//     letter in either case, since the font's glyphs name the capitals; elsewhere case counts.
//
// Every form must still equal the model's text character for character, so a missing, extra
// or wrong word fails as before.

// leaderBlank is the least blank around a leader dot, as a fraction of the dot's advance: LaTeX's
// contents leaders put 4.5 mu (0.9 of a dot) on each side, an ellipsis 1/6 em (0.6).
const leaderBlank = 0.75

// spacingAccents are the spacing accents NFKC leaves alone, with their combining marks; the
// others (acute, diaeresis, macron, cedilla, breve, dot, ring, ogonek, tilde, double acute)
// NFKC already turns into a space and the mark.
var spacingAccents = map[rune]rune{
	'`': 0x300, '^': 0x302, 'ˆ': 0x302, '~': 0x303, 'ˉ': 0x304, 'ˇ': 0x30c,
}

// readText is a marked content's text read back: as the glyphs spell it, and in reading form.
type readText struct {
	plain   string
	reading string
	// accented is the reading form with its accents composed and its leader dots kept: a dot the
	// check reads as a leader may be a full stop the source spaced from its word ("fluxes .", papers
	// 0069), and only the model says which.
	accented string
	leaders  int // leader dots the reading form dropped
	accents  int // accents it put after their base
}

func (r *readText) add(other readText) {
	r.plain += other.plain
	r.reading += other.reading
	r.accented += other.accented
	r.leaders += other.leaders
	r.accents += other.accents
}

// glyphRoles marks a page's leader dots and the accents placed over the glyph painted next.
type glyphRoles struct {
	leader []bool
	accent []rune // the combining mark of an accent glyph, 0 for any other
}

func newGlyphRoles(glyphs []paintedGlyph) glyphRoles {
	roles := glyphRoles{leader: make([]bool, len(glyphs)), accent: make([]rune, len(glyphs))}
	for index := range glyphs {
		roles.leader[index] = isLeaderDot(glyphs, index)
		roles.accent[index] = accentOver(glyphs, index)
	}
	return roles
}

// isLeaderDot: a dot with a neighbour on its own baseline on both sides, each at least
// leaderBlank of the dot's advance away. A side with no glyph on the line is not blank evidence.
func isLeaderDot(glyphs []paintedGlyph, index int) bool {
	dot := glyphs[index]
	if dot.advance <= 0 || string(normalized(dot.text)) != "." {
		return false
	}
	least := leaderBlank * dot.advance
	before, ok := lineNeighbour(glyphs, index, -1)
	if !ok || dot.baseline[0]-(before.baseline[0]+before.advance) < least {
		return false
	}
	after, ok := lineNeighbour(glyphs, index, 1)
	return ok && after.baseline[0]-(dot.baseline[0]+dot.advance) >= least
}

// lineNeighbour is the glyph painted just before or after, when it shares the baseline.
func lineNeighbour(glyphs []paintedGlyph, index, step int) (paintedGlyph, bool) {
	at := index + step
	if at < 0 || at >= len(glyphs) {
		return paintedGlyph{}, false
	}
	other := glyphs[at]
	same := other.text != "" && math.Abs(other.baseline[1]-glyphs[index].baseline[1]) < 0.5
	return other, same
}

// accentOver is the combining mark of a lone accent glyph whose box overlaps the next glyph's.
func accentOver(glyphs []paintedGlyph, index int) rune {
	mark := accentMark(glyphs[index].text)
	if mark == 0 || index+1 >= len(glyphs) {
		return 0
	}
	accent, base := glyphs[index], glyphs[index+1]
	if accent.advance <= 0 || base.advance <= 0 || base.text == "" {
		return 0
	}
	overlap := min(accent.baseline[0]+accent.advance, base.baseline[0]+base.advance) -
		max(accent.baseline[0], base.baseline[0])
	if overlap <= 0.01 {
		return 0
	}
	return mark
}

// accentMark is the combining mark a glyph's text stands for when it is one accent alone.
func accentMark(text string) rune {
	runes := normalized(text)
	if len(runes) != 1 {
		return 0
	}
	if unicode.Is(unicode.Mn, runes[0]) {
		return runes[0]
	}
	return spacingAccents[runes[0]]
}

// dotless are TeX's \i and \j as their fonts paint them, under an accent: the letters i and j of
// the composed character they stand for.
var dotless = map[string]string{"\u0131": "i", "\u0237": "j"}

// accentBase is the letter of the reading form an accented base glyph stands for.
func accentBase(base string) string {
	if letter, ok := dotless[base]; ok {
		return letter
	}
	return base
}

// glyphText reads a marked content's glyphs, given by their indices on the page.
func glyphText(glyphs []paintedGlyph, roles glyphRoles, indices []int) readText {
	var plain, reading, accented strings.Builder
	var text readText
	for at := 0; at < len(indices); at++ {
		index := indices[at]
		plain.WriteString(string(normalized(glyphs[index].text)))
		switch mark := roles.accent[index]; {
		case roles.leader[index]:
			text.leaders++
			accented.WriteString(string(normalized(glyphs[index].text)))
		case mark != 0 && at+1 < len(indices) && indices[at+1] == index+1:
			base := string(normalized(glyphs[index+1].text))
			plain.WriteString(base)
			reading.WriteString(accentBase(base) + string(mark))
			accented.WriteString(accentBase(base) + string(mark))
			text.accents++
			at++
		default:
			reading.WriteString(string(normalized(glyphs[index].text)))
			accented.WriteString(string(normalized(glyphs[index].text)))
		}
	}
	text.plain = plain.String()
	text.reading = norm.NFKC.String(reading.String())
	text.accented = norm.NFKC.String(accented.String())
	return text
}

// expectedText is a leaf's text in the model, with the letters set in small capitals marked.
type expectedText struct {
	text string
	caps []bool // per rune of text
}

// leafExpectations joins the model's text pieces per leaf. The whole text is normalized as one
// string; the small-capitals marks come from the pieces normalized alone and are dropped when
// the two disagree in length.
func leafExpectations(pieces []piece) map[string]expectedText {
	whole, caps := map[string]string{}, map[string][]bool{}
	for _, p := range pieces {
		if p.kind != pieceText {
			continue
		}
		whole[p.leaf] += p.text
		for range []rune(withoutHyphens(string(normalized(p.text)))) {
			caps[p.leaf] = append(caps[p.leaf], p.caps)
		}
	}
	out := map[string]expectedText{}
	for leaf, text := range whole {
		want := withoutHyphens(string(normalized(text)))
		mask := caps[leaf]
		if len(mask) != len([]rune(want)) {
			mask = nil
		}
		out[leaf] = expectedText{text: want, caps: mask}
	}
	return out
}

// matchKind says how the read text matched the model's: "" when it did not, "exact", or the
// normalization it needed ("small_caps", "leaders", "accents").
func matchKind(read readText, want expectedText, spelled bool) string {
	plain := withoutHyphens(read.plain)
	if sameText(plain, want.text, spelled) {
		return "exact"
	}
	if sameLetters(plain, want, spelled) {
		return "small_caps"
	}
	reading := withoutHyphens(read.reading)
	if reading != plain && sameLetters(reading, want, spelled) {
		if read.leaders > 0 {
			return "leaders"
		}
		return "accents"
	}
	// The dot the reading form dropped may be the model's full stop: try the form that keeps it.
	accented := withoutHyphens(read.accented)
	if accented != plain && accented != reading && sameLetters(accented, want, spelled) {
		return "accents"
	}
	return ""
}

// sameLetters compares character for character, a small capital matching in either case.
func sameLetters(read string, want expectedText, spelled bool) bool {
	got, expected := []rune(read), []rune(want.text)
	if len(got) != len(expected) {
		return false
	}
	for index, r := range got {
		w := expected[index]
		switch {
		case r == w, spelled && r == unicode.ReplacementChar:
		case want.caps != nil && want.caps[index] && unicode.ToLower(r) == unicode.ToLower(w):
		default:
			return false
		}
	}
	return true
}

// smallCaps reports whether a style's NFSS shape sets small capitals (sc, scit, scsl, ssc).
func smallCaps(shape string) bool { return strings.Contains(shape, "sc") }
