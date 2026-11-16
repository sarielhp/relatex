package figscan

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Printed text of Ipe labels: an Ipe label holds LaTeX source with the author's macros
// ($\rgnY1v$), a reader needs what is printed (Δ_R(v)). The page's trace has the printed glyphs;
// each traced run goes to the Ipe label whose window it starts in, nearest anchor first.
const (
	// printedNone marks an Ipe label no traced run matched: only its source is known.
	printedNone = "source"
	// windowHeight is how far (in text sizes) a run's baseline may sit above or below the anchor.
	windowHeight = 1.5
	// windowSlack is the width (in text sizes) a window gets beyond its estimated text width.
	windowSlack = 4.0
	// glyphWidth is the estimated width of one source character in text sizes.
	glyphWidth = 0.5
	// minipageLines is how many lines (in text sizes) a minipage window reaches below its anchor.
	minipageLines = 20.0
	// scriptShift is the baseline shift (in text sizes) that makes a smaller run a sub- or
	// superscript: TeX shifts scripts by 0.15 or more, a size change alone splits a label (sizeSlack).
	scriptShift = 0.05
	// scriptSize is the largest size (relative to the line's) a sub- or superscript has.
	scriptSize = 0.9
)

// printedRun is one traced label: its text, start, size (normalized) and whether a glyph is unmapped.
type printedRun struct {
	text     string
	at       Point
	size     float64
	unmapped bool
	// traced is the text as the glyphs carry it (lowercase for small caps), smallCaps marks a run with small caps.
	traced    string
	smallCaps bool
	// end is where the run's last glyph ends (nil when unknown).
	end *Point
}

// attachPrinted gives each Ipe text node the printed text of the traced runs in its window.
func attachPrinted(ipeNodes, traced []*Node, scale float64) {
	var texts []*Node
	for _, n := range flatten(ipeNodes) {
		if n.Kind == KindText && n.Point != nil {
			texts = append(texts, n)
			n.PrintedNote = printedNone
		}
	}
	matched := make([][]printedRun, len(texts))
	for _, run := range printedRuns(traced, scale) {
		if best := nearestWindow(texts, run); best >= 0 {
			matched[best] = append(matched[best], run)
		}
	}
	for i, n := range texts {
		if len(matched[i]) > 0 {
			n.Printed, n.PrintedNote = joinRuns(matched[i])
		}
	}
}

func printedRuns(traced []*Node, scale float64) []printedRun {
	var runs []printedRun
	for _, n := range flatten(traced) {
		if n.Kind != KindText || n.Point == nil || n.Text == "" {
			continue
		}
		size, err := strconv.ParseFloat(strings.TrimSuffix(n.Size, "pt"), 64)
		if err != nil || size <= 0 {
			size = 10
		}
		traced := n.Text
		if n.smallCaps {
			traced = strings.TrimFunc(n.traced, unicode.IsSpace)
		}
		runs = append(runs, printedRun{text: n.Text, at: *n.Point, size: size * scale,
			unmapped: strings.ContainsRune(n.Text, utf8.RuneError), traced: traced, smallCaps: n.smallCaps, end: n.traceEnd})
	}
	return runs
}

// nearestWindow is the index of the text whose window holds run's start nearest its anchor, -1 for none.
func nearestWindow(texts []*Node, run printedRun) int {
	best, bestDistance := -1, math.Inf(1)
	for i, n := range texts {
		if !inWindow(n, run) {
			continue
		}
		if d := math.Hypot(run.at[0]-n.Point[0], run.at[1]-n.Point[1]); d < bestDistance {
			best, bestDistance = i, d
		}
	}
	return best
}

// inWindow reports whether run starts in the window around n's anchor: the source's estimated
// width either side (any alignment), a line and a half up and down, many lines down for a minipage.
func inWindow(n *Node, run printedRun) bool {
	h := run.size
	width := (windowSlack + float64(glyphWidth*float64(utf8.RuneCountInString(n.Text)))) * h
	below := windowHeight * h
	if n.TextType == "minipage" {
		below = minipageLines * h
	}
	dx, dy := run.at[0]-n.Point[0], run.at[1]-n.Point[1]
	return math.Abs(dx) <= width && dy <= windowHeight*h && dy >= -below
}

// joinRuns reads the runs in lines top to bottom, left to right; a smaller run lower or higher
// than its line's first run is a sub- or superscript ("_", "^").
func joinRuns(runs []printedRun) (text, note string) {
	slices.SortStableFunc(runs, func(a, b printedRun) int {
		if math.Abs(a.at[1]-b.at[1]) > max(a.size, b.size) {
			return compareFloat(b.at[1], a.at[1])
		}
		return compareFloat(a.at[0], b.at[0])
	})
	var out, traced strings.Builder
	base, script, unmapped, smallCaps := runs[0], false, false, false
	for i, run := range runs {
		if i > 0 && math.Abs(run.at[1]-base.at[1]) > base.size {
			base = run
		}
		mark := scriptMark(run, base)
		if i > 0 && mark == "" && spaced(runs[i-1], run, base, script) {
			mark = " "
		}
		script = mark == "_" || mark == "^"
		out.WriteString(mark + run.text)
		traced.WriteString(mark + run.traced)
		unmapped = unmapped || run.unmapped
		smallCaps = smallCaps || run.smallCaps
	}
	var notes []string
	if unmapped {
		notes = append(notes, unmappedText)
	}
	if smallCaps {
		notes = append(notes, caseNote(traced.String()))
	}
	return out.String(), strings.Join(notes, "; ")
}

// spaced reports whether a space separates run from prev on base's line: a gap wider than a word
// space after prev's end when it is known, else any run that does not follow a script.
func spaced(prev, run, base printedRun, afterScript bool) bool {
	if prev.end == nil || math.Abs(run.at[1]-base.at[1]) > base.size || math.Abs(prev.at[1]-base.at[1]) > base.size {
		return !afterScript
	}
	return run.at[0]-prev.end[0] > spaceGap*base.size
}

// scriptMark is "_" for a run smaller than base and below its baseline, "^" above, else "".
func scriptMark(run, base printedRun) string {
	shift := run.at[1] - base.at[1]
	switch {
	case run.size >= scriptSize*base.size:
		return ""
	case shift < -scriptShift*base.size:
		return "_"
	case shift > scriptShift*base.size:
		return "^"
	}
	return ""
}
