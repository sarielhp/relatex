// sariel_biblatex.typ: the author's biblatex style (sariel_biblatex.sty) as a
// Pergamon 0.8.0 style (phase 2 of the Typst bibliography work).
//
// The .sty is biblatex `alphabetic` (standard.bbx drivers) with abbreviated strings, initials, no "In:",
// fields cleared per entry type, its own article and inproceedings layouts and sentence case for
// article, inproceedings and incollection titles. This file reproduces it with Pergamon's hooks:
//
// - `format-functions` replaces every bibliography driver (`driver-article`, ...) with a port of
//   standard.bbx's driver, run by a small model of biblatex's punctuation buffer (`run` below:
//   \newunit, \newblock, \setunit, \setunit*, \printunit and \printtext, the unit and block
//   punctuation, and the period, comma and colon rules of biblatex's punctuation tracker). Pergamon's
//   own drivers join with fixed separators and stop the document on a missing field.
// - `suppress-fields` holds the .sty's \AtEveryBibitem: inproceedings lose editor, publisher, series and
//   location; books lose publisher.
// - The citation style is Pergamon's alphabetic one with the .sty's maxalphanames=4 and minalphanames=3,
//   and its label generator is replaced by biber's labelalpha template (`label-generator`).
// - Names: every given name becomes initials ("J.-P."), with the prefix kept ("A. F. van der Stappen").
// - Titles link to the DOI, else the URL (`sariel:titlelink`).
//
// The bibliography data comes from relatex bibclean (decision 164): field values are Typst markup, and a
// brace group that protects case in a title is a content block, {#[VC]}, which the sentence case leaves
// alone.
#import "@preview/pergamon:0.8.0": *

// ---------------------------------------------------------------------------------------------------
// Strings: biblatex's english.lbx, abbreviated (abbreviate=true).

#let strings = (
  pages: "pp.", page: "p.", volume: "vol.", volumes: "vols.", editor: "ed.", editors: "eds.",
  byeditor: "ed. by", edition: "ed.", chapter: "chap.", number: "no.", andothers: "et al.", "and": "and",
  nodate: "n.d.", phdthesis: "PhD thesis", mathesis: "MA thesis", techreport: "tech. rep.",
  resreport: "research rep.", jourser: "ser.", translator: "trans.", bytranslator: "trans. by",
  version: "version", newseries: "new ser.", oldseries: "old ser.", candthesis: "Cand. thesis",
  software: "comp. software", datacd: "CD-ROM", audiocd: "audio CD", "in": "in", part: "part",
  issue: "issue", urlseen: "visited on", byauthor: "by",
)
#let month-names = ("Jan.", "Feb.", "Mar.", "Apr.", "May", "June", "July", "Aug.", "Sept.", "Oct.",
  "Nov.", "Dec.")
#let month-keys = ("january", "february", "march", "april", "may", "june", "july", "august",
  "september", "october", "november", "december")

#let upper-first(s) = {
  if s == none or s.len() == 0 { return s }
  let cs = s.clusters()
  upper(cs.first()) + cs.slice(1).sum(default: "")
}

// ---------------------------------------------------------------------------------------------------
// The punctuation buffer. A driver is an array of steps: content (a printed field; none prints
// nothing) or a dictionary made by the functions below.

#let newunit = (op: "newunit")
#let newblock = (op: "newblock")
#let setunit(p) = (op: "setunit", p: p)
#let setunit-star(p) = (op: "setunit-star", p: p)
#let printunit(p) = (op: "printunit", p: p)
// \printtext[wrap]{steps}: the steps' output, when there is any, wrapped and printed as one field.
#let printtext(wrap, ..steps) = (op: "printtext", wrap: wrap, steps: steps.pos())
// \nopunct: the next punctuation mark is not printed (its space is).
#let nopunct = (op: "nopunct")
// A field whose text depends on the start of a sentence (a bibstring is capitalized there):
// f(sentence-start) -> content or none.
#let sentence(f) = (op: "sentence", f: f)

// Punctuation marks: [mark, space after it].
#let marks = (
  period: (".", " "), comma: (",", " "), colon: (":", " "), semicolon: (";", " "), space: ("", " "),
  dot: (".", ""), dotspace: (".", " "), "none": ("", ""),
)

// What a piece of content ends in, when its text alone does not tell: math (biblatex sees the math, the text
// extraction nothing) or a pair of parentheses the style itself added (\mkbibparens resets the tracker, so
// "??(??)" still takes a period, while a ")" in field text passes the "?" before it on).
#let closer = metadata("bib-closer")
#let tail-kind(x) = {
  if type(x) != content { return none }
  if x.func() == metadata { return if x.value == "bib-closer" { "closer" } else { none } }
  if x.func() == math.equation { return "math" }
  if x.has("children") {
    let cs = x.children.filter(c => c.func() != [ ].func())
    return if cs.len() > 0 { tail-kind(cs.last()) } else { none }
  }
  if x.has("body") and type(x.body) == content { return tail-kind(x.body) }
  none
}

// The text a printed piece of content ends with, as the punctuation tracker sees it.
#let last-chars(x) = {
  if x == none { return "" }
  let kind = tail-kind(x)
  if kind != none { return "0" }
  let s = if type(x) == str { x } else { content-to-string(x) }
  if s == none { "" } else { s.trim(at: end) }
}

