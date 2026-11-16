// Package rubyuri reads a URI the way Ruby 3.3's URI.parse does (URI::RFC3986_Parser), for
// the parts relatex's link checks use: the scheme (lowercased, as URI::Generic stores it),
// the host, the userinfo and the opaque part, and the same error messages. Ruby's grammar
// is one large regular expression with possessive quantifiers and lookarounds; here it is
// a parser that accepts exactly the same strings.
package rubyuri

import (
	"github.com/sarielhp/relatex/internal/rec"
	"net"
	"regexp"
	"strings"
)

// URI holds the parsed components; a nil pointer field is Ruby's nil.
type URI struct {
	Scheme   *string
	Userinfo *string
	Host     *string
	Opaque   *string
	Query    *string
}

// InvalidURIError is Ruby's URI::InvalidURIError.
type InvalidURIError struct{ Message string }

func (e InvalidURIError) Error() string { return e.Message }

// InvalidComponentError is Ruby's URI::InvalidComponentError (a malformed mailto target).
type InvalidComponentError struct{ Message string }

func (e InvalidComponentError) Error() string { return e.Message }

var mailtoTo = regexp.MustCompile(`\A(?:[^@,;]+@[^@,;]+(?:\z|[,;]))*\z`)

// Parse is URI.parse.
func Parse(text string) (*URI, error) {
	for index := 0; index < len(text); index++ {
		if text[index] >= 0x80 {
			return nil, InvalidURIError{Message: "URI must be ascii only " + Dump(text)}
		}
	}
	if uri, ok := absolute(text); ok {
		return checked(uri)
	}
	if uri, ok := relative(text); ok {
		return uri, nil
	}
	return nil, InvalidURIError{Message: "bad URI(is not URI?): " + Inspect(text)}
}

// checked applies what URI::MailTo#initialize checks on a mailto URI.
func checked(uri *URI) (*URI, error) {
	if *uri.Scheme != "mailto" {
		return uri, nil
	}
	if uri.Opaque == nil && uri.Query != nil {
		opaque := "?" + *uri.Query
		uri.Opaque = &opaque
	}
	if uri.Opaque == nil {
		return nil, InvalidComponentError{Message: "missing opaque part for mailto URL"}
	}
	to, _, _ := strings.Cut(*uri.Opaque, "?")
	if !mailtoTo.MatchString(to) {
		return nil, InvalidComponentError{Message: "unrecognised opaque part for mailtoURL: " + *uri.Opaque}
	}
	return uri, nil
}

func pointer(text string) *string { return &text }

// absolute is RFC3986_URI: scheme ":" hier-part ["?" query] ["#" fragment].
func absolute(text string) (*URI, bool) {
	scheme := schemeLength(text)
	if scheme == 0 || scheme >= len(text) || text[scheme] != ':' {
		return nil, false
	}
	rest := text[scheme+1:]
	name := strings.ToLower(text[:scheme])
	if strings.HasPrefix(rest, "//") {
		if uri, ok := authorityForm(rest[2:]); ok {
			uri.Scheme = &name
			return uri, true
		}
	}
	// path-absolute: "/" followed by nothing or by segments not starting with "/".
	if strings.HasPrefix(rest, "/") {
		path := 1
		if len(rest) > 1 && rest[1] != '/' {
			path += segments(rest[1:], true)
		}
		if query, ok := tail(rest[path:]); ok {
			return &URI{Scheme: &name, Host: nil, Query: query}, true
		}
	}
	// path-rootless: segments, not starting with "/"; it is the opaque part.
	if !strings.HasPrefix(rest, "/") {
		path := segments(rest, true)
		if path > 0 {
			if query, ok := tail(rest[path:]); ok {
				opaque := rest[:path]
				if query != nil {
					opaque += "?" + *query
				}
				return &URI{Scheme: &name, Opaque: &opaque}, true
			}
		}
	}
	// path-empty.
	if query, ok := tail(rest); ok {
		return &URI{Scheme: &name, Query: query}, true
	}
	return nil, false
}

// relative is RFC3986_relative_ref (no scheme): only whether it matches matters here.
func relative(text string) (*URI, bool) {
	if strings.HasPrefix(text, "//") {
		if uri, ok := authorityForm(text[2:]); ok {
			return uri, true
		}
	}
	if strings.HasPrefix(text, "/") {
		if _, ok := tail(text[1+segments(text[1:], true):]); ok {
			return &URI{}, true
		}
	}
	if noScheme := segments(text, false); noScheme > 0 {
		path := noScheme
		if path < len(text) && text[path] == '/' {
			path += 1 + segments(text[path+1:], true)
		}
		if _, ok := tail(text[path:]); ok {
			return &URI{}, true
		}
	}
	if _, ok := tail(text); ok {
		return &URI{}, true
	}
	return nil, false
}

