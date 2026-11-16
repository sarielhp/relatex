// Package alttext is the author tool for figure descriptions (decision 086): descriptions drafted
// into the sidecar relatex/alt.toml by a command the author configures, and the review page that
// turns a draft into the author's text. The sidecar and the figures it describes are the document
// model's (figdesc). relatex never edits the LaTeX source, never replaces an entry the author
// wrote or reviewed, and holds no network code or keys.
package alttext

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/rec"
)

// PromptVersion names the instructions a draft was made with; a new version may redraft. Version 2
// asks for a long description of a complex figure after the alternative (decision 110); version 3
// adds the figure's geometry and asks to describe only what is visible (decision 123); version 4
// uses the printed labels, keeps coordinates and styles out of the prose and states the geometry's
// orientation (the figure A/B, plans/status/2026-11-11-figure-ab); version 5 asks for a short,
// purpose-first alternative and a long description of at most five short lines (the A/B v2 grading);
// version 6 asks for a short description of at most 40 words and, only when a reader following the
// text needs more, details (decision 125); version 7 bans the openings the figure-type survey found
// (purpose clauses, imperatives, "Illustrates"), asks for concrete content (axes with ranges, counts,
// values the picture prints), keeps a single panel to its own content and the purpose to what the
// picture shows, and asks for complete sentences (decision 133); version 8 adds which picture of its
// figure the describer sees: its place, subcaption, or Ipe view, layers and labels (decision 133);
// version 9 gives plot values only where the picture prints them, calls an axis logarithmic only by
// its tick labels, states connections only where drawn, and says OCR text may be wrong (decision 138); version 10 describes only what is visible or stated, gives a purpose only in the text's words, draws no conclusions, takes names only from printed labels and says "unlabeled" for the rest (decision 138, amended; plans/status/2026-11-16-a11y-night/describer-prompt.md).
const PromptVersion = "relatex-alt-10"

// Length limits a draft is held to (decisions 123, 125): an answer over them is asked again once
// with shortenNote appended to the prompt, and is kept as it is, never cut, when still over. They
// leave room above the prompt's own limits (150 characters, 40 and 150 words).
const (
	altCharLimit     = 200
	longWordLimit    = 60
	detailsWordLimit = 200
)

// detailsLabel opens the optional third part of a describer's answer (decision 125).
const detailsLabel = "Details:"

// shortenNote is appended to the prompt when a describer answer is over the length limits.
const shortenNote = "\nYour previous answer was too long. Shorten to the limits: the alternative one sentence of at most 150 characters, " +
	"the short description at most 40 words, the details (only when needed) at most 150 words.\n"

// Description is a describer's answer in its parts (decisions 110, 125): the alternative, the short
// description and the optional details.
type Description struct {
	Alt, Long, Details string
}

// TooLong reports an alternative over altCharLimit characters, a short description over
// longWordLimit words or details over detailsWordLimit words.
func (d Description) TooLong() bool {
	return len([]rune(d.Alt)) > altCharLimit || len(strings.Fields(d.Long)) > longWordLimit ||
		len(strings.Fields(d.Details)) > detailsWordLimit
}

// rasterDPI is the resolution a PDF figure is rendered at for a describer that reads pictures.
const rasterDPI = "200"

// describerOutputLimit bounds what a describer may print: an alternative is a few sentences.
const describerOutputLimit = 64 * 1024

// Drafter runs the author's describing command over the figures that need a draft.
type Drafter struct {
	Command []string      // the program and its arguments; the figure file is appended
	Model   string        // the name the drafts record
	Timeout time.Duration // per figure
	Today   string        // the date drafts record, YYYY-MM-DD
	Limit   int           // the most figures sent in one run; 0 sends all
	Redraft bool          // send figures whose draft is current again
	Notify  func(key, outcome string)
	Env     []string // extra NAME=VALUE variables for the command (the describer's settings)

	scans map[string]Geometry // figure geometry by identity and scanner version, for this run
	temp  string              // the run's own temporary directory, made by Run and removed after it
}

