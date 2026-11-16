# Converting a paper

## The short version

```sh
cd ~/papers/sums          # holds sums.tex
relatex                   # Markdown and HTML into ./markdown/ and ./html/
relatex --epub            # only sums.epub (a format flag selects the formats)
relatex --all             # every format (not the tagged PDF)
relatex --accessible      # HTML, EPUB and the tagged PDF, checked, with accessibility.html
```

`relatex` with no command means `relatex convert`. A small paper takes about 10 to 20 seconds the
first time; an unchanged paper converts again in a few seconds, from the cache.

## What reLaTeX reads

**A directory or a ZIP file.** `relatex [convert] [ZIP_OR_DIRECTORY]`; the default is the current
directory. A directory is first copied into a snapshot (a ZIP) and the conversion works on the
snapshot, so your files are never touched. Everything in the directory is part of the paper,
except the output directory, reLaTeX's own cache and the figure descriptions `relatex/alt.toml` ([Figure descriptions](figure-descriptions.md)). The directory may not contain symbolic links
or special files. Limits: a ZIP of at most 256 MiB, 2 GiB unpacked, 100,000 entries; encrypted
ZIP entries are refused.

**Only the paper.** The build runs in a sandbox that sees the paper, the TeX Live tree and the
system's libraries and fonts, and has no network. A `.bib`, `.sty` or figure kept elsewhere (for
example in `~/texmf`, or `\input{../macros}`) is not found; copy it into the paper directory.

**The main file** is chosen in this order:

1. `--main FILE` (a path inside the paper, ending in `.tex`);
2. else the only `.tex` file at the top level, not counting `prefix.tex` and `prelim.tex`;
3. else the only `.tex` file anywhere whose text has `\begin{document}` outside a comment.

If none of these gives exactly one file, reLaTeX stops and lists the candidates:

```text
Conversion failed in input: Cannot tell which file is the main one. Candidates:
  a.tex
  b.tex
Choose one with --main FILE, or list the others under main.ignore in relatex/config.jsonc
Evidence: /home/you/.cache/relatex/err/k2v9qd
```

