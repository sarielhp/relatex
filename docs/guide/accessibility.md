# Accessibility

Public universities in the US must make the documents they post meet WCAG 2.1 Level AA (the ADA
Title II rule of 2024). reLaTeX aims its HTML and EPUB at WCAG 2.2 Level AA, which includes 2.1 AA,
and its tagged PDF at PDF/UA-2. It builds the structure itself, measures the result, and states a
claim for each output only when every check passes. What only you can say (the language, a figure's
meaning, a table's header row) it lists with file and line; it never edits your source.

Below is what reLaTeX provides, then the walk-through in the order you do it. Figure descriptions have their own page:
[Figure descriptions](figure-descriptions.md).


What reLaTeX does for accessibility, each with where it is described:

- **Standards**: the web page and EPUB aim at WCAG 2.2 Level AA (which includes the 2.1 AA that the ADA
  Title II rule asks for), the EPUB at EPUB Accessibility 1.1, the tagged PDF at PDF/UA-2.
- **A measured claim per output**: each output is *Claimed* or *Not claimed* with its reasons, after axe-core
  in a browser, reLaTeX's structural checks and, for the PDF, veraPDF ([section 4](#4-reconvert-and-read-the-claims)).
- **Structure from what TeX typeset**: headings that never skip a level, table headers where the typesetting
  marks them, landmarks, a skip link, theorems and proofs as named groups, footnotes and the bibliography in their
  roles ([section 7](#7-what-relatex-does-without-you)).
- **Math readers can speak**: MathML written for speech, with a text alternative and the author's TeX
  ([section 7](#7-what-relatex-does-without-you)).
- **Language** of the document and of passages in other languages, from babel, polyglossia or the class.
- **Reading comfort**: colour contrast repaired with each change recorded, keyboard focus, reduced motion,
  forced colours, light and dark themes ([section 7](#7-what-relatex-does-without-you)).
- **Figure descriptions**: drafted for your review and kept in `alt.toml`, never invented silently
  ([Figure descriptions](figure-descriptions.md)).
- **A lint of what only you can supply** (a language, a figure's meaning, a table's header row), each at its
  file and line; reLaTeX never edits your source ([section 2](#2-lint-what-is-open-in-your-source)).
- **Tagged PDF** from the same model, the pages unchanged ([Tagged PDF](tagged-pdf.md)).

The walk-through:

1. [Convert](#1-convert)
2. [Lint: what is open in your source](#2-lint-what-is-open-in-your-source)
3. [Describe the figures](#3-describe-the-figures)
4. [Reconvert and read the claims](#4-reconvert-and-read-the-claims)
5. [Item kinds and their fixes](#5-item-kinds-and-their-fixes)
6. [Timings](#6-timings)
7. [What reLaTeX does without you](#7-what-relatex-does-without-you)

## 1. Convert

```sh
cd ~/papers/sums
relatex convert . --main M.tex --accessible --output output
```

`--accessible` makes the HTML page, the EPUB and the tagged PDF (as `--html --epub --pdf-ua`),
measures the page in a browser in both colour schemes (`--verify-browser`) and writes
`html/accessibility.html`, the report to keep with the material. `--all` never builds the tagged
PDF; only `--accessible` or `--pdf-ua` does. `--main` is needed only when the directory has several
mains. Other inputs and options: `relatex --help`.

- **veraPDF** validates the tagged PDF. Name its launcher once in the global configuration
  (`~/.config/relatex/config.jsonc`: `"tools": {"verapdf": "/path/to/verapdf/verapdf"}`), or put it on
  `PATH`, or set `RELATEX_VERAPDF`. `relatex doctor` shows which one it found. Without it the PDF/UA-2
  claim is withdrawn as `pdfua.unverified`: the PDF is written, never called conforming, and the report
  says veraPDF was not found. veraPDF has no distribution package; its installer is at
  https://verapdf.org/software/ (it needs Java).
- **A failed conversion sets the earlier products aside** in `_stale/` of the project cache's record of
  the output directory (the error message names it), with `_conversion-failed.txt` saying why, so no old `accessibility.json` or
  `index.html` looks current for a source that no longer builds. The next good conversion removes them.

The first conversion of a paper usually leaves items. They are the next step.

## 2. Lint: what is open in your source

```sh
relatex lint --accessibility --main M.tex --output output
```

reads your LaTeX (it follows `\input`, `\include` and `\subfile`, and reads `relatex/alt.toml`) and
lists the items only you can close, each as `FILE:LINE: kind: what is missing` with the fix, and a
`FILE:LINE: note:` giving the exact text to add:

```text
styles/prefix_latex.tex:22: language: the document declares no language; the export says English
  fix: english as a \documentclass option, or \usepackage[english]{babel} (polyglossia: \setdefaultlanguage{english}; \DocumentMetadata{lang=en})
styles/prefix_latex.tex:22: note: add english to the class options on this line: \documentclass[english,12pt,oneside,fleqn]{memoir} (a class option defines no command; babel, if loaded, takes it); the text reads as English (564 of its 3382 words are frequent English words)
1 items only the author can supply (1 language)
```

With `--output DIR` holding a conversion that has an `accessibility.json`, the lint also lists **every
open `table_headers` and `use_of_color` item of the export**, located at its `FILE:LINE` (a table at
its `\begin{tabular}`, a colour item at its caption), merged with the source scan's item at the same
place. Its last line says whether the claim would follow once they are done, or what
else is open that the source cannot show. Without a conversion it is the source scan alone.

| Option | What it gives |
|---|---|
| `--json` | the items as JSON (`kind`, `file`, `line`, `message`, `fix`, `suggestion`, `note`) |
| `--review-page DIR` | `DIR/index.html`: every open item with the source around it, the figures, and the excerpt of the PDF page at each place (`DIR/figs`, `DIR/crops`) |
| `--patch FILE` | the suggestions as a unified diff; apply it in the paper directory with `patch -p1 < FILE` |
| `--accept FILE` | with `--patch`: keep only the items the review page's saved decisions (`relatex/lint.toml`) tick |
| `--annotate DIR` | a **copy** of the source with each place marked, and `DIR/REVIEW.md` (below); your files are never changed |
| `--math` | one document-level note on bars (`\|x\|`) that no macro gives a meaning |
| `--strict` | exit status 1 when anything but advice is listed (otherwise 0) |

**The language goes in as a class option, never as a package.** The patch writes
`\documentclass[english,…]{memoir}`. A late `\usepackage[english]{babel}` defines commands and
silently redefines an author macro of the same name (on the author's book, babel's `\frq` replaced
`\newcommand{\frq}{f}`, and `f` read as `›`), and `\DocumentMetadata{lang=en}` switches on the tagging
kernel, which clashed there with `enumitem`. A class option defines nothing, and babel, if loaded, takes
it. When your source already has a `\DocumentMetadata`, or babel or polyglossia without a language, the
suggestion edits that instead.

**The marked copy** (`--annotate DIR`): each item is a comment line `%% relatex-a11y: KIND N — what is
missing. Fix: …` before the spot (`N` is the number in `REVIEW.md`), the note's line is written commented
out where it goes (uncomment it), and a figure without a description gets
`alt={TODO: describe figs/voronoi.pdf}`, or `alt={DRAFT: …}` holding an unreviewed draft: check it and
delete `DRAFT:`. Comments and `alt` keys change nothing on the page. Rebuild and run the lint on the
copy: a placeholder still reading `TODO:` or `DRAFT:` is listed again. A drawing named only by the last
conversion (`figs/figure-2`) has no source line and is in `REVIEW.md` alone. `DIR` must be new, empty or
an earlier review copy; the paper directory, a directory holding it and any other non-empty directory
are refused.

## 3. Describe the figures

A figure's text alternative comes from your LaTeX (`alt={…}`, `\Description`, `\relatexalt`) or from
`relatex/alt.toml`. The lint lists every figure that has neither, and every unreviewed draft. The file
format, the drafting command and what makes a good description: [Figure descriptions](figure-descriptions.md).

```sh
relatex alt draft . --allow-upload   # after a conversion; the drafting command is yours
relatex alt review .                 # alt-review.html: each figure with its draft
relatex alt accept --all .           # or: relatex alt accept NAME...
```

- **What is sent where.** reLaTeX has no model of its own. `alt draft` runs the command you set as
  `alt_text.command` in `~/.config/relatex/config.jsonc`, once per figure that has no current draft,
  and sends it that figure, its caption, the text citing it, the document's language and, for a vector
  figure, its geometry. Where that goes, and which models answer, is that command's setting
  (`alt_text.endpoint`, `alt_text.models`; the models are tried in order and the one that answered is
  recorded). A command that uploads needs `--allow-upload` once per paper (recorded in
  `relatex/alt.toml`). With no command nothing is sent.
- **Provenance.** Each entry records `provenance`: `author` (yours, the default), `model` (a draft
  nobody read) or `reviewed` (a draft you accepted), with the model and the dates. `alt accept` makes a
  read draft `reviewed`; `alt import` does the same for the decisions saved on the review page.
  **Entries that are yours or reviewed are never changed**, by a later draft or by anything else.
- **Drafts do not count.** A draft is used on the page so the reader gets text, but is counted apart
  (`image_drafts_reviewed`, warning `alt.unreviewed`); no claim rests on one.
- **The QED mark is automatic.** A proof's end mark drawn as an image (the author's duck) is named
  "End of proof" and is not a figure to describe; the lint does not list it.

You can skip the model: write `alt={…}` in the source, or the entries in `relatex/alt.toml`
(`relatex alt set NAME --alt "…"`).

## 4. Reconvert and read the claims

Apply the patch, fix the other items (section 5), and convert again with the command of section 1;
unchanged stages are reused. Then read:

- **`html/accessibility.html`**, the page to keep (`NAME-accessibility.html` when only `--pdf-ua` is asked for). One row per output: the web page against WCAG 2.2
  Level AA, the EPUB against EPUB Accessibility 1.1, the tagged PDF against PDF/UA-2, each *Claimed* or
  *Not claimed* with its reasons. Then the open items (yours, or reLaTeX's own), the lint's items at
  their `FILE:LINE`, where the descriptions, language and table headers came from (your source,
  `alt.toml`, reviewed drafts, unreviewed drafts, inferred, none), and the reLaTeX version. It has no
  scripts and reads well with a screen reader.
- **`accessibility.json`** (in the product): `claim` is `null` until axe-core finds nothing in either theme and
  every structural check passes, then `"WCAG 2.2 Level AA"`; `open_items` lists what is left, counted
  per check ([table below](#the-checks)).
- **`pdf-ua/export.json`** (the tagged PDF is `pdf-ua/JOB.pdf`): `status` is `conforming` only
  when veraPDF accepts the file as PDF/UA-2 and reLaTeX's own checks pass (every glyph bound to its
  element, the structure reading back in the document's order, every page looking as the ordinary
  PDF's), else `tagged` or `failed`. `verapdf.status` is `passed`, or says veraPDF was missing. The
  PDF's metadata declares PDF/UA-2 only when `conforming` and the web page holds its claim;
  `identification` says `kept`, `withdrawn` or `absent`. A conversion without HTML takes the web claim
  from an earlier `accessibility.json` named by `--web-report FILE`.
- **The EPUB**: `dcterms:conformsTo` "EPUB Accessibility 1.1 - WCAG 2.2 Level AA" is written only when
  the web claim is made, the package has its page list and no description is a draft. Its other
  accessibility metadata states only what the export shows. `epubcheck` and DAISY Ace validate a package.

Run the lint again. "Nothing for the author to supply was found in the source." and claims on the
report page mean you are done; otherwise each remaining item says what closes it.

## 5. Item kinds and their fixes

Items of `relatex lint --accessibility` (`kind` in `--json`), each at its file and line:

| Kind | Meaning | Example fix |
|---|---|---|
| `language` | no document language; the export says English (`language.undeclared`) | `\documentclass[english,12pt]{memoir}`; polyglossia: `\setdefaultlanguage{english}`. One line, usually in a shared prefix |
| `table` | a data table with no marked header row (reLaTeX never guesses one, nor gives one to a layout table) | `\midrule` under the header row and none between the body rows, or a longtable head, or the hint below |
| `caption` | a missing or uninformative caption | `\caption{Running time against n for both algorithms}` |
| `colour` | a figure or table caption names colours ("the red line"); colour alone may carry the meaning (WCAG 1.4.1) | say it with a word or pattern: "the dashed line (Algorithm B)". The check closes only when the caption names no colour |
| `link` | link text that names nothing ("here", a bare URL) | `\href{https://example.org/notes}{the lecture notes}` |
| `figure`, `drawing` | a graphic or TikZ drawing without a description | `\includegraphics[alt={…}]{f}`, or an entry in `relatex/alt.toml` |
| `draft` | a description is a model draft nobody reviewed | read it, then `relatex alt accept NAME` |
| `long` | a complex figure (a plot, a rich diagram) with only a short description (advice) | `relatex alt set NAME --long "…"` |
| `heading` | a paragraph that is only bold text looks like a heading (advice) | `\paragraph{…}` or `\subsection{…}` |
| `math` (`--math`) | bars that no macro gives a meaning (advice) | one macro per meaning: `\abs{x}`, `\card{S}`, `\norm{v}`; never a list per formula |

**Table header hints.** Where the table's rules are not what you mean, say so once in a comment, keyed
by the table's `\label` or the number its caption prints:

```latex
%% relatex tables: tab:results = "header_rows=1 header_columns=1"
```

Either field may be left out (that direction is then read from the typesetting; `0` says the table has
none). A plain key names a table float; a table inside a figure float (or one of several tables in a float) is
named by the float's label or number and the table's position in it, counted from 1: `%% relatex tables: fig:wspd#1 = "header_rows=1"`
(the lint gives the exact line). The hint says only what you write.

**Colour caption attestation.** A caption that names a colour is open for `use_of_color` even when the figure also
says it by line styles or markers; reLaTeX cannot see that, you can. Attest the caption as it stands in
`relatex/hints.json`, keyed by the float's `\label` (else its number) with the sha256 of the caption text, which the lint
prints with the exact line:

```json
{"colour_captions": {"fig:wspd": "29bce34e22fed06e947593104bf0a78f257375733c64750c44e2ac7f4b9b0d99"}}
```

The caption is then listed as attested (in `accessibility.json` and `accessibility.html`), not as a gap. Editing
the caption voids it. reLaTeX never attests for you, and it does not verify the figure.

<a id="the-checks"></a>
**The export's checks** (`accessibility.json`). Most are reLaTeX's own and need nothing from you; the
ones with an author's fix are marked.

| Check | WCAG | Passes when |
|---|---|---|
| `language_declared`, `language_parts` | 3.1.1, 3.1.2 | the source declares its language; other-language passages have a known tag |
| `image_alternatives` | 1.1.1 | every image has a text alternative or is marked decorative (yours) |
| `image_drafts_reviewed` | 1.1.1 | no alternative is an unreviewed model draft (yours) |
| `formula_alternatives` | 1.1.1 | every `<math>` has a text alternative (reLaTeX writes it) |
| `table_headers` | 1.3.1 | every data table has header cells (yours: rule or hint) |
| `use_of_color` | 1.4.1 | no caption names colours, or you attested it (yours) |
| `heading_levels`, `images_of_text` | 2.4.6, 1.4.5 | levels do not skip; no text is a picture |
| `landmarks`, `named_groups` | 1.3.1 | banner, `main`, bibliography landmark; theorems and proofs named |
| `focus_visible`, `reduced_motion`, `forced_colors`, `non_text_contrast` | 2.4.7, 2.3.3, 1.4.11 | the stylesheet's focus, motion, forced-colour and 3:1 rules hold in both themes |

The tagged PDF's diagnostics (`pdfua.invalid`, `pdfua.undescribed`, `pdfua.unverified`, …) are in
[Diagnostics](diagnostics.md).

## 6. Timings

From the night run of 2026-11-16 (one machine, nice 19). Author time is reading and editing; the rest
is compute.

**The author's book** (411 figure descriptions, reviewed by the author earlier):

| Step | Time |
|---|---|
| `lint --accessibility` | 50 s |
| `--review-page` (22 crops) | 60 s |
| `--patch` | 51 s |
| `--accessible` conversion | 2220 s on a quiet machine, 6272 s under load |

The lint listed 24 items (language 1, caption 8, colour 11, link 3, table 1; no figure). The patch had two
hunks (the language as a class option in the shared prefix, a `\midrule` in one chapter): 24 items became
22. The export still withheld the web claim, on 6 `table_headers` and 3 `use_of_color` items; the lint now
lists those 9 export items, each at its file and line, and says whether the claim follows. Five of the six
tables sit in figure floats, which no hint can name. veraPDF was not yet on `PATH` that run, so PDF/UA-2
was withdrawn as `pdfua.unverified`.

**The CS 574 homeworks** (8 documents sharing one prefix): one item each, the same one in the shared
prefix (the language, line 22). Author time about 1.5 minutes in all (reading the report, one edit or the
patch). Conversion took 46 to 108 s of compute per document, reconversion 50 to 93 s. After the one-line
patch: WCAG 2.2 AA and PDF/UA-2 conforming, veraPDF passed, axe-core clean in both themes, and the lint
had nothing left. A tagged PDF takes seconds for a paper and minutes for a book.

## 7. What reLaTeX does without you

Structure, reading order and MathML come from what TeX typeset; prose, tables and formulas are never
pictures, only real drawings are images.

- **Headings** take their level from the nesting and never skip. **Table headers** are set where the
  typesetting marks them (a longtable head, booktabs rules, a full-width rule under the first rows) and
  never guessed; a header cell holding only a formula stays plain.
- **Skip link**, **landmarks** (banner, `main`, contents, a bibliography section as `doc-bibliography`),
  every theorem-like environment and proof as a named group, footnotes as `doc-footnote`.
- **Keyboard focus** (a 2 px outline at 3:1 on every background), **reduced motion**, **forced colours**
  and 3:1 **rules and borders**, in both themes; the page follows the system colour scheme.
- **Colour contrast**: a text colour below 4.5:1 is darkened (light theme) or lightened (dark) by the
  smallest change keeping its hue, each change recorded (`color.low_contrast`); text you hid stays hidden
  and is `aria-hidden`.
- **Math** is MathML written for speech (named operators, invisible function application, grouped
  brackets) with an `alttext` for software without MathML, and the author's TeX as an annotation when
  proved. A display formula wider than the screen scrolls in a named, keyboard-reachable box.
- **Language** from babel, polyglossia, `\DocumentMetadata{lang=…}` or a class option; passages in
  another language (`\foreignlanguage`, `otherlanguage`) carry their own. Undeclared: English with
  `language.undeclared`; unmappable: `und` with `language.unknown`.
- **Link names**: a link with no readable text gets one (the formula's text, the target, "Reference: …");
  links back from footnotes are at least 24 × 24 px.
- **EPUB**: each PDF page is a `doc-pagebreak` marker named by its printed number, with a page list in the
  navigation document (a package exported from a saved bundle has none; convert to get one).
- **Tagged PDF**: the ordinary PDF with a structure tree (headings, paragraphs, lists, tables, notes,
  captions, links, contents), the language, the title (front matter, else the first heading), each figure's
  description as `/Alt` and its long description as an associated file, each formula's MathML as an
  associated file. The pages are not typeset again; it needs the layout (not `--no-layout`). A figure
  without a description gets its file name as alternative text (`pdfua.undescribed`), which passes
  veraPDF and not WCAG 1.1.1.

Not done: page-break markers in the web page (the EPUB has them), and confirming the output with real
screen readers, Acrobat and PAC.

The reasons are in the [design FAQ](../design-faq.md): [what the accessibility claim is](../design-faq.md#accessibility-claim),
[the tagged PDF](../design-faq.md#tagged-pdf), [how table headers are decided](../design-faq.md#tables),
[spoken math](../design-faq.md#math-speech), [figure descriptions](../design-faq.md#figure-descriptions) and
[what a failed conversion leaves behind](../design-faq.md#diagnostics).
