# Why relatex needs a paper that builds on LuaLaTeX

relatex converts a LaTeX paper to HTML, EPUB, Markdown and Typst. It works only on a paper
that **compiles correctly on LuaLaTeX, with no errors**. A paper that builds only with
xelatex, pdflatex or latex is not supported, and neither is one whose build reports an
error. This page explains why.

## LaTeX has no syntax tree

Most document converters read a file, build a tree (headings, paragraphs, formulas) and write
that tree in another format. LaTeX does not allow this. A `.tex` file is not a document; it
is a program written in TeX's macro language. `\section`, `\begin{theorem}` and `\cite` are
not data but macros that expand into other macros, which expand into more. What a paper means
(what is a heading, which number a theorem gets, what a citation points to) exists only after
all of that has been expanded, and usually only after several runs have settled the
cross-references.

TeX is also Turing complete. A paper can redefine any command, change how characters are read,
load packages that rewrite the internals of other packages, and compute text at typesetting
time. No parser that reads the source can know what the author's macros will do without
running them. The only faithful reader of a LaTeX paper is TeX itself.

## What relatex does instead

relatex does not parse your LaTeX. It runs LuaLaTeX on your sources and watches. It acts as a
parasite inside the typesetting run:

- relatex loads its own LaTeX package (`tex/relatex.sty`) and Lua code (`tex/relatex*.lua`)
  into the build. They register callbacks and hooks that observe what TeX really builds:
  paragraphs and boxes as node lists, each glyph with its font and position, each formula as
  TeX's own math list, the counters, labels and references as they are resolved.
- While TeX builds the PDF, this code records the structure (headings, theorems, floats,
  citations, links, formulas) as events. A later stage reconciles the events from the
  builds into a document model, and the exporters write that model.
- The observers are meant to change nothing in TeX's output. relatex builds the paper
  without them as well (the ordinary build) and the PDF of that build is the reference.
  Where the export and the PDF disagree, the PDF is right. The independent acceptance
  checks also take their expectations from what TeX did in the ordinary build (for
  example, `tex/relatex-oracle.lua` records every math list TeX builds), not from a second
  reader of your source.

So the PDF arbitrates fidelity. relatex never guesses what a macro meant; it records what
TeX did with it.

## Why only LuaLaTeX

The callbacks that expose nodes, fonts and math lists to a running program exist in LuaTeX.
pdfTeX, XeTeX and classic TeX offer no equivalent, so relatex has nothing to observe there.

A paper that builds on another engine may not build on LuaLaTeX. The usual causes are
input encodings (a Latin-1 source read by pdflatex's `inputenc`, where LuaLaTeX expects
UTF-8), pdfTeX-only primitives and packages, and font setups that differ between engines.
The paper that compiles for you on pdflatex can therefore fail here, and relatex does not
try to emulate the other engines.

## Why an error is fatal

When TeX meets an error, it does not stop. It recovers: it drops tokens, inserts a brace,
skips material or treats an unknown command as nothing, and carries on. The PDF that results
is wrong in ways nobody specified, and the damage depends on TeX's recovery rules, not on
anything the author wrote or intended.

relatex works by comparing what it records with the PDF. After an error the PDF is no
ground truth: there is nothing to say whether a missing formula or a shifted heading is
correct. The task, already hard, becomes close to impossible. relatex therefore refuses
rather than guess, and reports the first TeX error with its file and line. The same goes for
a paper whose builds never settle (references or citations still undefined after the last
pass), since the numbers in the PDF are not final.

## What you can do

1. Build the paper with `lualatex` (with `bibtex` or `biber` as the paper needs) until the
   build has no errors.
2. Run it enough times that no reference or citation is undefined.
3. Run relatex.

If the paper only builds with another engine, the fix is in the paper: make it build on
LuaLaTeX first (convert the input encoding to UTF-8, replace pdfTeX-only commands).
`relatex doctor` shows which programs relatex found, and `docs/guide/troubleshooting.md`
lists the messages you may see.