// Outcome lists the figures by what drafting did with them.
type Outcome struct {
	Drafted []string // sent and described
	Reused  []string // described by an entry for the same bytes under another name
	Cached  []string // a draft for these bytes was there already
	Kept    []string // the author's or a reviewed entry, never replaced
	Stale   []string // kept, but written for other bytes than the figure has now
	Waiting []string // beyond the limit; the next run sends them
	TooLong []string // drafted, but over the length limits after asking again (alt.too_long)
	Failed  []rec.Map
}

// Run drafts what the figures of document need into sidecar, saving it after every figure so an
// interrupted run keeps what it had.
func (d *Drafter) Run(document rec.Map, sidecar *figdesc.Sidecar) (Outcome, error) {
	outcome := Outcome{}
	d.temp = command.Scratch("alt-draft-")
	defer os.RemoveAll(d.temp)
	language := rec.ToS(rec.AsMap(document["language"])["tag"])
	sent := 0
	for _, figure := range figdesc.Figures(document) {
		entry := sidecar.Entries[figure.Key]
		identity := figure.Identity()
		switch {
		case figure.FromSource() || figure.FromMacro():
			outcome.Kept = append(outcome.Kept, figure.Key)
			continue
		case entry != nil && entry.Provenance != figdesc.Model:
			outcome.Kept = append(outcome.Kept, figure.Key)
			if entry.SHA256 != "" && entry.SHA256 != identity {
				outcome.Stale = append(outcome.Stale, figure.Key)
			}
			continue
		case !d.Redraft && entry != nil && entry.SHA256 == identity && figdesc.Normalize(entry.Alt) != "":
			outcome.Cached = append(outcome.Cached, figure.Key)
			continue
		}
		if other := sidecar.BySHA(identity, figure.Key); other != nil && !d.Redraft {
			draft := figdesc.Entry{Key: figure.Key, Alt: other.Alt, Long: other.Long, Details: other.Details, Model: other.Model, SHA256: identity,
				Prompt: other.Prompt, Drafted: other.Drafted, RestedOn: other.RestedOn}
			if err := d.save(sidecar, draft); err != nil {
				return outcome, err
			}
			outcome.Reused = append(outcome.Reused, figure.Key)
			continue
		}
		if d.Limit > 0 && sent >= d.Limit {
			outcome.Waiting = append(outcome.Waiting, figure.Key)
			continue
		}
		sent++
		description, answered, err := d.describeShort(document, figure, language)
		if err != nil {
			outcome.Failed = append(outcome.Failed, rec.Map{"figure": figure.Key, "reason": err.Error()})
			d.notify(figure.Key, "failed: "+err.Error())
			continue
		}
		if description.TooLong() {
			outcome.TooLong = append(outcome.TooLong, figure.Key)
		}
		draft := figdesc.Entry{Key: figure.Key, Alt: description.Alt, Long: description.Long, Details: description.Details, Model: d.modelOf(answered),
			SHA256: identity, Prompt: PromptVersion, Drafted: d.Today, RestedOn: answered.restedOn}
		if err := d.save(sidecar, draft); err != nil {
			return outcome, err
		}
		outcome.Drafted = append(outcome.Drafted, figure.Key)
		d.notify(figure.Key, "drafted")
	}
	return outcome, nil
}

// answer is one run of the describing command: what it printed, what the draft rests on, and the
// model the command reported as the one that answered ("" when it reported none).
type answer struct {
	text, restedOn, model string
}

// modelOf is the model a draft records: the one the command reported as answering, else the
// configured name (decision 133).
func (d *Drafter) modelOf(a answer) string {
	if a.model != "" {
		return a.model
	}
	return d.Model
}

