package svgoutline

import (
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// A repaint is one glyph painted again, identically, at the same place: a figure (an Ipe or
// TikZ drawing included as PDF) that draws its label twice. MuPDF's structured text keeps one
// character for the two and flags it bold; the paint trace and the page SVG keep both. The
// copies are told apart by evidence only: paint records with equal font, glyph, text and
// operation at one placement, and SVG uses with equal own attributes (outline, fill, stroke)
// at one accumulated matrix, opaque, so the second adds no ink to the first. One placement is
// one within the tolerance of every position here (Close: a figure placed twice by two
// transforms lands 0.0002 pt apart, papers/0182). A copy offset further is not a repaint
// (overprint, \pmb, is told apart by its offset in verifyCoverage).
//
// A plot that draws its series again embeds each copy's markers in a subset font of its own
// (papers/0209: nine copies of nine red bullets, in subsets AAAABT+ to AAAAEV+ of one font): the
// copies are one glyph of one font, its subsets named apart, and MuPDF writes each subset's
// outline under its own path id. Such paints are one glyph when their font names agree but for
// the subset tag, and such uses one outline when their plain path targets have equal geometry.

// paintName and paintPlacement are the fields of a paint record that make two paints of one glyph
// (samePaint, which also compares the font names): the names equal, the placement Close; a
// repaint also has an equal operation.
var (
	paintName      = []string{"glyph", "text"}
	paintPlacement = []string{"matrix", "baseline", "advance"}
)

// transparency are the SVG attributes under which a second identical paint changes the ink.
var transparency = []string{"opacity", "fill-opacity", "stroke-opacity", "mask", "filter", "mix-blend-mode", "style"}

// placementAttributes are a use's attributes that say where and which outline it draws, not how:
// a signature leaves them out, and every comparison of uses checks the outline and matrix itself.
var placementAttributes = map[string]bool{"transform": true, "xlink:href": true, "href": true}

// signature is a use's own attributes but its transform and target, sorted, or "" when it or an
// ancestor is not plainly opaque: such a use is never a repaint.
func signature(use *element) string { return useSignature(use, false) }

// shape is signature without the colors: whether the use fills and strokes, not in what (overpaint.go).
func shape(use *element) string { return useSignature(use, true) }

func useSignature(use *element, colorless bool) string {
	for node := use; node != nil; node = node.parent {
		for _, name := range transparency {
			if value, present := node.attrs[name]; present && value != "1" {
				return ""
			}
		}
	}
	pairs := make([]string, 0, len(use.attrs)+2)
	for name, value := range use.attrs {
		if !placementAttributes[name] && !(colorless && (name == "fill" || name == "stroke")) {
			pairs = append(pairs, name+"\x00"+value)
		}
	}
	if colorless {
		pairs = append(pairs, "fill\x00"+painted(use.attrs, "fill", "black"), "stroke\x00"+painted(use.attrs, "stroke", "none"))
	}
	slices.Sort(pairs)
	return strings.Join(pairs, "\x01")
}

// painted is whether a use fills (or strokes) its outline, whatever the opaque color: "none" or
// "paint", with the SVG default when the attribute is absent.
func painted(attrs map[string]string, name, fallback string) string {
	value, ok := attrs[name]
	if !ok {
		value = fallback
	}
	if value == "none" {
		return "none"
	}
	return "paint"
}

// repaintedUse: entry is other painted again, identically and opaquely.
func (o *Outlines) repaintedUse(entry, other rec.Map) bool {
	left, right := rec.FetchInt(entry, "use"), rec.FetchInt(other, "use")
	if left == right || left < 0 || right < 0 || int(max(left, right)) >= len(o.signatures) {
		return false
	}
	return o.signatures[left] != "" && o.signatures[left] == o.signatures[right] && o.outline(entry) == o.outline(other) &&
		Close(rec.FetchList(entry, "matrix"), rec.FetchList(other, "matrix"))
}

// outline is what a use draws: its plain path target's geometry, else the target itself.
func (o *Outlines) outline(entry rec.Map) string {
	path := rec.FetchString(entry, "path")
	if geometry, plain := o.geometries[path]; plain {
		return "d\x00" + geometry
	}
	return "id\x00" + path
}

// repaints: every match is the first painted again. The matches are the paints no glyph is bound
// to yet, in paint order: each structured glyph takes the first copy, the rest are repaints.
func repaints(matches []rec.Map) bool {
	if len(matches) < 2 {
		return false
	}
	for _, entry := range matches[1:] {
		if !rec.Equal(rec.Fetch(entry, "operation"), rec.Fetch(matches[0], "operation")) || !samePaint(entry, matches[0]) {
			return false
		}
	}
	return true
}