// TeX gives ) ] and closing quotes space factor code 0: they keep the mark before them ("(FOCS 2000?)" still ends
// in "?"), so the tracker looks through them.
#let tracked-end(text) = text.trim(regex("[)\]’”'\"]+"), at: end)
// Whether text ending in END ends a sentence for biblatex's tracker. TeX's space factor makes a period
// after a capital an abbreviation, one after anything else a full stop.
#let ends-sentence(end) = {
  let end = tracked-end(end)
  if end.ends-with("?") or end.ends-with("!") { return true }
  if not end.ends-with(".") { return false }
  let cs = end.clusters()
  if cs.len() < 2 { return true }
  let before = cs.at(-2)
  not (upper(before) == before and lower(before) != before)
}

// The mark biblatex's \addperiod, \addcomma, ... prints: "" when the tracker suppresses it. LASTMARK is
// a mark biblatex itself just added (none: the text's own end decides).
#let mark-after(mark, end, lastmark) = {
  if mark == "" { return "" }
  if lastmark != none and lastmark != "" { return "" }
  let end = tracked-end(end)
  let last = if end.len() > 0 { end.clusters().last() } else { "" }
  if last in (",", ";", ":") { return "" }
  if mark == "." { return if last in (".", "?", "!") { "" } else { mark } }
  // A comma, colon or semicolon that \blx@begunit adds is printed after the field's own period.
  mark
}

// Runs a driver's steps; none when nothing was printed.
#let run(steps) = {
  let out = ()
  let st = (insert: false, unit: false, block: false, punct: "period", keep: false, lastins: false, end: "",
    nopunct: false, mark: none)
  for step in steps {
    let op = if type(step) == dictionary { step.at("op", default: "field") } else { "field" }
    if op == "newunit" {
      if not st.keep { st.punct = "period"; st.unit = true }
    } else if op == "newblock" {
      st.block = true
    } else if op == "setunit" {
      if not st.keep { st.punct = step.p; st.unit = true }
    } else if op == "setunit-star" {
      if st.lastins and not st.keep { st.punct = step.p; st.unit = true }
    } else if op == "nopunct" {
      st.nopunct = true
    } else if op == "printunit" {
      st.punct = step.p
      st.unit = true
      st.keep = true
    } else {
      // \blx@begunit: the pending unit punctuation, then the block's period.
      let pre = ""
      let lastmark = none
      let start = not st.insert
      if st.insert {
        if st.unit {
          let (mark, space) = marks.at(st.punct)
          lastmark = if st.nopunct { "" } else { mark-after(mark, st.end, st.mark) }
          pre = lastmark + space
        }
        if st.block and (lastmark == none or lastmark == "") {
          let m = if st.nopunct { "" } else { mark-after(".", st.end, lastmark) }
          if m != "" { pre = m + " "; lastmark = m } else if pre == "" { pre = " " }
        }
        start = if lastmark == "." { true } else if lastmark != none and lastmark != "" { false } else {
          ends-sentence(st.end)
        }
        // after \nopunct no sentence starts ("Comput. Geom. vol. 3742")
        if st.nopunct { start = false }
      }
      let body = if op == "printtext" {
        let inner = run(step.steps)
        if inner == none { none } else { (step.wrap)(inner) }
      } else if op == "sentence" {
        (step.f)(start)
      } else { step }
      if body == none {
        st.lastins = false
      } else {
        // biblatex's \addcomma and friends \unspace first: the space an empty unit left goes ("99?, 1–3").
        if lastmark != none and lastmark != "" {
          while out.len() > 0 and last-chars(out.last()) == "" { let _ = out.pop() }
        }
        out.push(pre)
        out.push(body)
        st.insert = true
        st.lastins = true
        st.unit = false
        st.block = false
        st.keep = false
        let text = last-chars(body)
        if text != "" { st.end = text; st.mark = none; st.nopunct = false } else if lastmark != none and lastmark != "" {
          st.end = st.end + lastmark
          st.mark = lastmark
        } else if lastmark == "" and not st.nopunct and ends-sentence(st.end) and tracked-end(st.end).ends-with(".") {
          // An empty unit after a field's own period ("To appear.") leaves the tracker at a period, so the
          // next comma is dropped ("To appear. 2009", "Assoc. Comput. Mach. 1186–1195").
          st.mark = "."
        }
      }
    }
  }
  if out.len() == 0 { none } else { out.join() }
}

// ---------------------------------------------------------------------------------------------------
// Fields.

#let ev(s) = if s == none { none } else { eval(s, mode: "markup") }
#let raw-field(reference, field, options) = {
  let v = fd(reference, field, options)
  if v == none or (type(v) == str and v.trim() == "") { none } else { v }
}
#let field(reference, name, options) = ev(raw-field(reference, name, options))
#let has(reference, name, options) = raw-field(reference, name, options) != none
#let is-integer(s) = s.match(regex("^\d+$")) != none
#let ordinal(n) = {
  let i = int(n)
  let suffix = if calc.rem(calc.quo(i, 10), 10) == 1 { "th" } else {
    ("th", "st", "nd", "rd", "th", "th", "th", "th", "th", "th").at(calc.rem(i, 10))
  }
  str(i) + suffix
}
// A bibstring, capitalized at the start of a sentence.
#let bibstring(key, start) = {
  let s = strings.at(key, default: key)
  if start { upper-first(s) } else { s }
}
#let parens(x) = [(#x)#closer]
#let brackets(x) = [[#x]#closer]

// ---------------------------------------------------------------------------------------------------
// Names: given names as initials (giveninits), "given prefix family suffix".

