package a11yreport

import (
	"fmt"
	"html"
	"strings"

	"github.com/sarielhp/relatex/internal/a11ylint"
	"github.com/sarielhp/relatex/internal/pdfua"
	"github.com/sarielhp/relatex/internal/rec"
)

// checkTitles name relatex's structural checks (accessibility.json's structural rows) for a reader,
// in the order the page lists them.
var checkTitles = []struct{ id, title string }{
	{"language_declared", "Document language declared"},
	{"language_parts", "Passages in other languages marked"},
	{"image_alternatives", "Figures described"},
	{"image_drafts_reviewed", "Model-drafted descriptions reviewed"},
	{"formula_alternatives", "Formulas with a text alternative"},
	{"table_headers", "Data tables with header cells"},
	{"heading_levels", "Heading levels continuous"},
	{"images_of_text", "No text as a picture"},
	{"landmarks", "Page landmarks"},
	{"named_groups", "Theorems and proofs as named groups"},
	{"focus_visible", "Visible focus"},
	{"reduced_motion", "Reduced motion"},
	{"forced_colors", "Forced colours"},
	{"non_text_contrast", "Contrast of rules and text colours"},
	{"use_of_color", "Colour not the only signal"},
}

// checkActions say what closes a structural gap; a check missing here is relatex's own to close.
var checkActions = map[string]string{
	"language_declared":     "Declare the language: \\usepackage[english]{babel} (or \\DocumentMetadata{lang=en}); the source items below give the line.",
	"language_parts":        "Mark each passage in another language with \\foreignlanguage{LANGUAGE}{…}, a language babel knows.",
	"image_alternatives":    "Describe each figure: alt={…} on its \\includegraphics, or relatex/alt.toml (relatex alt draft, then relatex alt review).",
	"image_drafts_reviewed": "Review the model drafts: relatex alt review, accept or correct each one on the page, then relatex alt import FILE.",
	"table_headers":         "Mark each table's header row: a rule under it (\\midrule, \\hline), or say it in relatex/hints.json ({\"tables\": {\"tab:label\": {\"header_rows\": 1}}}).",
	"images_of_text":        "Set the text as text (prose, a table or a formula) instead of a picture, or say it in the caption.",
	"use_of_color":          "Say in words, markers or patterns what each colour says (WCAG 1.4.1); or, when the figure already does, attest the caption in relatex/hints.json (colour_captions; the lint gives the line).",
}

// taggedActions say what closes a tagged-PDF finding; a finding missing here is relatex's own.
var taggedActions = map[string]string{
	"pdfua.undescribed":     "Describe the figures (as for the web page); the tagged PDF uses the same descriptions.",
	"pdfua.figure_font":     "Re-export the figure named in the item with its font fully embedded as a single font (the item says what veraPDF rejects).",
	"pdfua.font_unwrapped":  "Re-export the figure named in the item with the font embedded as a single font; the tagged PDF already embeds the one face it names, so a reader shows the same glyphs.",
	"pdfua.font_cmap_added": "Re-export the figure named in the item with a Windows or Macintosh Roman cmap in its font; the tagged PDF's copy of the program already names the Unicode subtable as one, so a reader shows the same glyphs.",
	"pdfua.unverified":      "Install veraPDF (tools.verapdf in the configuration, RELATEX_VERAPDF or verapdf on PATH) and convert again.",
	"pdfua.invalid":         "veraPDF names the failed rules; most are relatex's to fix, a missing description is yours.",
}

const ownGap = "relatex's own to close: nothing to change in your source (report it with the document if it stays)."

func checkTitle(id string) string {
	for _, check := range checkTitles {
		if check.id == id {
			return check.title
		}
	}
	return strings.ReplaceAll(id, "_", " ")
}

