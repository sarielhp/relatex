# reLaTeX on Windows

reLaTeX runs natively on Windows 10 and 11, on x86-64 and on arm64, with TeX Live for Windows.
MiKTeX is not supported yet. Everything in the rest of this guide holds on Windows too; this page
covers what differs: installing, checking the installation, and the known limits. The rules are
in the [design FAQ](../design-faq.md#trusted-sources) (the sandbox) and [Go and Ruby](../design-faq.md#go-and-ruby).

## Install

### 1. reLaTeX

Either through [scoop](https://scoop.sh), with the manifest published beside the release zips
(`relatex.json`):

```powershell
scoop install https://RELEASE-URL/relatex.json
```

scoop then also installs MuPDF, qpdf and Poppler (the manifest's `depends`). Or from the release
zip (`relatex-VERSION-windows-arm64.zip` or `-amd64.zip`): unpack it into a folder on your `PATH`.
The zip holds `relatex.exe`, `rlx.exe` (the same program; it runs in the `rlx` mode by its name),
`README-windows.txt` and the `LICENSE`. Take the arm64 zip on an arm64 machine (Surface Pro X,
Snapdragon laptops) and the amd64 zip otherwise.

### 2. TeX Live

Install TeX Live 2026 for Windows with `install-tl-windows.exe` from
<https://tug.org/texlive/windows.html>, the full scheme (or a smaller scheme plus the packages
`relatex doctor` names as missing). reLaTeX finds the TeX Live whose `lualatex` is on `PATH`, else
the newest `texlive\YYYY` on the system drive (`C:\texlive\2026`) or in your profile
(`%USERPROFILE%\texlive\2026`), so TeX Live need not be on `PATH`. `RELATEX_TEXLIVE` names another
tree, and the `tools.lualatex` setting another engine.

TeX Live ships only x86-64 programs for Windows. On an arm64 machine they run under Windows' x64
emulation, which works but is slower: on an arm64 laptop, conversions took three to eight times
as long as on a Linux desktop (below). reLaTeX itself runs natively.

### 3. The PDF tools

| Program | What for | Install |
|---|---|---|
| MuPDF (`mutool`) **1.28.x** | layout capture, figures, table and text paint | `scoop install mupdf` |
| qpdf | PDF normalization, tagging | `scoop install qpdf` |
| Poppler (`pdfinfo`, `pdftotext`, `pdftoppm`, `pdftocairo`) | page images, figure geometry | `scoop install poppler` |
| Typst (optional) | `--typst` PDF | `scoop install typst` |
| zstd (optional) | cache compression | `scoop install zstd` |
| rsvg-convert (optional) | check of redrawn SVG figures | `scoop install rsvg-convert` |
| tesseract (optional) | OCR of text in bitmap figures | `scoop install tesseract` |
| a browser (optional) | `--verify-browser` | Microsoft Edge, part of Windows |

MuPDF must be a 1.28 release (1.28.0 to 1.28.x): a different series changes the structured text and
the SVG forms reLaTeX reads, and `relatex doctor` warns about it. If scoop has moved on to a newer
series, download 1.28.x from <https://mupdf.com/releases> and name it in the configuration (below).

### 4. Programs that are not on PATH

A program you unpacked yourself is named by path in the `tools` section of your configuration,
`%APPDATA%\relatex\config.jsonc` (`relatex config --init` writes a commented one). Backslashes are
doubled in JSON, or written as `/`:

```jsonc
{
  "tools": {
    "mutool": "C:\\Users\\me\\mupdf-1.28.5\\mutool.exe",
    "lualatex": "D:/texlive/2026/bin/windows/lualatex.exe",
    "browser": "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
  }
}
```

The `RELATEX_*` variables (`RELATEX_MUTOOL`, `RELATEX_ENGINE`, `RELATEX_CHROMIUM`, ...) override
these. A path that names no program is reported missing; reLaTeX never falls back to another copy.
See [Configuration](configuration.md#tools).

## Check the installation

```powershell
relatex doctor
```

It lists every program reLaTeX starts with its path, its version and where it was found (`config`,
a `RELATEX_*` variable, `TeX Live`, `beside lualatex`, `PATH`), and how to install a missing one.
It also names the TeX files kpsewhich cannot find, the sandbox mode, the cache, temporary and
configuration directories, and whether long paths are enabled. It exits with status 1 while a
required program is missing, and ends with `Ready.` when everything is there. A scoop shim is
reported as the program it starts.

Then convert a paper as on Linux:

```powershell
relatex C:\Users\me\papers\paper.zip --main paper.tex --html --epub
```

## Where reLaTeX keeps its files

| What | Where |
|---|---|
| configuration | `%APPDATA%\relatex\config.jsonc` |
| cache (stage cache, projects, the TeX font cache) | `%LOCALAPPDATA%\relatex` |
| temporary files | `%TEMP%\relatex` |

`XDG_CONFIG_HOME` and `XDG_CACHE_HOME` still override the first two when set.

## Known limits

- **No sandbox.** On Linux, TeX runs inside bubblewrap and sees only the paper, its output and the
  TeX tree. Windows has no such sandbox, so TeX runs with its own file limits only: no shell
  escape, `openin_any=p` and `openout_any=p` (TeX reads and writes no file outside the paper's
  directory, its output and the font cache). Lua code in a paper can still reach any file you can.
  Every export records this as the `sandbox.none` diagnostic (alert). **Convert only papers you
  trust.**
- **Your own fonts.** Without the sandbox, luaotfload's font database also indexes the fonts
  installed in Windows. A paper that asks for a font by name can then get one Linux would not
  find, and the build can differ from a Linux build.
- **Names Windows cannot store.** A paper with a file named `aux.tex`, `con.sty` or another device
  name, a name with `< > : " | ? *`, or a name that ends with a dot or a space is refused with an
  `input.unsafe` error naming the file. Two files whose names differ only in case are refused on
  every platform, and so are two figures whose names differ only in case (`Plot.pdf` and
  `plot.png`): their files would overwrite each other in `figs/`. Rename them in the source;
  reLaTeX never renames a file, because TeX would then read another file than you wrote.
- **Long paths.** reLaTeX's own file access has no length limit, but TeX, MuPDF, qpdf and Poppler
  stop at 260 characters unless long paths are enabled (`LongPathsEnabled`, which
  `relatex doctor` reports; an administrator enables it in the registry or with a group policy).
  Otherwise keep the paper's directory and your user name short.
- **Files held open.** A PDF viewer that locks the file it shows (Acrobat; SumatraPDF does not)
  can stop reLaTeX from replacing the PDF. reLaTeX retries for two seconds, then names the file
  as in use by another program.
- **Line ends.** Sources with CRLF line ends are fine. `relatex/alt.toml` and the sources the
  accessibility tools patch keep the line ends they have.
- **Hashes.** The PDF and the hashes of the helper programs differ from a Linux conversion, and
  with a different MuPDF patch release (1.28.5 from scoop, 1.28.2 on Linux) the font and text
  paint hashes of `document.json` can differ (reLaTeX passes `-N` on `mutool draw` so ICC
  support does not cause `text.paint_unverified`). The text and the structure are the same.

## Same text as on Linux

Three test papers and two books from the project's test sets were converted natively on an arm64
laptop (Windows 11, TeX Live 2026 emulated, scoop's MuPDF 1.28.5) and on Linux (x86-64, MuPDF
1.28.2) from the same commit, with the same build date. The text of every `document.json` is
identical, and so are its nodes:

| Document | What it exercises | Strings | Linux | Windows (arm64) |
|---|---|---|---|---|
| papers/0027 | a plain paper | 10112 | 16 s | 54 s |
| papers/0088 | BibTeX, EPS figures | 31422 | 36 s | 132 s |
| papers/0134 | BibTeX, files in subdirectories | 5014 | 15 s | 118 s |
| books/0002 (`testmath`) | amsmath's test book | 17728 | 27 s | 77 s |
| books/0001 (`amsldoc`) | amsmath's user guide | 20146 | 32 s | 101 s |

The times are whole conversions (JSON, HTML, Markdown, EPUB) with an empty project cache; the
laptop's TeX runs emulated. What differs, and why:

- the PDF's hash and everything that records it (layout, source map, SyncTeX, text and table
  paint), since the PDF names its producer build;
- the MuPDF helper's hash (another build) and `capabilities.helpers.bwrap` (no sandbox);
- the font files' `source`, the absolute path of the font in the TeX Live tree of each machine;
- EPS figures: TeX Live's Ghostscript for Windows converts them to slightly different PDF bytes,
  so their files and names in `assets` differ, not their place in the text;
- One paper alone: in the 1.28.5 run before `-N` was passed, the text paint could not be read from
  its PDF on Windows (`text.paint_unverified`), so its colours and linked source fonts were left out
  there because Windows' `mutool` has ICC support enabled and traced `/ICCBased` figure labels as
  `ICCBased(...)` instead of `DeviceRGB`. reLaTeX passes `-N` to every `mutool draw` call to
  disable the ICC workflow across platforms (pending re-verification of `papers/0088` on Windows).

## Troubleshooting

- **`relatex doctor` says MISSING for a program you installed.** scoop installs into
  `%USERPROFILE%\scoop\shims`, which must be on `PATH` (open a new terminal after installing), or
  name the program in `tools`. A program named in `tools` that does not exist is reported as
  "config names no program here".
- **The first conversion is slow.** reLaTeX unpacks its resources and luaotfload builds its font
  database once; biber unpacks itself into `%TEMP%` on its first run. Windows Defender scans the
  many small files of the cache: excluding `%LOCALAPPDATA%\relatex` from real-time scanning speeds
  up conversions noticeably.
- **`luaotfload` reports "no writeable cache path".** The run's font cache must be writable; it lies
  under `%LOCALAPPDATA%\relatex`. Check that the folder is not read-only or redirected.
- **"cannot be stored on Windows".** A file name of the paper is one Windows refuses (above).
- **An error names a path with `\\?\`.** That is Windows' long-path form; the path is too long for
  a TeX tool. Enable long paths or shorten the directories.

The general [Troubleshooting](troubleshooting.md) page (evidence directories, logs, diagnostics)
holds on Windows too; the evidence directory is under `%LOCALAPPDATA%\relatex\err`.
