package notes

import (
	"encoding/xml"
	"strings"
	"unicode"
)

// MathMLText is Nokogiri::XML(mathml).root&.text.to_s: the character data under the
// document's root element (CDATA included, comments and processing instructions not). The
// pipeline writes well-formed MathML; text read before a malformation is kept, as libxml2's
// recovering parser keeps it.
func MathMLText(mathml string) string {
	decoder := xml.NewDecoder(strings.NewReader(mathml))
	var text strings.Builder
	depth := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			return text.String()
		}
		switch typed := token.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 {
				return text.String()
			}
		case xml.CharData:
			if depth > 0 {
				text.Write(typed)
			}
		}
	}
}

// RemoveSpace removes Unicode White_Space (unicode.IsSpace), as Ruby's gsub of [[:space:]]+
// by the empty string does on UTF-8 text.
func RemoveSpace(text string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsSpace(character) {
			return -1
		}
		return character
	}, text)
}
