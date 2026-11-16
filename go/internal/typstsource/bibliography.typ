// The styled bibliography (decision 172): Pergamon prints references.bib with the style in
// bibstyle.typ. Each entry's first cell is an ltd-bibitem figure numbered with the label the style
// printed and labelled cite-KEY, as a literal entry is, so references and checks reach it.
#import "vendor/pergamon/lib.typ" as pergamon
#import "bibstyle.typ" as bibstyle

#let ltd-bib-style = bibstyle.style
// Each printed entry's .bib key and the label its citations reach (none: no citation reaches it).
#let ltd-bib-labels = (/*LABELS*/)
#let ltd-bib-numbered = /*NUMBERED*/

// An entry's text as runs of (text, italic, bold), for the writer's check (--input ltd-bibcheck=1). A
// quote is U+E000 (double) or U+E001 (single); a formula is U+E002, its text, U+E003; an element the
// walk does not know is U+E004 and its name, which no entry text equals.
#let ltd-bib-math(it) = {
  if type(it) == str { return it }
  if type(it) == array { return it.map(ltd-bib-math).join() }
  if type(it) != content { return "" }
  if it.func() == text { return it.text }
  it.fields().values().map(ltd-bib-math).join()
}
#let ltd-bib-runs(it, italic: false, bold: false) = {
  if type(it) == str { return ((it, italic, bold),) }
  if type(it) != content { return () }
  let f = repr(it.func())
  let walk(x, italic: italic, bold: bold) = ltd-bib-runs(x, italic: italic, bold: bold)
  if f == "text" {
    let slanted = it.at("style", default: "normal") != "normal"
    let heavy = it.at("weight", default: "regular") in ("semibold", "bold", "extrabold", "black")
    return ((it.text, italic != slanted, bold or heavy),)
  }
  if f == "emph" { return walk(it.body, italic: not italic) }
  if f == "strong" { return walk(it.body, bold: true) }
  if f in ("space", "linebreak", "parbreak") { return ((" ", italic, bold),) }
  if f == "smartquote" { return ((if it.double { "\u{E000}" } else { "\u{E001}" }, italic, bold),) }
  if f == "equation" { return (("\u{E002}" + ltd-bib-math(it.body) + "\u{E003}", false, false),) }
  if f in ("metadata", "h", "v", "counter-update", "state-update", "label") { return () }
  if it.has("children") { return it.children.map(c => walk(c)).join(default: ()) }
  if it.has("body") { return walk(it.body) }
  if it.has("child") { return walk(it.child) }
  if it.has("text") { return ((it.text, italic, bold),) }
  (("\u{E004}" + f, false, false),)
}
#let ltd-bib-plain(it) = ltd-bib-runs(it).map(r => r.at(0)).join(default: "").trim()

#let ltd-bib-reference(index, reference) = {
  let cells = (bibstyle.style.reference-style)(index, reference)
  let first = cells.at(0)
  let shown = ltd-bib-plain(first)
  if shown.starts-with("[") and shown.ends-with("]") { shown = shown.slice(1, -1) }
  let name = ltd-bib-labels.at(reference.entry_key, default: none)
  let item = figure(kind: "ltd-bibitem", supplement: none, outlined: false,
    numbering: if ltd-bib-numbered { _ => shown } else { none }, first)
  let head = if name == none { item } else [#item#label(name)]
  if sys.inputs.at("ltd-bibcheck", default: none) != none {
    head = [#metadata((key: reference.entry_key, cells: cells.map(c => ltd-bib-runs(c))))<ltd-bibcheck>] + head
  }
  (head, ..cells.slice(1))
}

// The bibliography where the paper printed it: its entries only, under the paper's own heading.
#let ltd-bibliography() = {
  let options = dictionary(bibstyle).at("print-options", default: (:))
  let chosen = options.at("filter", default: reference => true)
  options.insert("filter", reference => reference.entry_key in ltd-bib-labels and chosen(reference))
  pergamon.print-bibliography(format-reference: ltd-bib-reference, title: none, show-all: true, ..options)
}
