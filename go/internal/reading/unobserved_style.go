package reading

import (
	"fmt"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/textstyle"
)

func unobservedOwnerStyles(records []rec.Map, catalog textstyle.Catalog) map[any]rec.Map {
	return textstyle.UnobservedOwnerStyles(records, catalog)
}

// unobservedFontStyles: a direct TeX font's style by the ID its items name (item.font). An owner
// that prints two direct fonts keeps the face of every run this way (paper 0593's heading mixes
// Times bold with its small-caps face); a font whose face cannot be read stays unstyled.
func unobservedFontStyles(records []rec.Map, catalog textstyle.Catalog) map[int64]rec.Map {
	result := map[int64]rec.Map{}
	for _, row := range records {
		if row["kind"] != "diagnostic" || row["code"] != "unobserved_text_font" {
			continue
		}
		font, ok := row["font"].(int64)
		if !ok {
			continue
		}
		if style := textstyle.UnobservedStyle(row, catalog); style != nil {
			result[font] = style
		}
	}
	return result
}

// unobservedCellStyles: a table set in a direct TeX font is read from the paragraph that holds
// it, whose items name no font (paper 139's affiliations in \font\dos=cmti10). The cell's own
// flow owners carry the font: a cell whose owners print one direct font has that font's style.
func unobservedCellStyles(records []rec.Map, owners map[any]rec.Map) map[string]rec.Map {
	result := map[string]rec.Map{}
	conflicting := map[string]bool{}
	for _, row := range records {
		style, ok := owners[row["owner"]]
		if row["kind"] != "flow_candidate" || !ok {
			continue
		}
		for _, value := range rec.FetchList(row, "items") {
			name, inCell := cellKey(rec.AsMap(value))
			if !inCell {
				continue
			}
			if known, seen := result[name]; seen && !rec.Equal(known, style) {
				conflicting[name] = true
			}
			result[name] = style
		}
	}
	for name := range conflicting {
		delete(result, name)
	}
	return result
}

func cellKey(item rec.Map) (string, bool) {
	table, cell := item["table"], item["table_cell"]
	if table == nil || cell == nil {
		return "", false
	}
	return fmt.Sprint(table, "/", cell), true
}

// fallbackStyle is the direct TeX font style of an item: the font the item names, else its
// owner's, else its table cell's.
func (e *Evidence) fallbackStyle(owner, item rec.Map) rec.Map {
	if font, ok := item["font"].(int64); ok {
		if style, found := e.fontStyles[font]; found {
			return style
		}
	}
	if style, ok := e.ownerStyles[rec.Fetch(owner, "owner")]; ok {
		return style
	}
	if name, inCell := cellKey(item); inCell {
		return e.cellStyles[name]
	}
	return nil
}
