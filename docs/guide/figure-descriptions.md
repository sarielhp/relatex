# Figure descriptions

A reader who cannot see your figures hears or reads, in their place, a short text you give each
one: its *text alternative* (the `alt` text of the page). WCAG 2.1 AA, the bar US public
universities must meet (criterion 1.1.1), asks for one on every figure that carries information.
reLaTeX cannot know what your figures are for, so it marks every figure without one
(`alt="Image"` in the page) and reports it in the accessibility report.

There are two places for a description. If you already write them **in your LaTeX** (the
`alt` key of `\includegraphics`, acmart's `\Description`, reLaTeX's `\relatexalt`), reLaTeX reads
them and nothing else is needed. Otherwise this page shows how to give your figures descriptions without touching your
LaTeX source: you keep them in one file beside the paper, `relatex/alt.toml`; reLaTeX can have a
vision model **draft** them for you, and you **review** each draft before it counts as yours.
`relatex lint --accessibility` lists the figures that have neither, and with `--annotate DIR`
writes a copy of your source in which each such `\includegraphics` carries `alt={TODO: …}`, or
`alt={DRAFT: …}` holding an unreviewed draft from the file, to fill in
([Accessibility](accessibility.md#2-lint-what-is-open-in-your-source)).

- [Descriptions in your LaTeX](#descriptions-in-your-latex)
- [The short version](#the-short-version)
- [The descriptions file](#the-descriptions-file)
- [Setting up a drafting command](#setting-up-a-drafting-command)
- [Approval: sending figures off your machine](#approval-sending-figures-off-your-machine)
- [Drafting](#drafting)
- [Reviewing and accepting](#reviewing-and-accepting)
- [What makes a good description of a mathematical figure](#what-makes-a-good-description-of-a-mathematical-figure)
- [What the export and the report show](#what-the-export-and-the-report-show)
- [Privacy](#privacy)
- [Reference](#reference)

## Descriptions in your LaTeX

reLaTeX reads what the LaTeX tagging project and acmart already give authors, and it takes
precedence over the descriptions file (reLaTeX's own `\relatexalt`, below, yields to it):

```latex
\includegraphics[width=.6\linewidth, alt={A circle of radius r centred at p}]{disk}
\includegraphics[actualtext={∑}]{sum-sign}      % an image that is text: its text
\includegraphics[artifact]{ornament}            % decoration: screen readers skip it
\includegraphics[alt={}]{rule}                  % the same: an empty alternative

\begin{figure}                                  % acmart
  \includegraphics{voronoi}
  \Description[Voronoi diagram of seven points]{The Voronoi diagram of seven points in the
    plane. Each point lies in its own convex cell; the three cells around the shaded vertex v
    meet there, and v is equidistant from their three points.}
  \caption{…}
\end{figure}
```

- **`alt={…}`** is the figure's text alternative (graphicx and the kernel accept the key since
  2024; older LaTeX ignores it). Formatting commands in it are dropped (`\emph{x}` reads `x`).
- **`actualtext={…}`** is for an image that *is* text (a symbol, a logo spelling a word): it is
  read as that text.
- **`artifact`** or **`alt={}`** marks decoration (a rule, an ornament, a logo repeated on every
  page): the page writes `alt="" role="presentation"`, screen readers skip it, and the
  accessibility report counts it as decorative, not as missing.
- **`\Description[short]{long}`** (acmart) describes every included graphic of its figure (a figure drawn with TikZ
  still needs an entry in `relatex/alt.toml`), wherever in the
  figure it stands. The short form is the alternative and the long one the *long description*;
  without a short form, a text up to 150 characters is the alternative alone, and a longer one
  becomes the long description with its first sentence as the alternative. An `alt` key on a
  graphic wins over its figure's `\Description`.
- **`\relatexalt{alt}{short}[details]`** is reLaTeX's own: the alternative, the short description
  (about 40 words, `{}` for none) and optionally the details, the three texts of an entry in
  `relatex/alt.toml`, written in the figure. It describes every included graphic and every TikZ
  drawing of its figure, in any document class, and wins over the figure's `\Description`; an `alt`
  key on a graphic or a tikzpicture still wins over it. Put the no-op definition in your preamble so
  the paper compiles unchanged without reLaTeX (reLaTeX replaces it with its own):

  ```latex
  \ProvideDocumentCommand\relatexalt{m m o}{}   % LaTeX 2020-10 or later
  ...
  \begin{figure}
    \includegraphics{voronoi}
    \relatexalt{Voronoi diagram of seven points}
      {Each point lies in its own convex cell; three cells meet at the shaded vertex v.}
      [v is equidistant from the three points whose cells meet there, so it is the centre of the
       circle through them, and that circle holds no other point.]
    \caption{…}
  \end{figure}
  ```

  Unlike the other forms, it yields to your file: an entry of yours or a reviewed one in
  `relatex/alt.toml` for the figure replaces it (a model draft does not, and `relatex alt draft`
  skips the figure), so a text you correct on the review page wins over the copy in the source.
  The details may start on the next line; a blank line before `[` ends the macro.
- **`\DocumentMetadata{lang=de}`** declares the document's language when babel or polyglossia
  does not (see [Accessibility](accessibility.md)).

**Long descriptions.** A complex figure (a plot, a diagram) needs more than one sentence. Its
long description follows the figure's paragraph on the page as a disclosure, "Description of the
figure", that any reader can open; the image names it with `aria-details` and its text with
`aria-describedby`, so a screen reader reads the alternative and then offers the long text.
Markdown and Typst print it after the figure; a tagged PDF attaches it to the Figure as an
associated text file, its `/Alt` staying the short text. In the descriptions file, an entry's
`long = "…"` does the same (`relatex alt set NAME --long "…"` writes it): keep it to about 40
words. `relatex lint --accessibility` notes figures of the last conversion that look complex (many
text labels and drawn marks) and have a short description only.

**Details.** Only when a reader following the text needs more than that (a construction a proof
walks through, a table-like or step-by-step figure), an entry's `details = "…"` (up to about 150
words) adds a second disclosure, "More about this figure", inside the first one after the long
description. Only the page and the EPUB show details; Markdown, Typst and the tagged PDF leave them
out (the PDF has room for one alternative and one description). Details extend the long description:
an entry with `details` also has `long`.

These descriptions are recorded with `provenance` `"source"`; `relatex alt draft` never drafts
them and `relatex alt review` shows them as done. A `relatex/alt.toml` entry for such a figure is
kept but not used.

## The short version

```sh
cd ~/papers/sums
relatex                          # convert once: relatex needs the figures
relatex alt draft --allow-upload # draft a description per figure (asks your approval the first time)
relatex alt review               # writes alt-review.html here: open it in a browser
$EDITOR relatex/alt.toml         # correct what the drafts got wrong
relatex alt accept --all         # the drafts you have read become your text
relatex                          # convert again: the page and the EPUB carry the descriptions
```

You can also skip the model entirely and write the file yourself; see the next section.

The directory is positional for `draft`, `review`, `accept`, `set` and `import` (`--paper DIR` also works for the last three). `relatex lint --accessibility` lists every figure still without a description or with an unreviewed draft, and the walk-through that puts this step in its place (convert, lint, describe, reconvert, read the claims) is [Accessibility](accessibility.md). Decoration needs no description, and neither does a proof's QED mark drawn as an image: it is named "End of proof" automatically.

## The descriptions file

`relatex/alt.toml` lives in the paper directory, the one you convert. reLaTeX reads it on every
conversion and never changes your LaTeX. It is a [TOML](https://toml.io) file with one table per
figure:

```toml
[figs.disk]
alt = "A circle of radius r centred at the point p, with p marked by a dot."

[figs.voronoi-3]
alt = """
The Voronoi diagram of seven points in the plane: each point lies in its own convex cell,
and the three cells around the shaded vertex v meet there."""
```

**The figure's name** (`disk`, `voronoi-3`) is the name of its file in the export's `figs/`
directory without the extension: `html/figs/disk.svg` is `[figs.disk]`. reLaTeX names figures
after the file you included (`\includegraphics{figures/disk}` gives `disk`), and numbers
figures it drew itself (`fig-1`, `fig-2`, …). A name with a dot in it is written in quotes:
`[figs."plot.v2"]`. `relatex alt review` lists every name, so you never have to guess.

**What an entry holds:**

| Key | Meaning |
|---|---|
| `alt` | the description; one line, or `"""` for several (line breaks become spaces) |
| `long` | the long (short) description, about 40 words: a disclosure after the figure on the page, read after the alternative (`aria-describedby`, `aria-details`) |
| `details` | optional, only when a reader needs more (about 150 words): "More about this figure" inside that disclosure, on the page and in the EPUB only; needs `long` |
| `decorative` | `true` for decoration: an empty alternative screen readers skip (no `alt` needed) |
| `provenance` | who wrote the text: `"author"` (you; the default when the key is absent), `"model"` (a draft nobody has reviewed), `"reviewed"` (a draft a person accepted) |
| `sha256` | the figure the text was written for; reLaTeX fills it in for drafts. When the figure changes, reLaTeX tells you the text may be out of date |
| `model`, `prompt`, `drafted`, `rested_on`, `reviewed` | where a draft came from, what it rested on (`ipe`, `trace`, `bitmap`, `ocr` or `image`: the figure's Ipe source, a trace of its PDF, a PDF that is one bitmap, the text OCR (tesseract) read in a PNG or JPEG figure, or the picture only) and when it was accepted (written by reLaTeX) |

Write entries by hand whenever you like: without `provenance` they are yours. reLaTeX never
replaces an entry that is yours or reviewed, and edits the file line by line, so your comments
(`# …`) and the order of entries stay as you left them.

A mistake in the file (an unknown key, an unclosed string) is reported with its line number, and
the conversion goes on without the file's descriptions (`alt.invalid`).

The file is read only when you convert a directory; a ZIP input has no descriptions file. reLaTeX
leaves it out of its snapshot of your paper, so editing it does not rebuild the PDF: the next
conversion reuses the cached build and only writes the outputs again.

## Setting up a drafting command

reLaTeX has no model and no network code of its own. It runs a **command you choose** once per
figure: a script that calls a vision model (Claude, a self-hosted model, anything) and prints the
description. You set it in your *global* configuration, `~/.config/relatex/config.jsonc`:

```jsonc
{
  "alt_text": {
    "command": ["~/bin/describe-figure"],   // the program, then any arguments
    "uploads": true,                        // it sends figures to another machine
    "model": "claude-haiku-5-5",            // the name the drafts record
    "timeout_seconds": 120                  // per figure
  }
}
```

A paper's own `relatex/config.jsonc` may not set `alt_text`: a paper you received from someone else
must not be able to run a program on your machine, or declare a command local.

### The service's settings and its key

A command that talks to an OpenAI-style chat endpoint (such as a LiteLLM proxy) can take its
settings from reLaTeX's configuration instead of its own:

```jsonc
  "alt_text": {
    "command": ["~/prog/26/relatex/tools/describe_figure"],
    "uploads": true,
    "endpoint": "http://tqed:4000/v1/chat/completions",     // http:// or https://
    "models": ["qwen3.8-27b", "qwen3.8-flash", "muse-glimmer-30b"], // tried in order
    "key_file": "~/.config/auth/litellm_api_key"           // the API key alone, chmod 600
  }
```

reLaTeX hands them to the command as `RELATEX_DESCRIBER_ENDPOINT`, `RELATEX_DESCRIBER_MODELS`
(comma-separated) and `RELATEX_DESCRIBER_KEY_FILE` (the path, `~` expanded); a setting left out
sets no variable. `model`, the name drafts record, defaults to the first of `models`; a command that
writes the model that answered to `RELATEX_DESCRIBER_MODEL_FILE` (as `tools/describe_figure` does,
also after falling back to a later model) has that name recorded instead.

`tools/describe_figure` asks the first model up to three times: the first time as configured, then with
a larger token budget and low reasoning effort, since an empty answer is one whose reasoning used the
whole budget (it says so in its log), and an answer cut at the budget counts as none; only then the next
model.

The key lives in its own file, not in the configuration, so the configuration can be copied,
shared or committed with your dotfiles without giving the key away. `relatex alt draft` refuses
to run when the key file is missing or may be read by your group or by others (its mode must be
`600` or stricter: `chmod 600 ~/.config/auth/litellm_api_key`). reLaTeX only checks the file's
existence and mode; it never opens it, and the key never passes through reLaTeX. The command
reads the file itself when it calls the service (`tools/describe_figure` does, and refuses a
readable-by-others file too), and never prints the key.

### What the command receives

For each figure, reLaTeX runs the command with:

- **the figure as its last argument**: a PNG or JPEG as you included it; a PDF figure rendered to
  PNG at 200 dpi, its own page and box (with `pdftocairo`; the PDF itself if that fails); an SVG as it is;
- **the prompt on its standard input**: instructions for a good description (below), the
  figure's caption, the text around each of its first three references (`Figure~\ref{…}`, about 800
  characters each, the reference marked `→Figure 1←`), or the paragraph before an uncaptioned figure,
  which picture of its figure this is, when that is known (the subcaption printed with it and its place
  among the figure's pictures; for a view of an Ipe file, the view, its layers and labels),
  the document's language, and for a vector PDF figure its geometry ("Geometry of the figure": groups,
  paths, marks and labels read from its embedded Ipe source or a trace of the PDF, the output of
  `relatex alt geometry NAME`; for a PNG or JPEG figure, or a PDF that is one bitmap, the text OCR
  read in it, horizontal and, from passes on the picture turned a quarter each way, vertical: set bottom
  to top or top to bottom);
- **the same parts in the environment**, for a command that builds its own prompt:

  | Variable | Value |
  |---|---|
  | `RELATEX_FIGURE` | the figure file (also the last argument) |
  | `RELATEX_FIGURE_TYPE` | its MIME type: `image/png`, `image/jpeg`, `application/pdf`, `image/svg+xml` |
  | `RELATEX_FIGURE_NAME` | the figure's name in `relatex/alt.toml` |
  | `RELATEX_CAPTION` | the caption, with formulas as `$TeX$` |
  | `RELATEX_CONTEXT` | the text around the references, joined with empty lines |
| `RELATEX_FIGURE_GEOMETRY` | a file with the figure's geometry (text form), empty for a raster figure |
  | `RELATEX_LANGUAGE` | the document's language tag (`en`, `de`, …) |
  | `RELATEX_PROMPT_VERSION` | `relatex-alt-10` |
  | `RELATEX_DESCRIBER_MODEL_FILE` | a file (not yet there) where the command may write the name of the model that answered, on one line; the draft records it as `model` |

The command runs in an empty temporary directory, with your environment (so it sees your API
key variables). What it prints on standard output is the draft (white space collapsed): its first
paragraph the alternative, the paragraphs after an empty line the long description (`long`), and a
paragraph that starts with `Details:`, with any after it, the details (`details`). If it
exits with an error, prints nothing, or takes longer than `timeout_seconds`, that figure is
reported as failed, its last line of standard error with it, and the others go on.

### A sample: Claude through curl

This script sends the figure and the prompt to the Claude API and prints the answer. It needs
`curl`, `jq` and `base64`, and an API key in `ANTHROPIC_API_KEY`. Save it as
`~/bin/describe-figure` and make it executable (`chmod +x`).

```sh
#!/bin/sh
# describe-figure: print a text alternative for one figure, drafted by Claude.
# relatex runs it as: describe-figure FIGURE, with the prompt on standard input.
set -eu
figure=$1
case "$RELATEX_FIGURE_TYPE" in
  image/png|image/jpeg) ;;
  *) echo "describe-figure: cannot send $RELATEX_FIGURE_TYPE" >&2; exit 2 ;;
esac
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cat > "$work/prompt.txt"
base64 < "$figure" | tr -d '\n' > "$work/image.b64"
jq -n --rawfile prompt "$work/prompt.txt" --rawfile image "$work/image.b64" \
      --arg type "$RELATEX_FIGURE_TYPE" '{
  model: "claude-haiku-5-5",
  max_tokens: 1024,
  output_config: {effort: "low"},
  messages: [{role: "user", content: [
    {type: "image", source: {type: "base64", media_type: $type, data: $image}},
    {type: "text", text: $prompt}
  ]}]
}' > "$work/request.json"
curl -sS --fail-with-body https://api.anthropic.com/v1/messages \
  -H "x-api-key: $ANTHROPIC_API_KEY" \
  -H "anthropic-version: 2026-10-21" \
  -H "content-type: application/json" \
  --data-binary @"$work/request.json" > "$work/response.json" ||
  { cat "$work/response.json" >&2; exit 1; }
if [ "$(jq -r .stop_reason "$work/response.json")" = refusal ]; then
  echo "describe-figure: the model declined this figure" >&2; exit 1
fi
jq -r '[.content[] | select(.type == "text") | .text] | join(" ")' "$work/response.json"
```

Notes on the sample:

- `claude-haiku-5-5` is Anthropic's small, inexpensive model, which reads images; at the time of
  writing a figure costs a small fraction of a cent. For hard figures, `claude-sonnet-5-5` or
  `claude-opus-5-5` write better descriptions at a higher price. Set the same name in
  `alt_text.model` so your drafts record it.
- `effort: "low"` asks for a quick answer; a description needs little reasoning.
- The answer may hold several content blocks; the script prints only the text ones.
- The figure is sent to Anthropic: keep `"uploads": true` (see [Privacy](#privacy)).

### A local model

Any program that prints a description works: a script calling a model on your own machine or
your department's server (an OpenAI-compatible endpoint, `llama.cpp`, Ollama). If the figures
never leave machines you trust, set `"uploads": false`, and reLaTeX will not ask for approval.

## Approval: sending figures off your machine

Sending a figure to a model service publishes it to that service: an unpublished result, a
co-author's drawing, or data under an agreement may not be yours alone to send. So while
`alt_text.uploads` is `true` (the default), `relatex alt draft` refuses until you approve it for
the paper:

```text
relatex: error: The drafting command sends figures off this machine (alt_text.uploads is true).
Approve it once for this paper with relatex alt draft --allow-upload (recorded in relatex/alt.toml),
or set alt_text.uploads to false for a local command
```

`relatex alt draft --allow-upload` records your approval at the top of `relatex/alt.toml`, with
the date and the command's name, and drafts:

```toml
[approval]
# You approved sending this paper's figures to the drafting command's service.
upload = true
approved = "2026-10-30"
command = "describe-figure"
```

From then on `relatex alt draft` sends this paper's figures without asking. The approval belongs
to the paper (another paper asks again); to withdraw it, delete the `[approval]` table or set
`upload = false`.

## Drafting

```sh
relatex alt draft [DIRECTORY] [--output DIR] [--limit N] [--redraft] [--allow-upload]
```

`relatex alt draft` reads the figures of the **last conversion** (from the product's `document.json.gz` in the project cache,
or the `--output` directory you converted into), so convert first, and again after you change a
figure. For each figure, in reading order:

- an entry that is yours or reviewed is **kept**, never replaced (if the figure has changed since,
  it is also listed as **stale**);
- a draft made for exactly these bytes is **cached**: nothing is sent again. The identity is
  the SHA-256 of the file you included, so scaling or clipping it differently in LaTeX keeps its
  description;
- a figure whose bytes another entry already describes (you renamed the file) is **reused**:
  that text is copied, as a draft for you to review again, without sending the figure;
- otherwise the figure is sent and the answer is saved as a draft (`provenance = "model"`).

The file is saved after every figure, so stopping the command (Ctrl-C) keeps what it drafted.
`--limit N` sends at most N figures in one run (the rest are listed as waiting); `--redraft`
sends figures whose draft is current again, for instance after you changed the command or the
model (it still never touches your or a reviewed entry). It prints each figure as it goes and a
report:

```json
{ "status": "success", "drafted": ["disk", "voronoi-3"], "cached": [], "kept": ["fig-1"],
  "stale": [], "waiting": [], "failed": [] }
```

## Reviewing and accepting

```sh
relatex alt review [DIRECTORY] [--output DIR]
```

writes `alt-review.html` in the output directory (the directory you run in, unless the `output` setting says otherwise). Open it in any browser; it works offline. It
shows, for every figure:

- the picture, its caption and the paragraphs that cite it;
- its description and its state: **draft, not reviewed**; **reviewed**; **your text**;
  **no description**; or **written for an earlier version of the figure**;
- its long description and details, and for a figure that looks complex without a long one, a
  suggestion;
- what to do next, with the exact command;
- the description, the long description and the details in **editable fields**, with an **Accept** checkbox
  (not for a figure your LaTeX describes, which no entry overrides, nor one marked decorative).

### Reviewing in bulk on the page

Read each draft against its figure. Where it is right, tick *Accept*; where it is wrong, correct
it in the field (editing ticks *Accept* for you); for a figure without a description, write one.
*Tick every draft* ticks all the drafts at once, for when you have read them all. Then press
*Save the review file*: the browser saves `alt-review.toml` (in your downloads folder; keep it as `relatex/alt-review.toml`)
with the figures you ticked, and you take it back in the paper directory:

```sh
relatex alt import ~/Downloads/alt-review.toml
relatex alt import --paper ~/papers/sums ~/Downloads/alt-review.toml
```

- a draft you accepted **as shown** becomes `provenance = "reviewed"`, with the date;
- a text you **edited or wrote** becomes yours, `provenance = "author"` (the draft's model and date
  stay in the entry as its history);
- a figure you did **not tick** is left as it is: a draft stays a draft;
- a decision is **refused** when the figure's text in `relatex/alt.toml` changed after the page was
  written (a new draft, your own edit), when the entry describes another version of the figure, or
  when the text is empty; the command lists each refusal with its reason. Review those again.

The saved file records, for each ticked figure, the text the page showed beside your text, so
reLaTeX can tell an accepted draft from an edited one and never marks a text you did not see as
reviewed. It is not a `relatex/alt.toml` (it begins with a `[review]` table): saved over the
descriptions file by mistake, it is refused (`alt.invalid`) rather than taken for your descriptions.
The page's script only builds that file; with scripts off the page still shows everything, and the
commands below do the same work.

### Reviewing in the file

To write a text without opening the file, `relatex alt set NAME [DIRECTORY] --alt "…"`, `--long "…"` or
`--details "…"` (`--paper DIRECTORY`; `--long ""` or `--details ""` removes it) changes that line of the entry
and keeps the rest; a draft stays a draft until you accept it.

Read each draft against the figure. Correct the `alt` text in `relatex/alt.toml` where it is
wrong, then accept it:

```sh
relatex alt accept disk voronoi-3        # these figures
relatex alt accept --all                 # every draft in the file
relatex alt accept --all ~/papers/sums   # the directory is positional, as for draft and review
relatex alt accept --paper ~/papers/sums disk
```

Accepting changes `provenance = "model"` to `provenance = "reviewed"` and records the date; the
text is now yours and reLaTeX never replaces it. Editing a draft's text does not accept it:
the entry stays a draft until you run `accept`. Deleting an entry (or just its `alt`) makes the
figure undescribed again, and the next `relatex alt draft` drafts it afresh.

The review page is a tool's output, not part of your export: it is not in the EPUB, and the next
conversion replaces it. Run `relatex alt review` again whenever you want a fresh list.

**Why a saved file and a command?** A page opened from disk cannot write into your paper
directory without a server or a browser extension. Saving a file and importing it works in every
browser, offline, and keeps `relatex/alt.toml` a plain text file with a reviewable history in
version control.

## What makes a good description of a mathematical figure

The description replaces the figure for someone who cannot see it. Ask: *what does the reader
need from this figure to follow the argument?* Then say that, briefly.

1. **Purpose first.** Say what the figure shows *for the text*: "The construction in the proof
   of Lemma 3: the segment pq is shortcut by the dashed path through r." not "Points and lines."
2. **One to three sentences**, at most about 300 characters. Screen-reader users hear every word;
   a long text belongs in the paper itself (a paragraph, or a table of the data behind a plot).
3. **Don't repeat the caption.** The reader hears the caption too; add what only the picture
   holds.
4. **Speakable notation.** Write math as you would read it aloud, with the paper's own names:
   "the disk of radius r around p", "the graph of f(x) = x squared over the interval from 0 to
   1", "the triangle with vertices a, b, c". No LaTeX: `$\mathcal{D}_r(p)$` is read out
   character by character.
5. **Structure, not pixels.** For diagrams, say what is connected to what and what is marked:
   "A path of four vertices; the middle edge is drawn bold." Colours only if the text uses them,
   and then also say which element they mark.
6. **Plots and charts:** the axes (with units), the shape or trend, and any value the text relies
   on: "Running time against n on log-log axes: both algorithms grow linearly, the new one about
   ten times lower throughout."
7. **Don't start with "Image of" or "Figure showing"**: the reader already knows it is a figure.
8. **Don't guess.** If the figure only illustrates something the text states in full, say so
   briefly ("An illustration of the definition above."). A purely decorative image needs no
   description: mark it `artifact` in the source, or `decorative = true` in the file.

Models are good at describing what they see and bad at knowing what matters, and they misread
small labels and subscripts: check every symbol in a draft against the figure.

The prompt reLaTeX sends (version `relatex-alt-10`) asks the model for exactly these points: an
alternative of one sentence of at most 150 characters naming what the picture is and its main content;
after an empty line a short description of at most 40 words in complete sentences that adds to the
alternative, opening with something in the picture (no "Shows…", "Illustrates…", "This figure…", purpose
clause or imperative), concrete (a plot's axes with their ranges and the trend, with values only where the picture prints them; a
diagram's parts, their counts and connections, a connection only where a line or arrow is drawn; an axis
logarithmic only by its tick labels), in the figure's own symbols (T*, not "T star"); only when a
reader following the text needs more, a paragraph starting "Details:" of at most 150 words with what the
picture contains, never the caption retold; only what is visible and what the caption or text states (a purpose only in their words, no conclusion, every name from a printed label, the caption or the text, "unlabeled" otherwise, parts called parallel, aligned or in sequence only when the drawn lines make them so; prompt version alt-10); for one
panel of several, to describe only this one and to name it only when the input says which it is; to
trust the geometry over the picture for counts and labels but to check OCR text against the picture, to name labels by their printed text (the
geometry gives it beside the Ipe source), to keep the geometry's coordinates, radii and styles out of the
prose, and to state an orientation only when certain. An answer over 200 characters
of alternative, 60 words of long description or 200 words of details is asked for once more with a note to shorten it
appended to the prompt; one still over is kept whole and reported as `alt.too_long`. To
read it, point `alt_text.command` for a moment at a script that copies its standard input to a
file and prints nothing (every figure then fails, and no draft is written).

## What the export and the report show

On the next conversion every described figure carries its text:

| Output | Where |
|---|---|
| Page and EPUB | `<img alt="…">`; a long description as `<details>` after the figure's paragraph, named by `aria-details` and its text by `aria-describedby` (EPUB metadata `longDescription`), with the details as a nested `<details>`, "More about this figure"; decoration as `alt="" role="presentation"` |
| Markdown | `![…](figs/disk.svg)` (`![](…)` for decoration); a long description as a paragraph after it |
| Typst | `#image("…", alt: "…")`; a long description as a block after the figure |
| Tagged PDF | the Figure's `/Alt`; a long description as its associated file (`text/plain`, "Long description") |
| JSON / bundle | the image span's `alt`, `long_description`, `description_details` or `decorative`, and `alt_provenance` (`source`, `source_macro` for `\relatexalt`, `author`, `model` or `reviewed`) |

The order is: the source (`alt`, `actualtext`, `artifact`, `\Description`), then your or a
reviewed entry of the file, then `\relatexalt`, then a draft.

**Unreviewed drafts are used but stay marked.** A draft gives the figure a text, so the reader is
not left with "Image"; but it is counted apart. The conversion warns:

```text
paper: warning[alt.unreviewed]: 3 figure descriptions are model drafts nobody has reviewed (disk, …)
```

and with `--verify-browser` the accessibility report (`accessibility.json` in the product) has a row for
them, separate from figures with no text at all:

| Check | WCAG | Passes when |
|---|---|---|
| `image_alternatives` | 1.1.1 | every image has a text alternative (drafts count) |
| `image_drafts_reviewed` | 1.1.1 | no alternative is an unreviewed model draft |

reLaTeX will claim conformance only when every check passes, so no claim ever rests on a text
nobody read.

**When a figure changes** after its description was written (its bytes no longer match `sha256`),
the conversion warns `alt.stale`. Your own or a reviewed text is still used (check it, then
update or delete its `sha256`); a draft is left out until you draft again. An entry whose figure
is gone is reported as `alt.unmatched` (informational) and kept.

## Privacy

- **Nothing leaves your machine by default.** There is no drafting command until you configure
  one, and an uploading command needs your approval per paper.
- **What is sent:** the figure (as an image), its caption, the paragraphs that cite it, and the
  document's language, for figures that have no current draft. Nothing else of the paper.
- **Where it goes** is decided by your command alone. With the sample above, to Anthropic under
  your API account's terms; with a self-hosted model, nowhere outside your network.
- **No keys in reLaTeX.** reLaTeX never sees, stores or logs an API key; your command reads it
  from its environment or its own configuration.
- **Unpublished work.** Before approving, consider whether the figures are yours to send:
  co-authors' material, data under a use agreement, a submission under double-blind review.
  When in doubt, use a local model or write the descriptions yourself.
- **The file itself** holds only your descriptions, the figures' SHA-256 (which reveals nothing
  about their content), and your approval. It is safe to commit with the paper.

## Reference

| Command | Effect |
|---|---|
| `relatex alt draft [DIR] [--output DIR] [--allow-upload] [--limit N] [--redraft]` | draft missing descriptions with `alt_text.command` |
| `relatex alt review [DIR] [--output DIR]` | write `alt-review.html` and print the counts |
| `relatex alt accept NAME… \| --all [DIR] [--paper DIR]` | mark drafts reviewed |
| `relatex alt import FILE [DIR] [--paper DIR]` | apply the review page's saved decisions: accepted drafts reviewed, edited texts yours |
| `relatex lint --accessibility [DIR] [--main FILE] [--output DIR] [--math] [--strict] [--json] [--annotate DIR]` | list what only you can supply, from the source (see [Accessibility](accessibility.md#2-lint-what-is-open-in-your-source)); `--annotate` writes a marked copy of the source with `alt={TODO: …}` / `alt={DRAFT: …}` placeholders and `REVIEW.md` |

| Setting (global configuration only) | Default | Meaning |
|---|---|---|
| `alt_text.command` | `null` | the describing program and its arguments, as a list |
| `alt_text.uploads` | `true` | the command sends figures off the machine: approval needed per paper |
| `alt_text.model` | the first of `models`, else the program's name | the name drafts record in `model` |
| `alt_text.endpoint` | `null` | the service's address, passed as `RELATEX_DESCRIBER_ENDPOINT` |
| `alt_text.models` | `null` | model names in order, passed as `RELATEX_DESCRIBER_MODELS` |
| `alt_text.key_file` | `null` | the file holding the API key (mode 600; reLaTeX never reads it), passed as `RELATEX_DESCRIBER_KEY_FILE` |
| `alt_text.timeout_seconds` | `120` | per figure, 1 to 3600 |

| Diagnostic | Level | Meaning |
|---|---|---|
| `alt.invalid` | alert | `relatex/alt.toml` could not be read; none of it is used |
| `alt.stale` | warning | a description was written for an earlier version of its figure |
| `alt.unreviewed` | warning | some descriptions are drafts nobody has reviewed |
| `alt.unmatched` | whatever | entries for figures the paper does not have |

The design and its reasons (drafts, descriptions in the source, the lint, the annotated review copy and
the review page) are in the design FAQ: [who writes figure descriptions](../design-faq.md#figure-descriptions) and
[how an author steers a conversion](../design-faq.md#author-input).