Every failure reads this way: the step that failed and the message first, the evidence directory
on the last line ([Troubleshooting](troubleshooting.md#when-a-conversion-fails)). A failed run
leaves no empty output directory behind.

To make the choice permanent, put the other files under `main.ignore` in the paper's
`relatex/config.jsonc` (see [Configuration](configuration.md)):

```jsonc
{ "main": { "ignore": ["b.tex"] } }
```

The main file's name is the job name: `sums.tex` gives `sums.pdf` and `sums.epub`.

## What reLaTeX writes

**Where.** The formats you asked for go to the directory you run reLaTeX in (not the paper directory, when the
two differ), each in its own entry: `html/`, `markdown/`, `typst/`, `NAME.epub`, `NAME-tagged.pdf` (the tagged PDF
of `--pdf-ua`, named so that it never replaces the PDF you build with `rlx` or LuaLaTeX), `json/`, `json-gz/`, `pdf/`,
`typst-layout/`. Nothing else appears there, not even a hidden file: with `--typst` the directory gains `typst/` and
nothing more. `--output DIR` writes `DIR/html`, `DIR/markdown` and so on instead. reLaTeX keeps the list of the entries
it wrote in the project cache (one list per output directory), replaces exactly those on the next conversion and never
touches anything else; if an entry it is about to write exists and is not one of its own (an `html/` of yours), the
conversion stops at once, names the entry and `-o` as the way out, and deletes nothing. An `output/` directory of
an earlier reLaTeX is left as it is.

The full product (the document bundle, the layout, the records, every format, the files the next conversion reuses)
is kept in the project cache, `~/.cache/relatex/projects/NAME-HASH/product/`, and the run report names it
(`product`; `outputs` lists the deliverables). `--product DIR` writes it to DIR instead, and then writes no output
directory unless `-o` is also given; the development tools use it. `relatex alt`, `lint` and `tag-pdf` read the
last product from the cache.

**On the terminal.** The default output is for a person. While it converts, reLaTeX shows a busy
indicator with the current stage on stderr (only when stderr is a terminal; a pipe or a file gets
nothing). At the end it prints one line on stdout, with the place of what you asked for (for
`--typst`, the `typst` directory), the number of errors and warnings, and the run report:

```text
Conversion succeeded: /home/you/sums/typst (0 errors, 1 warning; report /home/you/.cache/relatex/projects/sums-db2189c12550/_work/conversion-20261030-535700-1fyecb5/run.json)
```

A conversion that stops reads `Conversion failed: <the first error> (report …)`. Diagnostics of level
`error` and above are listed first on stderr, at most ten, then `and N more in <report>`; the
report holds them all, warnings included.

`--report` prints the JSON run report on stdout instead of that line, with the attempt directory
and every diagnostic on stderr, as tools read it (`--no-report` is the default). The report is
`run.json` in the attempt directory, written either way.

The exit status is 0 when the conversion is clean, 3 when it wrote everything but has
diagnostics or unverified parts (the usual case), 1 when it failed, and 2 for a wrong option.
`relatex` with nothing to convert says so the same way. Wrong options are reported with a
suggestion (`relatex convert --no-chache` says `unknown flag`; `relatex conver` says
`Did you mean "convert"?`); abbreviated options (`--out`), options in another case (`--HTML`) and
`--no_cache` for `--no-cache` are errors, one spelling per option.

**The directory you run in** after `relatex --html --epub --typst` (your own files stay as they are):

```text
sums/
  sums.tex                 your source
  html/                    the reading page: index.html, preview.js, fonts/, figs/, accessibility.html
                           (a book: the contents page is index.html, one file per chapter beside it)
  sums.epub                the EPUB 3 package
  typst/                   the Typst project (main.typ, references.bib, chapters/, figs/, support/)
```

**The product** (in the project cache, or `--product DIR`) holds all of it and more:

```text
product/
  index.html, preview.js, fonts/, figs/   the reading page at the top
  sums.pdf                 the PDF LuaLaTeX built
  html/ markdown/ epub/ typst-source/ ...   one directory per format written
  document.json.gz         the document bundle every format is made from
  layout/                  the bundle's page layout, one file per chapter or page range
  accessibility.json       the accessibility measurement (with --verify-browser)
  browser-verification.json  what the browser checked (with --verify-browser)
  manifest.json            every file with its SHA-256
  provenance.json          the source's SHA-256, the main file and the settings used
```

Each format directory of the product has an `export.json` (its status and losses) and a `stage.json` (the
receipt of the step that wrote it). Figures are files under `figs/`, named after the LaTeX source
they came from; a figure's original (for example the source PDF) is kept beside the SVG or PNG the
page shows. A book adds one file per chapter at the top level ([Books](books.md)). Every page at the top
level finds the files it loads beside it; reLaTeX checks this before it publishes.

## Formats

The default formats are Markdown and HTML. A format flag (`--html`, `--markdown`, `--epub`, `--typst`,
`--pdf-ua` and so on) or `--format` (a comma-separated list) selects exactly the formats named: the
defaults apply only when you name none. `--all` selects every format.

| Format name | Flag | Writes |
|---|---|---|
| `html` | `--html` | `html/index.html`: the reading page (a book: contents page and chapter files) |
| `markdown` | `--markdown` | `markdown/document.md` |
| `epub` | `--epub` | `NAME.epub`: EPUB 3 |
| `typst-source` | `--typst` | `typst/main.typ`: a Typst project that keeps your labels, references and citations |
| `typst` | `--typst-layout` | `typst-layout/document.typ`: Typst that reproduces the reading page instead |
| `json` | `--json` | `json/document.json`: the document model |
| `json.gz` | `--json-gz` | `json-gz/document.json.gz` |
| `pdf` | `--pdf` | a PDF reconstructed from the page graphics (a development aid; not your PDF) |

For example, `relatex --format html,epub` writes only the page and the EPUB.

`--all` leaves out the tagged PDF (`pdf-ua`), which is opt-in; `--pdf-ua` writes `NAME-tagged.pdf`. `--accessible` is the
accessibility conversion: it adds HTML, EPUB and the tagged PDF, measures the page in a browser
(`--verify-browser`) and writes `accessibility.html`, the report to keep ([Accessibility](accessibility.md#1-convert)).

**HTML.** A self-contained page: open it from disk, no server needed. It follows your system's light or dark
setting; the button at the top right switches between the light theme (the PDF's colours) and
the dark one, and the browser remembers the choice. Every reference, citation and footnote is a link, and hovering shows the
target. Formulas are MathML, which current browsers render natively.

**EPUB** (`--epub`). The reading page as an EPUB 3 package: XHTML with MathML, the figures the
page shows (not their originals), the fonts, and a navigation document. It has no scripts, opens
in the light palette (reading systems bring their own night mode), and carries EPUB Accessibility
1.1 metadata ([Accessibility](accessibility.md)). A paper is split into one content document per
top-level section when its links allow it; a book into one per chapter. The same source gives the
same bytes; the modification date is `SOURCE_DATE_EPOCH` when set, else the PDF's creation date.

**Markdown.** Inline formulas are your own TeX (`$n \ge 1$`) where reLaTeX could typeset it again
and get the same formula; otherwise TeX derived from the MathML, or the MathML itself. Tables,
the title block and some formulas stay as embedded HTML. `--no-author-math` uses derived TeX for
every formula. HTML and EPUB read the same TeX for meaning: a formula whose macros say norm, transpose or
absolute value carries that as MathML intent, which costs a second typesetting run (about 10 s on a 480-formula paper);
`--no-author-math` skips both.

**Typst** (`--typst`). See the [design FAQ](../design-faq.md#typst-markdown) for what it
keeps. `--compile-typst` also compiles it with the `typst` program to `main.pdf`, and fails the
conversion if that fails.

## Browser verification

`--verify-browser` loads the finished page in Chromium, offline, and checks that its blocks,
formulas, references and notes are in the page and render; it also runs axe-core for the
accessibility report. The result is `browser-verification.json` and `accessibility.json`. Without
a configured endpoint, reLaTeX starts `/usr/bin/chromium` (or `RELATEX_CHROMIUM`); a remote Chrome
is set with `browser.endpoint` ([Configuration](configuration.md#browser)). If no browser can be
reached, the export is still published, with the diagnostic `browser.unverified`.

## When a conversion fails

A failed conversion leaves no report that looks current. The products of the conversion before it
(`accessibility.json`, `accessibility.html`, the pages) move into `_stale/` of the product and of the project cache's record of the output directory (the error message names
it; your directory gains nothing), and `_conversion-failed.txt` there says what failed. The next successful conversion
removes both.

## Caching, attempts and reruns

reLaTeX keeps its work outside your paper, in a project cache:

```text
~/.cache/relatex/projects/<paper directory name>-<hash>/
  _work/conversion-<date>-<id>/    one attempt: every step's files and logs
  _cache/stages/                   finished steps, reused by later runs
```

(`~/.cache` is `$XDG_CACHE_HOME` when set.) A rerun reuses every step whose inputs did not change,
so converting an unchanged paper again, or with other formats, rebuilds nothing in TeX. The two newest attempts are kept;
older ones are removed, and the kept ones are compressed with zstd when it is installed.

| Option | Effect |
|---|---|
| `--no-cache` | run every step afresh, write no cache |
| `--force STAGES` | rerun these steps and everything after them, for example `--force export`. Steps: `input`, `engine`, `reconcile`, `layout`, `synchronize`, `export`, `publish`; or one format, such as `export.html` |
| `--resume ATTEMPT` | continue from an earlier attempt directory, with its settings |
| `relatex replay ATTEMPT [--force STAGES]` | the same as `--resume ATTEMPT` |
| `--work DIR` | put the attempt in DIR (inside the output directory or the project cache) |
| `--product DIR` | write the full product to DIR instead of the project cache, and no output directory unless `-o` is given (for tools) |
| `--jobs N` | parallel LaTeX builds, 1 to 8 (default 2) |
| `--no-layout` | skip the page graphics; also turns off source synchronization |
| `--no-sync` | skip the word-to-source map |

The setting `"work": "output"` keeps attempts and the cache in `_product/` of an output directory given with `-o`; when the\noutput directory is the paper's own (the default), the cache is used, so `_product` never appears beside your files.

## SyncTeX

reLaTeX can write a SyncTeX file beside the PDF, so a PDF viewer can jump between the PDF and your
source:

- `--synctex`: built from reLaTeX's own word map; the most precise, down to the word and column;
- `--synctex-native`: the file TeX recorded, retargeted to the packaged sources;
- `--no-synctex`: none (the default for `relatex`).

`--publish` copies the PDF, the SyncTeX file and the `.bbl` beside your main file, as latexmk
does.

## rlx: reLaTeX as latexmk

`rlx` is the same program under another name (`tools/install` makes the link `~/bin/rlx`). It
builds the PDF and a word-level SyncTeX file and puts them beside the main file:

```text
$ cd ~/papers/sums
$ rlx
wrote sums.pdf, sums.synctex.gz (4s)
$ rlx
up to date: sums.pdf, sums.synctex.gz
```

- It skips the page graphics, the document model and the exports, so it is fast: one
  instrumented LuaLaTeX pass after an edit, seeded with the previous run's auxiliary files.
- When nothing changed (sources, toolchain, reLaTeX itself) it says `up to date` and builds
  nothing. Deleting a product or editing a source rebuilds.
- It also copies `sums.bbl` when there is a bibliography.
- Exit status 0 when the products were written, even with warnings.
- Naming a format (`rlx --html`) runs a full conversion as well, into `html/` and the other format entries, and still
  publishes the PDF beside the main file.
- `--report` prints the JSON run report as `relatex` does; `--no-publish` leaves the products in
  the output directory; `--no-synctex` or `--synctex-native` choose the SyncTeX file.

## Your TeX search path (TEXINPUTS)

reLaTeX reads `TEXINPUTS` as `lualatex` does, so a paper builds with the same files as your own
build. With `TEXINPUTS=~/papers/styles//:.:../:../../:`, a newer `sariel_biblatex.sty` in
`~/papers/styles` wins over a stale copy beside the paper, for `relatex`, `rlx` and
`relatex synctex` alike:

- the directories are searched in your order; a relative entry (`.`, `../`) is read from the main
  file's directory, as `lualatex` run there reads it, and `//` searches below a directory;
- an empty entry (a leading, doubled or trailing colon) is TeX's own tree; without one, only your
  directories are searched, as kpathsea does;
- reLaTeX's own TeX files come just before TeX's tree;
- what your directories hold counts as a source: a changed shared style rebuilds, and `rlx` is no
  longer `up to date`;
- the run records the path as you wrote it (your home directory as `~`) in `run.json` and the
  output's `provenance.json` (`tex_search_path`), and `relatex` prints it with the engine lines.

`--hermetic` builds without it: the paper's own files and TeX's tree only, as an archive is built.
A ZIP or tar archive (an arXiv source, say) is always built so: it is the paper as shipped.

## reLaTeX's SyncTeX with another build tool

If another tool builds your PDF (leeni, latexmk, a plain `lualatex`), `relatex synctex` gives that
PDF reLaTeX's word-level SyncTeX file, so your viewer jumps to the word and column instead of the
line. Run it beside your own build, in the background:

```text
$ relatex synctex . --main sums.tex --for sums.pdf &
$ lualatex -interaction=nonstopmode -synctex=1 sums.tex   # your build, as usual
relatex synctex: installed sums.synctex.gz (9s)
```

What it does:

1. It builds what the word-level SyncTeX needs and nothing else: the instrumented LuaLaTeX build
   (seeded with the previous run's auxiliary files, so one pass after an edit), and the SyncTeX
   writer. No page graphics, HTML, MathML, browser, EPUB or bundle.
2. The bibliography is the `.bbl` beside the main file (your tool's); reLaTeX runs BibTeX or Biber
   only when there is none.
3. It waits (`--wait SECONDS`, default 600) for `--for PDF` to be written after it started, and
   for the file to stop growing.
4. It installs `sums.synctex.gz` only when your PDF is byte for byte the PDF reLaTeX built, except
   the trailer `/ID` (LuaTeX hashes the build directory into it). It writes `sums.synctex.gz.tmp`
   and renames it over `sums.synctex.gz`, so a viewer that reloads SyncTeX never reads half a file.
   It never writes the PDF or anything else beside the paper.
5. Otherwise it leaves your SyncTeX file alone and says why on one line, naming the sources that
   changed while it built (an edit, or the `.bbl` your tool rewrote: the next build matches) or the
   first page that differs.

**A preview from another engine (`--replace-pdf`).** XeLaTeX's own SyncTeX names the right files
where LuaLaTeX's can send clicks into a package (a TikZ library), and its PDF never equals
reLaTeX's LuaLaTeX build. With `--replace-pdf` your tool builds a quick preview with XeLaTeX, and
reLaTeX replaces the whole pair with its own: it notes the first PDF written after it started
(its SHA-256), builds, writes `sums.pdf.tmp` and then `sums.synctex.gz.tmp`, and, if `sums.pdf` is
still the file it noted and every source still holds what its build read, renames the SyncTeX
file into place first and the PDF second. A viewer that rereads the SyncTeX file when the PDF
changes finds the new one already there; one that rereads a changed SyncTeX file sees it beside
the old PDF only for the microseconds between the two renames. Otherwise (a newer build wrote the
PDF, or you edited a source) it installs nothing: `relatex synctex: superseded: …`, exit status 4.
The viewer shows XeLaTeX's PDF first and LuaLaTeX's a few seconds later, so this suits papers
whose two builds look alike. leeni does it with `--xelatex-preview`.

Exit status: 0 installed, 2 refused (`relatex synctex: refused: …`), 3 no new PDF within
`--wait`, 4 superseded (`--replace-pdf` only), 1 failed. SIGTERM or SIGINT stop it with every program it started (LuaLaTeX in its
sandbox included) and remove its working files. It runs at nice 19.

**Give both builds the same environment.** The PDFs match only when both builds typeset the same
text with the same dates:

- `SOURCE_DATE_EPOCH` and `FORCE_SOURCE_DATE=1`: pin one build time for both, or `\today` and
  the PDF dates differ (set them for your tool's `lualatex` too);
- `TZ`: reLaTeX uses yours, or the host's local time when it is unset;
- `TEXINPUTS`: reLaTeX searches your directories in your order (a shared styles directory such as
  `~/papers/styles//` wins over a copy beside the paper, as it does for your `lualatex`).

leeni does all of this with `l -e lualatex --relatex-synctex paper.tex`.

## Other commands

| Command | Does |
|---|---|
| `relatex export DOCUMENT.json.gz --output DIR --format FORMAT` | writes one format from a saved bundle, into a new `DIR/export-<date>-<id>/`, without rebuilding. (HTML is written as one page, without the browser check or the book split.) The bundle's `layout/` directory must sit beside it. |
| `relatex sync DOCUMENT.json.gz --source FILE --line N` | forward search: where a source line landed in the PDF. `--page N --x BP --y BP` searches backwards. |
| `relatex inspect BUNDLE --node ID` | shows one element of the document (also `--asset`, `--cell`, `--word`) |
| `relatex layers ATTEMPT` | lists an attempt's steps and files |
| `relatex capabilities` | what this build implements |
| `relatex config [DIR]` | the settings a conversion in DIR would use ([Configuration](configuration.md)) |
| `relatex alt draft\|review\|accept\|import` | figure descriptions in `relatex/alt.toml` ([Figure descriptions](figure-descriptions.md)) |
| `relatex synctex [DIR] --main FILE --for PDF` | the word-level SyncTeX for a PDF another tool builds ([above](#relatexs-synctex-with-another-build-tool)) |
| `relatex lint --accessibility [DIR]` | what only you can supply for an accessible export, read from the source in a moment; `--annotate DIR` writes a marked copy of the source to fill in ([Accessibility](accessibility.md#2-lint-what-is-open-in-your-source)) |

The commands that report on a saved bundle or an attempt (`export`, `reconstruct`, `validate`, `inspect`, `sync`, `layers`, `capabilities`, `config`, `tag-pdf` and `alt draft|review|accept|import|set`) print a short text for a person: what they did, where the result is and what went wrong. `--report` prints the JSON report on stdout instead, as `convert` does, for programs to read; a tool that reads the output of one of them passes it. `lint` and `doctor` print text already and give their JSON with `--json`; `synctex` and `alt geometry` print text.

Every command prints its options with `-h` (short) or `--help` (with its examples; `-E` shows only the examples), and `relatex help COMMAND` shows the same (`relatex replay --help`). `relatex completion install` sets up tab completion for your shell and `relatex manpage` prints a manual page; `relatex --version` prints `relatex VERSION`.

Independent acceptance (checking an export against an authored proof of the paper) is a
development tool of the reLaTeX repository, not part of the product: `relatex verify` and the
options `--verify`, `--authored-proof` and `--frozen-pdf` of the archived Ruby program are not in
the help and stop with a message saying so. To check the page in a browser, use `--verify-browser`.