// authorityForm reads [userinfo "@"] host [":" port] path-abempty, then the query and
// fragment, after "//".
func authorityForm(text string) (*URI, bool) {
	uri := &URI{}
	rest := text
	if user := userinfoLength(rest); user < len(rest) && rest[user] == '@' {
		// URI::Generic splits the userinfo at ":"; an empty one leaves no user (nil).
		if user > 0 {
			uri.Userinfo = pointer(rest[:user])
		}
		rest = rest[user+1:]
	}
	host, ok := hostLength(rest)
	if !ok {
		return nil, false
	}
	uri.Host = pointer(rest[:host])
	rest = rest[host:]
	if strings.HasPrefix(rest, ":") {
		port := 1
		for port < len(rest) && rest[port] >= '0' && rest[port] <= '9' {
			port++
		}
		rest = rest[port:]
	}
	if strings.HasPrefix(rest, "/") {
		rest = rest[1+segments(rest[1:], true):]
	}
	query, ok := tail(rest)
	if !ok {
		return nil, false
	}
	uri.Query = query
	return uri, true
}

// tail reads ["?" query] ["#" fragment] to the end; it returns the query.
func tail(text string) (*string, bool) {
	var query *string
	if strings.HasPrefix(text, "?") {
		end := strings.IndexByte(text, '#')
		if end < 0 {
			end = len(text)
		}
		query = pointer(text[1:end])
		text = text[end:]
	}
	if strings.HasPrefix(text, "#") {
		fragment := text[1:]
		for index := 0; index < len(fragment); {
			width := characterLength(fragment[index:], fragmentCharacter)
			if width == 0 {
				return nil, false
			}
			index += width
		}
		text = ""
	}
	return query, text == ""
}

func schemeLength(text string) int {
	if text == "" || !isAlpha(text[0]) {
		return 0
	}
	length := 1
	for length < len(text) && (isAlpha(text[length]) || isDigit(text[length]) || strings.IndexByte("+-.", text[length]) >= 0) {
		length++
	}
	return length
}

// segments counts the longest run of path characters (with "/" or, for path-noscheme,
// without "/" and ":").
func segments(text string, slash bool) int {
	test := segmentCharacter
	if !slash {
		test = noSchemeCharacter
	}
	length := 0
	for length < len(text) {
		width := characterLength(text[length:], test)
		if width == 0 {
			break
		}
		length += width
	}
	return length
}

func userinfoLength(text string) int {
	length := 0
	for length < len(text) {
		width := characterLength(text[length:], userinfoCharacter)
		if width == 0 {
			break
		}
		length += width
	}
	return length
}

// hostLength is HOST: an IP literal in brackets, or a reg-name (which also covers an IPv4
// address, the same characters).
func hostLength(text string) (int, bool) {
	if strings.HasPrefix(text, "[") {
		end := strings.IndexByte(text, ']')
		if end < 0 || !ipLiteral(text[1:end]) {
			return 0, false
		}
		return end + 1, true
	}
	length := 0
	for length < len(text) {
		width := characterLength(text[length:], regNameCharacter)
		if width == 0 {
			break
		}
		length += width
	}
	return length, true
}

var ipvFuture = regexp.MustCompile(`\Av[0-9A-Fa-f]+\.[!$&-.0-9:;=A-Z_a-z~]+\z`)

func ipLiteral(text string) bool {
	if ipvFuture.MatchString(text) {
		return true
	}
	if !strings.Contains(text, ":") || strings.Contains(text, "%") {
		return false
	}
	return net.ParseIP(text) != nil
}

// characterLength is the length of one grammar character at the start of text: a
// percent-encoded octet ("%" and two hex digits) or one character the test admits.
func characterLength(text string, test func(byte) bool) int {
	if text[0] == '%' {
		if len(text) >= 3 && isHex(text[1]) && isHex(text[2]) {
			return 3
		}
		return 0
	}
	if test(text[0]) {
		return 1
	}
	return 0
}

// unreservedSub is [!$&-.0-9;=A-Z_a-z~]: "&-." is the range & ' ( ) * + , - .
func unreservedSub(c byte) bool {
	return c == '!' || c == '$' || (c >= '&' && c <= '.') || isDigit(c) || c == ';' || c == '=' || isAlpha(c) || c == '_' || c == '~'
}

func regNameCharacter(c byte) bool  { return unreservedSub(c) }
func userinfoCharacter(c byte) bool { return unreservedSub(c) || c == ':' }
func noSchemeCharacter(c byte) bool { return unreservedSub(c) || c == '@' }
func segmentCharacter(c byte) bool  { return unreservedSub(c) || c == ':' || c == '@' || c == '/' }
func fragmentCharacter(c byte) bool { return segmentCharacter(c) || c == '?' }

func isAlpha(c byte) bool { return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// Inspect is Ruby's String#inspect for the text a link holds.
func Inspect(text string) string { return rec.InspectString(text) }

// Dump is Ruby's String#dump: non-ASCII characters as \u escapes.
func Dump(text string) string { return rec.DumpString(text) }
