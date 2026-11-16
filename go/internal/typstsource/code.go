package typstsource

import (
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// code is a listing (decision 120): a raw block of its lines' printed text, one per line, after
// the anchors of labels on its lines. A line holding anything but text (escaped math, a link) is
// not raw text: the listing is then written as its lines.
func (e *exporter) code(node rec.Map) string {
	lines, anchors := []string{}, []string{}
	for _, id := range children(node) {
		line := e.node(id)
		if toS(line["type"]) != "paragraph" {
			return e.children(node)
		}
		var text strings.Builder
		for _, span := range content(line) {
			switch span["type"] {
			case "text":
				text.WriteString(rec.FetchString(span, "text"))
			case "rule":
			default:
				return e.children(node)
			}
		}
		lines = append(lines, strings.TrimRight(text.String(), " "))
	}
	for _, id := range children(node) {
		if anchor := e.anchors(e.node(id)); anchor != "" {
			anchors = append(anchors, anchor)
		}
	}
	return joinPresent(" ", strings.Join(anchors, " "), "#raw(block: true, "+typstString(strings.Join(lines, "\n"))+")")
}