// The words of a given name: split at SEPARATORS outside a protected #[...] (a braced token).
#let given-words(given, separators: (" ", "~", "\u{00A0}")) = {
  let words = ()
  let word = ""
  let depth = 0
  for c in given.clusters() {
    if c == "[" and word.ends-with("#") { depth += 1 } else if c == "]" and depth > 0 { depth -= 1 }
    if depth == 0 and c in separators {
      if word != "" { words.push(word) }
      word = ""
    } else { word += c }
  }
  if word != "" { words.push(word) }
  words
}
// The first letter of a word, skipping the markup of a protected token.
#let first-letter(word) = {
  let m = word.match(regex("\w"))
  if m == none { word.clusters().first() } else { word.slice(m.start).clusters().first() }
}
// biber's initials (giveninits): one per word, "J.-P." for a hyphenated word, one for a protected token
// ({Hsien{-}Chih} gives "H.").
#let initials(given) = {
  given-words(given).map(word => {
    if word.starts-with("#[") { return first-letter(word) + "." }
    // a braced hyphen does not split ("Shang{-}Hua" gives "S.")
    given-words(word, separators: ("-",)).map(part => first-letter(part) + ".").join("-")
  }).join(" ")
}
#let format-person(name) = {
  let parts = ()
  let given = name.at("given", default: "")
  if given != "" { parts.push(initials(given)) }
  let prefix = name.at("prefix", default: "")
  if prefix != "" { parts.push(prefix) }
  parts.push(name.at("family", default: ""))
  let suffix = name.at("suffix", default: "")
  if suffix != "" { parts.push(suffix) }
  parts.join(" ")
}
#let is-others(name) = name.at("family", default: "") == "others" and name.at("given", default: "") == ""
// \printnames: maxnames=40, minnames=1; "A and B", "A, B, and C".
#let names(list) = {
  if list == none or list.len() == 0 { return none }
  let others = is-others(list.last())
  let people = if others { list.slice(0, -1) } else { list }
  if people.len() > 40 { people = people.slice(0, 1); others = true }
  let shown = people.map(format-person)
  let joined = if others {
    // english.lbx's andothersdelim: \finalandcomma when more than one name is shown ("A, B, et al.").
    shown.join(", ") + if shown.len() > 1 { ", " } else { " " } + strings.andothers
  } else if shown.len() == 1 {
    shown.first()
  } else if shown.len() == 2 {
    shown.join(" and ")
  } else {
    shown.slice(0, -1).join(", ") + ", and " + shown.last()
  }
  ev(joined)
}
#let name-list(reference, name, options) = {
  if name in options.suppressed-fields { return none }
  let parsed = reference.fields.at("parsed-" + name, default: none)
  if parsed == none or parsed.len() == 0 { none } else { parsed }
}
#let name-count(list) = if list == none { 0 } else if is-others(list.last()) { 99 } else { list.len() }

// ---------------------------------------------------------------------------------------------------
// Sentence case (\MakeSentenceCase*): the first letter upper, every other letter outside math, a
// protected content block #[...] and a function name lower.

#let sentence-case(s) = {
  let cs = s.clusters()
  let out = ()
  let i = 0
  let first = true
  let n = cs.len()
  while i < n {
    let c = cs.at(i)
    if c == "\\" and i + 1 < n {
      out.push(c + cs.at(i + 1))
      first = false
      i += 2
    } else if c == "$" {
      let j = i + 1
      while j < n and cs.at(j) != "$" { j += 1 }
      out.push(cs.slice(i, calc.min(j + 1, n)).join())
      first = false
      i = j + 1
    } else if c == "#" {
      // #name[...] keeps its name; #[...] is protected, as is #name(...) and #sym.x
      let j = i + 1
      while j < n and cs.at(j).match(regex("[\w.-]")) != none { j += 1 }
      let name = cs.slice(i + 1, j).sum(default: "")
      out.push(cs.slice(i, j).join())
      if j < n and cs.at(j) == "[" and name == "" {
        let depth = 0
        let k = j
        while k < n {
          if cs.at(k) == "\\" { k += 2; continue }
          if cs.at(k) == "[" { depth += 1 }
          if cs.at(k) == "]" { depth -= 1; if depth == 0 { break } }
          k += 1
        }
        out.push(cs.slice(j, calc.min(k + 1, n)).join())
        first = false
        i = k + 1
      } else {
        i = j
        if j < n and cs.at(j) == ";" { out.push(";"); i = j + 1 }
      }
    } else if c.match(regex("\w")) != none {
      out.push(if first { upper(c) } else { lower(c) })
      first = false
      i += 1
    } else {
      // only a title's first character is upper-cased: "(approximate) ..." stays lower
      if c.trim() != "" and c not in ("[", "]") { first = false }
      out.push(c)
      i += 1
    }
  }
  out.sum(default: "")
}

// ---------------------------------------------------------------------------------------------------
// Titles: sentence case for article, inproceedings and incollection; italic elsewhere; linked to the
// DOI, else the URL (sariel:titlelink).

#let sentence-case-types = ("article", "inproceedings", "incollection")
#let title-link(reference, body) = {
  let doi = reference.fields.at("doi", default: none)
  let url = reference.fields.at("url", default: none)
  if doi != none { link("https://doi.org/" + doi, body) } else if url != none { link(url, body) } else { body }
}
#let titlecase(reference, s) = {
  if s == none { none } else if reference.entry_type in sentence-case-types { ev(sentence-case(s)) } else { ev(s) }
}
#let plain-title-types = ("article", "inproceedings", "incollection")
// biblatex.def's title macro with the .sty's title format.
#let title-macro(reference, options) = {
  let t = raw-field(reference, "title", options)
  let sub = raw-field(reference, "subtitle", options)
  let steps = ()
  if t != none or sub != none {
    let wrap = if reference.entry_type in plain-title-types { x => title-link(reference, x) } else {
      x => title-link(reference, emph(x))
    }
    steps.push(printtext(wrap, titlecase(reference, t), setunit("period"), titlecase(reference, sub)))
    steps.push(newunit)
  }
  steps.push(field(reference, "titleaddon", options))
  steps
}

