# Configuration

reLaTeX works without any configuration. Settings come from three places, each overriding the one
before:

1. the built-in defaults;
2. your global file, `~/.config/relatex/config.jsonc` (`$XDG_CONFIG_HOME/relatex/config.jsonc`
   when `XDG_CONFIG_HOME` is set; on Windows `%APPDATA%\relatex\config.jsonc`);
3. the paper's own `relatex/config.jsonc`, in the paper directory;

and command-line flags override all three. Nested objects (`main`, `browser`, `alt_text`, `tools`) are merged key by
key; other values are replaced.

The files are JSON with comments (`//`, `/* */`) and trailing commas allowed. An unknown key or a
value of the wrong type stops the conversion with the file named, so a typo never silently
changes a conversion:

```text
relatex: error: relatex/config.jsonc: unknown setting "jobz"
```

The paper's `relatex/config.jsonc` is read only when the input is a directory, not a ZIP file. It is part
of the paper, so changing it counts as a change to the source.

## The config command

```sh
relatex config                 # the settings a conversion here would use, and the files read
relatex config ~/papers/sums   # the same for another directory
relatex config --init          # write the commented global template
relatex config . --init-project  # write ./relatex/config.jsonc with every setting commented out
```

Neither `--init` form overwrites an existing file.

`--init` writes the complete template, with every setting at its default. `--init-project` writes
the same settings with every one commented out, so the new file changes nothing: the paper's file
overrides the global one key by key, so uncomment only the settings you mean to set for this
paper.

## Settings

