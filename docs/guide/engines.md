# pdfLaTeX and XeLaTeX sources

reLaTeX builds everything with **LuaLaTeX**, because its recording packages are written in Lua.
You do not choose the engine. Most sources written for pdfLaTeX or XeLaTeX build under LuaLaTeX
unchanged: `inputenc` (UTF-8), `fontenc`, `fontspec`, `unicode-math`, `polyglossia` and `iftex`
all work. For the rest reLaTeX has two fallbacks.

## The compatibility layer

Every build first runs your source exactly as it is. Only when the first LuaLaTeX pass fails
does reLaTeX read its log for one of these causes:

| The log shows | Typical cause | Shim |
|---|---|---|
| `! Undefined control sequence.` for a pdfTeX primitive (`\pdfoutput`, `\pdfliteral`, `\pdfstrcmp`, …) | arXiv's customary `\pdfoutput=1`, or `xy` | `pdftex` |
| "pdfTeX version 1.40.0 or higher is needed" | the `xy` package's PDF driver | `pdftex` |
| an undefined XeTeX primitive (`\XeTeXlinebreaklocale`, …) | XeLaTeX-only code | `xetex` |
| "works only with XeTeX" | `xunicode` | `xetex` |
| an undefined `\setmainfont`, `\setsansfont`, `\setmonofont`, `\newfontfamily` or `\defaultfontfeatures` | an engine branch that calls them before loading `fontspec` | `fontspec` |

If it finds one, it starts the build again from nothing with a small package,
`relatex-compat`, loaded before your main file. The package only defines names that are still
undefined:

- `pdftex`: the pdfTeX primitive names mapped to their LuaTeX equivalents, without
  `\pdftexversion`, so `iftex` and `ifpdf` still see LuaTeX and take their LuaTeX branch;
- `xetex`: XeTeX's parameters as inert registers and its other primitives as commands that do
  nothing; `xunicode` treated as loaded;
- `fontspec`: the font commands load `fontspec` when first used.

At most three restarts happen. A source that builds without the layer never gets it.

The layer is part of the build that the export is read from and checked against, so it is not a
loss. It is recorded: the terminal shows a line such as

```text
engine: compatibility layer pdftex (undefined \pdfoutput)
```

and the export carries the diagnostic `source.engine_compatibility` (level `whatever`, hidden from
the terminal summary).

## Errors on the first pass

Some documents report errors on the very first pass, before the `.aux` file exists, and build
cleanly afterwards; per-page footnote symbols (`\MakePerPage{footnote}` with `\fnsymbol`) are an
example. latexmk forgives this, and so does reLaTeX: the first pass of a build that starts without
an `.aux` file runs without `-halt-on-error`. If it fails but wrote its `.aux` file and did not stop
fatally, its errors are recorded and the build goes on. Every later pass must run without error,
and the final pass must be clean. The recorded errors appear as the diagnostic
`source.settling_errors` (level `whatever`).

## What still does not work

- `inputenc` encodings other than UTF-8 (`latin1`, `cp1252`, …): LuaTeX reads UTF-8 only.
  Convert the files to UTF-8.
- XeTeX-only packages beyond `xunicode`, such as `xeCJK` and `xgreek`.
- pdfTeX's `\pdfmdfivesum` and `\pdffilemoddate`.
- Packages that need shell escape or Python: `minted`, `pythontex`.
- Figures meant for a DVI driver (`graphicx` with `dvips`, `\special{psfile=…}`): they are not
  in a PDF build. EPS files themselves are fine: reLaTeX converts each `.eps` in the paper to PDF
  with TeX Live's `epstopdf` before the build.