// ---------------------------------------------------------------------------------------------------
// Macros of standard.bbx and biblatex.def, with the .sty's renewals.

#let author-macro(reference, options) = (names(name-list(reference, "author", options)),)
#let editor-string(list) = if name-count(list) > 1 { "editors" } else { "editor" }
#let editor-others(reference, options) = {
  let list = name-list(reference, "editor", options)
  if list == none { return () }
  (names(list), setunit("comma"), sentence(start => bibstring(editor-string(list), start)))
}
#let translator-others(reference, options) = {
  let list = name-list(reference, "translator", options)
  if list == none { return () }
  let s = if name-count(list) > 1 { "translators" } else { "translator" }
  (names(list), setunit("comma"), sentence(start => bibstring(s, start)))
}
#let author-translator-others(reference, options) = {
  if name-list(reference, "author", options) != none { author-macro(reference, options) } else {
    translator-others(reference, options)
  }
}
#let author-editor-others-translator-others(reference, options) = {
  if name-list(reference, "author", options) != none { author-macro(reference, options) }
  else if name-list(reference, "editor", options) != none { editor-others(reference, options) }
  else { translator-others(reference, options) }
}
// editor+others clears the editor once it printed it as the name (books, proceedings, misc without author).
#let byeditor-others(reference, options, named: false) = {
  let steps = ()
  let list = name-list(reference, "editor", options)
  if list != none and not (named and name-list(reference, "author", options) == none) {
    steps += (sentence(start => [#bibstring("byeditor", start) #names(list)]), newunit)
  }
  let tr = name-list(reference, "translator", options)
  if tr != none {
    steps += (sentence(start => [#bibstring("bytranslator", start) #names(tr)]), newunit)
  }
  steps
}
#let byauthor(reference, options) = ()
#let language(reference, options) = (field(reference, "language", options),)

// The .sty's journal macro: \printtext[journaltitle] (italic) of the plain journal title.
#let journal-macro(reference, options) = {
  let jt = raw-field(reference, "journaltitle", options)
  let js = raw-field(reference, "journalsubtitle", options)
  let steps = ()
  if jt != none or js != none {
    steps.push(printtext(emph, ev(jt), setunit("period"), ev(js)))
  }
  steps.push(field(reference, "journaltitleaddon", options))
  steps
}
// The .sty's volume+number+eid: \printunit{, } volume number(parens in articles) \setunit{, } eid.
#let volume-number-eid(reference, options) = {
  let number = field(reference, "number", options)
  if number != none and reference.entry_type == "article" { number = parens(number) }
  (printunit("comma"), field(reference, "volume", options), number, setunit("comma"),
    field(reference, "eid", options))
}
#let date-text(reference, options, start) = {
  let d = reference.fields.at("parsed-date", default: none)
  if d == none or "year" not in d {
    // A year biber cannot read as a date ("to appear", "March 1997") prints as written.
    let raw = raw-field(reference, "year", options)
    return if raw == none { none } else { ev(raw) }
  }
  let month = d.at("month", default: none)
  if "month" in options.suppressed-fields { month = none }
  let m = if month == none { none } else if type(month) == int and month >= 1 and month <= 12 {
    month-names.at(month - 1)
  } else if type(month) == str { month } else { none }
  if m == none { str(d.year) } else { m + " " + str(d.year) }
}
#let date-types-without-date = ("article", "inproceedings")
// \usebibmacro{date}: the .sty's date format is empty for articles and inproceedings, but \printdate
// still counts as printed and so takes the pending punctuation.
#let date-macro(reference, options) = {
  if reference.entry_type in date-types-without-date {
    let d = reference.fields.at("parsed-date", default: none)
    (if d != none and "year" in d { "" } else { none },)
  } else {
    (sentence(start => date-text(reference, options, start)),)
  }
}
// The .sty's issue+date: articles print only the issue in parentheses.
#let issue-date(reference, options) = {
  if reference.entry_type == "article" {
    let issue = field(reference, "issue", options)
    (if issue == none { none } else { parens(issue) }, newunit)
  } else {
    (printtext(parens, field(reference, "issue", options), setunit-star("space"),
      ..date-macro(reference, options)), newunit)
  }
}
#let issue-macro(reference, options) = {
  let it = raw-field(reference, "issuetitle", options)
  let ist = raw-field(reference, "issuesubtitle", options)
  let steps = ()
  if it != none or ist != none {
    steps += (printtext(emph, ev(it), setunit("period"), ev(ist)), newunit)
  }
  steps.push(field(reference, "issuetitleaddon", options))
  steps
}
#let series-field(reference, options) = {
  let s = raw-field(reference, "series", options)
  if s == none { return none }
  if reference.entry_type in ("article", "periodical") and is-integer(s) {
    // The bibstring follows the ordinal, so it never starts a sentence ("2nd ser.").
    return ordinal(s) + " " + bibstring("jourser", false)
  }
  if reference.entry_type in ("article", "periodical") and s in strings { return sentence(start => bibstring(s, start)) }
  ev(s)
}
#let journal-issuetitle(reference, options) = {
  let steps = journal-macro(reference, options)
  steps.push(setunit-star("space"))
  if has(reference, "series", options) {
    steps += (newunit, series-field(reference, options), setunit("space"))
  }
  steps += volume-number-eid(reference, options)
  steps.push(setunit("space"))
  steps += issue-date(reference, options)
  steps.push(setunit("colon"))
  steps += issue-macro(reference, options)
  steps.push(newunit)
  steps
}
#let pages-field(reference, options) = {
  let p = raw-field(reference, "pages", options)
  if p == none { return none }
  // biber reads pages as a range list: any dash run becomes \bibrangedash (913---917 prints 913–917).
  p = p.replace(regex("\s*(-+|–|—)\s*"), "–")
  if reference.entry_type in ("article", "inproceedings") { return ev(p) }
  // \mkpageprefix adds p./pp. only to numbers (63.1 too), ranges and lists of them (\ifpages): "??" prints bare.
  let numeral = "[0-9]+(\\.[0-9]+)*|[ivxlcdm]+|[IVXLCDM]+"
  if p.match(regex("^\s*(" + numeral + ")(\s*[–,;&+]\s*(" + numeral + "))*\s*$")) == none { return ev(p) }
  let plural = p.match(regex("[-–—,;+&]|\s(and|to)\s")) != none
  sentence(start => [#bibstring(if plural { "pages" } else { "page" }, start) #ev(p)])
}
#let bibpagespunct(reference) = if reference.entry_type == "article" { "colon" } else { "comma" }
// The .sty's note+pages: articles end with the year.
#let note-pages(reference, options) = {
  let steps = (field(reference, "note", options), setunit(bibpagespunct(reference)), pages-field(reference, options))
  if reference.entry_type == "article" { steps += (setunit("comma"), field(reference, "year", options)) }
  steps.push(newunit)
  steps
}
#let chapter-field(reference, options) = {
  let c = raw-field(reference, "chapter", options)
  if c == none { none } else { sentence(start => [#bibstring("chapter", start) #ev(c)]) }
}
// The .sty's chapter+pages: inproceedings print chapter, eid, pages and year after commas.
#let chapter-pages(reference, options) = {
  if reference.entry_type == "inproceedings" {
    (setunit("comma"), chapter-field(reference, options), setunit("comma"), field(reference, "eid", options),
      setunit("comma"), pages-field(reference, options), setunit("comma"), field(reference, "year", options),
      newunit)
  } else {
    let punct = bibpagespunct(reference)
    (chapter-field(reference, options), setunit(punct), field(reference, "eid", options), setunit(punct),
      pages-field(reference, options), newunit)
  }
}
#let maintitle-macro(reference, options) = {
  let t = raw-field(reference, "maintitle", options)
  let s = raw-field(reference, "mainsubtitle", options)
  let steps = ()
  if t != none or s != none {
    steps += (printtext(emph, titlecase(reference, t), setunit("period"), titlecase(reference, s)), newunit)
  }
  steps.push(field(reference, "maintitleaddon", options))
  steps
}
#let volume-field(reference, options) = {
  let v = raw-field(reference, "volume", options)
  if v == none { return none }
  if reference.entry_type in ("article", "periodical") { return ev(v) }
  sentence(start => [#bibstring("volume", start)~#ev(v)])
}
// The .sty's booktitle macro: the booktitle format (italic; in inproceedings followed by a comma when
// pages or a year follow) around the plain booktitle.
#let booktitle-macro(reference, options) = {
  let t = raw-field(reference, "booktitle", options)
  let s = raw-field(reference, "booksubtitle", options)
  let steps = ()
  if t != none or s != none {
    let comma = reference.entry_type == "inproceedings" and (has(reference, "pages", options) or has(reference, "year", options))
    // \addcomma\nopunct inside the format: the tracker drops the comma after a full stop.
    let wrap = if comma {
      // biblatex allows a comma after ? and !, not after any period here ("Providence, R.I. 45–62")
      x => if tracked-end(last-chars(x)).ends-with(".") { emph(x) } else { [#emph(x),] }
    } else { emph }
    steps.push(printtext(wrap, ev(t), setunit("period"), ev(s)))
    if comma { steps.push(nopunct) }
    steps.push(newunit)
  }
  steps.push(field(reference, "booktitleaddon", options))
  steps
}
#let maintitle-booktitle(reference, options) = {
  let steps = ()
  if has(reference, "maintitle", options) {
    steps += maintitle-macro(reference, options)
    steps += (newunit, newblock)
    if has(reference, "volume", options) {
      steps += (volume-field(reference, options), field(reference, "part", options), setunit("colon"))
    }
  }
  steps += booktitle-macro(reference, options)
  steps.push(newunit)
  steps
}
#let maintitle-title(reference, options) = {
  let steps = ()
  let mt = raw-field(reference, "maintitle", options)
  if mt != none and mt != raw-field(reference, "title", options) {
    steps += maintitle-macro(reference, options)
    steps += (newunit, newblock)
    if has(reference, "volume", options) {
      steps += (volume-field(reference, options), field(reference, "part", options), setunit("colon"))
    }
  }
  steps += title-macro(reference, options)
  steps.push(newunit)
  steps
}
#let event-venue-date(reference, options) = {
  (field(reference, "eventtitle", options), newunit, field(reference, "eventtitleaddon", options),
    setunit("space"), printtext(parens, field(reference, "venue", options)), newunit)
}
#let volume-part(reference, options) = {
  if has(reference, "maintitle", options) { () } else {
    (volume-field(reference, options), field(reference, "part", options))
  }
}
#let volumes-field(reference, options) = {
  let v = raw-field(reference, "volumes", options)
  if v == none { none } else { [#ev(v)~#strings.volumes] }
}
#let series-number(reference, options) = {
  (field(reference, "series", options), setunit-star("space"), field(reference, "number", options), newunit)
}
#let list-field(reference, name, options) = {
  let v = raw-field(reference, name, options)
  if v == none { return none }
  let items = v.split(regex("\s+and\s+"))
  let joined = if items.len() == 1 { items.first() } else if items.len() == 2 { items.join(" and ") } else {
    items.slice(0, -1).join(", ") + ", and " + items.last()
  }
  ev(joined)
}
#let xxx-location-date(reference, options, who) = {
  let steps = (list-field(reference, "location", options),)
  if who != none {
    steps.push(setunit-star(if has(reference, who, options) { "colon" } else { "comma" }))
    steps.push(list-field(reference, who, options))
  }
  steps.push(setunit-star("comma"))
  steps += date-macro(reference, options)
  steps.push(newunit)
  steps
}
#let edition-field(reference, options) = {
  let e = raw-field(reference, "edition", options)
  if e == none { return none }
  if is-integer(e) { return ordinal(e) + " " + bibstring("edition", false) }
  ev(e)
}
#let type-field(reference, options) = {
  let t = raw-field(reference, "type", options)
  if t == none { return none }
  if t in strings { return sentence(start => bibstring(t, start)) }
  ev(t)
}
#let eprint-field(reference, options) = {
  let e = raw-field(reference, "eprint", options)
  if e == none { return none }
  let kind = raw-field(reference, "eprinttype", options)
  let class = raw-field(reference, "eprintclass", options)
  if kind != none and lower(kind) == "arxiv" {
    let body = if class == none { raw(e) } else { [#raw(e) #raw("[" + class + "]")] }
    return [arXiv: #link("https://arxiv.org/abs/" + e, body)]
  }
  let label = if kind == none { "eprint" } else { kind }
  let body = if class == none { raw(e) } else { [#raw(e) (#class)] }
  [#label: #body]
}
#let doi-eprint-url(reference, options) = (newunit, newblock, eprint-field(reference, options), newunit, newblock)
#let addendum-pubstate(reference, options) = {
  (field(reference, "addendum", options), newunit, newblock, field(reference, "pubstate", options))
}
#let begin(reference, options, name-steps) = {
  name-steps + (setunit("period"), newblock)
}
#let finish(reference, options) = {
  (newunit, newblock) + doi-eprint-url(reference, options) + (newunit, newblock) + addendum-pubstate(reference, options)
}

// ---------------------------------------------------------------------------------------------------
// Drivers (standard.bbx).

#let ublock = (newunit, newblock)

#let driver-article(reference, options) = run(
  begin(reference, options, author-translator-others(reference, options))
  + title-macro(reference, options) + (newunit,) + language(reference, options) + ublock
  + byauthor(reference, options) + ublock
  + (field(reference, "version", options),) + ublock
  + journal-issuetitle(reference, options) + (newunit,)
  + byeditor-others(reference, options) + (newunit,)
  + note-pages(reference, options)
  + finish(reference, options))

#let driver-inproceedings(reference, options) = run(
  begin(reference, options, author-translator-others(reference, options))
  + title-macro(reference, options) + (newunit,) + language(reference, options) + ublock
  + byauthor(reference, options) + ublock
  + maintitle-booktitle(reference, options) + ublock
  + event-venue-date(reference, options) + ublock
  + byeditor-others(reference, options) + ublock
  + volume-part(reference, options) + (newunit, volumes-field(reference, options)) + ublock
  + series-number(reference, options) + ublock
  + (field(reference, "note", options),) + ublock
  + (list-field(reference, "organization", options), newunit)
  + xxx-location-date(reference, options, "publisher") + ublock
  + chapter-pages(reference, options)
  + finish(reference, options))

