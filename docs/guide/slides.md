# Slides (Beamer)

reLaTeX converts a Beamer deck into a reading page with one section per frame. It is meant for
lecture slides that students read, search and listen to, not for presenting: the result has no
slide layout and no overlays.

```sh
cd ~/courses/lecture3     # holds talk.tex
relatex
```

This is the first version of Beamer support; see [what is not done yet](#not-done-yet).

## The final state of each frame: handout mode

reLaTeX builds the deck in Beamer's **handout mode**: before your main file it passes
`\PassOptionsToClass{handout}{beamer}`. Every frame is then typeset once, with all its overlays
shown, and the PDF has one page per frame. That PDF (`talk.pdf`, in the product) is the reference the
export is read from, so it is the handout, not your presentation PDF.

For the usual cumulative overlays this is exactly the last slide of the frame: `\pause`,
`\item<2->`, `<+->`, `\uncover<2->`, `\visible<2->`, `\onslide<2->` all end with everything
visible.

You keep control with Beamer's own `handout:` overlay specifications, which you may already use
for printed handouts. For example `\only<1| handout:0>{...}` hides that material in the export.

A deck whose class options already choose a static mode (`handout` or `trans`) is built as it is.
A deck that puts several frames on one sheet with `pgfpages` (`\pgfpagesuselayout{2 on 1}`) is
built one frame per page anyway: the layout commands are disabled for the build.

**Non-cumulative overlays differ from the last slide.** In handout mode Beamer shows whatever is
visible on *some* slide: `\only<1>{A}\only<2>{B}` shows both A and B, and `\alt<1>{A}{B}` shows A.
reLaTeX does not warn about this yet. If it matters, add `handout:` specifications to choose what
the reader sees.

## Frames as sections

- Each frame with a title (`\frametitle{...}` or `\begin{frame}{...}`) opens a top-level section
  whose heading is the frame title.
- A frame without a title opens no section: its content continues the previous frame's.
- Beamer's `\section` and `\subsection` print nothing on a frame, so they are not headings in the
  export.
- The headline, footline, sidebars, background, logo and navigation symbols are page decoration,
  like a paper's running heads, and are left out.
- Blocks (`block`, `alertblock`, `exampleblock`) are read as paragraphs of the frame: the title
  and body are kept, the coloured box is decoration.
- `columns` are read in source order, left column first, which is the order a narrow screen
  needs.
- Lists with overlay specifications (`\item<2->`) are ordinary lists. Beamer's item marker, the
  triangle, is kept.

## Not done yet

- Real final-state semantics for non-cumulative overlays (`\only<1>`, `\alt`, closed ranges like
  `<1-2>`), and a warning when a deck uses them.
- Blocks as their own element (an aside with a heading), and columns as a responsive layout.
- Beamer's `\section` grouping the frames, as chapters group sections in a book.
- Numbers for untitled frames; continuation titles of `allowframebreaks` frames; the title page as
  front matter.
- Decks that need shell escape (`minted`) or packages outside TeX Live cannot be built.

A plain deck in the default theme currently reports a `figure.source_unsupported` warning on
every frame ("Unbound native PDF literal has no inspectable original graphic source"): a PDF
drawing on each page that reLaTeX cannot attribute to a figure in your source. Compare the
frame's text in the export with your handout; if a drawing of yours is missing, that is a real
loss.
