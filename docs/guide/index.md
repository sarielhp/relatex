# reLaTeX user guide

reLaTeX turns a LaTeX source into documents a reader can use on a screen: a reading page in the
browser, an EPUB book, Markdown, Typst, and a JSON model of the document. This guide is for the
author of the LaTeX source. It describes reLaTeX 0.1.36.

## Pages

- [Converting a paper](converting.md): what reLaTeX reads, what it writes and where, formats,
  caching, and the `rlx` mode.
- [Books](books.md): documents with `\chapter`, split into one file per chapter, with a contents
  page and a linked index.
- [All features](features.md): everything reLaTeX does, grouped, with where each is described.
- [EPUB](epub.md): the EPUB 3 package, books as chapters, checking it with EPUBCheck and Ace.
- [Markdown and Typst](markdown-typst.md): where they go, math and figures, `--compile-typst`,
  `relatex bibclean`.
- [The document model](document-model.md): `document.json`, its schemas, `validate`, `inspect`,
  `export`.
- [Tagged PDF](tagged-pdf.md): `relatex tag-pdf`, PDF/UA-2, and how far it goes.
- [Word-level SyncTeX and rlx](rlx.md): a click lands on the word, `rlx` as latexmk,
  `relatex synctex`, `relatex sync`.
- [`relatex doctor`](doctor.md): the programs reLaTeX needs and what a missing one costs.
- [Slides](slides.md): Beamer decks, read in handout mode, one section per frame.
- [pdfLaTeX and XeLaTeX sources](engines.md): how sources written for another engine are built.
- [Diagnostics](diagnostics.md): the five levels and what each code means.
- [Accessibility](accessibility.md): what reLaTeX does for you, and what only you can do.
- [Figure descriptions](figure-descriptions.md): text alternatives for your figures in
  `relatex/alt.toml`, drafted by a model of your choice and reviewed by you.
- [Configuration](configuration.md): `config.jsonc`, `relatex/config.jsonc` and environment variables.
- [Troubleshooting](troubleshooting.md): failed conversions, where the logs are.
- [Windows](windows.md): installing on Windows (scoop or zip, TeX Live, MuPDF 1.28.x),
  `relatex doctor`, and the known limits (no sandbox).

## The outputs

| Output | File | Use it for |
|---|---|---|
| Reading page | `html/index.html` | Reading on a screen. Reflows to any width, formulas in MathML, links for every reference, citation and footnote, hover previews of the targets, a dark and a light theme. |
| EPUB 3 | `NAME.epub` (with `--epub`) | E-readers and library systems. The same content as the page, without scripts, in the light palette, with EPUB accessibility metadata. |
| Markdown | `markdown/document.md` | Pasting into notes, wikis, or tools that read Markdown. Formulas are your own TeX where reLaTeX could verify it. Tables and some structure stay as embedded HTML, so it reads best in a viewer that renders HTML inside Markdown. |
| Typst | `typst/main.typ` (with `--typst`) | Continuing to edit the paper in Typst. It keeps your labels, references and citations. |
| JSON | `json/document.json` (with `--json`) | Programs. `document.json.gz` is the bundle the other formats are made from; `relatex export` turns it into any format later. |
| PDF | `NAME.pdf` in the product (the project cache) | The PDF LuaLaTeX built from your source (the instrumented build the document was read from). `--pdf-ua` writes the tagged PDF to `NAME-tagged.pdf` beside the paper, so it never replaces the PDF you build yourself. |

`--pdf` is something else: a PDF reconstructed from the recorded page graphics, a development aid
that loses text search. You rarely want it.

## How the checking works

reLaTeX never guesses what your LaTeX means. In plain words:

1. **It builds your paper twice.** Once exactly as you would (the *ordinary* build), and once
   with reLaTeX's recording packages loaded (the *instrumented* build). The recording packages
   watch TeX typeset: every glyph with its font and position, which paragraph, heading, list,
   table, footnote or theorem it belongs to, every label and reference, and each formula as
   MathML (through `luamml`). Both builds must compile and settle, with every reference and
   citation resolved, or the conversion stops.
2. **It reads the document from what was typeset**, not from your source text. Reading order,
   numbering, references and citations are what the PDF shows.
3. **It checks the result.** Each output is written from that one document, which is checked
   against its schemas. With `--verify-browser`, a browser (Chromium, locally or remote) loads
   the page offline and checks that the blocks, formulas, references and notes survived into the
   page and render; it also runs the axe-core accessibility checker in both themes.
4. **What it could not reproduce, it reports.** Instead of stopping, reLaTeX writes the export
   and lists each loss as a diagnostic with a level, from `error` (content lost) down to
   `whatever` (informational). See [Diagnostics](diagnostics.md).

The run report says what was and was not verified. A normal conversion is reported as
`"qualified": false` and its formats as `generated_unverified`: the full comparison against the
ordinary PDF (that recording did not change it, that every word and formula of the PDF is in the
page, read by checkers written independently of the converter) is the project's acceptance
suite, run by the developers on test papers. It is not part of `relatex` itself.

The fonts in the reading page are your paper's own fonts where their files can be linked, and
browser fonts otherwise. The page is a faithful reading copy, not a picture of the PDF: line
breaks, page breaks and exact spacing are the browser's.

## What reLaTeX does not do

- It does not build with pdfLaTeX or XeLaTeX. Everything is built with LuaLaTeX; sources written
  for the other engines get a small compatibility layer when they need one
  ([details](engines.md)).
- It does not run shell escape: `minted`, `pythontex` and similar packages do not work.
- It does not reach files outside the paper directory, or the network, during the build.
- It does not certify accessibility. It measures it and reports the open items
  ([details](accessibility.md)).
- It does not edit your source.
