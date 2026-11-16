# EPUB

`--epub` writes the converted paper as an EPUB 3 package, for e-readers and library systems.

```sh
cd ~/papers/sums          # holds sums.tex
relatex --epub            # sums.epub, and nothing else
relatex --html --epub     # the reading page and the package
```

The package is `NAME.epub`, where NAME is the main file's name. It is made from the same
verified document as the reading page ([Converting a paper](converting.md#formats)), so it has the
same text, numbering, references and citations.

## What is in the package

```text
$ unzip -l sums.epub
mimetype
META-INF/container.xml
EPUB/package.opf             the manifest, the reading order and the metadata
EPUB/nav.xhtml               contents, landmarks and a page list
EPUB/style.css
EPUB/index.xhtml             the text
EPUB/figs/box.svg            the figures the page shows
EPUB/fonts/...otf            the fonts, as files
```

- **XHTML with MathML.** Formulas are MathML, each with a text alternative. There are no scripts.
  The package opens in the light palette; a reading system brings its own night mode.
- **Figures** are SVG, PNG or JPEG files: the ones the reading page shows. A figure's original
  (for example the source PDF) is not in the package; it is in `html/figs/` of the output.
- **Fonts** are the paper's own font files, as OpenType or TrueType files in `EPUB/fonts/`.
  reLaTeX never turns a font into data inside the text ([why](../design-faq.md#figures-and-fonts)).
- **The navigation document** has the table of contents, the landmarks and a page list that
  follows the page numbers of the PDF reLaTeX read.
- **Metadata**: title, author, language, the PDF the page numbers come from (its SHA-256 is
  recorded as the print source), a modification date, and the EPUB accessibility metadata (access
  modes, features such as MathML and page navigation, and a summary). The summary says what is
  missing, for example how many figures have no text alternative, and claims conformance only
  when the accessibility checks established it ([Accessibility](accessibility.md)).

The same source gives the same bytes. The modification date is `SOURCE_DATE_EPOCH` when set.

## Books

A book (a document with `\chapter`, see [Books](books.md)) becomes one XHTML file per chapter, in
reading order, beside a contents file when the book prints one:

```text
EPUB/index.xhtml  EPUB/contents.xhtml  EPUB/chapter-01.xhtml  EPUB/chapter-02.xhtml ...
```

`nav.xhtml` lists the chapters with their sections. A paper is split into one file per top-level
section only when its links allow it; otherwise it stays one file.

## Checking the package

reLaTeX does not run EPUBCheck or DAISY Ace. The package is written for them, but run them
yourself before you depend on that:

```sh
epubcheck sums.epub
ace -o ace-report sums.epub
```

Whatever they report is about the package as written. reLaTeX's own account of what it could not
reproduce is in the run report and the diagnostics ([Diagnostics](diagnostics.md)).

## From a saved bundle

`relatex export` writes the package later from the document bundle, without rebuilding:

```sh
relatex export json-gz/document.json.gz --output out --format epub
```

An export made this way has no page list; convert to get one. See [The document
model](document-model.md).