// describeShort describes the figure and, when the answer is over the length limits, asks once
// more with shortenNote; a second answer that fails or is empty leaves the first one. The answer it
// returns is the run whose text it keeps, so the draft names the model that wrote that text.
func (d *Drafter) describeShort(document rec.Map, figure *figdesc.Figure, language string) (Description, answer, error) {
	first, err := d.describe(document, figure, language, "")
	if err != nil {
		return Description{}, answer{}, err
	}
	description := SplitDescription(first.text)
	if !description.TooLong() {
		return description, first, nil
	}
	d.notify(figure.Key, "over the length limits; asking again")
	again, err := d.describe(document, figure, language, shortenNote)
	if err != nil {
		return description, first, nil
	}
	return SplitDescription(again.text), again, nil
}

func (d *Drafter) save(sidecar *figdesc.Sidecar, draft figdesc.Entry) error {
	if err := sidecar.PutDraft(draft); err != nil {
		return err
	}
	return sidecar.Write()
}

func (d *Drafter) notify(key, outcome string) {
	if d.Notify != nil {
		d.Notify(key, outcome)
	}
}

// describe runs the command on one figure: the picture as its last argument and in
// RELATEX_FIGURE, the prompt on its standard input, the parts of the prompt in the environment.
// A vector figure's geometry is in the prompt and in the file RELATEX_FIGURE_GEOMETRY (decision 123).
// What it prints is the description; the model it names in the file RELATEX_DESCRIBER_MODEL_FILE is
// the one that answered (decision 133).
func (d *Drafter) describe(document rec.Map, figure *figdesc.Figure, language, note string) (answer, error) {
	directory, err := os.MkdirTemp(d.temp, "figure-")
	if err != nil {
		return answer{}, err
	}
	defer os.RemoveAll(directory)
	path, mime, err := writeImage(document, figure, directory)
	if err != nil {
		return answer{}, err
	}
	geometry := d.geometry(document, figure)
	geometryPath := ""
	if geometry.Text != "" {
		geometryPath = filepath.Join(directory, geometryFile)
		if err := os.WriteFile(geometryPath, []byte(geometry.Text), 0o600); err != nil {
			return answer{}, err
		}
	}
	modelPath := filepath.Join(directory, modelFile)
	limited, cancel := context.WithTimeout(context.Background(), d.Timeout)
	defer cancel()
	run := exec.CommandContext(limited, d.Command[0], append(append([]string{}, d.Command[1:]...), path)...)
	run.Dir = directory
	run.Env = append(os.Environ(), "RELATEX_FIGURE="+path, "RELATEX_FIGURE_TYPE="+mime, "RELATEX_FIGURE_NAME="+figure.Key,
		"RELATEX_CAPTION="+figure.Caption, "RELATEX_CONTEXT="+figure.Context, "RELATEX_LANGUAGE="+language,
		"RELATEX_PROMPT_VERSION="+PromptVersion, "RELATEX_FIGURE_GEOMETRY="+geometryPath,
		"RELATEX_DESCRIBER_MODEL_FILE="+modelPath)
	run.Env = append(run.Env, d.Env...)
	run.Stdin = strings.NewReader(Prompt(figure, language, geometry) + note)
	var stdout, stderr bytes.Buffer
	run.Stdout, run.Stderr = &stdout, &stderr
	if err := run.Run(); err != nil {
		if errors.Is(limited.Err(), context.DeadlineExceeded) {
			return answer{}, fmt.Errorf("no answer within %s", d.Timeout)
		}
		return answer{}, fmt.Errorf("%v%s", err, lastLine(stderr.String()))
	}
	if stdout.Len() > describerOutputLimit {
		return answer{}, fmt.Errorf("printed %d bytes; a description is a few sentences", stdout.Len())
	}
	text := strings.TrimSpace(stdout.String())
	if figdesc.Normalize(text) == "" {
		return answer{}, fmt.Errorf("printed no description%s", lastLine(stderr.String()))
	}
	return answer{text: text, restedOn: geometry.RestedOn, model: answeringModel(modelPath)}, nil
}

// modelFile is the file in the run's directory where the command may name the model that answered.
const modelFile = "answered-model"

// modelNameLimit bounds a reported model name; a longer first line is not a name.
const modelNameLimit = 200

