# `relatex doctor`

`relatex doctor` lists the programs reLaTeX starts: where each was found, its version, and how to
install one that is missing. `relatex doctor PAPER` checks a paper's classes, packages and styles
instead ([below](#packages-of-your-paper)). Run it after installing, and whenever a conversion fails for a reason
that sounds like a missing tool.

```text
$ relatex doctor
relatex 0.1.74 (linux/amd64)

Programs
  ok       lualatex         /home/you/.texlive/bin/x86_64-linux/lualatex (TeX Live)
                            This is LuaHBTeX, Version 1.24.0 (TeX Live 2026)
  ok       kpsewhich        ... (beside lualatex)
  MISSING  qpdf             install: apt install qpdf
  optional typst            install: https://github.com/typst/typst/releases
  ...

TeX Live  /home/you/.texlive
Sandbox   bubblewrap: TeX sees the source, its output and the TeX tree only
Cache     /home/you/.cache/relatex
Temporary /tmp/you/relatex
Config    /home/you/.config/relatex/config.jsonc

Not ready: install the missing required programs above.
```

Each line says `ok`, `MISSING` (required) or `optional`. The exit status is 1 when a required program
or a TeX file reLaTeX needs is missing, else 0; `--json` prints the same as JSON for a script.
The source shown is where reLaTeX found the program: a `RELATEX_*` variable, the configuration,
TeX Live, beside the engine, or `PATH`.

## Required programs

Without one of these reLaTeX cannot convert:

| Program | What it is for |
|---|---|
| `lualatex`, `kpsewhich`, `luaotfload-tool`, `vftovp` | the engine and TeX Live's file and font tools |
| `mutool` | the page graphics and figures (the MuPDF series reLaTeX was built against; `doctor` names it) |
| `qpdf` | handling PDFs, among them figures |
| `pdfinfo`, `pdftocairo`, `pdftotext`, `pdftoppm` | poppler: PDF facts, figures and page images |
| `bwrap` (Linux) | the sandbox the paper is built in; see [Troubleshooting](troubleshooting.md) |

## Optional programs, and what a missing one costs

| Program | Without it |
|---|---|
| `biber`, `bibtex` | a paper whose bibliography needs that program needs it; without it the `.bbl` beside the main file is the only way in |
| `makeindex` | papers that build an index with `\makeindex` need it |
| `epstopdf` | papers with EPS figures need it to convert them |
| `typst` | `--compile-typst` cannot run; the Typst source is still written |
| browser (Chromium or Edge, or `RELATEX_CHROMIUM`, or a remote one in the configuration) | no `--verify-browser`; the export is published with the diagnostic `browser.unverified` |
| `verapdf` | the tagged PDF is not validated and is never called `conforming` ([Tagged PDF](tagged-pdf.md)) |
| `zstd` | the cache is kept uncompressed |
| `rsvg-convert` | the check that a redrawn SVG figure matches its source reports a render failure instead of a verdict |
| `tesseract` | no OCR of text inside bitmap figures; their alt-text drafts rest on the picture alone |
| `mktextfm`, `mf` | fonts with no ready TFM metrics (Metafont fonts) cannot be generated, and the build fails on them |
| `fc-list` | installed system fonts are not found; papers that load them by name (fontspec) fall back to TeX Live fonts |

The install hints are for Linux (`apt install ...`) or, on Windows, `scoop` and the vendors' pages
([Windows](windows.md)).

## TeX files

After the programs, `doctor` asks `kpsewhich` for the TeX files reLaTeX's own support needs and
lists any it cannot find (`missing TeX files: ...`). That means the TeX Live installation is
incomplete or the wrong one is on the path; `RELATEX_TEXLIVE` or `RELATEX_ENGINE` points reLaTeX at
another.

## Packages of your paper

Give `doctor` a paper (a `.tex` file, a directory or a ZIP, as `relatex` takes) and it checks the paper
instead: it reads the main file and the files it inputs, looks up each document class, package, Beamer
theme, TikZ library and bibliography style they name with `kpsewhich` (unless the paper ships the file
itself), and says whether the bibliography needs biber or BibTeX and whether that program is installed.

```text
$ relatex doctor paper.tex
Paper     paper.tex (1 source files read)
Checked   3 classes, packages and styles: 0 in the paper, 1 missing
  MISSING  nosuchpkgxyz.sty         paper.tex:2 (\usepackage)
                                    install: tlmgr install nosuchpkgxyz (the TeX Live package may be named differently; --tlmgr looks it up)
Bibliography  none needs a program
Not complete by design: only the files the source names are checked; the packages those load are not followed.

Not ready: install the missing files and programs above.
```

`--tlmgr` asks `tlmgr` which TeX Live package holds each missing file (slow; it needs the network).
`--main FILE` names the main file when `doctor` cannot choose it. The exit status is 1 when something
is missing.

The check is not complete by design: a package that another package loads is not followed, so a paper
that passes can still stop on a missing file during conversion. It then fails with a
`missing_dependency` diagnostic (`File x.sty not found`); install the file with `tlmgr install NAME`,
or put it in the paper directory ([Troubleshooting](troubleshooting.md)).