#let driver-incollection(reference, options) = run(
  begin(reference, options, author-translator-others(reference, options))
  + title-macro(reference, options) + (newunit,) + language(reference, options) + ublock
  + byauthor(reference, options) + ublock
  + maintitle-booktitle(reference, options) + ublock
  + byeditor-others(reference, options) + ublock
  + (edition-field(reference, options), newunit)
  + volume-part(reference, options) + (newunit, volumes-field(reference, options)) + ublock
  + series-number(reference, options) + ublock
  + (field(reference, "note", options),) + ublock
  + xxx-location-date(reference, options, "publisher") + ublock
  + chapter-pages(reference, options)
  + finish(reference, options))

#let driver-inbook(reference, options) = run(
  begin(reference, options, author-translator-others(reference, options))
  + title-macro(reference, options) + (newunit,) + language(reference, options) + ublock
  + byauthor(reference, options) + ublock
  + maintitle-booktitle(reference, options) + ublock
  + byeditor-others(reference, options) + ublock
  + (edition-field(reference, options), newunit)
  + volume-part(reference, options) + (newunit, volumes-field(reference, options)) + ublock
  + series-number(reference, options) + ublock
  + (field(reference, "note", options),) + ublock
  + xxx-location-date(reference, options, "publisher") + ublock
  + chapter-pages(reference, options)
  + finish(reference, options))

