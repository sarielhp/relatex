package pdftag

import (
	"fmt"
	"math"
)

// Each page's content interpreted, checked against the layout, marked and rewritten.

// positionTolerance is how far, in bp, a show operator's first glyph may lie from the layout's.
const positionTolerance = 0.05

// pageResult is a page after rewriting.
type pageResult struct {
	content []byte
	marks   []mark
	glyphs  int
	checked int
	failure string // why the page was left all artifact
}

// pageBox is the page's crop box (else its media box), the layout's frame.
func (f *pdfFile) pageBox(p page) [4]float64 {
	for _, key := range []string{"/CropBox", "/MediaBox"} {
		if box := f.array(p.dict[key]); len(box) == 4 {
			return [4]float64{numberOf(box[0]), numberOf(box[1]), numberOf(box[2]), numberOf(box[3])}
		}
	}
	return [4]float64{0, 0, 612, 792}
}

// pageFrame is the page's box with its /Rotate: the layout is the page as displayed (rotated),
// the content stream and the annotations are in the unrotated user space.
type pageFrame struct {
	box    [4]float64
	rotate int // 0, 90, 180 or 270 degrees clockwise
}

// frameOf is a page's frame; a /Rotate that is not a right angle is taken as none.
func (f *pdfFile) frameOf(p page) pageFrame {
	rotate := (int(numberOf(f.resolve(p.dict["/Rotate"])))%360 + 360) % 360
	if rotate%90 != 0 {
		rotate = 0
	}
	return pageFrame{box: f.pageBox(p), rotate: rotate}
}

// layout is a user-space point's position in the layout: from the page's top left, y down, as displayed.
func (f pageFrame) layout(ux, uy float64) (float64, float64) {
	across, down := ux-f.box[0], f.box[3]-uy
	width, height := f.box[2]-f.box[0], f.box[3]-f.box[1]
	switch f.rotate {
	case 90:
		return height - down, across
	case 180:
		return width - across, height - down
	case 270:
		return down, width - across
	}
	return across, down
}

// tex is a layout position in the frame of TeX's glyph owners: the unrotated page from its top left,
// y down (the page as the content stream draws it).
func (f pageFrame) tex(x, y float64) (float64, float64) {
	ux, uy := f.user(x, y)
	return ux - f.box[0], f.box[3] - uy
}

// user is the user-space point of a layout position, the inverse of layout.
func (f pageFrame) user(x, y float64) (float64, float64) {
	width, height := f.box[2]-f.box[0], f.box[3]-f.box[1]
	across, down := x, y
	switch f.rotate {
	case 90:
		across, down = y, height-x
	case 180:
		across, down = width-x, height-y
	case 270:
		across, down = width-y, x
	}
	return f.box[0] + across, f.box[3] - down
}

// glyphMapper maps the glyphs the content paints to the layout's paint indices, checking each
// show operator's first glyph against the layout's position. The layout can leave out glyphs a
// form XObject paints (a figure's hidden labels); the offset grows only there. The layout numbers
// paints, not glyphs: a glyph filled and stroked has two (paints.go).
type glyphMapper struct {
	painted []paintedGlyph
	frame   pageFrame
	paints  tracePaints
	mapping []int // content ordinal → layout index, -1 when the layout has no such glyph
	offset  int   // paint indices minus layout indices so far
	slack   int   // paints of forms since the last checked operator, which the layout may lack
	forms   []item
	checked int
}

// mapPaints builds the page's glyph mapping by the glyphs' paint indices, or says why the page
// cannot be bound.
func mapPaints(items []item, painted []paintedGlyph, frame pageFrame, paints tracePaints) (*glyphMapper, error) {
	total := len(paints.of)
	m := &glyphMapper{painted: painted, frame: frame, paints: paints, mapping: make([]int, total)}
	for i := range m.mapping {
		m.mapping[i] = -1
	}
	for _, entry := range items {
		if entry.count == 0 {
			continue
		}
		if entry.kind == kindObject {
			m.slack += paints.span(entry.first, entry.first+entry.count)
			m.forms = append(m.forms, entry)
			continue
		}
		if err := m.show(entry); err != nil {
			return m, err
		}
	}
	left := paints.total - m.offset
	if !m.ends(left) && !m.ends(left-m.slack) {
		return m, fmt.Errorf("the content paints %d glyphs, the layout %d", total, len(painted))
	}
	if m.ends(left) {
		m.mapForms()
	}
	return m, nil
}

// ends reports whether the layout ends after left paints: all of them, or all but the strokes
// after the last glyph's fill, which bind no glyph.
func (m *glyphMapper) ends(left int) bool {
	return left == len(m.painted) || left-m.paints.trailing() == len(m.painted)
}

// show checks and maps a show operator's glyphs.
func (m *glyphMapper) show(entry item) error {
	if entry.placed {
		delta, ok := m.locate(entry)
		if !ok {
			return fmt.Errorf("glyph %d is not where the layout has it", entry.first)
		}
		if delta == 0 {
			m.mapForms()
		}
		m.offset += delta
		m.slack, m.forms = 0, nil
		m.checked++
	}
	for ordinal := entry.first; ordinal < entry.first+entry.count; ordinal++ {
		index := m.paints.of[ordinal] - m.offset
		if index < 0 || index >= len(m.painted) {
			return fmt.Errorf("glyph %d is beyond the layout's %d", ordinal, len(m.painted))
		}
		m.mapping[ordinal] = index
	}
	return nil
}

// locate finds how many of the forms' glyphs since the last check the layout lacks. A paint index
// the layout skipped (the top piece of an extensible delimiter, after its repeated bars: papers
// 0074, 0124, 0175, 0226) has no position to check; the glyph in its place is taken as it.
func (m *glyphMapper) locate(entry item) (int, bool) {
	x, y := m.frame.layout(entry.origin[0], entry.origin[1])
	for delta := 0; delta <= m.slack; delta++ {
		index := m.paints.of[entry.first] - m.offset - delta
		if index < 0 || index >= len(m.painted) {
			continue
		}
		if m.painted[index].unknown && delta == 0 {
			return 0, true
		}
		want := m.painted[index].baseline
		if math.Abs(x-want[0]) <= positionTolerance && math.Abs(y-want[1]) <= positionTolerance {
			return delta, true
		}
	}
	return 0, false
}

// mapForms maps the pending forms' glyphs when the layout kept all of them.
func (m *glyphMapper) mapForms() {
	for _, form := range m.forms {
		for ordinal := form.first; ordinal < form.first+form.count; ordinal++ {
			if index := m.paints.of[ordinal] - m.offset; index >= 0 && index < len(m.painted) {
				m.mapping[ordinal] = index
			}
		}
	}
	m.forms = nil
}
