# reLaTeX design FAQ

Why reLaTeX works the way it does: the choices an outsider asks about, each with its reason. For what
the program does and how to run it, read the [user guide](guide/index.md); this page is about the design.

- Project: [how reLaTeX was built](#how-built)
- Engine and proof: [LuaLaTeX](#engine-lualatex), [PDF verification](#verify-against-pdf),
  [independent checks](#independent-checks), [diagnostics](#diagnostics), [trusted sources](#trusted-sources)
- Outputs: [EPUB 3](#epub3), [the export directory](#export-directory), [figures and fonts](#figures-and-fonts),
  [Typst and Markdown](#typst-markdown), [books](#books), [slides](#slides)
- Content: [math](#math-model), [tables](#tables), [references](#references), [source positions](#synctex)
- Accessibility: [the claim](#accessibility-claim), [spoken math](#math-speech), [descriptions](#figure-descriptions),
  [author input](#author-input), [tagged PDF](#tagged-pdf)
- Engineering: [Go and Ruby](#go-and-ruby), [incremental conversion](#incremental), [TeX Live updates](#toolchain-updates),
  [limits](#not-supported)

<a id="how-built"></a>
## How was reLaTeX built?

reLaTeX was built by AI coding agents doing the work. The code (Go, the LuaLaTeX collector, the
development tools), its tests and its documentation, this FAQ included, were written by AI agents:
Anthropic's Claude models, with smaller models taking bounded, well-specified jobs. The author designed
the project, set the standing rules the agents work under, decided the open questions and reviewed the
results.

The rules are what make this work. Every change passes the same gates (build, static analysis, unit
tests), and every change to the conversion is verified on real papers against their PDFs by checks that
are independent of the converter (see [verification](#verify-against-pdf) and
[independent checks](#independent-checks)). A check is never loosened to make a paper pass; a choice
that is a genuine fork goes to the author. The design records behind this FAQ are the agents' and the
author's shared memory of why each choice was made.

<a id="engine-lualatex"></a>
## Why run LuaLaTeX instead of emulating TeX?

LaTeX documents are programs. Packages redefine each other, classes change what a heading is, and the
same macro can print different things in different places. A converter that re-implements this
behaviour is always one package behind. reLaTeX lets the real TeX engine typeset the document and
collects what it needs while the engine still has it: LuaLaTeX's callbacks give access to the
typeset material (paragraphs, math, boxes) before it is flattened into a PDF page.

The collector only reads. It does not change a node, insert anything visible or rewrite the output, and
the build with the collector must produce the same text and the same page images as an ordinary build
(see [verification](#verify-against-pdf)). Running the engine does not by itself reveal structure,
reading order or the meaning of a formula; those are recovered from what the engine recorded and then
checked, which is most of the work.

<a id="verify-against-pdf"></a>
## Why verify against the PDF?

The PDF is the one artifact the author has already accepted. reLaTeX therefore treats it as the
reference and makes every claim about the document answer to it: the instrumented build must be
indistinguishable from the ordinary build (same extracted text, same page rasters), and the reading
that is exported must account for the text of the PDF without losing, repeating or reordering
content. A conversion that compiles but silently drops a paragraph is the failure mode this guards against.

The same idea reaches the browser: the finished page can be loaded in a real browser, offline, and its
blocks, formulas, links and styles compared with the model they came from. Verification that is slow or
optional gets skipped, so it is part of the run, and a writer and the check that judges it are changed
together.

<a id="independent-checks"></a>
## Why are the checks independent of the converter?

A check that shares the converter's code shares its mistakes. The acceptance checks are written
separately and read the evidence of a separate probe build, the reference PDF and what the converter
published, then compare them. An early version had the checker re-parse the author's LaTeX to know what to
expect; it needed another special case for nearly every new paper and never converged, because a small
reader only approximates TeX. The checks now take their expectations from what TeX itself did (the
expanded math lists, the converged auxiliary files) and never expand a macro themselves.

<a id="diagnostics"></a>
## Why diagnostics instead of stopping?

Real documents contain things no converter reproduces exactly. Stopping on the first one gives the
reader nothing; silently approximating gives them something wrong. reLaTeX writes the export and
reports each loss the way a compiler reports warnings: a stable code, one of five levels
(fatal, error, alert, warning, whatever), a message and a location. The level says how visible the loss
is, from output that says something the paper does not, down to information nobody sees.

Only a conversion that cannot produce anything (a TeX error, no main file) is fatal. A failed conversion
does not leave misleading products behind: results of an earlier run are moved aside with a note saying
that they no longer describe the source.

<a id="trusted-sources"></a>
## Is it safe to convert someone else's LaTeX?

Not by design. TeX can run Lua and read files, so the measures around a build limit accidents, they do
not make hostile input safe. On Linux the build runs in a sandbox that sees the paper, the TeX
installation and system libraries, and has no network; where no sandbox exists (Windows) TeX's own file
limits apply, and every run records that fact as a diagnostic so the export says how it was made.
reLaTeX is meant for sources you trust.

<a id="epub3"></a>
## Why is EPUB 3 the target?

A converted paper is a web document with files: HTML, MathML, SVG and raster figures, fonts. EPUB 3 is
the standard container for exactly that, with a manifest of every file, in a ZIP that reading systems
and many browsers already open. The alternatives (a plain ZIP, MHTML, web archives, tar) either have no
reading-system support or hide files inside encodings.

This shapes every other output decision: the exports use only what EPUB 3 readers support (MathML for
math, SVG, PNG and JPEG for images, OpenType, TrueType or WOFF for fonts, no scripts in the content), and
the package is a late, mechanical step on top of the verified page. The package carries accessibility
metadata, a page list that follows the PDF's page numbers, and is checked with EPUBCheck and Ace.

<a id="export-directory"></a>
## What is in an export, and why?

An export is one self-contained directory: `index.html`, `document.json` and the files they use, with
relative references. `document.json` holds structure only and no binary content (embedded files had been
most of its size); figures and fonts are files next to it, and a manifest keeps a SHA-256 for each.
Nothing depends on where the directory lives, so it can be zipped, served or opened from disk.

The output directory holds only the formats you asked for, each under a plain name. Intermediate records
(the engine run, the page layout, caches) are reLaTeX's own and live in a cache, not in your directory.
A book is also one file per chapter, but the whole page is built and verified first and the split is cut
from it afterwards (see [books](#books)).

<a id="figures-and-fonts"></a>
## Why are figures kept as originals and SVG, and fonts only by name?

Figures are files named after the LaTeX source they came from: `\includegraphics{figs/plot.pdf}` becomes
`figs/plot.svg` for display and keeps `figs/plot.pdf` beside it. A hash is not a name a person can find;
a name from the source is. A figure the paper draws itself (TikZ, for example) is numbered in reading
order. The original always stays next to the derived form so nothing is lost by the conversion, and the
same names serve HTML, Markdown and Typst. A vector figure whose SVG would be enormous is shown as a raster
image instead.

Fonts are named, or linked as their original files where the licence allows, and never converted or
embedded as data. Text is set with real browser fonts rather than copies of the PDF's glyphs, so the
page is a faithful reading of the document, not a picture of the PDF. The same approach is used for the
Unicode meaning of glyphs: where a font carries no usable mapping, reLaTeX consults the font's own tables
and, as a last resort, the matching installed font, and reports what it cannot map.

<a id="typst-markdown"></a>
## Why Typst and Markdown, and how do they differ from the HTML?

Both are text an author edits afterwards, so they aim to read like source. The Typst writer keeps the
author's labels, references, citations, numbering, structure and wording, compiles with stock Typst, and
prints the same numbers as the LaTeX PDF; a check compares the project with the document model and the
PDF. When the bibliography can be reproduced it is a real bibliography built from the cleaned `.bib`,
not literal entries.

Markdown shows math as the author wrote it (`\R`, `\norm{x}`) wherever reLaTeX has shown that the
author's TeX typesets the same formula again; otherwise it writes TeX derived from the MathML. HTML and EPUB
render from the MathML, which stays the truth.

<a id="books"></a>
## Why are books split after verification?

A book on one page is unusable, and readers and EPUB systems expect one file per chapter. But the whole
verification (document model, browser checks, accessibility measures) rests on a single page. Splitting
the model, or verifying each chapter alone, would weaken what is proven and touch every check. So reLaTeX
builds and verifies the whole page exactly as for a paper, then cuts it into a contents page and chapter
files mechanically, and a separate check proves the cut faithful to the whole. A document without chapters
is not split.

Large books also drove the storage format: layout data is kept as part files that no reader holds whole,
and large traces are read page by page, so memory stays bounded by a page rather than a book. An index is
produced the way latexmk does, and linked, since the page numbers it prints name pages the export does
not have.

<a id="slides"></a>
## How are Beamer slides handled?

A deck is converted from beamer's handout state: one page per frame, not one per overlay, so the export
does not repeat a frame's text for every overlay step. Frame titles become headings, and slide
furniture (headline, footline, navigation) is told apart from content. Markers beamer draws instead of
printing (balls, squares) are read as list markers rather than coloured rules.

<a id="math-model"></a>
## How is math represented?

Every formula is MathML built from the math lists TeX itself produced, so its layout is what the
engine decided. MathML is the truth that HTML, EPUB and Typst render from. Next to it, a formula can
carry TeX in two forms: the author's own source, shipped only where typesetting it again with the
author's macros gives the same MathML; and standard LaTeX written from the MathML with a fixed set of
packages and no author macros, for places that need TeX without the author's preamble.

Meaning from the author's macros (a macro that means norm, absolute value or transpose) is carried as MathML
`intent`, on by default, because a macro the author wrote is evidence a guess is not. Where the evidence
is thin the plain reading wins.

<a id="tables"></a>
## How are tables, headers and similar structure decided?

A table becomes a grid of typed cells that keep what the engine knew: cell contents, spans, the rules that
were painted, cell colours, and nested tables to any depth. Whatever cannot be modelled falls back
per table, never for the whole document, and the fallback is text rather than a picture.

Header cells are decided from typesetting evidence, in a fixed order: what the author states (a hint
file or the source's own head declaration, such as a long table's repeated head or a class's table-head
command), then rules and bold rows that set rows apart, then row groups and first columns that behave as
row headers. A table with no cue stays without headers and the report says so; reLaTeX does not invent
semantics the typesetting does not show. The independent checker mirrors each rule, so a rule is not trusted
until two implementations agree.

<a id="references"></a>
## How are citations, cross-references and the index kept?

Citations, bibliography entries and cross-references form a typed graph in the document: a call knows its
keys and targets, and each entry points at its block. This is read from the package that produced it
(natbib, biblatex, plain `\bibitem`) at the point where the engine resolves it, so the exported links go
where the PDF's links go. Links and citations get names that say what they point to. A cluster is one
call, and a citation that runs through mathematics keeps its identity.

<a id="synctex"></a>
## How does it relate to SyncTeX and my editor?

Every word a reader sees maps back to the source file, line and, where TeX allows, column, and each link
says how it was established. reLaTeX writes a word-level SyncTeX file, so an editor that reads one can
go from a word to its source and back instead of to a line. The `rlx` command works like latexmk for this:
it builds only the products an editor needs, seeded with the previous run's converged files, and it
honours `TEXINPUTS`, because a tool that behaves like a LaTeX engine has to find files the way one does.

<a id="accessibility-claim"></a>
## Why does reLaTeX care about accessibility, and what does it claim?

Relatex already holds what accessibility needs and plain LaTeX output lacks: verified structure, a
reading order proven against the PDF, and MathML for every formula. Course material is increasingly
required to meet accessibility rules, so the HTML, EPUB and PDF aim at WCAG 2.2 Level AA (the bar in force
is 2.1 AA), PDF/UA-2 and EPUB Accessibility 1.1, and reLaTeX measures the result the way it measures
fidelity: with a browser, automated rule engines and a reading check that linearizes what a screen reader
would speak, in order, once.

The claim is conservative. reLaTeX states a conformance claim only when its checks support it, withdraws it
when something only the author knows is open (a figure description, a language, whether a colour carries
meaning), and lists exactly what is open with its source line. Automated rules cannot prove accessibility,
and the page says so.

<a id="math-speech"></a>
## How does accessible math work?

Presentation MathML says how a formula looks; a speech engine has to guess what it means. reLaTeX
repairs the cheap cases in the markup (an operator name split into letters, a variant attribute that
changes nothing, missing function application, fences not grouped) and states meaning where the source
makes it knowable, using MathML 4 `intent` with concept names from a shared list. The result is graded
with a real screen-reader math engine on a reviewed sample, not by inspection.

Readings that cannot be derived stay the author's: a macro such as `\Im` means imaginary part in one
paper and image in another, so the author may give its spoken name once, in the paper's own files. A
document-level default is allowed, a per-occurrence mark is not.

<a id="figure-descriptions"></a>
## Who writes figure descriptions?

Only the author knows what a figure is for. reLaTeX takes descriptions in a fixed order: the source's own
keys first (graphicx `alt`, an `\Description`), then a reviewed sidecar entry, then, if asked, a drafted
one. A draft comes from a vision model given the picture, the caption, the citing text and measured
geometry of the drawing, is recorded as a draft with the model that wrote it, and never counts as reviewed
until a person accepts or edits it. A short description is about a sentence; a longer one is optional and
shown on demand. The review happens on a page or in an annotated copy of the LaTeX, never in the original
source.

<a id="author-input"></a>
## How does an author steer a conversion?

Everything the author owns for reLaTeX lives in one visible directory, `relatex/`, beside the paper's main
file, and is committed with the paper: settings, hints, figure descriptions, saved review decisions.
Everything reLaTeX generates is in the output directory and can be regenerated. Hints also come as
comments in any source file the paper loads, and say what the typesetting cannot: how a macro reads
aloud, what a figure shows, which rows of a table are headers, that a caption's colour word is not the
only way to tell lines apart. A hint is always about the author's document, never a way to loosen a check.

<a id="tagged-pdf"></a>
## What is the tagged PDF route?

Handouts are often the PDF itself, so the PDF has to be accessible too: a structure tree, alternative
texts, MathML for formulas, language and title. Two routes exist. LaTeX's own tagging rebuilds the
document with the kernel's tagging code, which keeps the PDF TeX's own but fails for classes and packages
it does not cover. The route reLaTeX relies on tags the ordinary PDF afterwards from its verified model and
layout: it binds each painted glyph to its structure element by paint order and checks the position, and it
leaves a page it cannot prove untagged-as-artifact and reported rather than guessed. The appearance of
the pages cannot change, and every document reLaTeX can convert can be tagged. Where the tagged text and
the model disagree the fix is made at the source of the disagreement, not forgiven in the check.

<a id="go-and-ruby"></a>
## Why Go, and what is Ruby for?

The first implementation was Ruby. A fresh conversion should cost little more than the one instrumented
LuaLaTeX pass it cannot avoid, and much of the layout work is per page and can run in parallel, which
Ruby's global lock prevents. The product therefore moved to Go, stage by stage behind stable files, and
the Go program is now the product and its own reference: the old implementation is archived and its output
is no longer a yardstick.

Ruby stays for the development tools: the test and cohort harness and the independent checkers that judge
the product. They are not on the product path, which is why a different language is an advantage: no code
is shared between what is judged and what judges. The TeX side stays Lua. There is no Python anywhere
in the program; external programs (LuaLaTeX, qpdf, MuPDF, Poppler, Chromium) are used as they are.
The Go code is split into layers (utilities, model, core, outputs) so each output reads a finished
document model and never reaches into the analysis. Arithmetic is rounded explicitly so that the same source
gives the same bytes on Windows arm64 as on Linux.

<a id="incremental"></a>
## How does incremental conversion and caching work?

Two builds exist: the full conversion, with every output and proof, and the products-only mode that `rlx`
uses. The full conversion splits into stages whose results are cached by the hash of their inputs, so an
unchanged stage is read back rather than run, and the output is the same bytes as an uncached run; each
speed-up was accepted only by comparing outputs before and after. The products-only mode runs the single
instrumented build, seeded with the converged files of the previous run like latexmk's second pass, so
one pass is enough when nothing the auxiliary files record has changed.

<a id="toolchain-updates"></a>
## What happens when TeX Live is updated?

A TeX Live update rewrites shared catalogues and sometimes a package, and a conversion should not stop
because of something unrelated to the document. reLaTeX pins what each document actually opened, not the
whole installation: packages, classes, definition files, font metrics, and the sources the format is built
from. The global toolchain fingerprint is recorded and warns, but never stops a run. Only a change in a
file the document used triggers a rebuild of its reference, which must match exactly in text and page
pixels; where it does not, a tolerant comparison accepts a rebuild whose glyphs are all present and moved
only within a small distance, or whose size variants of delimiters differ, and reports the evidence.

<a id="not-supported"></a>
## What does reLaTeX not do?

- **No pdfLaTeX or XeLaTeX emulation.** A source must build under LuaLaTeX. A small set of shims makes the
  common pdfTeX and XeTeX idioms build (primitive names, legacy input encodings, a few packages), but
  a document that only works with DVI-era graphics or other engines is out of scope.
- **No repair of silent TeX failures.** A build that ends with an unbalanced group, or one that
  skips a missing `\include`, produces a PDF that does not show what the author wrote, so it is excluded
  rather than converted.
- **No pixel-faithful reproduction of the PDF.** The reading is verified against the PDF; fonts and
  layout of the page come from the browser.
- **No certification.** Accessibility claims are conservative and automated; they are not a replacement for
  a review with real screen readers.
- **No untrusted input** (see [above](#trusted-sources)), and no MiKTeX yet on Windows.
