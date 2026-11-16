package svgoutline

import "github.com/sarielhp/relatex/internal/rec"

// A glyph shown in text render mode 3 (neither filled nor stroked) paints no ink: the hidden
// LaTeX source LaTeXiT keeps in its formula PDFs (papers/0209, 1179), the OCR text over a scanned
// page (papers2/0466). MuPDF's paint trace records it as an ignore_text and its structured text
// keeps it, transparent, but the page SVG draws nothing for it. Such a glyph binds to its paint
// with the status invisible_text and no bounds, but only when the SVG has no use of its text at
// its placement; a glyph with an outline always binds to it.

const (
	invisibleText = "invisible_text"
	// invisiblePaint is the paint trace's operation for text in render mode 3.
	invisiblePaint = "ignore_text"
)

// invisibleBinding is the binding of a glyph painted in render mode 3, or nil when its paint
// is another one.
func (b *binder) invisibleBinding(paint rec.Map) rec.Map {
	if rec.Fetch(paint, "operation") != invisiblePaint {
		return nil
	}
	if b.svg.placed(paint) {
		rec.Fail("Unsupported stroked or invisible glyph outline")
	}
	return rec.Map{"status": invisibleText, "paint": paint, "bounds": nil}
}

// invisibleRun: the paints are all invisible, as when LaTeXiT sets its hidden source at a size
// of a millionth of a point, so that a line's glyphs lie within the tolerance of one point and
// a letter repeated in it matches several paints. None of them adds ink, and the structured text
// keeps them in paint order: the glyph binds to the first one no earlier glyph took.
func invisibleRun(matches []rec.Map) bool {
	if len(matches) < 2 {
		return false
	}
	for _, entry := range matches {
		if entry["operation"] != invisiblePaint {
			return false
		}
	}
	return true
}

// validateInvisible checks an invisible_text binding as validateOffPage checks an outside_page one.
func (v *validator) validateInvisible(glyph, binding rec.Map) {
	paint := rec.FetchMap(binding, "paint")
	want := rec.Map{"status": invisibleText, "paint": paint, "bounds": nil, "origin_binding": binding["origin_binding"]}
	if !rec.Equal(binding, want) || v.svg.placed(paint) {
		rec.Fail("Invalid invisible glyph binding")
	}
	v.validateUnplaced(glyph, paint, invisiblePaint, rec.Fetch(binding, "origin_binding"))
}
