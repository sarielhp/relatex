# The document model

`document.json` is the document as reLaTeX read it, for programs: a search index, a checker, a
converter of your own. Every other format is written from it.

```sh
relatex --json                  # json/document.json
relatex --json-gz               # json-gz/document.json.gz
relatex validate json-gz/document.json.gz
```

`document.json.gz` is the same bundle compressed, and is what the later commands read. The full
product in the project cache keeps it too ([Converting a paper](converting.md#what-relatex-writes)).

## What it holds

The file is an envelope with a `schema_version`, a `sha256` of its payload and the payload itself.
The payload has:

- `document`: the structure. A tree of nodes (sections, paragraphs, lists, tables, figures,
  footnotes, theorems, formulas) in reading order, with each node's text, its style, its numbering
  and a pointer to where it came from in your source; the labels, citations and external links;
  the language, the title and author; the fonts and the assets.
- `source_map`: which source file, line and column each word came from.
- `layout`: the pages' geometry that the document refers to (the page graphics themselves are
  files in `layout/` beside the bundle).
- `losses`: what reLaTeX could not reproduce (compare [Diagnostics](diagnostics.md)).
- `capabilities`: what the build that wrote the file implemented.

Formulas keep their MathML, and the TeX or Typst forms derived from it
([Markdown and Typst](markdown-typst.md)).

## What it does not hold

It is structure only and holds no binary content. A figure is a record with its MIME type, size
and SHA-256, and the figure itself is a file in `figs/` named after the LaTeX source it came from.
A figure's original (for example the source PDF) stays beside the SVG or PNG. Fonts are files in
`fonts/`, named by their original names. This keeps the model small and lets any tool open the
files directly ([why](../design-faq.md#export-directory)).

An export written by `relatex export` has a `manifest.json` that lists every file of the export
with its SHA-256. The full product in the project cache has one too.

## Schemas

The schemas are in the `schemas/` directory of the reLaTeX repository: `bundle.schema.json` for the
envelope, `document.schema.json` and `document-node.schema.json` for the document and its nodes,
and `layout.schema.json` for the page layout. They are JSON Schema files you can use in your own
tool.

## Commands

| Command | Does |
|---|---|
| `relatex validate BUNDLE` | checks a bundle against its schema and says what it holds |
| `relatex inspect BUNDLE --node ID` | shows one node; also `--asset SHA`, `--word ID`, `--cell ID --table ID` |
| `relatex inspect BUNDLE --node ID --projection tex` | a formula as `html`, `markdown`, `tex` or `typst` (`all` for every form) |
| `relatex inspect FILE.mathml --mathml --projection tex` | a standalone MathML file, shown as TeX |
| `relatex export BUNDLE --output DIR --format FORMAT` | writes a format from the bundle, without rebuilding |
| `relatex sync BUNDLE ...` | between source and PDF ([Word-level SyncTeX and rlx](rlx.md)) |

```text
$ relatex validate json-gz/document.json.gz
Bundle verified: 15 nodes, 1 images, 4 fonts
  scope: IR, embedded assets and cross-layer bindings; no TeX/browser or source-semantic acceptance
```

The scope line is the point: `validate` checks that the file is a consistent bundle, not that the
conversion is faithful. `--report` prints the JSON report on every one of these commands.

`export` writes into a new `DIR/export-<date>-<id>/` and takes `--format` html (the default),
markdown, typst-source, epub and the other format names of [Converting a
paper](converting.md#formats). HTML written this way is one page, without the browser check or
the book split; the bundle's `layout/` directory must sit beside it.
