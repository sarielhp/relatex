package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/textstyle"
)

func TestUnobservedTeXFontStyleFollowsMatchingFlowOwners(t *testing.T) {
	base := rec.Map{"source": "nfss-font-id", "font_id": int64(21), "font_name": "LMRoman12-Regular", "size_sp": int64(786_432),
		"encoding": "TU", "family": "ptm", "series": "m", "shape": "n", "generic": "serif"}
	catalog := textstyle.Catalog{21: base}
	records := []rec.Map{
		{"kind": "context", "entity": int64(15), "role": "section"},
		{"kind": "diagnostic", "code": "unobserved_text_font", "font": int64(18), "font_name": "ptmb", "size_sp": int64(720_896), "first_char": int64(51)},
		{"kind": "flow_candidate", "owner": int64(334), "entity": int64(15), "role": "section", "items": []any{rec.Map{"type": "text", "text": "3"}}},
		{"kind": "flow_candidate", "owner": int64(335), "entity": int64(15), "role": "section", "items": []any{rec.Map{"type": "text", "text": "3.1 Annotation Protocol"}}},
		{"kind": "context", "entity": int64(16), "role": "paragraph"},
		{"kind": "flow_candidate", "owner": int64(336), "entity": int64(16), "role": "paragraph", "items": []any{rec.Map{"type": "text", "text": "3.1 The text"}}},
		{"kind": "context", "entity": int64(17), "role": "section"},
		{"kind": "flow_candidate", "owner": int64(337), "entity": int64(17), "role": "section",
			"items": []any{rec.Map{"type": "text", "text": "3.2. Statistics"}}},
		{"kind": "flow_candidate", "owner": int64(338), "entity": int64(18), "role": "paragraph",
			"items": []any{rec.Map{"type": "text", "text": "A paragraph with 3"}}},
	}
	styles := unobservedOwnerStyles(records, catalog)
	want := rec.Map{"source": "unobserved-tex-font-id", "font_id": int64(18), "font_name": "ptmb", "size_sp": int64(720_896),
		"encoding": "unknown", "family": "ptm", "series": "b", "shape": "n", "generic": "serif"}
	if !rec.Equal(styles[int64(334)], want) || !rec.Equal(styles[int64(335)], want) {
		t.Fatalf("heading owner styles = %v", styles)
	}
	if _, found := styles[int64(336)]; found {
		t.Fatalf("style leaked to following paragraph: %v", styles[int64(336)])
	}
	if !rec.Equal(styles[int64(337)], want) {
		t.Fatalf("later unstyled section lost its direct TeX font: %v", styles[int64(337)])
	}
	if _, found := styles[int64(338)]; found {
		t.Fatalf("section style leaked into prose: %v", styles[int64(338)])
	}

	e := &Evidence{textStyles: textstyle.Catalog{}, ownerStyles: styles}
	block := rec.Map{"content": []any{}}
	e.appendText(block, rec.Map{"text": "Annotation Protocol", "space": ""}, spanPaint{}, styles[int64(335)])
	span := rec.AsMap(rec.FetchList(block, "content")[0])
	if !rec.Equal(span["style"], want) {
		t.Fatalf("heading text style = %v", span["style"])
	}
}

func TestUnobservedTeXFontMatchesItsDecodedFirstCharacter(t *testing.T) {
	records := []rec.Map{
		{"kind": "context", "entity": int64(0), "role": "paragraph"},
		{"kind": "diagnostic", "code": "unobserved_text_font", "font": int64(20), "font_name": "cmti10", "size_sp": int64(786_432),
			"first_char": int64(16), "first_text": "ı"},
		{"kind": "flow_candidate", "owner": int64(12), "entity": int64(0), "role": "paragraph",
			"items": []any{rec.Map{"type": "text", "text": "a Departamento de F\u0131sica, "}}},
	}
	styles := unobservedOwnerStyles(records, textstyle.Catalog{})
	if rec.FetchString(styles[int64(12)], "shape") != "it" {
		t.Fatalf("affiliation owner styles = %v", styles)
	}
}

// The table is read from the paragraph holding it, whose items name no font; the cell's own
// owner printed the direct font, so the cell's unstyled items take its style (paper 139).
func TestUnobservedTeXFontStylesTheCellsItsOwnersPrint(t *testing.T) {
	records := []rec.Map{
		{"kind": "context", "entity": int64(0), "role": "paragraph"},
		{"kind": "diagnostic", "code": "unobserved_text_font", "font": int64(20), "font_name": "cmti10", "size_sp": int64(786_432),
			"first_char": int64(16), "first_text": "ı"},
		{"kind": "flow_candidate", "owner": int64(17), "entity": int64(0), "role": "paragraph",
			"items": []any{rec.Map{"type": "text", "text": "ı", "table": int64(1), "table_cell": int64(2)}}},
		{"kind": "flow_candidate", "owner": int64(31), "entity": int64(0), "role": "paragraph", "items": []any{
			rec.Map{"type": "text", "text": "A", "style": int64(51), "table": int64(1), "table_cell": int64(1)},
			rec.Map{"type": "text", "text": "a", "table": int64(1), "table_cell": int64(2)}}},
	}
	e := &Evidence{ownerStyles: unobservedOwnerStyles(records, textstyle.Catalog{})}
	e.cellStyles = unobservedCellStyles(records, e.ownerStyles)
	owner := records[3]
	if style := e.fallbackStyle(owner, rec.AsMap(rec.FetchList(owner, "items")[1])); rec.FetchString(style, "shape") != "it" {
		t.Fatalf("cell item style = %v", style)
	}
	if style := e.fallbackStyle(owner, rec.AsMap(rec.FetchList(owner, "items")[0])); style != nil {
		t.Fatalf("a cell outside the font got %v", style)
	}
}