// Page is accessibility.html.
func Page(in Inputs) string {
	var out strings.Builder
	title := "Accessibility report"
	if in.Title != "" {
		title += ": " + in.Title
	}
	out.WriteString("<!DOCTYPE html>\n<html lang=\"en\"><head><meta charset=\"utf-8\">" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>" + html.EscapeString(title) +
		"</title><style>" + pageStyle + "</style></head><body><main>\n<h1>" + html.EscapeString(title) + "</h1>\n")
	fmt.Fprintf(&out, "<p class=\"meta\">Written by relatex %s on %s from <code>%s</code>. The claims below are made only when "+
		"every automatic check passes; a person reading the outputs with a screen reader remains the final test.</p>\n",
		html.EscapeString(in.Version), html.EscapeString(in.Date), html.EscapeString(in.Main))
	writeClaims(&out, Claims(in))
	writeOpenItems(&out, in)
	writeAttested(&out, in)
	writeSourceItems(&out, in)
	writeProvenance(&out, ProvenanceRows(in))
	out.WriteString("<h2>Files</h2>\n<p><code>accessibility.json</code> holds the web page's measurements (axe-core in both " +
		"colour schemes and relatex's structural checks); <code>pdf-ua/export.json</code> holds the tagged PDF's (veraPDF, " +
		"relatex's structure and fidelity checks). This page is written from them and from the source.</p>\n")
	out.WriteString("</main></body></html>\n")
	return out.String()
}

func writeClaims(out *strings.Builder, claims []Claim) {
	out.WriteString("<h2>Claims</h2>\n<table><caption>The conformance each output reaches</caption>\n" +
		"<thead><tr><th scope=\"col\">Output</th><th scope=\"col\">Standard</th><th scope=\"col\">Result</th></tr></thead><tbody>\n")
	for _, claim := range claims {
		result := "<strong class=\"pass\">Claimed: " + html.EscapeString(claim.Reached) + "</strong>"
		if claim.Reached == "" {
			result = "<strong class=\"open\">Not claimed</strong><ul>"
			for _, reason := range claim.Reasons {
				result += "<li>" + html.EscapeString(reason) + "</li>"
			}
			result += "</ul>"
		}
		output := html.EscapeString(claim.Output)
		if claim.File != "" {
			output += " <span class=\"file\">(" + html.EscapeString(claim.File) + ")</span>"
		}
		fmt.Fprintf(out, "<tr><th scope=\"row\">%s</th><td>%s</td><td>%s</td></tr>\n", output, html.EscapeString(claim.Target), result)
	}
	out.WriteString("</tbody></table>\n")
}

// writeOpenItems lists the measured outputs' open items with what to do.
func writeOpenItems(out *strings.Builder, in Inputs) {
	out.WriteString("<h2>Open items in the outputs</h2>\n")
	rows := []string{}
	for _, value := range listOf(in.Web, "open_items") {
		item := rec.AsMap(value)
		count, _ := rec.Number(item["count"])
		action := ownGap
		if known, ok := checkActions[rec.ToS(item["id"])]; ok && item["source"] != "axe" {
			action = known
		}
		rows = append(rows, itemRow("Web page, EPUB", itemName(item), fmt.Sprint(int64(count)), action))
	}
	for _, found := range pdfua.Diagnostics(in.Tagged) {
		action := ownGap
		if known, ok := taggedActions[found.Code]; ok {
			action = known
		}
		rows = append(rows, itemRow("Tagged PDF", found.Code+": "+found.Message, fmt.Sprint(found.Magnitude), action))
	}
	if len(rows) == 0 {
		out.WriteString("<p>None.</p>\n")
		return
	}
	out.WriteString("<table><caption>What the checks found, and what closes it</caption>\n<thead><tr><th scope=\"col\">Output</th>" +
		"<th scope=\"col\">Item</th><th scope=\"col\">Count</th><th scope=\"col\">What to do</th></tr></thead><tbody>\n")
	out.WriteString(strings.Join(rows, ""))
	out.WriteString("</tbody></table>\n")
}

func itemRow(output, item, count, action string) string {
	return "<tr><td>" + html.EscapeString(output) + "</td><td>" + html.EscapeString(item) + "</td><td class=\"number\">" +
		html.EscapeString(count) + "</td><td>" + html.EscapeString(action) + "</td></tr>\n"
}

// writeSourceItems lists the lint's items, each at its line, grouped by kind.
func writeSourceItems(out *strings.Builder, in Inputs) {
	out.WriteString("<h2>Items in your source</h2>\n")
	if in.LintNote != "" {
		out.WriteString("<p>" + html.EscapeString(in.LintNote) + "</p>\n")
		return
	}
	if len(in.Lint) == 0 {
		out.WriteString("<p>The accessibility lint found nothing for you to supply.</p>\n")
		return
	}
	fmt.Fprintf(out, "<p>%d items from <code>relatex lint --accessibility</code>: only you can supply them. Each names the file "+
		"and line, and the exact text to add where relatex can say it. <code>relatex lint --accessibility --annotate DIR</code> "+
		"writes a copy of the source with each place marked.</p>\n", len(in.Lint))
	if in.LintSummary != "" {
		out.WriteString("<p><strong>" + html.EscapeString(in.LintSummary) + "</strong></p>\n")
	}
	kinds, byKind := groupItems(in.Lint)
	for _, kind := range kinds {
		fmt.Fprintf(out, "<h3>%s (%d)</h3>\n<ul class=\"items\">\n", html.EscapeString(kindTitle(kind)), len(byKind[kind]))
		for _, item := range byKind[kind] {
			writeLintItem(out, item)
		}
		out.WriteString("</ul>\n")
	}
}

