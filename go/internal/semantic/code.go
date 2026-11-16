package semantic

import (
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// codeText is a listing's text (decision 120): its lines' printed text, one per line, and whether
// every span of them is text a raw block can hold. A line with a formula, an image or a link
// (a listing with escaped math) is not, and the listing is then written as its lines.
func (e *export) codeText(node rec.Map) (string, bool) {
	lines := []string{}
	for _, child := range rec.FetchList(node, "children") {
		line := e.node(child)
		if rec.FetchString(line, "type") != "paragraph" {
			return "", false
		}
		var text strings.Builder
		for _, value := range rec.FetchList(line, "content") {
			span := rec.AsMap(value)
			switch span["type"] {
			case "text":
				text.WriteString(spokenText(span))
			case "rule":
			default:
				return "", false
			}
		}
		lines = append(lines, strings.TrimRight(text.String(), " "))
	}
	return strings.Join(lines, "\n"), true
}

// fence is a run of backticks longer than any in text (CommonMark: a code fence closes only at
// a run at least as long).
func fence(text string) string {
	longest, run := 0, 0
	for _, char := range text {
		if char != '`' {
			run = 0
			continue
		}
		run++
		longest = max(longest, run)
	}
	return strings.Repeat("`", max(3, longest+1))
}

// code is a listing as a fenced code block after the listing's anchor.
func (m *markdown) code(node rec.Map, text string) string {
	marks := fence(text)
	return "<a id=\"" + toS(rec.Fetch(node, "id")) + "\"></a>\n\n" + marks + "\n" + text + "\n" + marks
}

// code is a listing as a raw block with the listing's label.
func (t *typst) code(node rec.Map, text string) string {
	return "#raw(block: true, " + quote(text) + ") " + label(rec.Fetch(node, "id"))
}
