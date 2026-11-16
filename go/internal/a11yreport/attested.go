package a11yreport

import (
	"fmt"
	"html"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// attestedCaptions are the captions naming colours that the author attested in relatex/hints.json
// (decision 188), from the use_of_color row of accessibility.json.
func attestedCaptions(in Inputs) []rec.Map {
	captions := []rec.Map{}
	for _, value := range listOf(in.Web, "structural") {
		row := rec.AsMap(value)
		if row["check"] != "use_of_color" {
			continue
		}
		for _, entry := range listOf(row, "attested") {
			captions = append(captions, rec.AsMap(entry))
		}
	}
	return captions
}

// writeAttested says which captions that name colours the claim rests on the author's word for:
// they are listed, not gaps, and an edit to the caption voids the attestation.
func writeAttested(out *strings.Builder, in Inputs) {
	captions := attestedCaptions(in)
	if len(captions) == 0 {
		return
	}
	out.WriteString("<h2>Captions attested by the author</h2>\n<p>These captions name colours. The author attested in " +
		"<code>relatex/hints.json</code> (<code>colour_captions</code>) that each figure also says it by other means " +
		"(line styles, markers, labels), so the check lists them and counts no gap; relatex did not verify it. Changing a " +
		"caption's text voids its attestation.</p>\n<ul class=\"items\">\n")
	for _, caption := range captions {
		where := ""
		if file := rec.ToS(caption["file"]); file != "" {
			where = fmt.Sprintf(" <code class=\"location\">%s:%v</code>", html.EscapeString(file), caption["line"])
		}
		fmt.Fprintf(out, "<li><code>%s</code>%s %s</li>\n", html.EscapeString(rec.ToS(caption["key"])), where,
			html.EscapeString(rec.ToS(caption["text"])))
	}
	out.WriteString("</ul>\n")
}
