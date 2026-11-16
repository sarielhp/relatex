// Native cells/rules, measured before row sizing. Sections paginate with the table.
#let l2d-table-rule(rule, width, height) = {
  let start = if rule.axis == "horizontal" {
    (width * rule.start, (if rule.side == "top" {0pt} else {height}) + rule.offset)
  } else {
    ((if rule.side == "left" {0pt} else {width}) + rule.offset, height * rule.start)
  }
  let end = if rule.axis == "horizontal" {
    (width * rule.end, start.at(1))
  } else {
    (start.at(0), height * rule.end)
  }
  place(top + left, line(start: start, end: end, stroke: rule.thickness + black))
}
#let l2d-table-cell(cell, width, height, first-label, last-label) = context {
  let chosen = cell
  if cell.alternate != none {
    let anchor = if cell.section == "header" {first-label} else {last-label}
    let found = query(label(anchor))
    let terminal = if found.len() > 0 {found.first().location().page()} else {here().page()}
    if ((cell.section == "header" and here().page() != terminal)
      or (cell.section == "footer" and here().page() == terminal)) {
      chosen = cell.alternate
    }
  }
  box(width: width, height: height)[
    #for rule in chosen.rules { l2d-table-rule(rule, width, height) }
    #align(chosen.align)[#pad(left: chosen.padding, right: chosen.padding)[#chosen.body]]
  ]
}
#let l2d-table(columns, minima, cells, first-label, last-label, repeat-head: false, repeat-foot: false) = context {
  let heights = minima
  for cell in cells {
    let width = columns.slice(cell.x, cell.x + cell.colspan).sum()
    let variants = if cell.alternate == none {(cell,)} else {(cell, cell.alternate)}
    let required = calc.max(..variants.map(value => measure(block(width: width - 2 * value.padding, breakable: false, value.body)).height))
    let current = heights.slice(cell.y, cell.y + cell.rowspan).sum()
    if required > current {
      for row in range(cell.y, cell.y + cell.rowspan) {
        heights.at(row) += (required - current) / cell.rowspan
      }
    }
  }
  let render-cell(cell) = {
    let width = columns.slice(cell.x, cell.x + cell.colspan).sum()
    let height = heights.slice(cell.y, cell.y + cell.rowspan).sum()
    table.cell(x: cell.x, y: cell.y, colspan: cell.colspan, rowspan: cell.rowspan,
      breakable: false, inset: 0pt, l2d-table-cell(cell, width, height, first-label, last-label))
  }
  let headers = cells.filter(cell => cell.section == "header").map(render-cell)
  let body = cells.filter(cell => cell.section == "body").map(render-cell)
  let footers = cells.filter(cell => cell.section == "footer").map(render-cell)
  let entries = ()
  if headers.len() > 0 {entries.push(table.header(repeat: repeat-head, ..headers))}
  entries += body
  if footers.len() > 0 {entries.push(table.footer(repeat: repeat-foot, ..footers))}
  table(columns: columns, rows: heights, gutter: 0pt, inset: 0pt, stroke: none, ..entries)
}
