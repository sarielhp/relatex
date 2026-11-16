package html

import (
	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
)

// nameTables marks the table a table_float holds as named by its caption, so table navigation
// announces that caption instead of only the table's size (a11y-804). htmlkit.TableName has the
// rule the browser checks read too.
func (p *Paper) nameTables(float rec.Map) {
	table, caption, named := htmlkit.TableName(float, p.nodes)
	if !named {
		return
	}
	if p.tableNames == nil {
		p.tableNames = map[string]string{}
	}
	p.tableNames[table] = caption
}

// tableName is the aria-labelledby attribute of a table_grid the float's caption names, else "".
func (p *Paper) tableName(span rec.Map) string {
	caption, named := p.tableNames[rec.ToS(rec.Fetch(span, "id"))]
	if !named {
		return ""
	}
	return " aria-labelledby=\"" + htmlkit.Escape(caption) + "\""
}
