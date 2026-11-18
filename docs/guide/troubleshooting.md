# Troubleshooting

## Checking the installation

`relatex doctor` lists every program reLaTeX starts (LuaLaTeX and the TeX Live tools beside it,
MuPDF, qpdf, Poppler, Typst, the browser, veraPDF, zstd, rsvg-convert, tesseract, Metafont
(`mf`, `mktextfm`), fontconfig's `fc-list` and, on Linux, bubblewrap) with its path,
its version and where it was found (a `RELATEX_*` variable, the `tools` section of the
configuration (`config`), TeX Live, beside `lualatex`, or
`PATH`), and for a missing one how to install it on Linux and on Windows. It also names the TeX
files kpsewhich cannot find, the sandbox, and the cache, temporary and configuration directories.
It exits with status 1 when a required program is missing; `--json` prints the same as JSON. A
missing optional program is listed as `optional` with a `without it:` line saying what the
conversion loses (no OCR of bitmap figures, no SVG figure check, no Metafont fonts, ...).

### Checking one paper

`relatex doctor PAPER` (a `.tex` file, a directory or a ZIP, as `convert` takes; `--main FILE`
names the main file) answers a different question: can this machine build this paper? It reads
the main file and the files it inputs, collects the document class, `\usepackage` and
`\RequirePackage` packages (comma lists and options), beamer themes, TikZ and pgfplots libraries
and `\bibliographystyle` styles, and looks each up with `kpsewhich` unless the paper has the file
itself. It lists the missing ones with `tlmgr install NAME`; the TeX Live package can have another
name than the file, so `--tlmgr` asks `tlmgr search --global --file /NAME.sty` for the real one
(slow, and it needs the network). It also says whether the bibliography needs `biber` (biblatex's
default backend) or `bibtex`, and whether that program is installed.

```text
$ relatex doctor paper.zip
Paper     main.tex (4 source files read)
Checked   27 classes, packages and styles: 0 in the paper, 2 missing
  MISSING  ghostpkg.sty             main.tex:1 (\usepackage)
                                    install: tlmgr install ghostpkg (the TeX Live package may be named differently; --tlmgr looks it up)
Bibliography  biblatex, backend biber: ok /home/user/.texlive/bin/x86_64-linux/biber
Not ready: install the missing files and programs above.
```

The exit status is 0 when everything is there and 1 when something is missing. The check is not
complete by design: a package that another package loads is not followed, so a missing second-level
file shows only when the conversion runs. Fonts that `fontspec` loads by name are not checked.

## When a conversion fails

A failed conversion ends with a `Conversion failed:` line naming the step that failed and the run
report, then where it stopped and an evidence directory, and exits with status 1 (with `--report`
the first line is `relatex: error:` instead, as tools read it):

```text
Conversion failed in engine: LuaLaTeX failed on the paper: Undefined control sequence. (report /home/you/.cache/relatex/projects/paper-749dd027ae9c/_work/conversion-20261115-1122932-1heati7/run.json)
  at paper.tex:3: Hello \undefinedmacro
Evidence: /home/you/.cache/relatex/err/s1anva
```

The steps are `input` (reading the paper, choosing the main file), `engine` (the LaTeX builds),
`reconcile` (reading the document), `layout`, `synchronize`, `export` and `publish`.

**The evidence directory** (`~/.cache/relatex/err/<id>`) is a link to the failed attempt. Inside:

```text
run.json                         the run report
layers/01-input/input/files/     the snapshot of your paper that was built
layers/02-engine/source-probe-*/control/output/   the ordinary build: pass-1.log, pass-2.log, …, NAME.log, bibtex-1.log, …
layers/02-engine/source-probe-*/external/output/  the instrumented build, the same files
layers/03-reconcile/diagnostics.json              every diagnostic, when the run got that far
```

`pass-N.log` is the LaTeX log of pass N; the last one is the one that decided. Files of a
successful attempt are compressed with zstd after the run (`*.zst`); `zstd -d` restores them.

## The build stopped: common causes

**"LuaLaTeX failed on the paper: …".** A real TeX error, with the file and line. Fix it as you
would in your own build. If your source builds with pdfLaTeX or XeLaTeX but not here, see
[pdfLaTeX and XeLaTeX sources](engines.md).

**"The builds did not converge: references or citations stayed undefined".** Both builds compiled,
but after the last pass LaTeX still reported undefined references or citations; the message names
the first five (`reference sec:two, citation knuth84 and 3 more`). reLaTeX refuses to convert a
paper whose PDF still shows `??` or `[?]`. The last `pass-N.log` of `control/output/` has every
`LaTeX Warning: Reference … undefined` and `Citation … undefined`; look in `bibtex-1.log` or the
`biber` logs for missing entries. ("LaTeX still asked for another run" means the references kept
changing from pass to pass without becoming undefined.) The usual
causes:

- a `\ref` to a label that does not exist (a typo, or a label in a file that is not included);
- a bibliography database outside the paper directory (below);
- a `.bib` entry missing, or a BibTeX error.

**Bibliography outside the source.** The build sees only the paper directory, so a database in
`~/texmf/bibtex/bib/`, in `../`, or given by an absolute path is not found, and the citations stay
undefined. Copy the `.bib` file into the paper directory. Alternatively ship the generated
`NAME.bbl` beside the main file, as arXiv does: when the databases are missing and `NAME.bbl` is
present, reLaTeX uses the `.bbl` and runs no BibTeX or Biber.

**"Cannot tell which file is the main one".** Several `.tex` files could be the main file. Use
`--main FILE` or `main.ignore` ([Converting a paper](converting.md#what-relatex-reads)). Before it
asks, reLaTeX narrows several documents by three rules, in this order, and takes the file that
remains alone: the one top-level file of arXiv's own `00README.json` (or `00README.XXX`); the one
that the others `\input` or `\include`; the only one in the paper's root directory. The choice is
recorded as `selection` in the input manifest. A paper with its supplement (two top-level files
in the README, or two documents in the root) is never decided for you.

**"No .tex file begins a document".** No `.tex` file has `\begin{document}` outside a comment;
check the directory, or name the file with `--main`.

**"Selected TeX installation lacks makeindex" (or bibtex, biber).** The TeX Live reLaTeX uses
lacks the program, or it is not beside `lualatex`. See
[Configuration](configuration.md#environment-variables).

**"Missing bubblewrap for untrusted paper compilation"** or **"TeX sandbox is unavailable".**
Install bubblewrap (`bwrap`); it must be able to create its sandbox as your user (the sandbox check's log is named in the message).

**"Source contains symlinks or special files".** Replace symbolic links in the paper directory by
the files they point to.

**"html exists in DIR and reLaTeX did not write it".** An entry reLaTeX is about to write (`html/`, `markdown/`,
`typst/`, `NAME.epub`, ...) exists in the output directory and an earlier conversion did not write it, so it is yours.
Nothing was deleted. Move it away, or write elsewhere with `-o DIR`.

**"Bundle exceeds expanded byte limit".** The book is too large for the document bundle (about
700 dense pages).

## Sources reLaTeX does not accept

The project tests reLaTeX on randomly drawn arXiv papers. Before a paper enters a test cohort, a
screen builds it plainly with LuaLaTeX and excludes it for one of the reasons below. These are the
kinds of source reLaTeX is not designed to handle; the converter itself may fail on them, or
convert them with losses, and does not always say why. If your paper has one of these problems,
fix it in the source first.

| Reason | What it means | What to do |
|---|---|---|
| `compile_failure` | LuaLaTeX could not build the paper, even with the compatibility layer | read the log; see [engines](engines.md) |
| `missing_dependency` | a package, class or style file is not in TeX Live or the paper (`File 'x.sty' not found`, a missing `.bst`) | install it with `tlmgr`, or put the file in the paper directory |
| `missing_asset` | a figure or `.bib` file the paper names is missing | add the file |
| `timeout` | the build took longer than its time limit (5 minutes for the screen) | — |
| `bibliography_failure` | BibTeX or Biber failed | read `bibtex-1.log` |
| `bibliography_limit` | the bibliography asked to be rerun more than twice | — |
| `invalid_pdf` | the build produced no readable PDF with pages | — |
| `missing_glyphs` | the log has `Missing character: There is no … in font …`: characters were silently left out of the PDF | use a font that has those characters, or a package that provides them |
| `unbalanced_input` | the log ends with `(\end occurred inside a group at level N)` or `(\end occurred when \ifx on line N was incomplete)`: a `{` or `\begingroup` never closed, or a missing `\fi`. TeX finishes anyway, but everything after that point is set inside the group | find the unclosed group or conditional (often a `\endinput` inside an `\if…\fi` guard) |
| `missing_include` | the log has `No file NAME.tex.` from an `\include`: a whole chapter is silently absent | fix the file name or add the file |
| `unresolved_references` | undefined references or citations after the last pass | as above |
| `nonconvergent` | LaTeX still asks for a rerun after five passes, or the `.aux` keeps changing | look for labels or page references that change with each pass |
| `dvi_only_graphics` | figures meant for a DVI driver (`graphicx[dvips]`, `\special{psfile=…}`), which a PDF build drops | use `\includegraphics` without a driver option |
| `undeclared_authors` | the paper has a `\title` but no author command at all (`\author`, a class spelling such as `\icmlauthor`, spconf's `\name` or `\twoauthors`): its authors are hand-typed text in the title block | declare the authors with `\author` and `\maketitle` |

Of these, reLaTeX itself stops on build errors, missing files, and unresolved references. It does
not check for missing glyphs, unbalanced input or missing `\include` files: a paper with them may
convert, and its export then reproduces a PDF that is itself wrong. Search your own LaTeX log
for `Missing character`, `\end occurred` and `No file`. A paper with undeclared authors converts
too: its title, authors and abstract stay body text (`frontmatter.unsupported`).

## The export looks wrong

**Characters show as `�`.** A glyph had no Unicode mapping (`text.unmapped_glyph`,
`math.unmapped_glyph`, level error). See [Diagnostics](diagnostics.md).

**A reference prints as plain text, not a link.** Look for `reference.*` and `label.*` alerts.

**Fonts differ from the PDF.** The page uses your fonts where their files could be linked, and
browser fonts otherwise (`fonts.source_unavailable`). The fonts are files in `fonts/` beside the page; keep
that directory with the page when you copy it.

**A drawing's labels are not selectable.** TikZ and other drawings without an image file are
shown as a crop of the PDF (`figure.rendered_from_pdf`).

**A formula is too wide.** Display formulas scroll sideways inside their own box; the page itself
does not.

**Something is missing and no diagnostic says so.** That is a bug. Keep the attempt directory
(the path printed as `Conversion attempt:`) and report it with your source.

## Logs and evidence of a successful run

The attempt directory is printed first (`Conversion attempt: …`). It holds every step's files
under `layers/`, the LaTeX logs as above, and `run.json`. reLaTeX keeps the two newest attempts
per paper in `~/.cache/relatex/projects/<paper>-<hash>/_work/`. `relatex layers ATTEMPT` lists an
attempt's steps.

To start over completely, delete the paper's project directory under
`~/.cache/relatex/projects/`, or run with `--no-cache`.