#let book-like(reference, options, name-steps) = run(
  begin(reference, options, name-steps)
  + maintitle-title(reference, options) + (newunit,) + language(reference, options) + ublock
  + byauthor(reference, options) + ublock
  + byeditor-others(reference, options, named: true) + ublock
  + (edition-field(reference, options), newunit)
  + volume-part(reference, options) + (newunit, volumes-field(reference, options)) + ublock
  + series-number(reference, options) + ublock
  + (field(reference, "note", options),) + ublock
  + xxx-location-date(reference, options, "publisher") + ublock
  + chapter-pages(reference, options) + (newunit, field(reference, "pagetotal", options))
  + finish(reference, options))

#let driver-book(reference, options) = book-like(reference, options,
  author-editor-others-translator-others(reference, options))
#let driver-collection(reference, options) = book-like(reference, options, editor-others(reference, options))

#let driver-proceedings(reference, options) = run(
  begin(reference, options, editor-others(reference, options))
  + maintitle-title(reference, options) + (newunit,) + language(reference, options) + ublock
  + event-venue-date(reference, options) + ublock
  + byeditor-others(reference, options, named: true) + ublock
  + volume-part(reference, options) + (newunit, volumes-field(reference, options)) + ublock
  + series-number(reference, options) + ublock
  + (field(reference, "note", options),) + ublock
  + (list-field(reference, "organization", options), newunit)
  + xxx-location-date(reference, options, "publisher") + ublock
  + chapter-pages(reference, options) + (newunit, field(reference, "pagetotal", options))
  + finish(reference, options))

