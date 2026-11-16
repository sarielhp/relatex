// Helpers for a paper converted from LaTeX by relatex (prefix.typ). main.typ imports them and
// applies the rules with `#show: relatex-setup`: show and set rules of an imported module do not apply to
// the importer, so they are the function relatex-setup, which wraps the paper.

// A theorem-like environment: a numbered block with a bold head, like amsthm's.
// A restated theorem (thm-restate) shows its number again without counting.
#let ltd-theorem(kind, supplement, numbering: "1", body-style: emph, head-style: strong) = (body, title: none, number: auto, restated: false) => figure(
  kind: kind, supplement: supplement, outlined: false,
  numbering: if restated or number == none { none } else if number == auto { numbering } else { _ => number },
  block(width: 100%, align(left, {
    head-style({
      supplement
      if number != none [ #context counter(figure.where(kind: kind)).display(if number == auto { numbering } else { _ => number })]
    })
    if title != none [ (#title)]
    head-style[.]
    [ ]
    body-style(body)
  })),
)
#let proof(body, title: [Proof]) = block(width: 100%, {
  emph(title)
  [. ]
  body
  h(1fr)
  sym.square.stroked
})

#let abstract(body, title: [Abstract]) = block(width: 100%, inset: (x: 2em), {
  align(center, strong(title))
  parbreak()
  body
})

// A bibliography entry printed with the label the paper printed ([12], [ADM25]), or none
// (author-year).
#let bibitem(printed, body) = figure(kind: "ltd-bibitem", supplement: none, outlined: false,
  numbering: if printed == none { none } else { _ => printed },
  block(width: 100%, align(left, if printed == none { body } else [\[#printed\] #body])))
// An author-year citation as the paper printed it, linked to its first entry.
#let citeas(..args) = { let a = args.pos(); link(a.first(), a.last()) }

// The heading numbering segment (main matter, an appendix in letters).
#let ltd-segment = state("ltd-segment", "main")

// The rules every paper converted by relatex shares, applied to the paper that follows `#show: relatex-setup`.
#let relatex-setup(body) = {
  set ref(supplement: none)
  // \ref to a labelled footnote prints its number as text, not another footnote mark.
  show ref: it => if it.element != none and it.element.func() == footnote {
    link(it.element.location(), context numbering(it.element.numbering, ..counter(footnote).at(it.element.location())))
  } else { it }
  // Theorem kinds are ltd-thm-<counter>: environments sharing a LaTeX counter share a kind.
  show figure: it => if type(it.kind) == str and it.kind.starts-with("ltd-thm") { it.body } else { it }
  show figure.where(kind: "ltd-bibitem"): it => it.body
  body
}