| Setting | Default | Meaning |
|---|---|---|
| `main.ignore` | `["prefix.tex", "prelim.tex"]` | file names never taken as the main file (compared without case, by base name) |
| `formats` | `["markdown", "html"]` | formats written by default: any of `markdown`, `html`, `typst-source`, `typst`, `json`, `json.gz`, `pdf`, `epub`. Flags such as `--epub` add to the list; `--format` replaces it |
| `author_math` | `null` | your own TeX in Markdown formulas and the meaning your macros give HTML and EPUB formulas (`\norm`, `^\top`): `true`, `false`, or `null` for "whenever Markdown, HTML or EPUB is written" (`--[no-]author-math`) |
| `output` | `"."` | where results go, relative to the directory reLaTeX runs in (`--output`): by default that directory itself, each format in an entry of its own (`html/`, `markdown/`, `NAME.epub`), and nothing else |
| `work` | `"cache"` | where attempts and the stage cache go: `"cache"` keeps them in `~/.cache/relatex/projects/`, so the output holds only results; `"output"` keeps them in `_product/` of an output directory given with `-o` (never in the paper's own directory, which keeps using the cache) |
| `jobs` | `2` | parallel LaTeX builds, 1 to 8 (`--jobs`) |
| `source_fonts` | `true` | link the paper's own fonts in HTML and Markdown (`--[no-]source-fonts`) |
| `browser` | see below | the browser for `--verify-browser` |
| `alt_text` | see below | the command that drafts figure descriptions (global file only) |
| `tools` | see below | explicit paths of the programs reLaTeX starts (global file only) |

### browser

| Setting | Default | Meaning |
|---|---|---|
| `browser.endpoint` | `null` | a remote Chrome's DevTools address, `http://` with host and port, such as `"http://192.0.2.10:9223"`. `null` starts the local Chromium. `RELATEX_BROWSER_ENDPOINT` overrides it |
| `browser.sessions` | `null` | the most pages checked at once, 1 to 64; `null` means 6 on a remote endpoint and half the processors (at least 2) locally |
| `browser.token_file` | `null` | a file holding a bearer token, for an endpoint that requires one |

The page is handed to the browser by reLaTeX itself, so a remote browser needs no access to your
files or to the internet.

### Examples

A global file that always writes the EPUB and uses a remote browser:

```jsonc
// ~/.config/relatex/config.jsonc
{
  "formats": ["markdown", "html", "epub"],
  "browser": { "endpoint": "http://192.0.2.10:9223" },
}
```

A paper whose directory also holds a second document:

```jsonc
// ~/papers/sums/relatex/config.jsonc
{ "main": { "ignore": ["prefix.tex", "prelim.tex", "response-to-referees.tex"] } }
```

`main.ignore` replaces the default list, so repeat `prefix.tex` and `prelim.tex` if you still want
them ignored.

### alt_text

The command that drafts figure descriptions (`relatex alt draft`). Read from the global file
only: a paper's `relatex/config.jsonc` that sets it is refused, so a paper from someone else can never
choose a program to run. See [Figure descriptions](figure-descriptions.md).

| Setting | Default | Meaning |
|---|---|---|
| `alt_text.command` | `null` | the program and its arguments, as a list (`["~/bin/describe-figure"]`); reLaTeX appends the figure file and writes the prompt on its standard input |
| `alt_text.uploads` | `true` | the command sends figures off this machine; each paper then needs your approval once (`relatex alt draft --allow-upload`). `false` for a local model |
| `alt_text.model` | `null` | the name drafts record (default: the program's file name) |
| `alt_text.timeout_seconds` | `120` | per figure, 1 to 3600 |

### tools

Where the programs reLaTeX starts are, when `PATH` and TeX Live's own directory do not find them:
a MuPDF unpacked from a zip, a Chrome in your home directory, a second TeX Live. Read from the
global file only, for the same reason as `alt_text`. Each setting is `null` (find the program as
usual) or the program's path; `~` is your home directory, and on Windows backslashes are doubled
(`"C:\\Users\\me\\mupdf\\mutool.exe"`) or written as `/`.

| Setting | Program | Overridden by |
|---|---|---|
| `tools.lualatex` | the engine; TeX Live's other programs (kpsewhich, biber, bibtex, makeindex, epstopdf) are taken from its directory, and its tree is the TeX Live tree | `--engine`, `RELATEX_ENGINE`, `RELATEX_TEXLIVE` (the tree) |
| `tools.mutool` | MuPDF 1.28.x | `RELATEX_MUTOOL` |
| `tools.qpdf` | qpdf | `RELATEX_QPDF` |
| `tools.pdfinfo`, `tools.pdftocairo`, `tools.pdftotext`, `tools.pdftoppm` | poppler | `RELATEX_PDFINFO`, ... |
| `tools.tesseract` | OCR of figure text | `RELATEX_TESSERACT` |
| `tools.typst` | Typst, for `--compile-typst` and the Typst PDF | `RELATEX_TYPST` |
| `tools.browser` | Chromium, Chrome or Edge for `--verify-browser` | `RELATEX_CHROMIUM`; `browser.endpoint` uses a remote browser instead |

A path that names no program is reported missing; reLaTeX never falls back to another copy of the
program. `relatex doctor` lists every program with its path and where it came from (`config`,
`RELATEX_...`, `TeX Live`, `PATH`):

```jsonc
{
  "tools": {
    "mutool": "C:\\Users\\me\\scoop\\apps\\mupdf\\1.28.5\\mutool.exe",
    "browser": "~/opt/chrome/chrome"
  }
}
```

## Command-line options without a setting

`--main FILE`, `--engine PATH` (the `lualatex` to use), `--work DIR`, `--cache DIR` / `--no-cache`,
`--resume`, `--force`, `--split`, `--synctex` and its variants, `--publish`, `--report`,
`--verify-browser`, `--accessible`, `--pdf-ua`, `--compile-typst`, `--no-layout`, `--no-sync`, `--allow-fallback image`. See
[Converting a paper](converting.md) and `relatex --help`.

`--allow-fallback image` lets Markdown and Typst show a formula they cannot express as a picture
taken in the browser, instead of failing. Pictures of formulas are not accessible; prefer to report
the formula.

## Environment variables

| Variable | Effect |
|---|---|
| `RELATEX_BROWSER_ENDPOINT` | overrides `browser.endpoint` |
| `RELATEX_CHROMIUM` | the local browser to start, overriding `tools.browser` (default `/usr/bin/chromium`; on Windows Microsoft Edge) |
| `RELATEX_TEXLIVE` | the TeX Live tree (default `~/.texlive`); reLaTeX uses `bin/<platform>/lualatex` in it |
| `RELATEX_ENGINE` | the `lualatex` to use, overriding the tree (`--engine` overrides both). Without either, reLaTeX uses the tree's `lualatex`, else `lualatex` on `PATH` |
| `RELATEX_QPDF`, `RELATEX_MUTOOL`, `RELATEX_PDFINFO`, `RELATEX_PDFTOTEXT`, `RELATEX_PDFTOCAIRO`, `RELATEX_PDFTOPPM`, `RELATEX_TESSERACT`, `RELATEX_TYPST` | the path of that helper program, instead of `tools` or the one on `PATH` |
| `XDG_CACHE_HOME` | the cache root (default `~/.cache`); reLaTeX uses `relatex/` in it |
| `XDG_CONFIG_HOME` | the configuration root (default `~/.config`) |
| `SOURCE_DATE_EPOCH`, `FORCE_SOURCE_DATE` | passed to LuaLaTeX (with `FORCE_SOURCE_DATE=1`, `\today` is that date); `SOURCE_DATE_EPOCH` also dates the EPUB. A build with another date never shares a cache entry |
| `NO_COLOR` | no colour in terminal messages |
| `RELATEX_TRACE=1` | prints the time of each step |

BibTeX, Biber, makeindex, kpsewhich and epstopdf are taken from the same directory as the
`lualatex` reLaTeX uses.