// answeringModel is the model name the command wrote to path: its first line, trimmed, when that is
// a plain name (printable, without white space, quotes or backslashes, at most modelNameLimit
// bytes); "" otherwise (no file, or not a name), and the draft then records the configured name.
func answeringModel(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 4*modelNameLimit {
		return ""
	}
	name, _, _ := strings.Cut(string(data), "\n")
	name = strings.TrimSpace(name)
	plain := func(r rune) bool { return r <= ' ' || r == 0x7f || r == '"' || r == '\\' || !unicode.IsPrint(r) }
	if name == "" || len(name) > modelNameLimit || strings.ContainsFunc(name, plain) {
		return ""
	}
	return name
}

// paragraphBreak is an empty line between two paragraphs of a describer's answer.
var paragraphBreak = regexp.MustCompile(`\n[ \t\r]*\n`)

// SplitDescription is a describer's answer in its parts, each on one line: the alternative is its
// first paragraph; the paragraphs after it are the short description (decision 110), up to one that
// starts with "Details:", which with the rest is the details (decision 125), without the label.
// Details given without a short description are the short description: details only extend one.
func SplitDescription(text string) Description {
	paragraphs := paragraphBreak.Split(strings.TrimSpace(text), -1)
	description := Description{Alt: figdesc.Normalize(paragraphs[0])}
	short, details := []string{}, []string{}
	for _, paragraph := range paragraphs[1:] {
		trimmed := strings.TrimSpace(paragraph)
		if len(details) == 0 && strings.HasPrefix(trimmed, detailsLabel) {
			trimmed = strings.TrimPrefix(trimmed, detailsLabel)
			details = append(details, trimmed)
			continue
		}
		if len(details) > 0 {
			details = append(details, trimmed)
		} else {
			short = append(short, trimmed)
		}
	}
	description.Long = figdesc.Normalize(strings.Join(short, " "))
	description.Details = figdesc.Normalize(strings.Join(details, " "))
	if description.Long == "" {
		description.Long, description.Details = description.Details, ""
	}
	return description
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return ": " + last
	}
	return ""
}

// placeImage puts the figure in directory as a file a describer can read: PNG and JPEG as they
// are, a PDF rendered to PNG (the PDF itself when pdftocairo is missing or fails), anything
// else as it is. A PDF is rendered at the figure's own page and box (decision 123). It returns
// the file and its MIME type.
func placeImage(document rec.Map, figure *figdesc.Figure, directory string) (string, string, error) {
	data, mime := figure.Image(document)
	extension := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "application/pdf": ".pdf",
		"image/svg+xml": ".svg"}[mime]
	path := filepath.Join(directory, "figure"+extension)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", "", err
	}
	if mime != "application/pdf" {
		return path, mime, nil
	}
	tool := command.Resolve("pdftocairo")
	if tool == "" {
		return path, mime, nil
	}
	target := filepath.Join(directory, "figure")
	render := exec.Command(tool, rasterArgs(figure, path, target)...)
	if render.Run() != nil {
		return path, mime, nil
	}
	return target + ".png", "image/png", nil
}

// rasterArgs are pdftocairo's arguments for the figure's page and box (decision 123): the whole
// page, page one only, and for a view a crop of the placement's box, whose coordinates are from the
// page box the geometry was measured in. A whole page is rendered in the box the paper shows (the
// crop box, graphicx's default; decision 123 amendment 3), as its geometry is read: the media box
// may hold leftovers of the drawing program around the figure, which the paper never prints.
func rasterArgs(figure *figdesc.Figure, path, target string) []string {
	page := figure.Page
	if page < 1 {
		page = 1
	}
	number := strconv.Itoa(page)
	args := []string{"-png", "-singlefile", "-r", rasterDPI, "-f", number, "-l", number}
	// pdftocairo renders only the media and crop boxes; a view measured in another box renders whole.
	if len(figure.Box) != 4 && slices.Contains([]string{"", "cropbox"}, figure.PageBox) {
		args = append(args, "-cropbox")
	}
	if len(figure.Box) == 4 && slices.Contains([]string{"", "mediabox", "cropbox"}, figure.PageBox) {
		if figure.PageBox == "cropbox" {
			args = append(args, "-cropbox")
		}
		x, y, width, height := cropPixels(figure.Box)
		args = append(args, "-x", strconv.Itoa(x), "-y", strconv.Itoa(y), "-W", strconv.Itoa(width), "-H", strconv.Itoa(height))
	}
	return append(args, path, target)
}

