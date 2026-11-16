package xmltree

import "bytes"

// AttributeWhitespace is data with XML's attribute-value normalization of literal white space
// applied (XML 1.0, 3.3.3): a tab, line feed, carriage return or CR LF pair written inside an
// attribute value reads as one space, as every XML parser (libxml2, a browser's XHTML) reads it.
// encoding/xml keeps it as written, so a reader of SVG markup that writes it on (an inline figure in
// an HTML page, whose parser keeps a newline) normalizes first. A character reference (&#10;) is
// not literal white space and stays for the decoder. Comments, CDATA sections, processing
// instructions and declarations are copied unchanged.
func AttributeWhitespace(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for index := 0; index < len(data); {
		if data[index] != '<' {
			out = append(out, data[index])
			index++
			continue
		}
		end := constructEnd(data, index)
		if end < 0 {
			end = tagEnd(data, index)
			out = appendTag(out, data[index:end])
		} else {
			out = append(out, data[index:end]...)
		}
		index = end
	}
	return out
}

// constructEnd is the end of the comment, CDATA section, processing instruction or declaration
// starting at index, or -1 for an element tag.
func constructEnd(data []byte, index int) int {
	rest := data[index:]
	switch {
	case bytes.HasPrefix(rest, []byte("<!--")):
		return through(data, index, "-->")
	case bytes.HasPrefix(rest, []byte("<![CDATA[")):
		return through(data, index, "]]>")
	case bytes.HasPrefix(rest, []byte("<?")):
		return through(data, index, "?>")
	case bytes.HasPrefix(rest, []byte("<!")):
		closing := bytes.IndexByte(rest, '>')
		if open := bytes.IndexByte(rest, '['); open >= 0 && (closing < 0 || open < closing) {
			return through(data, index, "]>")
		}
		return through(data, index, ">")
	}
	return -1
}

// through is the index just past the first terminator after index, or the data's end.
func through(data []byte, index int, terminator string) int {
	at := bytes.Index(data[index:], []byte(terminator))
	if at < 0 {
		return len(data)
	}
	return index + at + len(terminator)
}

// tagEnd is the index just past the element tag starting at index: its closing > outside quotes.
func tagEnd(data []byte, index int) int {
	var quote byte
	for at := index + 1; at < len(data); at++ {
		switch value := data[at]; {
		case quote != 0:
			if value == quote {
				quote = 0
			}
		case value == '"' || value == '\'':
			quote = value
		case value == '>':
			return at + 1
		}
	}
	return len(data)
}

// appendTag appends an element tag with the literal white space inside its quoted values as
// spaces, a CR LF pair as one.
func appendTag(out, tag []byte) []byte {
	var quote byte
	for at := 0; at < len(tag); at++ {
		value := tag[at]
		switch {
		case quote != 0 && value == quote:
			quote = 0
		case quote == 0 && (value == '"' || value == '\''):
			quote = value
		case quote != 0 && (value == '\t' || value == '\n' || value == '\r'):
			if value == '\r' && at+1 < len(tag) && tag[at+1] == '\n' {
				at++
			}
			value = ' '
		}
		out = append(out, value)
	}
	return out
}
