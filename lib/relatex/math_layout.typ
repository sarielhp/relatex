// Native presentation operations. No formula images or renderer subprocesses.
#let l2d-natural(body) = [
  // Inline math normally consumes paragraph-leading slack at both edges. That
  // is useful in prose, but corrupts measured ascent/depth and placed box ink.
  #set par(leading: 0pt)
  #set text(top-edge: 0pt, bottom-edge: 0pt)
  #body
]
#let l2d-metrics(body) = {
  let measured = measure(l2d-natural(body))
  // Two known-baseline struts recover ascent/descent from layout, rather than
  // assuming a font's depth is a fixed fraction of its height.
  let guard = measured.height + 1000pt
  let upper = box(width: 0pt, height: guard, baseline: bottom)
  let lower = box(width: 0pt, height: guard, baseline: top)
  let above = measure(l2d-natural(math.equation(block: false, lower + body))).height - guard
  let below = measure(l2d-natural(math.equation(block: false, upper + body))).height - guard
  (width: measured.width, height: above, depth: below)
}
#let l2d-length(spec, natural: 0pt, metrics: (width: 0pt, height: 0pt, depth: 0pt)) = {
  if spec == none { return natural }
  let (amount, unit, adjustment) = spec
  let scale = if unit == "em" { 1em }
    else if unit == "ex" { measure(text(top-edge: "x-height", bottom-edge: "baseline", "x")).height }
    else if unit == "px" { 0.75pt }
    else if unit == "pt" { 1pt }
    else if unit == "pc" { 12pt }
    else if unit == "in" { 1in }
    else if unit == "cm" { 1cm }
    else if unit == "mm" { 1mm }
    else if unit == "mu" { 1em / 18 }
    else if unit == "%" { natural / 100 }
    else if unit in ("width", "height", "depth") { metrics.at(unit) }
    else { panic("Unsupported mathematical length unit: " + unit) }
  let result = amount * scale
  if adjustment { natural + result } else { result }
}
#let l2d-padded(body, width: none, height: none, depth: none, lspace: none, voffset: none, body-metrics: none) = context {
  let body = l2d-natural(body)
  let metrics = if body-metrics == none { l2d-metrics(body) } else { body-metrics }
  let w = l2d-length(width, natural: metrics.width, metrics: metrics)
  let h = l2d-length(height, natural: metrics.height, metrics: metrics)
  let d = l2d-length(depth, natural: metrics.depth, metrics: metrics)
  let x = if lspace == none { 0pt } else { l2d-length(lspace, natural: metrics.width, metrics: metrics) }
  let y = if voffset == none { 0pt } else { l2d-length(voffset, natural: metrics.height, metrics: metrics) }
  box(width: calc.max(0pt, w), height: calc.max(0pt, h + d), baseline: (at: top, shift: -h))[
    #place(top + left, dx: x, dy: h - metrics.height - y, body)
  ]
}
#let l2d-space(width, height: none, depth: none, fill: none) = context {
  let w = l2d-length(width)
  if height == none and depth == none and fill == none { h(w) }
  else {
    let a = l2d-length(height)
    let d = l2d-length(depth)
    box(width: calc.max(0pt, w), height: calc.max(0pt, a + d), baseline: (at: top, shift: -a), fill: fill)
  }
}
#let l2d-frac(numerator, denominator, thickness, display: false, level: 0) = context {
  let style = (math.inline, math.script, math.sscript).at(if display { 0 } else { calc.min(2, level + 1) })
  let numerator-body = l2d-natural(math.equation(style(numerator.body)))
  let denominator-body = l2d-natural(math.equation(style(denominator.body)))
  let a = l2d-metrics(numerator-body)
  let b = l2d-metrics(denominator-body)
  let width = calc.max(a.width, b.width) + 0.2em
  let rule = l2d-length(thickness)
  let gap = if display { 0.2em } else { 0.12em }
  let height = a.height + a.depth + gap + rule + gap + b.height + b.depth
  let baseline = height / 2 + 0.25em
  box(width: width, height: height, baseline: (at: top, shift: -baseline))[
    #place(top + center, numerator-body)
    #if rule > 0pt { place(top + left, dy: a.height + a.depth + gap, line(length: width, stroke: rule)) }
    #place(top + center, dy: a.height + a.depth + gap + rule + gap, denominator-body)
  ]
}
#let l2d-fenced(body, minsize: none, maxsize: none, reference: none, axis-body: none) = context {
  let metrics = l2d-metrics(if reference == none { body } else { reference })
  let axis-metrics = l2d-metrics(if axis-body == none { math.equation(sym.plus) } else { axis-body })
  let axis = (axis-metrics.height - axis-metrics.depth) / 2
  let natural = 2 * calc.max(metrics.height - axis, metrics.depth + axis)
  let lower = if minsize == none { 0pt } else { l2d-length(minsize, natural: 1em) }
  let upper = if maxsize == none { calc.max(lower, natural) } else { l2d-length(maxsize, natural: 1em) }
  assert(lower <= upper, message: "Inconsistent mathematical fence size bounds")
  math.lr(size: calc.min(upper, calc.max(lower, natural)), body)
}
#let l2d-matrix(rows, alignments: none, vertical-alignments: none,
  column-gaps: ((0.5, "em", false),), row-gaps: ((0.3, "em", false),)) = context {
  let rows = rows.map(row => row.map(cell => l2d-natural(cell)))
  let columns = rows.first().len()
  let metrics = rows.map(row => row.map(cell => l2d-metrics(cell)))
  let widths = range(columns).map(x => calc.max(..metrics.map(row => row.at(x).width)))
  let ascents = metrics.map(row => calc.max(..row.map(cell => cell.height)))
  let descents = metrics.map(row => calc.max(..row.map(cell => cell.depth)))
  let column-spacing = range(calc.max(0, columns - 1)).map(x => l2d-length(column-gaps.at(calc.min(x, column-gaps.len() - 1))))
  let row-spacing = range(calc.max(0, rows.len() - 1)).map(y => l2d-length(row-gaps.at(calc.min(y, row-gaps.len() - 1))))
  let cells = ()
  for (y, row) in rows.enumerate() {
    for (x, cell) in row.enumerate() {
      let metric = metrics.at(y).at(x)
      let height = ascents.at(y) + descents.at(y)
      let horizontal = if alignments == none { "center" } else { alignments.at(y).at(x) }
      let vertical = if vertical-alignments == none { "baseline" } else { vertical-alignments.at(y).at(x) }
      let alignment = if horizontal == "left" { left } else if horizontal == "right" { right } else { center }
      let offset = if vertical == "top" { 0pt }
        else if vertical == "bottom" { height - metric.height - metric.depth }
        else if vertical == "center" { (height - metric.height - metric.depth) / 2 }
        else { ascents.at(y) - metric.height }
      cells.push(box(width: widths.at(x), height: height, baseline: (at: top, shift: -ascents.at(y)))[
        #place(top + alignment, dy: offset, cell)
      ])
    }
  }
  let body = grid(columns: widths, column-gutter: column-spacing, row-gutter: row-spacing, ..cells)
  box(baseline: horizon, body)
}
