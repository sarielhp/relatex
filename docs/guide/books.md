# Books

A document whose headings use `\chapter` (the `book`, `report` and `memoir` classes, and others
that define chapters) is a book. reLaTeX converts it like a paper and then cuts the verified page
into one file per chapter, because a single page for a long book is too large to read or load.

```sh
cd ~/books/tiny           # holds book.tex
relatex --html --epub
```

## What you get

```text
html/
  index.html                    the contents page: open this
  front.html                    the title page and whatever precedes the first chapter
  contents.html                 the book's own \tableofcontents, when it prints one
  chapter-01.html               one file per chapter
  chapter-02.html
  book-index.html               the index, when the book prints one
  preview.js, figs/, fonts/
book.epub                       with --epub, beside html/
```

**File names.** The title page is `front.html`, each `\part` opens a `part-N.html`, and each
numbered chapter is `chapter-01.html`, `chapter-02.html`, …, whether its heading shows the number
alone ("1"), with a period ("1. First Steps") or after a word, as the standard classes print it
("Chapter 1", "Kapitel 1"). Lettered appendices are `appendix-a.html` ("Appendix A"). A chapter
whose heading has no number is named from its words (`first-steps.html`). Other top-level units (preface, bibliography, index, errata) are named from
their titles (`preface.html`, `bibliography.html`, `book-index.html`). `split/split.json` lists
every file with its title, kind, size and SHA-256.

**The contents page** (`index.html`) shows the book's title, a link to the first file, and the
list of parts, chapters and other units with links into their files. A chapter's sections are
not listed there; each chapter file opens with its own list of sections.

**Each chapter file** has previous / contents / next links at the top and the bottom, and its
title is the chapter's heading. Links into another chapter (references, citations, page
references, index entries) point at that file (`chapter-2-primes.html#…`). Footnotes go with the
chapter that cites them. Hover previews work across files.

The chapter files share `figs/`, `fonts/` and `preview.js` with the contents page, so keep the
output directory together when you copy or publish it.

## How the split is checked

The split happens after verification: reLaTeX verifies the whole book as one page
(`html/index.html`), then cuts it mechanically and checks that the cut is faithful: the chapters'
contents, put back together in order, are exactly the verified page, and every link into another
file names an element that file holds. Diagnostics:

| Code | Level | Meaning |
|---|---|---|
| `split.chapters` | whatever | the book was published as a contents page and chapter files |
| `split.whole` | warning | the book could not be cut (an element referenced by an id that cannot cross files, such as a table header or a drawing's internal reference); it is published as one page |
| `split.unfaithful` | error | the cut differed from the verified page; the chapter files are dropped and the book is published as one page |

`--split none` publishes the whole book as one page (`index.html`), as for a paper.

A document without `\chapter` is never split.

## The EPUB of a book

With `--epub`, the package has one content document per unit of the split (front matter, parts,
chapters, index), so a reading system's table of contents lists the chapters. It has no previous /
next navigation of its own: the reading system provides it.

## The index

When the book uses `\makeindex` and `\printindex` (makeidx, imakeidx, or memoir's own index),
reLaTeX runs `makeindex` during the build, as latexmk does, so the index is printed in the PDF
and in the export.

In the export each index entry's page numbers are links. A page number leads to the place where
the matching `\index` command was issued on that page: the start of the paragraph or block
holding it (not the exact word). The printed text of the entry stays as in the PDF. Entries,
subentries and sub-subentries keep their levels.

What is not supported:

- `makeindex` runs with its default style only; a style given with `-s` in your own build is not
  known, and `xindy` is not run;
- `\see` and `\seealso` print but are not links;
- glossaries, nomenclature and lists of notation are not linked;
- a page number with no `\index` of that term on that page prints unlinked
  (`index.page_unmatched`); one not found in the entry's printed text prints unlinked
  (`index.locator_unplaced`); an index that does not read as the entries of its `.ind` file
  prints as plain paragraphs (`index.unstructured`).

## Page references

`\pageref` prints the page number as in the PDF and links to the target, in whichever chapter
file it is. A page reference whose number reLaTeX cannot match to a page its target is set on
prints unlinked, with the alert `reference.page_unverified`.

## Limits

- The document bundle holds every glyph of the book; it is limited to 1 GiB, about 700 dense
  pages. A larger book fails in the export step with "Bundle exceeds expanded byte limit".
- A page reference such as "see page 7" names a page the reading page does not have; there are
  no page-break markers in the export yet.