#let driver-misc(reference, options) = run(
  begin(reference, options, author-editor-others-translator-others(reference, options))
  + title-macro(reference, options) + (newunit,) + language(reference, options) + ublock
  + byauthor(reference, options) + ublock
  + byeditor-others(reference, options, named: true) + ublock
  + (field(reference, "howpublished", options),) + ublock
  + (type-field(reference, options), newunit, field(reference, "version", options), newunit,
     field(reference, "note", options)) + ublock
  + xxx-location-date(reference, options, "organization") + ublock
  + finish(reference, options))

// standard.bbx's manual: edition, series and number, type, version, note, then the organization in its own unit
// and publisher+location+date ("SAND94-1100. Sandia National Laboratories. Albuquerque, NM, May 1994").
#let driver-manual(reference, options) = run(
  begin(reference, options, author-editor-others-translator-others(reference, options))
  + title-macro(reference, options) + (newunit,) + language(reference, options) + ublock
  + byauthor(reference, options) + ublock
  + byeditor-others(reference, options, named: true) + ublock
  + (edition-field(reference, options),) + ublock
  + series-number(reference, options) + ublock
  + (type-field(reference, options), newunit, field(reference, "version", options), newunit,
     field(reference, "note", options)) + ublock
  + (list-field(reference, "organization", options), newunit)
  + xxx-location-date(reference, options, "publisher") + ublock
  + chapter-pages(reference, options) + (newunit, field(reference, "pagetotal", options))
  + finish(reference, options))

#let driver-unpublished(reference, options) = run(
  begin(reference, options, author-macro(reference, options))
  + title-macro(reference, options) + (newunit,) + language(reference, options) + ublock
  + byauthor(reference, options) + ublock
  + (field(reference, "howpublished", options),) + ublock
  + (type-field(reference, options),) + ublock
  + event-venue-date(reference, options) + ublock
  + (field(reference, "note", options),) + ublock
  + xxx-location-date(reference, options, none) + ublock
  + finish(reference, options))

#let driver-thesis(reference, options) = run(
  begin(reference, options, author-macro(reference, options))
  + title-macro(reference, options) + (newunit,) + language(reference, options) + ublock
  + byauthor(reference, options) + ublock
  + (field(reference, "note", options),) + ublock
  + (type-field(reference, options), newunit)
  + xxx-location-date(reference, options, "institution") + ublock
  + chapter-pages(reference, options) + (newunit, field(reference, "pagetotal", options))
  + finish(reference, options))

#let driver-report(reference, options) = run(
  begin(reference, options, author-macro(reference, options))
  + title-macro(reference, options) + (newunit,) + language(reference, options) + ublock
  + byauthor(reference, options) + ublock
  + (type-field(reference, options), setunit-star("space"), field(reference, "number", options)) + ublock
  + (field(reference, "version", options), newunit, field(reference, "note", options)) + ublock
  + xxx-location-date(reference, options, "institution") + ublock
  + chapter-pages(reference, options) + (newunit, field(reference, "pagetotal", options))
  + finish(reference, options))

#let drivers = (
  "driver-article": driver-article,
  "driver-inproceedings": driver-inproceedings,
  "driver-incollection": driver-incollection,
  "driver-inbook": driver-inbook,
  "driver-book": driver-book,
  "driver-collection": driver-collection,
  "driver-proceedings": driver-proceedings,
  "driver-misc": driver-misc,
  "driver-unpublished": driver-unpublished,
  "driver-thesis": driver-thesis,
  "driver-report": driver-report,
  "driver-booklet": driver-misc,
  "driver-manual": driver-manual,
  "driver-online": driver-misc,
  "driver-dataset": driver-misc,
  "driver-patent": driver-misc,
  "driver-periodical": driver-misc,
)

// ---------------------------------------------------------------------------------------------------
// Labels: biber's default labelalpha template with maxalphanames=4, minalphanames=3. One name gives
// the first three characters of its family name (no prefix: useprefix=false), several the first
// letter of each, more than four the first three names' letters and "+"; then the year's last two
// digits. Biber drops punctuation, symbols and control characters (\DeclareNolabel) first; a tie is
// a no-break space there.

