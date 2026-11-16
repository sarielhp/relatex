# reLaTeX features

Everything reLaTeX does, grouped. The [README](../../README.md) names the highlights; each line here
links to where it is described.

## Outputs

- **HTML with MathML**, one reading page, or one page per chapter for a book ([converting](converting.md)).
- **EPUB 3** packages with navigation and page list ([EPUB](epub.md)).
- **Markdown and Typst**, math as the TeX you wrote where it is proved to match ([Markdown and Typst](markdown-typst.md)).
- **Tagged PDF** (PDF/UA-2) from the same model, the pages unchanged ([tagged PDF](tagged-pdf.md)).
- **A JSON document model**, the structure for other tools, with `validate`, `inspect` and `export`
  ([document model](document-model.md)).
- **Books** split into chapters, and **Beamer decks** as one section per frame ([books](books.md), [slides](slides.md)).
- Only the formats you ask for are written, beside the paper or under `-o DIR`; nothing else lands in your
  directory ([converting](converting.md)).

## Accessibility

- Web page and EPUB aimed at WCAG 2.2 Level AA, the tagged PDF at PDF/UA-2, with a claim per output made only
  when its checks pass, measured in a browser with `--verify-browser` ([accessibility](accessibility.md)).
- `relatex lint` lists what only the author can supply (a language, a figure's meaning, a table's header row)
  at its file and line ([accessibility](accessibility.md#2-lint-what-is-open-in-your-source)).
- `relatex alt` drafts figure descriptions for your review and keeps the accepted ones in `alt.toml`
  ([figure descriptions](figure-descriptions.md)).

## SyncTeX and building

- **Word-level SyncTeX**: a click lands on the word, not just the line ([word-level SyncTeX and rlx](rlx.md)).
- **`rlx`** builds the PDF and its SyncTeX file like `latexmk` ([rlx](rlx.md#rlx)).
- `relatex synctex` gives a PDF another tool built the word-level file; `relatex sync` answers the lookups
  from the command line ([rlx](rlx.md)).
- `relatex demo-sync` writes a page with the paper beside its LaTeX source, a click on a word finding the other ([the demo](https://sarielhp.github.io/relatex/demo/sync/), made from the author's own paper).
- Sources written for pdfLaTeX or XeLaTeX build under LuaLaTeX, with fallbacks for what does not
  ([pdfLaTeX and XeLaTeX sources](engines.md)).
- TeX runs in a sandbox without network (bubblewrap on Linux) ([troubleshooting](troubleshooting.md)).
- A second conversion reuses the stages whose inputs did not change, so an unchanged paper converts again in
  seconds ([converting](converting.md)); `relatex replay` continues an earlier attempt.

## Diagnosing

- **Diagnostics in the style of a compiler**: what cannot be reproduced is reported with its severity, never
  dropped silently ([diagnostics](diagnostics.md)).
- **`relatex doctor`** lists the programs reLaTeX starts, what a missing one costs and how to install it;
  `relatex doctor PAPER` lists the classes and packages a paper needs that TeX Live lacks ([doctor](doctor.md)).
- `relatex layers` lists an attempt's stages and files; `relatex capabilities` prints what this build implements.

## Setup and tools

- Settings in a global and a per-paper configuration file, shown by `relatex config` ([configuration](configuration.md)).
- `relatex bibclean` cleans `.bib` files for Typst's bibliography engine ([Markdown and Typst](markdown-typst.md)).
- Shell completion (`relatex completion`) and a manual page (`relatex manpage`).
- `try_arxiv` downloads arXiv papers and converts them, to try reLaTeX on real sources ([README](../../README.md#try-it-on-arxiv-papers)).
- Linux is the supported platform; Windows and macOS builds exist ([Windows](windows.md)).