// cropPixels is a box in bp from the page's top-left corner as pdftocairo's crop integers: pixels
// at rasterDPI.
func cropPixels(box []float64) (int, int, int, int) {
	dpi, _ := strconv.ParseFloat(rasterDPI, 64)
	pixels := func(bp float64) int { return int(math.Round(bp * dpi / 72)) }
	x, y := pixels(box[0]), pixels(box[1])
	width, height := pixels(box[2])-x, pixels(box[3])-y
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return x, y, width, height
}

// Prompt is the instruction the describer receives on its standard input: what a good
// alternative for a figure in a mathematical paper is, then the figure's caption and the text
// around each reference to it, and the geometry of a vector figure (decision 123).
func Prompt(figure *figdesc.Figure, language string, geometry Geometry) string {
	var out strings.Builder
	out.WriteString(promptText)
	if language != "" {
		out.WriteString("\nWrite in the document's language: " + language + ".\n")
	}
	out.WriteString("\nCaption:\n" + orNone(figure.Caption) + "\n\n")
	if panelKnown(figure.Panel, geometry) {
		out.WriteString("Which picture of the figure this is:\n" + panelText(figure.Panel, geometry) + "\n\n")
	}
	out.WriteString("Text around each reference to the figure:\n" + references(figure.Citations) + "\n")
	if geometry.Text != "" {
		out.WriteString("\nGeometry of the figure (read from its file; x to the right, y up, 0-100 along the longer side):\n" + strings.TrimRight(geometry.Text, "\n") + "\n")
	}
	return out.String()
}

// panelKnown says whether the prompt names which picture of its figure this is (decision 138): only
// when the figure prints a subcaption for it (or its group) or it is a view of an Ipe file with
// several views. Measured blind, the section helped there and hurt where it could only say that
// nothing is known (picture i of n without subcaption).
func panelKnown(panel figdesc.Panel, geometry Geometry) bool {
	return panel.Subcaption != "" || panel.RowText != "" || strings.Contains(geometry.View, " view ")
}

// panelText says which picture of its figure the describer sees (decision 133): its place among
// the figure's pictures and its subcaption, and for a view of an Ipe file with several views, the
// view, its layers and its labels.
func panelText(panel figdesc.Panel, geometry Geometry) string {
	text := "This picture is " + panel.Describe()
	if !strings.Contains(geometry.View, " view ") {
		return text
	}
	text += "\nIt is " + geometry.View + " of its Ipe file"
	if len(geometry.Layers) > 0 {
		text += ", showing the layers " + strings.Join(geometry.Layers, ", ")
	}
	if len(geometry.Labels) > 0 {
		text += "; its labels: " + strings.Join(geometry.Labels, ", ")
	}
	return text + "."
}

// references numbers the citation windows, one per reference to the figure.
func references(citations []string) string {
	if len(citations) == 0 {
		return "(none)"
	}
	var out strings.Builder
	for i, window := range citations {
		if i > 0 {
			out.WriteString("\n\n")
		}
		fmt.Fprintf(&out, "%d. %s", i+1, orNone(window))
	}
	return out.String()
}

func orNone(text string) string {
	if strings.TrimSpace(text) == "" {
		return "(none)"
	}
	return text
}

