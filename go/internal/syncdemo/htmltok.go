package syncdemo

import (
	"regexp"
	"strings"
)

// token is one piece of the export's HTML: text, or a tag with its name, whether it closes and
// whether it has no content. The export writes well-formed HTML, so a scanner is enough.
type token struct {
	raw   string
	text  bool
	name  string
	close bool
	void  bool
}

var voidElements = map[string]bool{"br": true, "hr": true, "img": true, "meta": true, "link": true, "input": true,
	"col": true, "source": true, "area": true, "base": true, "wbr": true}

var rawTextElements = map[string]bool{"script": true, "style": true}

var idAttribute = regexp.MustCompile(`(?:^|\s)id\s*=\s*(?:"([^"]*)"|'([^']*)')`)

// tokenize splits an HTML document into text and tags; a script or style body is one text token.
func tokenize(source string) []token {
	var tokens []token
	for position := 0; position < len(source); {
		if source[position] != '<' {
			end := strings.IndexByte(source[position:], '<')
			if end < 0 {
				end = len(source) - position
			}
			tokens = append(tokens, token{raw: source[position : position+end], text: true})
			position += end
			continue
		}
		end := tagEnd(source, position)
		tag := parseTag(source[position:end])
		tokens = append(tokens, tag)
		position = end
		if rawTextElements[tag.name] && !tag.close && !tag.void {
			stop := strings.Index(strings.ToLower(source[position:]), "</"+tag.name)
			if stop > 0 {
				tokens = append(tokens, token{raw: source[position : position+stop], text: true, name: "raw"})
				position += stop
			}
		}
	}
	return tokens
}

// tagEnd is the index after the '>' closing the tag or comment at start, quotes honoured.
func tagEnd(source string, start int) int {
	if strings.HasPrefix(source[start:], "<!--") {
		if end := strings.Index(source[start:], "-->"); end >= 0 {
			return start + end + 3
		}
		return len(source)
	}
	var quote byte
	for i := start + 1; i < len(source); i++ {
		c := source[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return i + 1
		}
	}
	return len(source)
}

func parseTag(raw string) token {
	tag := token{raw: raw}
	body := strings.TrimSuffix(strings.TrimPrefix(raw, "<"), ">")
	if strings.HasPrefix(body, "!") || strings.HasPrefix(body, "?") {
		tag.void = true
		return tag
	}
	if strings.HasPrefix(body, "/") {
		tag.close = true
		body = body[1:]
	}
	end := strings.IndexAny(body, " \t\r\n/")
	if end < 0 {
		end = len(body)
	}
	tag.name = strings.ToLower(body[:end])
	tag.void = voidElements[tag.name] || strings.HasSuffix(body, "/")
	return tag
}

// idOf is a tag's id attribute, "" without one.
func idOf(tag token) string {
	attributes := tag.raw
	if end := strings.IndexAny(attributes, " \t\r\n"); end >= 0 {
		attributes = attributes[end:]
	}
	found := idAttribute.FindStringSubmatch(attributes)
	if found == nil {
		return ""
	}
	return found[1] + found[2]
}

// withAttributes is the opening tag with extra attributes before its closing bracket.
func withAttributes(tag token, extra string) string {
	raw := strings.TrimSuffix(tag.raw, ">")
	suffix := ">"
	if strings.HasSuffix(raw, "/") {
		raw, suffix = strings.TrimSuffix(raw, "/"), "/>"
	}
	return raw + " " + extra + suffix
}
