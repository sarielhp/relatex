# Word-level SyncTeX and rlx

SyncTeX connects your PDF and your source: click in the viewer and the editor jumps to the source,
or the reverse. reLaTeX writes a SyncTeX file with a record for every word, and `rlx` builds it
the way latexmk builds a PDF.

## Word-level, not line-level

TeX's own SyncTeX file (`lualatex -synctex=1`) is line-level. A click lands on the line of a
paragraph, often the first or the last, and a click in a formula, a table or a package's macro
may land in the package. reLaTeX's file comes from its map of every word to the source file, line
and column it was typeset from, so a click on a word lands on that word.

The same click (page 1, at 150 bp, 370 bp) asked of both files with the `synctex` program that
comes with TeX Live:

```text
$ synctex edit -o 1:150:370:p.pdf        # relatex's p.synctex.gz
Input:/home/you/sums/p.tex
Line:8
Column:5

$ synctex edit -o 1:150:370:q.pdf        # the same source built with -synctex=1
Input:/home/you/sums/./q.tex
Line:8
Column:-1
```

The column is the difference: `-1` means TeX's file cannot say. The file's format is ordinary
SyncTeX (version 1, gzip-compressed), so a viewer or editor that reads SyncTeX reads it as it
reads any other, with no plugin. The author uses it daily with Emacs and a modified Okular; whether a
stock viewer passes the column on to the editor depends on the viewer. In your setup:

1. Build with `rlx` (below), which writes `NAME.synctex.gz` beside `NAME.pdf`, where a viewer looks
   for it.
2. Configure the viewer's inverse search to call your editor with the file and line as usual. A
   viewer that passes the column on to the editor puts the cursor on the word; one that passes
   only the line works as before.
3. Forward search is the viewer's too: it asks for a source position and lands on that word.

The same lookups are in `relatex sync` ([below](#relatex-sync)).

## rlx

`rlx` is the same program under another name (the installer links it). It builds the PDF and the
SyncTeX file and puts them beside the main file:

```text
$ cd ~/papers/sums
$ rlx
wrote sums.pdf, sums.synctex.gz (4s)
$ rlx
up to date: sums.pdf, sums.synctex.gz
```

- It skips the page graphics, the model and the exports, so it is fast: one instrumented
  LuaLaTeX pass after an edit, seeded with the previous run's auxiliary files.
- When nothing changed (sources, toolchain, reLaTeX itself) it says `up to date` and builds
  nothing.
- It copies `sums.bbl` when there is a bibliography.
- It reads `TEXINPUTS` as `lualatex` does, so a shared styles directory is found the same way;
  `--hermetic` builds with the paper's own files only ([details](converting.md#your-tex-search-path-texinputs)).
- `--synctex-native` writes TeX's own file instead, `--no-synctex` none, `--report` prints the JSON
  run report, `--no-publish` leaves the products in the output directory.
- Naming a format (`rlx --html`) runs a full conversion too, into `html/` and the other format entries.

## `relatex synctex`: a PDF another tool built

If your tool builds the PDF (latexmk, a plain `lualatex`), `relatex synctex` gives that PDF the
word-level file. Start it beside your build, in the background:

```text
$ relatex synctex . --main sums.tex --for sums.pdf &
$ lualatex -interaction=nonstopmode -synctex=1 sums.tex      # your build, as usual
relatex synctex: installed sums.synctex.gz (9s)
```

It builds only what the SyncTeX needs, waits (`--wait SECONDS`, default 600) until your PDF is
written, and installs `sums.synctex.gz` only if your PDF is byte for byte the PDF reLaTeX built
(the trailer `/ID` aside), so the positions are right. The write is atomic: a viewer never
reads half a file. Otherwise it leaves your file alone and says why on one line.

Both builds must typeset the same: give them the same `SOURCE_DATE_EPOCH` with
`FORCE_SOURCE_DATE=1`, the same `TZ` and the same `TEXINPUTS`. The bibliography is the `.bbl` beside
the main file. `--replace-pdf` is for a preview built by another engine: reLaTeX then replaces
the PDF and the SyncTeX file together, if the PDF is still the one it waited for.

Exit status: 0 installed, 2 refused (the PDFs differ), 3 no new PDF within `--wait`, 4 superseded
(`--replace-pdf` only), 1 failed. It runs at nice 19.

## `relatex sync`

`relatex sync` asks the saved document bundle the same two questions, as text or JSON, for a tool
of your own:

```sh
relatex sync json-gz/document.json.gz --source sums.tex --line 42
relatex sync json-gz/document.json.gz --page 1 --x 120 --y 300
```

The first lists the words a source line produced, with their boxes on the page; `--column`,
`--end-line` and `--end-column` narrow the selection. The second lists the words at a point of a
PDF page, in bp measured from the top left of the page. `--open-editor` with `--editor-argv` opens
the first match in your editor. `relatex inspect BUNDLE --word ID` shows one word with its place
in the PDF and in the source ([The document model](document-model.md)).
