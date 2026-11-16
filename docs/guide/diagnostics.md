# Diagnostics

reLaTeX does not stop when it cannot reproduce something. It writes the export and reports each
loss as a diagnostic, the way a compiler reports warnings: a stable code, a level, a message and
where it happened.

## The five levels

| Level | Meaning |
|---|---|
| `fatal` | No output was produced. |
| `error` | The output says something the paper does not: content lost, garbled or wrong. |
| `alert` | All content is present, but a reader clearly notices the degradation. |
| `warning` | Noticeable only on careful comparison with the PDF. |
| `whatever` | Not visible, or informational; hidden by default. |

A conversion that fails outright (TeX error, missing main file) is not a diagnostic: reLaTeX stops
with `relatex: error: ...` and exit status 1 ([Troubleshooting](troubleshooting.md)).

## Reading the report

Diagnostics of level `warning` and above print on stderr, most severe first, identical ones once
with a count:

```text
page 3 · label sec:x: alert[label.unresolved_owner]: ...
paper: warning[language.undeclared]: the source declares no language with a known tag (babel, polyglossia or a class option); the export is marked as English (en)
page 1: warning[figure.source_unsupported]: a picture the converter cannot read is not reproduced (...) (×2)
```

The part before the level says where: a page, a label, a node, or `paper` for the whole document.
On a colour terminal only the level word is coloured; set `NO_COLOR` to turn colour off.

The JSON run report on stdout repeats the counts per level (`"diagnostics"`) and these lines
(`"diagnostic_lines"`). The complete list, including `whatever` entries, is in the attempt
directory (printed as `Conversion attempt:`), in `layers/03-reconcile/diagnostics.json`.

The exit status is 3 whenever the run has diagnostics or unverified parts; that is the normal
result of a successful conversion. `rlx` exits 0 instead.

## What to do about the common ones

**`language.undeclared`** (warning). Declare the document language, so screen readers pronounce
it correctly: `\usepackage[english]{babel}`, `\usepackage{polyglossia}\setdefaultlanguage{english}`,
or a language class option. Most papers have this.

**`reference.unresolved`, `label.unresolved_owner`, `label.ambiguous`** (alert). A reference prints
as plain text. Check that each `\label` follows the thing it names (after `\caption`, inside the
equation) and that no label is defined twice.

**`citation.unverified`** (alert). Citations and the bibliography print without links. reLaTeX could
not verify which entry each citation names; the text is still complete.

**`table.unmodeled`** (alert). A table is read as text without its grid. Unusual table packages or
tables built from boxes cause it.

**`math.unmapped_glyph`, `text.unmapped_glyph`** (error). A glyph's font has no Unicode mapping, so
it prints as the replacement character (U+FFFD). Find the symbol from the location in the
message, and set it with a font or package that gives it a Unicode mapping.

**`text.paragraph_incomplete`** (error). Text is missing from the output. Report it with the
source; it is a converter limitation.

**`figure.rendered_from_pdf`** (warning). A TikZ or other drawing without an image file is shown as
a crop of the PDF: correct to look at, but its labels are not text.

