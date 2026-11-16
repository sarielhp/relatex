package alttext

import (
	"fmt"
	"html"
	"strings"

	"github.com/sarielhp/relatex/internal/figdesc"
)

// ReviewName is the review page, written in the output directory beside index.html (it is a
// tool's page, not part of the export, and the next conversion replaces it).
const ReviewName = "alt-review.html"

// Status is a figure's state in review.
type Status struct {
	Key, Label, Class string
	Entry             *figdesc.Entry
}

// States are the figures' review states, in reading order.
func States(figures []*figdesc.Figure, sidecar *figdesc.Sidecar) []Status {
	states := make([]Status, 0, len(figures))
	for _, figure := range figures {
		entry := sidecar.Entries[figure.Key]
		identity := figure.Identity()
		state := Status{Key: figure.Key, Entry: entry}
		switch {
		case figure.FromSource():
			state.Label, state.Class = "described in the LaTeX source (alt, artifact or \\Description)", "done"
		case figure.Derived():
			state.Label, state.Class = "described from what the LaTeX source draws (a chess position; an entry of yours in relatex/alt.toml replaces it)", "done"
		case figure.FromMacro():
			state.Label, state.Class = "described in the LaTeX source (\\relatexalt; an entry of yours in relatex/alt.toml replaces it)", "done"
		case entry != nil && entry.Decorative:
			state.Label, state.Class = "decorative", "done"
		case entry == nil || figdesc.Normalize(entry.Alt) == "":
			state.Label, state.Class = "no description", "missing"
		case entry.SHA256 != "" && entry.SHA256 != identity && entry.Provenance == figdesc.Model:
			state.Label, state.Class = "draft of an earlier version of the figure (not used; draft again)", "stale"
		case entry.SHA256 != "" && entry.SHA256 != identity:
			state.Label, state.Class = "written for an earlier version of the figure: check it", "stale"
		case entry.Provenance == figdesc.Model:
			state.Label, state.Class = "draft, not reviewed", "draft"
		case entry.Provenance == figdesc.Reviewed:
			state.Label, state.Class = "reviewed", "done"
		default:
			state.Label, state.Class = "your text", "done"
		}
		states = append(states, state)
	}
	return states
}

// ReviewPage is the review page: each figure with its picture, caption, the text that cites it
// and its description, with what to do. Every figure the sidecar can describe has its description
// in an editable field and an accept checkbox, and a button saves the ticked ones as a review file
// (ReviewFile) for relatex alt import (decision 112: a bulk review counts as review). The page is a
// local tool, not part of the export: its script only builds that file; read without scripts, it
// still shows everything, and the sidecar and relatex alt accept work as before.
func ReviewPage(figures []*figdesc.Figure, sidecar *figdesc.Sidecar) string {
	states := States(figures, sidecar)
	counts := map[string]int{}
	for _, state := range states {
		counts[state.Class]++
	}
	var out strings.Builder
	title := "Figure descriptions to review"
	out.WriteString("<!DOCTYPE html>\n<html lang=\"en\"><head><meta charset=\"utf-8\">" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>" + title + "</title><style>" +
		reviewStyle + "</style></head><body><main><h1>" + title + "</h1>\n")
	fmt.Fprintf(&out, "<p>%d figures: %d drafts to review, %d without a description, %d to check against a changed figure, "+
		"%d done. Descriptions live in <code>%s</code> in the paper directory.</p>\n", len(states), counts["draft"],
		counts["missing"], counts["stale"], counts["done"], figdesc.FileName)
	writeJumpList(&out, states, counts)
	out.WriteString(reviewHelp)
	out.WriteString(reviewControls)
	shared := sharedPictures(figures, states)
	for index, figure := range figures {
		writeFigure(&out, figure, states[index], shared[index])
	}
	out.WriteString(reviewControls)
	out.WriteString("</main><script>" + reviewScript + "</script></body></html>\n")
	return out.String()
}

