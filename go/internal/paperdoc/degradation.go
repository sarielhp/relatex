package paperdoc

import (
	"fmt"
	"strings"

	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/rec"
)

// place is row.slice('table', 'page', 'block') as a diagnostic location.
func place(row rec.Map) []diagnostics.Place {
	places := []diagnostics.Place{}
	for _, key := range []string{"table", "page", "block"} {
		if value, ok := row[key]; ok {
			places = append(places, diagnostics.Place{Key: key, Value: value})
		}
	}
	return places
}

var fixedMessages = map[string]string{
	"interleaved_reading_paragraph":     "a paragraph continues after material printed inside it",
	"incomplete_reading_paragraph":      "the end of a paragraph was not observed and is missing",
	"reference_inside_math_unsupported": "a reference inside a formula prints its number, unlinked",
	"footnote_call_number_mismatch":     "a footnote mark differs from its recorded number; the printed mark is kept",
	"footnote_body_number_mismatch":     "a footnote mark differs from its recorded number; the printed mark is kept",
	"footnote_body_without_call":        "a footnote has no call in the text; its body is kept",
	"footnote_title_note":               "an author note's mark is printed by the class in the author line, unlinked; its body is kept",
	"unresolved_footnote_call":          "a footnote call has no footnote text; its mark prints unlinked",
	"unread_math_nodes":                 "formula nodes outside the reading flow print from the formula's MathML",
	"citation_inside_math":              "citations inside a formula print in it, unlinked",
	"unsupported_qed_definition":        "an end-of-proof mark other than the default prints as its own content, untyped",
	"unsupported_graphic_transform":     "a graphic scaled or rotated as a group is not transformed; it prints as its own content",
	"unsupported_qed_context":           "an end-of-proof mark other than the default prints as its own content, untyped",
}

var placedMessages = map[string]string{
	"unmapped_reading_glyph":    "a glyph without a Unicode mapping prints as U+FFFD",
	"unsupported_reading_node":  "a painted rule is omitted",
	"unclassified_running_rule": "a painted rule is omitted",
	"table_inside_formula":      "a table set inside a formula reads as part of its MathML, not as a grid",
	"clamped_multirow":          `a \multirow reaching past the last row spans the rows that exist`,
	"upward_multirow":           `an upward \multirow prints in its own row, not across the rows above`,
}

// degradation is the message and location of a degraded reading diagnostic.
func degradation(row rec.Map) (string, []diagnostics.Place) {
	code := rec.FetchString(row, "code")
	if message, ok := fixedMessages[code]; ok {
		return message, nil
	}
	if name, named := row["name"].(string); named && code == "unmapped_reading_glyph" {
		return fmt.Sprintf("the glyph %s has no Unicode character; it prints as U+FFFD, named %q for screen readers",
			name, inline.GlyphLabel(name)), place(row)
	}
	if message, ok := placedMessages[code]; ok {
		return message, place(row)
	}
	label := func() []diagnostics.Place { return []diagnostics.Place{{Key: "label", Value: rec.Fetch(row, "label")}} }
	switch code {
	case "unresolved_reference_use":
		return fmt.Sprintf("reference to %s prints as unlinked text", toS(rec.Fetch(row, "target"))),
			[]diagnostics.Place{{Key: "reference", Value: rec.Fetch(row, "reference")}}
	case "unverified_page_reference":
		pages := []string{}
		for _, page := range rec.FetchList(row, "pages") {
			pages = append(pages, toS(page))
		}
		where := "an unknown page"
		if len(pages) > 0 {
			where = "page " + strings.Join(pages, ", ")
		}
		return fmt.Sprintf("page reference to %s prints %q, but its target is set on %s; it prints as unlinked text",
				toS(rec.Fetch(row, "target")), toS(rec.Fetch(row, "printed")), where),
			[]diagnostics.Place{{Key: "reference", Value: rec.Fetch(row, "reference")}}
	case "unsupported_table_layout":
		reason, ok := row["reason"]
		if !ok {
			reason = "unsupported layout"
		}
		return fmt.Sprintf("table read as plain text, without its rules (%s)", toS(reason)), place(row)
	case "unsupported_graphic_source":
		return fmt.Sprintf("a picture the converter cannot read is not reproduced (%s)", toS(rec.Fetch(row, "reason"))), place(row)
	case "unsupported_external_alignment":
		return fmt.Sprintf("a row of %s reads with its formula but not as a separate row", toS(rec.Fetch(row, "environment"))), nil
	case "unsupported_list_context":
		return fmt.Sprintf("a list inside %s reads without its list structure", toS(rec.Fetch(row, "environment"))), nil
	case "unclosed_structural_context":
		contexts := []string{}
		for _, context := range rec.FetchList(row, "contexts") {
			contexts = append(contexts, toS(context))
		}
		return strings.Join(contexts, ", ") + " never closed; what follows may sit inside it", nil
	case "unsupported_external_link":
		return fmt.Sprintf("a link to %s prints as plain text (%s)", toS(rec.Fetch(row, "href")), toS(rec.Fetch(row, "reason"))), nil
	case "multiply_defined_label":
		return fmt.Sprintf("label %s is defined twice; references use the last, as LaTeX does", toS(rec.Fetch(row, "label"))), label()
	}
	return fmt.Sprintf("label %s has no verified numbered element", toS(rec.Fetch(row, "label"))), label()
}
