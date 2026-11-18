package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func joinSpans(parts ...any) []any {
	content := []any{}
	for _, part := range parts {
		if text, ok := part.(string); ok {
			content = append(content, rec.Map{"type": "text", "text": text})
		} else {
			content = append(content, rec.Map{"type": "math"})
		}
	}
	return content
}

// joinedHTML renders the spans as the paragraph loop does: a text piece is <t>…</t>, a formula <m/>.
func joinedHTML(content []any) string {
	var joins joinRun
	var out strings.Builder
	for index, value := range content {
		span := rec.AsMap(value)
		html, split := joins.text(content, index, func(piece string, cont bool) string {
			if cont {
				return "<c>" + piece + "</c>"
			}
			return "<t>" + piece + "</t>"
		})
		switch {
		case span["type"] == "math":
			html = joins.formula(content, index, "<m/>")
		case !split:
			html = "<t>" + rec.FetchString(span, "text") + "</t>"
		}
		out.WriteString(html)
	}
	return out.String()
}

func TestJoinsKeepPunctuationWithItsFormula(t *testing.T) {
	const open = "<span data-join>"
	cases := []struct {
		name    string
		content []any
		want    string
	}{
		{"period after a formula", joinSpans("of ", 1, ". However, we"),
			"<t>of </t>" + open + "<m/><t>.</t></span><c> However, we</c>"},
		{"comma alone", joinSpans(1, ","), open + "<m/><t>,</t></span>"},
		{"space after a formula", joinSpans("of ", 1, " However"), "<t>of </t><m/><t> However</t>"},
		{"opening bracket before a formula", joinSpans("see (", 1, ") now"),
			"<t>see </t>" + open + "<c>(</c><m/><t>)</t></span><c> now</c>"},
		{"opening quote before a formula", joinSpans("say “", 1, "”."),
			"<t>say </t>" + open + "<c>“</c><m/><t>”.</t></span>"},
		{"word before a formula", joinSpans("of ", 1), "<t>of </t><m/>"},
		{"formula between two joined marks", joinSpans("(", 1, ")"), open + "<t>(</t><m/><t>)</t></span>"},
		{"two formulas in a row", joinSpans(1, ")", 1, "."),
			open + "<m/><t>)</t></span>" + open + "<m/><t>.</t></span>"},
		{"no formula", joinSpans("a.", "b"), "<t>a.</t><t>b</t>"},
	}
	for _, test := range cases {
		if got := joinedHTML(test.content); got != test.want {
			t.Errorf("%s:\n got %s\nwant %s", test.name, got, test.want)
		}
	}
}

// A span a link finds by its identity (a descriptor's block and position, not a key of the span:
// papers2/0412's footnote mailto) is never cut: the cut copy has another identity and loses the link.
func TestJoinsLeaveIdentityKeptSpansWhole(t *testing.T) {
	linked := rec.Map{"type": "text", "text": ". Mail me: a@example.com"}
	content := []any{rec.Map{"type": "math"}, linked}
	joins := joinRun{kept: func(span rec.Map) bool { return rec.Identity(span) == rec.Identity(linked) }}
	html, split := joins.text(content, 1, func(piece string, _ bool) string { return piece })
	if split || html != "" {
		t.Errorf("an identity-kept span was cut: %q", html)
	}
	if !startsJoined(content, 1, nil) {
		t.Errorf("without the predicate the span joins its formula")
	}
}

func TestJoinsLeaveSpansWithLinksWhole(t *testing.T) {
	content := []any{rec.Map{"type": "math"}, rec.Map{"type": "text", "text": ". See", "external_link": "l1"}}
	if got := joinedHTML(content); strings.Contains(got, "data-join") {
		t.Errorf("a linked span was cut: %s", got)
	}
}