const promptText = `You write the text alternative (alt text) of a figure in a mathematical or scientific paper,
for a reader who cannot see it: someone listening to a screen reader or reading braille. Short
texts serve that reader best: a long, tedious one loses the listener. A useful text is concrete:
it says what is drawn and what can be read from it, not that something is drawn.

Answer in two or three paragraphs, separated by one empty line:

1. The alternative: one sentence of at most 150 characters naming what the picture is and its
   main content, for example "Log-log plot of running time against n for three solvers; the
   new one is fastest above n = 1000."
2. A short description: at most 40 words in complete sentences that add to the alternative
   (never repeat it). Its first word names something in the picture; for example "The shaded
   region is the separator. Its vertices split the rest of the planar graph into two parts."
   For a plot: the axes with their quantities and ranges as the tick labels print them, and the
   trend in words, with a value only where the picture prints it or a curve plainly meets a
   labelled tick (leave out a value you would have to estimate). For a diagram: its main parts,
   how many there are (from the geometry) and how they connect.
3. Only when a reader following the text needs more (a construction a proof walks through, a
   table-like or step-by-step figure, several panels, values the text relies on): a paragraph
   that starts with "Details:" and gives what the picture contains in at most 150 words: labels,
   counts, values, each panel in turn. Never retell the caption. Otherwise print no third
   paragraph.

- Banned openings for every part: "Shows", "Illustrates", "Demonstrates", "Depicts",
  "Visualizes", "Presents", "Displays", "This figure", "The figure", "Image of", "Figure
  showing", "Supports", a purpose clause ("To compare ...", "For ..."), an imperative
  ("Compare ...", "Prove ...") or a heading followed by a colon ("Purpose:", "Overview:").
- Describe only what is visible in the picture and what the caption or the text states. Say what
  the figure is for or what it means only in the caption's or the text's words ("The text says
  this shows ..."), and leave the purpose out when they do not say it. Draw no conclusion of
  your own: not that curves agree, a method wins, a trend is "close" or "best", or a part
  "represents" or "combines" something, unless the caption or the text says so. Never tell a
  story the picture cannot show (a process over time in one still image, a comparison with
  something not in the picture, a step of a proof).
- Take every name from the picture's printed labels, the caption or the text: a material, a
  method, a phase, a quantity, a unit, a solver. Do not complete, translate or guess one (a
  curve labelled "vac" stays "vac"); when a part has no label, call it "unlabeled" and describe
  its look or place. Add no domain fact that is not printed or stated.
- The image may be only one panel or view of the figure while the caption and the text speak
  of all of them. Describe only what this image contains. Name its panel (letter or position)
  only when the input says which one it is ("Which picture of the figure this is" below, when
  given: its subcaption, or an Ipe view with its layers and labels); never guess it, never give
  this image the content or conclusion of other panels.
- Give counts that a reader would otherwise have to ask for (vertices, curves, points, panels),
  taken from the geometry when there is one; mention colour only when it tells things apart.
- Call an axis logarithmic only when its tick labels grow by a constant factor (1, 10, 100 or
  10^k); judge each axis on its own.
- Say that two parts are connected only where a drawn line or arrow joins them, and give an
  arrow's direction from its tail to its head. Call parts parallel, aligned, in sequence or
  merging only when the drawn lines make them so; when a layout is hard to follow, list the
  parts and the lines you are sure of and stop there.
- Use the figure's own terms and symbols from the caption and the text as they are printed
  (T*, not "T star"); write no LaTeX.
- When a geometry of the figure follows, trust it over the picture for counts and labels: it is
  read exactly from the figure file. Text marked "read by OCR" is different: a program read it
  from the picture and may miss or misread words (a lost minus sign, a skipped tick label); use
  it for the wording of small print and check its numbers against the picture. Use the picture
  for the overall layout.
- Name each label by its printed text (the geometry's "printed" text, or what the picture shows),
  never by its LaTeX source or macro names.
- Never put the geometry's coordinates, radii, pen widths or opacities in the text; values the
  figure prints (tick labels, numbers on it) are fine. Say where things are in words ("upper
  left", "inside", "between").
- In the geometry x grows to the right and y grows upward. State an orientation or order
  (clockwise, above, left of) only when you are certain of it; leave out what you cannot tell,
  without listing it.
- A tiny decoration (a bullet, a logo, an ornament) gets a few words and no description.
- End every part with a complete sentence. Print only the text, with no preface, heading (other
  than "Details:"), list markers or quotation marks.
`
