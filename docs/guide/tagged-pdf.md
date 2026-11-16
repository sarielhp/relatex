# Tagged PDF

The PDF LuaLaTeX builds has no structure a screen reader can follow. reLaTeX adds one: it tags
that PDF afterwards from its own document model, aiming at PDF/UA-2. The pages are not typeset
again, so they look as they did.

```sh
relatex --pdf-ua                 # NAME-tagged.pdf, the tagged PDF
relatex --accessible             # HTML, EPUB and the tagged PDF, with accessibility.html
relatex tag-pdf                  # tag the last conversion's PDF again (see below)
```

`--all` does not build it; only `--pdf-ua` and `--accessible` do. It needs the page layout (not
`--no-layout`), and takes seconds for a paper and longer for a book.

## What it does

- Builds a structure tree from the model: headings, paragraphs, lists, tables with their headers,
  notes, captions, links and contents.
- Binds each painted glyph to its structure element by paint order, and checks the position. A
  page it cannot prove is reported, not guessed.
- Sets the document language and the title (the front matter, else the first heading).
- Gives each figure its description as `/Alt` and its long description as an associated file
  ([Figure descriptions](figure-descriptions.md)). A figure with no description gets its file
  name as alternative text and the diagnostic `pdfua.undescribed`: that is valid PDF/UA and says
  nothing a reader can use. Write the descriptions.
- Attaches each formula's MathML as an associated file.
- Checks its own work: every glyph bound, the structure reading back in the document's order, every
  page looking as the ordinary PDF's.

## What it does not do

- It does not make an accessible document of an inaccessible source. What only you can supply
  (figure descriptions, for one) stays open; see
  [Accessibility](accessibility.md).
- It does not claim more than it measured. The file says PDF/UA-2 in its metadata only when
  reLaTeX's checks pass and veraPDF accepted it; otherwise the claim is withdrawn or absent, and
  `export.json` says `tagged` or `failed` instead of `conforming`.
- It has not been tried with real screen readers, Acrobat or PAC. Those are yours to run.
- It does not bind a PDF that differs from the one reLaTeX read: the content is bound through the
  model's layout, so a PDF from another build can fail the checks.

## veraPDF

When veraPDF is installed, reLaTeX runs it on the result (the PDF/UA-2 profile) and reports its
verdict. Name its launcher in `~/.config/relatex/config.jsonc` (`"tools": {"verapdf": "..."}`), put
`verapdf` on `PATH`, or set `RELATEX_VERAPDF`; `relatex doctor` shows which it found. Without it
the PDF is still tagged, but `verapdf.status` says it was not found and the file is never
`conforming`. veraPDF's installer is at https://verapdf.org/software/ (it needs Java).

## `relatex tag-pdf`

`relatex tag-pdf [OUTPUT_DIRECTORY]` tags the PDF of a finished conversion again, from the
conversion's saved model and layout, and writes `pdf-ua2/NAME.pdf` and `export.json` beside them:

```text
$ relatex tag-pdf
Tagged .../product/pdf-ua2/sums.pdf (PDF/UA-2, conforming)
  language: en
  diagnostics: 1 entry
    code=pdfua.undescribed message=1 graphics have no description ...
```

- `--pdf FILE` tags another build of the same document; `--target FILE` chooses where the
  tagged PDF is written.
- `--web-report accessibility.json` is the web page's measurement from an earlier conversion. The
  PDF/UA-2 identification is kept only when the page's claim is held too
  ([details](accessibility.md)). A conversion with HTML supplies it by itself.
- `--report` prints the JSON report instead of the text.

For the reasoning behind this route and the one LaTeX's own tagging takes, see the
[design FAQ](../design-faq.md#tagged-pdf).