// writeJumpList is the page's top: the count of each state and a link to its first figure. It jumps,
// it does not filter (a filter would need a script).
func writeJumpList(out *strings.Builder, states []Status, counts map[string]int) {
	first := map[string]string{}
	for _, state := range states {
		if _, seen := first[state.Class]; !seen {
			first[state.Class] = state.Key
		}
	}
	out.WriteString("<nav class=\"jump\"><p><strong>Jump to the first figure in each state:</strong> ")
	links := []string{}
	for _, group := range []struct{ class, text string }{
		{"draft", "drafts to review"}, {"missing", "without a description"},
		{"stale", "to check against a changed figure"}, {"done", "done"},
	} {
		if counts[group.class] > 0 {
			links = append(links, fmt.Sprintf("<a href=\"#fig-%s\">%d %s</a>",
				html.EscapeString(first[group.class]), counts[group.class], group.text))
		}
	}
	out.WriteString(strings.Join(links, " · ") + "</p></nav>\n")
}

// sharedPicture says that a figure is another name for a picture shown above: Same is that key and
// Keys the names one decision on its form covers (import takes a decision per key).
type sharedPicture struct {
	Same string
	Keys []string
}

// sharedPictures gives one decision form to the pictures included under several names: the later
// names point at it, and its form writes a decision for each of them.
func sharedPictures(figures []*figdesc.Figure, states []Status) []sharedPicture {
	pictures := make([]sharedPicture, len(figures))
	first := map[string]int{}
	for index, figure := range figures {
		identity := figure.Identity()
		if identity == "" || !editable(figure, states[index]) {
			continue
		}
		previous, seen := first[identity]
		if !seen {
			first[identity] = index
			pictures[index].Keys = []string{figure.Key}
			continue
		}
		if shown(figure, states[index]) != shown(figures[previous], states[previous]) || states[index].Class != states[previous].Class {
			continue // another name shows another text or is in another state: it keeps its own form
		}
		pictures[index].Same = figures[previous].Key
		pictures[previous].Keys = append(pictures[previous].Keys, figure.Key)
	}
	return pictures
}

// shown is the description the page shows for a figure, from the sidecar or the LaTeX source.
func shown(figure *figdesc.Figure, state Status) Description {
	if figure.FromSource() || figure.FromMacro() {
		return Description{Alt: figure.Alt(), Long: figure.Long(), Details: figure.Details()}
	}
	if state.Entry == nil {
		return Description{}
	}
	return Description{Alt: figdesc.Normalize(state.Entry.Alt), Long: figdesc.Normalize(state.Entry.Long),
		Details: figdesc.Normalize(state.Entry.Details)}
}

func writeFigure(out *strings.Builder, figure *figdesc.Figure, state Status, shared sharedPicture) {
	key := html.EscapeString(figure.Key)
	text := shown(figure, state)
	fmt.Fprintf(out, "<section class=\"figure %s\" id=\"fig-%s\"><h2>%s <span class=\"badge\">%s</span></h2>\n",
		state.Class, key, key, html.EscapeString(state.Label))
	fmt.Fprintf(out, "<img src=\"%s\" alt=\"%s\" loading=\"lazy\" decoding=\"async\">\n", html.EscapeString(figure.File), html.EscapeString(text.Alt))
	out.WriteString("<dl>")
	writeTerm(out, "Caption", figure.Caption)
	writeTerm(out, "Cited in", figure.Context)
	writeTerm(out, "Description", text.Alt)
	// A short description (decisions 110, 125), shown when there is one or the figure may need one,
	// and its details when there are some.
	complexity := figure.Complexity()
	if text.Long != "" || complexity.Complex() {
		writeTerm(out, "Short description", text.Long)
	}
	if text.Details != "" {
		writeTerm(out, "Details", text.Details)
	}
	if state.Entry != nil && state.Entry.Model != "" {
		writeTerm(out, "Drafted by", state.Entry.Model+" on "+state.Entry.Drafted+restedOn(state.Entry.RestedOn))
	}
	out.WriteString("</dl>\n")
	switch {
	case shared.Same != "":
		fmt.Fprintf(out, "<p class=\"same\">Same picture as <a href=\"#fig-%s\">%s</a>: one decision there covers this figure too.</p>\n",
			html.EscapeString(shared.Same), html.EscapeString(shared.Same))
	case editable(figure, state):
		writeDecision(out, figure, text, shared.Keys)
	}
	if text.Long == "" && complexity.Complex() && state.Class != "missing" {
		fmt.Fprintf(out, "<p class=\"do\">This figure looks complex (%s): consider a short description, "+
			"<code>relatex alt set %s --long \"…\"</code> (or <code>\\Description[short]{long}</code> in an acmart figure)</p>\n",
			html.EscapeString(complexity.String()), key)
	}
	switch state.Class {
	case "draft":
		fmt.Fprintf(out, "<p class=\"do\">Correct the text under <code>[figs.%s]</code> if needed, then accept it: "+
			"<code>relatex alt accept %s</code></p>\n", key, key)
	case "missing":
		fmt.Fprintf(out, "<p class=\"do\">Write <code>[figs.%s]</code> with <code>alt = \"…\"</code>, or draft one: "+
			"<code>relatex alt draft</code></p>\n", key)
	case "stale":
		fmt.Fprintf(out, "<p class=\"do\">The figure changed since this text was written. Update it (and its "+
			"<code>sha256</code> to <code>%s</code>), or delete the entry and draft again.</p>\n", figure.Identity())
	}
	out.WriteString("</section>\n")
}

