# reLaTeX

reLaTeX extends LuaLaTeX: it runs your unchanged document through LuaLaTeX, records what TeX typesets,
and from that record provides:

- **Export** to [HTML](docs/guide/converting.md), [EPUB 3](docs/guide/epub.md), [Markdown and Typst](docs/guide/markdown-typst.md),
  [tagged PDF](docs/guide/tagged-pdf.md) and a [JSON document model](docs/guide/document-model.md), for papers,
  [books](docs/guide/books.md) and [slides](docs/guide/slides.md).
- **Accessibility**: accessible web pages, EPUB and PDF, with a claim for each output made only when its checks
  pass, and help with what only the author can supply ([accessibility](docs/guide/accessibility.md)).
- **Word-level SyncTeX**: click a word in the PDF and land on that word in the source, and back. `rlx` builds
  it like `latexmk` ([word-level SyncTeX and rlx](docs/guide/rlx.md); a [click-to-source demo](https://sarielhp.github.io/relatex/demo/sync/)
  shows the idea in a browser).
- **And more**: `relatex doctor` for what a paper needs, diagnostics instead of silent loss, a click-to-source
  demo, figure descriptions, incremental rebuilds ([all features](docs/guide/features.md)).

Because every export comes from what TeX typeset, it says what your PDF says.

> **Requirements.** The paper must compile on **LuaLaTeX with no errors**. Papers that work only with
> xelatex, pdflatex or latex are not supported, and a TeX error stops the conversion. Build the paper with
> `lualatex` first. Why: [docs/why-lualatex.md](docs/why-lualatex.md).

**Status.** Version 0.1.x: it converts real papers well but is young; expect rough edges, and read the
diagnostics. Linux is the supported platform; Windows and macOS builds exist but are less tested
([Windows](docs/guide/windows.md)).

## How reLaTeX was built

The code, tests and documentation of reLaTeX were written by AI coding agents: Anthropic's Claude models, with
smaller models for bounded jobs. The author designed the project, set its standing rules, decided the open
questions and reviewed the results. Every change passes the project's gates and is verified on real papers
against their PDFs. More in the [design FAQ](docs/design-faq.md#how-built).

## Requirements

| Program | Needed for |
|---|---|
| Go 1.26 | building reLaTeX (nothing else is needed to build it) |
| TeX Live 2026 or later (LuaLaTeX, BibTeX, Biber, makeindex, kpsewhich) | every conversion. reLaTeX uses the current `luamml`, `tagpdf` and `latex-lab` code, so it needs a current TeX Live. A TeX Live you installed yourself with the TeX Live installer (`~/.texlive`, or the tree named by `RELATEX_TEXLIVE`) is the tested setup; a distribution's packaged TeX Live is untested |
| bubblewrap (`bwrap`) | every conversion: TeX runs in a sandbox without network |
| qpdf, mutool (MuPDF), poppler-utils | every conversion: reading and checking PDFs and figures |
| Chromium or a remote Chrome | only with `--verify-browser` |
| typst | only with `--compile-typst` |

epubcheck, DAISY Ace and veraPDF are not run by reLaTeX; run them on its output yourself if you want to.

## Build and install

```sh
git clone https://github.com/sarielhp/relatex
cd relatex
make build        # stages the resources, then builds build/relatex with them embedded
make test         # go test ./... (tests that need data not in this repository skip)
make install      # copies it to ~/bin/relatex and links ~/bin/rlx to it
```

Or install it with Go alone: `go install github.com/sarielhp/relatex/cmd/relatex@latest` (the release
commit carries the staged resources, so no build step is needed).

The binary is self-contained: on first use it unpacks its resources into `~/.cache/relatex/runtime/`.
`relatex capabilities` lists what this build implements; it does not check that the programs above are
installed (a missing one stops the conversion with a message naming it).

## Quick start

```sh
cd path/to/my-paper       # the directory with your main .tex file
relatex                   # converts it; results go to ./html/ and ./markdown/
```

Open `html/index.html` in a browser. Beside it are `markdown/document.md`, the PDF reLaTeX built and
`manifest.json`, which lists every file with its SHA-256. Warnings print on the terminal, one per line:

```text
paper: warning[language.undeclared]: the source declares no language with a known tag (babel, polyglossia or a class option); the export is marked as English (en)
```

Add `--epub` for an EPUB 3 package, `--verify-browser` to check the page in a browser and measure its
accessibility, or `--all` for every format. `relatex --help` lists the rest.

## Try it on arXiv papers

```sh
build/try_arxiv --id 2401.00001 --dest ./arxiv-papers   # one paper
build/try_arxiv --random 5 --dest ./arxiv-papers         # five recent cs.* papers
```

`try_arxiv` downloads each paper's LaTeX source from arXiv (one request at a time, at least three seconds apart,
identified as `relatex-try_arxiv`), unpacks it under `--dest` and runs reLaTeX there, printing one line per paper.
[`tested-papers.txt`](tested-papers.txt) lists the arXiv identifiers reLaTeX has been tested on.

## Documentation

- [Overview](docs/guide/index.md), [Converting a paper](docs/guide/converting.md), [Books](docs/guide/books.md),
  [Slides](docs/guide/slides.md), [pdfLaTeX and XeLaTeX sources](docs/guide/engines.md)
- [Diagnostics](docs/guide/diagnostics.md), [Accessibility](docs/guide/accessibility.md),
  [Configuration](docs/guide/configuration.md), [Troubleshooting](docs/guide/troubleshooting.md)

## Contributing

This repository holds one commit per release; reLaTeX is developed in a private repository and published
here at each release. Bug reports are welcome as [GitHub issues](https://github.com/sarielhp/relatex/issues), which is also how to reach
the author. Include the reLaTeX version (`relatex --version`),
the diagnostics it printed and, if you can share it, a minimal .tex source. Pull requests are read and,
if accepted, applied in the development repository and appear in the next release (they are not merged
here, because the next release replaces the tree).

## License

reLaTeX is licensed in two parts:

- `tex/` (the TeX and LuaTeX collector that runs inside LuaLaTeX) is under the
  [LaTeX Project Public License 1.3c](LICENSES/LPPL-1.3c.txt). Each file carries the notice.
- Everything else is under the [GNU General Public License v3.0 or later](LICENSE).

Files from other projects keep their own licenses: the Typst packages in `profiles/typst-vendor` (MIT, MIT-0,
Apache-2.0), axe-core (`lib/relatex/axe.min.js`, MPL-2.0) and the math accent font (GUST Font License). The
ORCID iD icon that reLaTeX embeds beside ORCID identifiers is a trademark of ORCID, Inc.; ORCID's
[brand guidelines](https://info.orcid.org/brand-guidelines/) permit the iD icon beside ORCID identifiers.
`REUSE.toml` maps every file to its license and `LICENSES/` holds the texts.