#let nolabel = regex("[\p{Pc}\p{Ps}\p{Pe}\p{Pi}\p{Pf}\p{Po}\p{S}\p{C}]")
#let label-part(family, width) = {
  // A tie inside the name is a no-break space ([J M67]); one at its ends is dropped, as biber trims it
  // (L. ~Guibas gives G).
  let s = family.trim(regex("[~\s\u{00A0}]+")).replace("~", "\u{00A0}").replace(nolabel, "")
  let cs = s.clusters()
  cs.slice(0, calc.min(width, cs.len())).sum(default: "")
}
#let label-generator(index, reference) = {
  let label-field = reference.fields.at("label", default: none)
  let list = reference.fields.at("labelname", default: ())
  let others = list.len() > 0 and is-others(list.last())
  let people = if others { list.slice(0, -1) } else { list }
  // One name keeps three letters, also before "and others" ("V. Cappellini and others" is [Cap+87]).
  let alpha = if label-field != none { label-field } else if people.len() == 1 {
    label-part(people.first().at("family", default: ""), 3) + if others { "+" } else { "" }
  } else if people.len() > 4 or others {
    people.slice(0, calc.min(3, people.len())).map(p => label-part(p.at("family", default: ""), 1)).sum(default: "") + "+"
  } else {
    people.map(p => label-part(p.at("family", default: ""), 1)).sum(default: "")
  }
  let d = reference.fields.at("parsed-date", default: none)
  // A year that is not a number keeps its last two characters, as biber's substring does ("to appear" → ar).
  let raw-year = str(reference.fields.at("year", default: "")).trim().replace(nolabel, "")
  let year = if d != none and "year" in d { str(d.year) } else { raw-year }
  let cs = year.clusters()
  let lbl = alpha + cs.slice(calc.max(0, cs.len() - 2)).sum(default: "")
  (lbl, lbl)
}

// ---------------------------------------------------------------------------------------------------
// The bundle.

#let style = {
  let bundle = alphabetic-style(
    citation: (maxalphanames: 4, minalphanames: 3),
    reference: (
      bibstring-style: "short",
      link-titles: true,
      print-url: false,
      print-doi: false,
      print-isbn: false,
      suppress-fields: (
        "inproceedings": ("editor", "publisher", "series", "location"),
        "book": ("publisher",),
      ),
      format-functions: drivers,
    ),
  )
  bundle.insert("label-generator", label-generator)
  // Pergamon's type alias turns @techreport into @report and overwrites its `type` field with
  // "techreport"; biber's driver source map sets it only when the entry has none ("Tech. Report").
  let reference-style = bundle.reference-style
  bundle.insert("reference-style", (index, reference) => {
    if reference.entry_type == "techreport" {
      reference.insert("entry_type", "report")
      if "type" not in reference.fields { reference.fields.insert("type", "techreport") }
    }
    reference-style(index, reference)
  })
  bundle
}

// biblatex alphabetic sorts by label, names, year and title (anyt); the order decides which entry of a
// shared label is "a". Biber collates case-insensitively; markup and punctuation are dropped here.
// (Pergamon's "anyt" also stops on an entry without a title.) Entries equal in every field keep their
// citation order, which the harness makes the .bib order, as \nocite{*} does.
//
// The tie-break of equal sort text: markup and braces dropped, case and punctuation kept. biblatex's sortupper
// puts upper case first (Pipes, Cigars before Pipes, cigars), as code points do; a trailing period sorts last.
#let tie-text(s) = s.replace(regex("#\w*\[|[\[\]{}\\\\]"), "").replace(regex("[~\u{00A0}\s]+"), " ").trim()
// Accents fold to the base letter, as biber's collation compares letters first (García-Lopez, J. before
// Garcia-Lopez, Jesus).
#let accent-fold = (
  "a": "áàâäãåāăą", "c": "çćčĉ", "d": "ďđ", "e": "éèêëěēėę", "g": "ğĝ", "i": "íìîïīı", "l": "łľĺ", "n": "ñńňņ",
  "o": "óòôöõøőō", "r": "řŕ", "s": "śšşŝ", "t": "ťţ", "u": "úùûüůűū", "y": "ýÿ", "z": "žźż",
)
#let fold-accents(s) = {
  for (base, marked) in accent-fold { s = s.replace(regex("[" + marked + "]"), base) }
  s.replace("ß", "ss")
}
// Sort text: markup and punctuation dropped, ties as spaces (biber collates P.~K. as P. K.), lower case.
#let sort-text(s) = {
  let s = s.replace(regex("[~\u{00A0}]"), " ")
  let s = fold-accents(lower(s.replace(regex("#\w*\[|[\[\]\\\\{}$#;.,:'’`\"()-]"), "")))
  s.replace(regex("\s+"), " ").trim()
}
// biblatex's anyt: presort, sortkey (biber maps BibTeX's key to it; final, so it ends the key), labelalpha,
// names, year, title, volume (left-padded to four digits, 0000 without one).
#let sort-key(reference) = {
  let presort = sort-text(reference.fields.at("presort", default: "mm"))
  let sortkey = reference.fields.at("sortkey", default: reference.fields.at("key", default: none))
  if sortkey != none { return (presort, sort-text(sortkey)) }
  let list = reference.fields.at("labelname", default: none)
  let people = if list == none { () } else { list }
  // name by name, as biber compares them ("Franciosa, P." before "Franciosa, P. G.")
  let names = people.map(p => sort-text(p.at("family", default: "") + " " + p.at("given", default: "")))
  let d = reference.fields.at("parsed-date", default: none)
  let year = if d != none and "year" in d { str(d.year) } else { "" }
  let volume = str(reference.fields.at("volume", default: ""))
  let volume = if volume.len() < 4 { "0" * (4 - volume.len()) + volume } else { volume }
  let title = reference.fields.at("title", default: "")
  (presort, lower(reference.label), names, year,
    sort-text(title), tie-text(title), volume)
}
#let print-options = (sorting: sort-key)