// restedOn says what a draft was made from (decision 123), for the "Drafted by" line; nothing for a
// draft from before it was recorded.
func restedOn(source string) string {
	labels := map[string]string{RestedIpe: "the figure's Ipe source", RestedTrace: "a trace of the figure's PDF",
		RestedBitmap: "the picture (the PDF holds one bitmap)",
		RestedOCR:    "the picture and the text OCR read in it", RestedImage: "the picture only"}
	if label, ok := labels[source]; ok {
		return ", from " + label + " (rested_on = " + source + ")"
	}
	return ""
}

func writeTerm(out *strings.Builder, term, text string) {
	if strings.TrimSpace(text) == "" {
		text = "(none)"
	}
	out.WriteString("<dt>" + term + "</dt><dd>" + html.EscapeString(text) + "</dd>")
}

const reviewHelp = `<details><summary>How to review</summary><p>Read each draft against the figure, its caption
and the text that cites it. A good description says what a reader who cannot see the figure needs
in order to follow the argument, in words that can be read aloud, without repeating the caption.
On this page: correct the text in the field where it is wrong and tick <em>Accept</em> (editing ticks it
for you), then <em>Save the review file</em> and run <code>relatex alt import ` + ReviewFile + `</code>
(with the path your browser saved it to) in the paper directory. A draft you accepted as shown becomes
reviewed, a text you edited or wrote becomes yours; figures you did not tick stay as they are. Without
the page: correct the <code>alt</code> text in the file, then run <code>relatex alt accept NAME</code>
(or <code>--all</code> once you have read them all). Either way relatex never replaces your text again.
Convert again to carry the descriptions into the page, the EPUB and the tagged PDF.</p></details>
`

const reviewStyle = `:root{color-scheme:light dark;--fg:#1b1b1b;--bg:#fff;--muted:#555;--line:#ccc;--draft:#8a4b00;--done:#1d6b2f;--stale:#a01d1d}
@media (prefers-color-scheme:dark){:root{--fg:#e8e8e8;--bg:#161616;--muted:#aaa;--line:#444;--draft:#f0b45c;--done:#7fd18f;--stale:#ff8a8a}}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.5 system-ui,sans-serif}
main{max-width:60rem;margin:0 auto;padding:1rem}
section.figure{border-top:1px solid var(--line);padding:1rem 0}
img{max-width:100%;max-height:24rem;background:#fff;border:1px solid var(--line)}
dt{font-weight:600;margin-top:.5rem}dd{margin:0 0 0 1rem;color:var(--muted)}
.badge{font-size:.8rem;font-weight:600;border:1px solid currentColor;border-radius:.3rem;padding:0 .4rem}
.draft .badge{color:var(--draft)}.done .badge{color:var(--done)}.stale .badge,.missing .badge{color:var(--stale)}
code{font-size:.9em}.do{font-weight:500}
fieldset{border:1px solid var(--line);margin:.75rem 0}fieldset label{display:block;margin-top:.4rem}fieldset p label{display:inline}
textarea{width:100%;box-sizing:border-box;font:inherit;color:var(--fg);background:var(--bg);border:1px solid var(--line)}
.controls{position:sticky;top:0;background:var(--bg);padding:.5rem 0;border-bottom:1px solid var(--line)}
.jump a{margin-right:.75rem}.same{font-weight:500}
button{font:inherit}:focus-visible{outline:2px solid currentColor;outline-offset:2px}`
