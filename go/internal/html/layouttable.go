package html

import (
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablelayout"
)

// layoutTableRole is the role attribute of a table_grid: a grid the accessibility report presumes
// a layout (one row or one column, decision 111 (2)) is written role="presentation", so a screen
// reader does not announce it as a data table; a grid written as a data table says nothing. A
// grid whose header cells are known (an inferred or marked header row) is never presumed a layout.
// The page, the report and the browser checks read tablelayout's predicate and header source, so
// they cannot drift.
func layoutTableRole(grid, nodes rec.Map) string {
	if !tablelayout.Presentational(grid, nodes) {
		return ""
	}
	return ` role="presentation"`
}
