package pdftag

// A longtable repeats rows on every page: its \endhead rows (and the "continued" caption it holds)
// after the first page, its \endfoot rows before each break. The model lists those rows as cells
// of their own, set again on every page (roles repeat_head, repeat_foot, and a caption row below
// the first). Only the first occurrence is content: the first head and the last foot are the
// table's header and footer rows. The copies the page breaks add are pagination, Artifacts: they
// are no TR of the structure and their glyphs are not bound to a leaf (ISO 32000-2 14.8.2.2.2).

// repeatedCells are the ids of the cells of grid that longtable sets again on each page.
func repeatedCells(grid *tableGrid) map[string]bool {
	repeated := map[string]bool{}
	firstHead, repeating, captionRow := false, false, -1
	for _, cell := range grid.Cells {
		switch cell.Role {
		case "first_head":
			firstHead = true
		case "repeat_head", "repeat_foot":
			repeating = true
		case "caption":
			if captionRow < 0 || cell.Row < captionRow {
				captionRow = cell.Row
			}
		}
	}
	if !repeating {
		return repeated
	}
	for _, cell := range grid.Cells {
		switch {
		case cell.Role == "repeat_foot", cell.Role == "repeat_head" && firstHead,
			cell.Role == "caption" && cell.Row > captionRow:
			repeated[cell.ID] = true
		}
	}
	return repeated
}

// withoutRepeats are the cells that are content, and the ids of the repeated ones.
func withoutRepeats(grid *tableGrid) ([]tableCell, []string) {
	repeated := repeatedCells(grid)
	if len(repeated) == 0 {
		return grid.Cells, nil
	}
	kept, dropped := make([]tableCell, 0, len(grid.Cells)), []string{}
	for _, cell := range grid.Cells {
		if repeated[cell.ID] {
			dropped = append(dropped, cell.ID)
			continue
		}
		kept = append(kept, cell)
	}
	return kept, dropped
}