// {\uno Abstract}. We ...: the first owner mixes the direct font with declared ones; its
// unstyled items carry the direct font (paper 139).
func TestUnobservedTeXFontStylesTheUnstyledItemsOfAMixedFirstOwner(t *testing.T) {
	records := []rec.Map{
		{"kind": "context", "entity": int64(0), "role": "paragraph"},
		{"kind": "diagnostic", "code": "unobserved_text_font", "font": int64(19), "font_name": "cmcsc10", "size_sp": int64(786_432),
			"first_char": int64(65), "first_text": "A"},
		{"kind": "flow_candidate", "owner": int64(30), "entity": int64(0), "role": "paragraph", "items": []any{
			rec.Map{"type": "text", "text": "A"}, rec.Map{"type": "text", "text": ".", "style": int64(17)}}},
	}
	e := &Evidence{ownerStyles: unobservedOwnerStyles(records, textstyle.Catalog{})}
	owner := records[2]
	if style := e.fallbackStyle(owner, rec.AsMap(rec.FetchList(owner, "items")[0])); rec.FetchString(style, "shape") != "sc" {
		t.Fatalf("abstract heading style = %v", style)
	}
}

// A heading that mixes Times bold with its small-caps face (paper 0593) has no single owner
// style: each item names its own font, so the caps word keeps its shape and the rest does not.
func TestUnobservedDirectFontsKeepTheirRunsApart(t *testing.T) {
	records := []rec.Map{
		{"kind": "diagnostic", "code": "unobserved_text_font", "font": int64(51), "font_name": "ptmb",
			"size_sp": int64(786_432), "first_char": int64(51)},
		{"kind": "diagnostic", "code": "unobserved_text_font", "font": int64(54), "font_name": "ptmbc",
			"size_sp": int64(786_432), "first_char": int64(87)},
		{"kind": "flow_candidate", "owner": int64(80), "role": "section", "items": []any{
			rec.Map{"type": "text", "text": "3.5 The ", "space": "", "font": int64(51)},
			rec.Map{"type": "text", "text": "Waveguide", "space": "", "font": int64(54)},
			rec.Map{"type": "text", "text": " Plan Space", "space": "", "font": int64(51)}}},
	}
	catalog := textstyle.Catalog{}
	e := &Evidence{ownerStyles: unobservedOwnerStyles(records, catalog), fontStyles: unobservedFontStyles(records, catalog)}
	if style := e.ownerStyles[int64(80)]; style != nil {
		t.Fatalf("mixed heading owner style = %v", style)
	}
	owner := records[2]
	shapes := []string{}
	for _, value := range rec.FetchList(owner, "items") {
		shapes = append(shapes, rec.FetchString(e.fallbackStyle(owner, rec.AsMap(value)), "shape"))
	}
	if len(shapes) != 3 || shapes[0] != "n" || shapes[1] != "sc" || shapes[2] != "n" {
		t.Fatalf("run shapes = %v", shapes)
	}
	block := rec.Map{"content": []any{}}
	for _, value := range rec.FetchList(owner, "items") {
		item := rec.AsMap(value)
		e.appendText(block, item, spanPaint{}, e.fallbackStyle(owner, item))
	}
	if content := rec.FetchList(block, "content"); len(content) != 3 ||
		rec.FetchString(rec.AsMap(rec.AsMap(content[1])["style"]), "shape") != "sc" {
		t.Fatalf("heading spans = %v", content)
	}
}

// A formula item has no text: the owner search reads past it.
func TestUnobservedTeXFontSearchReadsPastFormulaItems(t *testing.T) {
	records := []rec.Map{
		{"kind": "context", "entity": int64(0), "role": "paragraph"},
		{"kind": "diagnostic", "code": "unobserved_text_font", "font": int64(19), "font_name": "cmcsc10", "size_sp": int64(786_432),
			"first_char": int64(65), "first_text": "A"},
		{"kind": "flow_candidate", "owner": int64(29), "entity": int64(0), "role": "paragraph", "items": []any{
			rec.Map{"type": "math", "formula": int64(5), "space": ""}}},
		{"kind": "flow_candidate", "owner": int64(30), "entity": int64(0), "role": "paragraph", "items": []any{
			rec.Map{"type": "text", "text": "A"}}},
	}
	if styles := unobservedOwnerStyles(records, textstyle.Catalog{}); styles[int64(30)] == nil {
		t.Fatalf("styles = %v", styles)
	}
}