func groupItems(items []a11ylint.Item) ([]string, map[string][]a11ylint.Item) {
	kinds := []string{}
	byKind := map[string][]a11ylint.Item{}
	for _, item := range items {
		if _, seen := byKind[item.Kind]; !seen {
			kinds = append(kinds, item.Kind)
		}
		byKind[item.Kind] = append(byKind[item.Kind], item)
	}
	return kinds, byKind
}

func writeLintItem(out *strings.Builder, item a11ylint.Item) {
	location := item.File
	if item.Line > 0 {
		location += fmt.Sprintf(":%d", item.Line)
	}
	fmt.Fprintf(out, "<li><code class=\"location\">%s</code> %s<br><span class=\"fix\">Fix: %s</span>", html.EscapeString(location),
		html.EscapeString(item.Message), html.EscapeString(item.Fix))
	if item.Suggestion != "" {
		fmt.Fprintf(out, "<br><span class=\"fix\">Add at line %d:</span> <code>%s</code>", max(item.NoteLine, 1),
			html.EscapeString(item.Suggestion))
	}
	out.WriteString("</li>\n")
}

var kindTitles = map[string]string{"figure": "Figures without a description", "draft": "Unreviewed model drafts",
	"drawing": "Drawings without a description", "language": "Language", "table": "Tables without a marked header row",
	"link": "Link texts", "colour": "Colour as the only signal", "math": "Ambiguous notation", "long": "Complex figures (advice)"}

func kindTitle(kind string) string {
	if title, ok := kindTitles[kind]; ok {
		return title
	}
	return kind
}

func writeProvenance(out *strings.Builder, rows []Provenance) {
	out.WriteString("<h2>Where the descriptions came from</h2>\n<table><caption>Counts by origin: your source, your " +
		"relatex/alt.toml entries, model drafts you reviewed, unreviewed drafts, what relatex inferred or detected, and none" +
		"</caption>\n<thead><tr><th scope=\"col\">What</th>")
	for _, origin := range Origins {
		out.WriteString("<th scope=\"col\">" + origin + "</th>")
	}
	out.WriteString("</tr></thead><tbody>\n")
	for _, row := range rows {
		out.WriteString("<tr><th scope=\"row\">" + html.EscapeString(row.Subject) + "</th>")
		for _, origin := range Origins {
			fmt.Fprintf(out, "<td class=\"number\">%d</td>", row.Counts[origin])
		}
		out.WriteString("</tr>\n")
	}
	out.WriteString("</tbody></table>\n<p>Only the source, your entries and the drafts you reviewed count as yours; an " +
		"unreviewed draft keeps the web page and the EPUB from their claim.</p>\n")
}

// pageStyle: system fonts, contrast above 7:1 for text in both schemes, no script.
const pageStyle = `:root{color-scheme:light dark;--fg:#1b1b1b;--bg:#fff;--muted:#4a4a4a;--line:#767676;--pass:#1d6b2f;--open:#a01d1d}
@media (prefers-color-scheme:dark){:root{--fg:#ececec;--bg:#141414;--muted:#c4c4c4;--line:#8a8a8a;--pass:#8fe0a0;--open:#ffa8a8}}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.5 system-ui,sans-serif}
main{max-width:64rem;margin:0 auto;padding:1rem}
table{border-collapse:collapse;width:100%;margin:1rem 0}caption{text-align:left;font-weight:600;padding:.25rem 0}
th,td{border:1px solid var(--line);padding:.35rem .5rem;text-align:left;vertical-align:top}
td.number{text-align:right;font-variant-numeric:tabular-nums}
.pass{color:var(--pass)}.open{color:var(--open)}.meta,.file,.fix{color:var(--muted)}
ul.items li{margin:.4rem 0}code{font-size:.9em;overflow-wrap:anywhere}
a:focus-visible{outline:2px solid currentColor;outline-offset:2px}`
