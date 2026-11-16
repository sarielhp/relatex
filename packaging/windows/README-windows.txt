relatex for Windows
===================

relatex converts a LaTeX paper into HTML, EPUB 3, Markdown and Typst. This zip holds two copies of
one program:

  relatex.exe   the converter: relatex PAPER-DIRECTORY writes output\ there
  rlx.exe       the same program run as rlx: like latexmk, it publishes the products beside the
                main .tex file

Put both in a folder on your PATH (or install through scoop, below).

What relatex needs
------------------

1. TeX Live 2026 for Windows: https://tug.org/texlive/windows.html (install-tl-windows.exe).
   relatex finds the newest C:\texlive\YYYY or %USERPROFILE%\texlive\YYYY itself; TeX Live need
   not be on PATH. RELATEX_TEXLIVE names another tree.
2. The PDF tools, most easily from scoop (https://scoop.sh):

     scoop install mupdf qpdf poppler
     scoop install typst zstd        (optional: Typst output, cache compression)

   relatex needs MuPDF 1.28.x (any 1.28 release); another series changes figures and tables.
   A program that is not on PATH can be named in the "tools" section of the configuration
   (below), for example "tools": {"mutool": "C:\\mupdf\\mutool.exe"}.
3. Microsoft Edge (part of Windows) for the browser checks; RELATEX_CHROMIUM names another
   Chromium-based browser.

Then run

  relatex doctor

It lists every program relatex starts, where it found it and its version, and how to install a
missing one. It exits with status 1 while a required program is missing.

Known limits on Windows
-----------------------

- No sandbox: on Linux TeX runs inside bubblewrap; on Windows it runs with TeX's own file limits
  only (openin_any=p, openout_any=p), and every export records a sandbox.none diagnostic. Convert
  only papers you trust.
- Names Windows cannot store (aux.tex, con.sty, names with < > : " | ? *, or two names that differ
  only in case) are refused with an input.unsafe error; rename them in the source.
- Long paths: enable LongPathsEnabled (relatex doctor reports it) or keep the project and the
  cache (%LOCALAPPDATA%\relatex) paths short; TeX stops at 260 characters.
- A PDF viewer that keeps the PDF open (Acrobat) can block relatex from replacing it; relatex
  retries for two seconds, then reports the file as in use by another program.

Configuration is %APPDATA%\relatex\config.jsonc (relatex config --init writes a commented
one); the cache is %LOCALAPPDATA%\relatex; temporary
files go to %TEMP%\relatex.
