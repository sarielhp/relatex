# Markdown and Typst

Both formats are text you edit afterwards, so they read like source. Markdown is written by
default; Typst is opt-in.

```sh
relatex                          # Markdown and HTML
relatex --typst                  # typst/ only
relatex --format markdown,typst-source
relatex --typst --compile-typst  # also compile it with typst
```

## Markdown

The file is `markdown/document.md`, with its figures and fonts beside it. Read it in a
viewer that renders HTML inside Markdown.

- **Math.** An inline formula is your own TeX (`$n \ge 1$`) and a display is `$$ ... $$` with its
  equation number after it, where reLaTeX typeset your TeX again and got the same formula.
  Otherwise the TeX is derived from the MathML, or the MathML itself is written.
  `--no-author-math` derives the TeX for every formula ([why](../design-faq.md#typst-markdown)).
- **Figures** are image links to SVG or PNG files beside the document
  (`![Source figure](box.svg)`), with the caption in a `<figure>` block.
- **What stays HTML.** Tables, the title block, sections and some formulas are embedded HTML. The
  file begins with a `<style>` block for the paper's fonts, which are linked as files, so a plain
  Markdown reader shows that block as it is.
- **References** are links to anchors in the file.

## Typst

`--typst` writes a Typst project to `typst/`: `main.typ`, `prefix.typ` (the setup it
imports), `figs/`, and a bibliography file when the paper has one. It keeps your labels,
references, citations, numbering and wording, and compiles with stock Typst:

```sh
cd typst
typst compile --ignore-system-fonts main.typ
```

- **Math** is Typst's own math, converted from the MathML (`$ sum_(i = 1)^n i = ... $`).
- **Figures** are SVG files in `figs/`, named after the LaTeX figure files.
- **`--compile-typst`** runs the `typst` program on the project and writes `main.pdf` there; the
  conversion fails if that fails. It needs `typst` installed (`relatex doctor` shows whether it
  is). `relatex export BUNDLE --output DIR --format typst-source --compile-typst` does the same for a
  saved bundle.
- **`--typst-layout`** is another thing: Typst that reproduces the reading page, not an editable
  source.

## Cleaning a .bib for Typst

`relatex bibclean` prepares `.bib` files for Typst's bibliography engine. It expands `@string` and
plain `@preamble` macros, converts text-mode LaTeX to Typst markup, normalizes Unicode, keeps
case-protecting braces and plain `$...$` math, and writes the selected entries (with `crossref`
and `xdata` parents) to standard output:

```sh
relatex bibclean --all refs.bib > clean.bib
relatex bibclean --keys knuth84,lamport94 --report report.json refs.bib > clean.bib
```

`--keys-file FILE` takes the keys one per line. `--report FILE` writes the per-entry JSON report
(to stderr without it).

## Known limits

- Neither format reproduces the pages: line breaks, page breaks and spacing are the reader's or
  Typst's.
- Markdown keeps tables and some structure as HTML, and a formula is MathML where no TeX could be
  verified.
- Losses are reported as diagnostics, not hidden ([Diagnostics](diagnostics.md)); read the run
  report after a conversion.
- A Typst project is known to compile on your Typst version only when you run
  `--compile-typst` or `typst compile` yourself.
