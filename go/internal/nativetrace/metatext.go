package nativetrace

import (
	"bytes"
	"html"

	"github.com/sarielhp/relatex/internal/xmltree"
)

var (
	metatextOpen = []byte(`<metatext type="`)
	metatextText = []byte(`" txt="`)
	metatextEnd  = []byte(`">`)
)

// readable is the trace as XML can read it: unreadable characters as U+FFFD (xmltree.Characters)
// and every metatext's text escaped.
func readable(data []byte) []byte {
	return xmltree.Characters(metatexts(data))
}

// metatexts escapes the text of the trace's metatext elements. MuPDF's trace device (1.28) writes a marked
// content's text (an /Alt or /ActualText such as "<exemplar demonstrations>", third cohort 0639) into
// the txt attribute as it is, so a '<', '&' or '"' in it breaks the XML. The element is written on a
// line of its own, `<metatext type="T" txt="TEXT">`, so its text is everything between `" txt="` and
// the line's final `">`, read back character for character once escaped. Data without a metatext
// whose text needs escaping is returned as is.
func metatexts(data []byte) []byte {
	if !bytes.Contains(data, metatextOpen) {
		return data
	}
	var out []byte
	last := 0
	for start := 0; ; {
		found := bytes.Index(data[start:], metatextOpen)
		if found < 0 {
			break
		}
		open := start + found
		lineEnd := bytes.IndexByte(data[open:], '\n')
		if lineEnd < 0 {
			lineEnd = len(data) - open
		}
		line := data[open : open+lineEnd]
		start = open + lineEnd
		textAt := bytes.Index(line, metatextText)
		if textAt < 0 || !bytes.HasSuffix(bytes.TrimRight(line, "\r"), metatextEnd) {
			continue
		}
		textStart := open + textAt + len(metatextText)
		textEnd := open + len(bytes.TrimRight(line, "\r")) - len(metatextEnd)
		if textEnd < textStart {
			continue
		}
		text := data[textStart:textEnd]
		if !bytes.ContainsAny(text, `<>&"`) {
			continue
		}
		if out == nil {
			out = make([]byte, 0, len(data)+64)
		}
		out = append(append(out, data[last:textStart]...), html.EscapeString(string(text))...)
		last = textEnd
	}
	if out == nil {
		return data
	}
	return append(out, data[last:]...)
}
