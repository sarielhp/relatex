package tables

import (
	"slices"
	"strconv"
	"strings"
)

// declaredHeaderRows are the logical rows (from 0) that the LaTeX tagging project's
// \tagpdfsetup{table/header-rows={…}} in force for table names (decision 111): its numbers count
// from 1, a negative one from the last row (latex-lab's \seq_item:Nn), and a number outside the
// table names no row. Nil without the setting.
func (e *rich) declaredHeaderRows(table any, count int) []any {
	setting, ok := e.declared[key(table)]
	if !ok {
		return nil
	}
	rows := []int64{}
	for _, item := range strings.Split(setting, ",") {
		number, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || number == 0 {
			continue
		}
		row := number - 1
		if number < 0 {
			row = count + number
		}
		if row >= 0 && row < count && !slices.Contains(rows, int64(row)) {
			rows = append(rows, int64(row))
		}
	}
	slices.Sort(rows)
	result := make([]any, len(rows))
	for index, row := range rows {
		result[index] = row
	}
	return result
}