**`color.low_contrast`** (warning). A text colour of yours is too light to read on the page, or on the coloured box it is set on; the
light theme shows it slightly darker. See [Accessibility](accessibility.md#7-what-relatex-does-without-you).

**`fonts.source_unavailable`** (warning). Your fonts could not be linked; the browser substitutes
its own. The text is complete.

**`browser.unverified`** (whatever). `--verify-browser` was asked for but no browser answered.
Check `browser.endpoint` or Chromium ([Configuration](configuration.md#browser)).

**`accessibility.violations`** (whatever). axe-core or reLaTeX's own checks found accessibility
gaps. Read `accessibility.json` in the product ([Accessibility](accessibility.md)).

**`alt.unreviewed`** (warning). Some figures are described by model drafts you have not
accepted yet. Run `relatex alt review`, then `relatex alt accept`
([Figure descriptions](figure-descriptions.md)).

## All codes

Generated from `profiles/diagnostics.json` (77 codes), grouped by subject, most severe first in
each group.

### `accessibility`

| Code | Level | Meaning |
|---|---|---|
| `accessibility.unmeasured` | whatever | axe-core could not finish on the page (an exception, or longer than its time limit); the export carries no accessibility measurement. |
| `accessibility.violations` | whatever | axe-core found WCAG 2.x A/AA violations in the light or dark theme, or a structural check (alternatives, table headers, heading levels, language, images of text) has gaps; the export is not yet accessible. Phase 0 measures only: the level rises to alert once phase 1 is done. |

### `alt`

| Code | Level | Meaning |
|---|---|---|
| `alt.invalid` | alert | relatex/alt.toml could not be read (the message names the line); no figure description from it is used until it is fixed. |
| `alt.stale` | warning | An entry in relatex/alt.toml was written for other figure bytes than the paper has now: an author's or reviewed text is still used and should be checked; a model draft is left out until drafted again. |
| `alt.unreviewed` | warning | Figure descriptions from relatex/alt.toml are model drafts nobody has reviewed; the page uses them as the figures' text alternatives, and the accessibility report counts them under image_drafts_reviewed until accepted. |
| `alt.unmatched` | whatever | relatex/alt.toml has entries for figure names the paper does not have (a figure removed or renamed); they are kept and unused. |

### `browser`

| Code | Level | Meaning |
|---|---|---|
| `browser.unverified` | whatever | The browser timed out or could not be reached on every attempt; the export is published without its browser verification. |

### `citation`

| Code | Level | Meaning |
|---|---|---|
| `citation.unlinked_in_math` | alert | Citations typeset inside a formula print there as text, without links to the bibliography. |
| `citation.unverified` | alert | Citation evidence could not be verified; citations and bibliography entries are plain text, without links. |
| `citation.link_number_differs` | warning | A citation link recorded a number other than its bibliography entry (natbib compressed ranges); the link uses the entry. |
| `citation.repeated_key` | warning | A bibliography key names more than one item (a repeated or empty \bibitem key); as in LaTeX, its citations link to the last of them. |

### `color`

| Code | Level | Meaning |
|---|---|---|
| `color.low_contrast` | warning | A text colour of the PDF reads below 4.5:1 on the light page or on the author's fill behind it (WCAG 1.4.3); the light theme shows it with the smallest lightness change that reads, hue kept, and the document records each one (ink_adjustments). |

### `content`

| Code | Level | Meaning |
|---|---|---|
| `content.rule_omitted` | warning | A painted rule (table border, separator or \rule) is not reproduced. |

### `figure`

| Code | Level | Meaning |
|---|---|---|
| `figure.rendered_from_pdf` | warning | A figure without an inspectable source (such as a TikZ picture) is a crop of the PDF: drawn as in the paper, but its labels are not text in the reading copy. |
| `figure.source_unsupported` | warning | A picture whose source the converter cannot bind (a class's own image outside the paper, an unsupported format) is not reproduced. |

### `float`

| Code | Level | Meaning |
|---|---|---|
| `float.body_missing` | alert | A float keeps its caption, but its body (an image format or content the reading did not retain) is missing. |
| `float.split` | warning | A float broken across pages (a figure in a beamer frame with allowframebreaks, its image on one slide and its caption on the next) is kept as its parts where they were set; its caption is not joined to its body. |

### `fonts`

| Code | Level | Meaning |
|---|---|---|
| `fonts.source_unavailable` | warning | Source fonts could not be verified and linked; browser fonts substitute, so glyph shapes and widths differ. |

### `frontmatter`

| Code | Level | Meaning |
|---|---|---|
| `frontmatter.unsupported` | alert | Title, authors and abstract were not typed; they remain in the body as ordinary text. |
| `frontmatter.abstract_missing` | warning | The source declares an abstract that reads as empty; title and authors are typed without it, and the body keeps whatever is printed. |
| `frontmatter.decoration_unowned` | warning | Header or footer glyphs without a reading owner (a URL in a running foot) are not kept as page decorations. |

### `graphic`

| Code | Level | Meaning |
|---|---|---|
| `graphic.transform_unsupported` | warning | A scale or rotation applied to a group of graphics is not reproduced; the graphics keep their natural size. |

### `index`

| Code | Level | Meaning |
|---|---|---|
| `index.unstructured` | alert | The typeset index does not read as the entries of its .ind file; it prints as plain paragraphs, its page numbers unlinked. |
| `index.anchor_unplaced` | warning | An index entry whose paragraph the reading did not keep has no anchor; its page links lead elsewhere or nowhere. |
| `index.locator_unplaced` | warning | An index page number is not found in its entry's printed text; it prints unlinked. |
| `index.page_unmatched` | warning | An index page number has no anchor of its term issued on that page; it prints unlinked. |

### `label`

| Code | Level | Meaning |
|---|---|---|
| `label.ambiguous` | alert | A label is defined more than once; references to it print as unlinked text. |
| `label.unresolved_math_row` | alert | The display-math row a label names was not found; references to it print as unlinked text. |
| `label.unresolved_owner` | alert | The numbered element a label names was not found; references to it print as unlinked text. |
| `label.multiply_defined` | warning | A label name is defined more than once; as in LaTeX, references use the last definition. |
| `label.unowned_unreferenced` | whatever | A label without a numbered owner (often a class's page label) that nothing references; nothing visible is lost. |

### `language`

| Code | Level | Meaning |
|---|---|---|
| `language.undeclared` | warning | The source declares no language (babel, polyglossia or a class option); the export is marked as English, which a screen reader may mispronounce (WCAG 3.1.1). |
| `language.unknown` | warning | A language the source selects has no known BCP 47 tag; its text is not marked with a language (WCAG 3.1.1, 3.1.2). |

### `link`

| Code | Level | Meaning |
|---|---|---|
| `link.unsupported` | warning | A link whose target no reader can follow, or that wraps a graphic or another link, prints as its plain text. |

### `list`

| Code | Level | Meaning |
|---|---|---|
| `list.context_unsupported` | alert | A list in an unexpected environment (itemize inside center) reads without its list structure. |

### `math`

| Code | Level | Meaning |
|---|---|---|
| `math.unmapped_glyph` | error | A formula glyph has no Unicode mapping (its font has none); it prints as U+FFFD. |
| `math.alignment_unrowed` | warning | A display alignment row whose native row was not recognized reads as part of its formula, without its own row (a tag on it may not link). |
| `math.nodes_unread` | warning | Some nodes of a formula were not met in the reading flow; the formula prints in full from its MathML. |
| `math.nothing_visible` | warning | A formula has no visible nodes the reading could verify (its box holds a paragraph); it prints from its MathML. |
| `math.source_differs` | whatever | The author's TeX, typeset again with standard packages and the kept macros, gives a different formula; Markdown keeps the TeX derived from its MathML. |
| `math.source_missing` | whatever | A formula has no recorded source position (math a class or bibliography typeset); Markdown keeps the TeX derived from its MathML. |
| `math.source_outside_paper` | whatever | A formula was read from a file outside the paper (a class or package); Markdown keeps the TeX derived from its MathML. |
| `math.source_rows_differ` | whatever | An alignment's rows in the source do not match its typeset rows; Markdown keeps the TeX derived from its MathML. |
| `math.source_unlocated` | whatever | A formula's text was not found uniquely at its recorded position; Markdown keeps the TeX derived from its MathML. |
| `math.source_untested` | whatever | The author's TeX was found but not verified; Markdown keeps the TeX derived from its MathML. |

### `note`

| Code | Level | Meaning |
|---|---|---|
| `note.uncalled` | alert | A footnote body has no call in the text; the note is kept but nothing links to it. |
| `note.number_differs` | warning | A footnote mark could not be matched to its recorded number; the printed mark is kept and linked. |
| `note.title_mark` | warning | An author note whose mark the class prints in the author line; the note is kept, its mark is not linked. |
| `note.unlinked_call` | warning | A footnote call whose text never comes as a footnote (REVTeX footinbib puts it in the bibliography) prints its mark, unlinked. |

### `paragraph`

| Code | Level | Meaning |
|---|---|---|
| `paragraph.interrupted` | warning | A paragraph is split around material printed inside it (a float, a page break with other content); its text keeps its order. |

### `pdfua`

| Code | Level | Meaning |
|---|---|---|
| `pdfua.failed` | alert | The tagged PDF (pdf-ua, see [the tagged PDF route](../design-faq.md#tagged-pdf)) was not written: route 2 could not tag the ordinary PDF (the message says why, for example a conversion without layout); the other outputs are unaffected. |
| `pdfua.invalid` | alert | veraPDF rejects the tagged PDF as PDF/UA-2 (the message names the failed rules); the file is written, but it does not conform and pdf-ua/export.json says so. |
| `pdfua.figure_font` | alert | An included figure's font makes veraPDF reject the tagged PDF; the message names the figure file, the font, the failed rule and the program that made the figure, so the author can re-export it. The claim is withdrawn as for pdfua.invalid. |
| `pdfua.font_unidentified` | alert | An included figure's font subset has no character map (no cmap, post names or ToUnicode), and reLaTeX could not identify some of its glyphs from the original font installed on the machine: they read as U+FFFD in the tagged PDF. The message names the font; installing it lets reLaTeX identify the glyphs. |
| `pdfua.font_cmap_added` | warning | An included figure embedded a non-symbolic TrueType font whose program has only the Unicode platform's cmap subtable, which PDF/UA-2 (8.4.5.7) does not accept; the tagged PDF's copy names that same subtable as Microsoft Unicode (3,1) as well (glyphs, widths and outlines unchanged, the figure file untouched). The message names the figure and the font so the author can re-export. |
| `pdfua.font_unwrapped` | warning | An included figure embedded a font as a whole TrueType collection, which PDF does not allow; the tagged PDF embeds the one face the font names instead (glyphs and their data unchanged, the figure file untouched). The message names the figure and the font so the author can re-export. |
| `pdfua.undescribed` | warning | Graphics of the tagged PDF have no description in relatex/alt.toml; they are tagged with their file name as alternative text, which does not meet WCAG 1.1.1. |
| `pdfua.unfaithful` | warning | Route 2's own checks fail for the tagged PDF: a page could not be bound to the layout and is all artifact, the structure does not read back as the document, or a page does not look as the ordinary PDF's; it is not called conforming. |
| `pdfua.claimed` | alert | The tagged PDF does not conform, but reLaTeX could not take the PDF/UA-2 identification out of its metadata: the file states a conformance it did not earn; trust pdf-ua/export.json, not the file. |
| `pdfua.unverified` | whatever | The tagged PDF was not validated: veraPDF is not installed (tools.verapdf in the global configuration, RELATEX_VERAPDF, or verapdf on PATH) or gave no report. |

### `proof`

| Code | Level | Meaning |
|---|---|---|
| `proof.qed_untyped` | whatever | A customized end-of-proof mark prints as its own content (text, math or a rule) without the end-of-proof type. |

### `reference`

| Code | Level | Meaning |
|---|---|---|
| `reference.page_unverified` | alert | A page reference (\pageref) prints a page that is not the printed number of a page its target is set on, or the page is unknown; it prints as unlinked text. |
| `reference.unresolved` | alert | A reference has no verified target or printed number; it prints as unlinked text. |
| `reference.unlinked_in_math` | warning | A reference inside a formula (\eqref in math) prints its number in the formula but is not a link. |

### `sandbox`

| Code | Level | Meaning |
|---|---|---|
| `sandbox.none` | alert | TeX ran without an operating-system sandbox (Windows has none for it): only TeX's own limits applied (no shell escape, files read and written only below the run, openin_any and openout_any p), so a paper's code could still read the user's files. |

### `source`

| Code | Level | Meaning |
|---|---|---|
| `source.engine_compatibility` | whatever | The source was written for pdfLaTeX or XeLaTeX; it was built under LuaLaTeX with a compatibility shim, and the export is checked against that build. |
| `source.settling_errors` | whatever | The first TeX pass, before the aux files existed, reported errors that the later passes did not; the build is judged by its error-free final pass. |

### `split`

| Code | Level | Meaning |
|---|---|---|
| `split.unfaithful` | error | The chapter files cut from the verified page differ from it; they are left out and the book is published as one page. |
| `split.whole` | warning | The book could not be cut into chapter files (a reference that cannot cross files, an element to continue without an id); it is published as one page. |
| `split.chapters` | whatever | The book is published as a contents page and one file per chapter, cut from the verified page and checked faithful to it ([why](../design-faq.md#books)); the whole page stays in html/. |

### `structure`

| Code | Level | Meaning |
|---|---|---|
| `structure.context_unclosed` | warning | An environment's end was never observed; blocks after it may be nested inside it. |
| `structure.displaced_block` | warning | A block printed inside another section than the one that owns it (a page-1 affiliation note, a later section's rule) belongs to the section where it is printed. |

### `table`

| Code | Level | Meaning |
|---|---|---|
| `table.unmodeled` | alert | A table's layout could not be modeled; its cells are read as plain text without the grid. |
| `table.inside_formula` | warning | A table set inside a formula (a tabular in an equation) is read as part of the formula, not as a modeled table. |
| `table.paint_unverified` | warning | A table's painted rules and text could not be bound to the PDF; it renders from its cell grid, so spacing may differ. |
| `table.span_flattened` | warning | A table cell spanning the rows above it (\multirow{-n}) prints in its own row instead. |

### `text`

| Code | Level | Meaning |
|---|---|---|
| `text.paragraph_incomplete` | error | The end of a paragraph was not observed in the reading; that text is missing from the output. |
| `text.unmapped_glyph` | error | A glyph has no Unicode mapping; its character is lost and prints as U+FFFD. |
