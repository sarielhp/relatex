package frontmatter

import (
	"slices"
	"strings"
)

// The address fields acmart.cls (v2.12, 2024/12/28, lines 2555-2588) gives its own separators.
// \streetaddress and \postcode keep their older reading (finish prints their argument).
var (
	acmLineFields    = []string{"position", "institution", "department"}
	acmAddressFields = []string{"city", "state", "country"}
	acmClassOptions  = rx(`\\documentclass\s*(?:\[([^\]]*)\])?\s*\{\s*acmart\s*\}`)
	acmObeyOption    = rx(`(?:^|,)\s*obeypunctuation\s*(?:=\s*(\w+))?\s*(?:,|$)`)
	// acmart.cls lines 219-246: these formats set \@ACM@journalfalse; manuscript, acmsmall,
	// acmlarge, acmtog and acmcp are journals.
	acmConferenceFormats = []string{"sigconf", "siggraph", "sigplan", "sigchi", "sigchi-a", "acmengage"}
)

// acmJournal is \if@ACM@journal: the class's format option names no conference format.
func (t *TexMetadata) acmJournal() bool {
	match := acmClassOptions.FindStringSubmatch(t.clean)
	if match == nil {
		return true
	}
	for _, option := range strings.Split(match[1], ",") {
		option = strings.TrimPrefix(strings.TrimSpace(option), "format=")
		if slices.Contains(acmConferenceFormats, strings.TrimSpace(option)) {
			return false
		}
	}
	return true
}

// acmObeyPunctuation is \affiliation[obeypunctuation=true]{...} (or the bare key, whose
// default is true, acmart.cls line 1633); every affiliation starts with it false.
func acmObeyPunctuation(options string) bool {
	match := acmObeyOption.FindStringSubmatch(options)
	return match != nil && (match[1] == "" || match[1] == "true")
}

// acmAddress is an acmart \affiliation argument with its fields as acmart.cls prints them, the
// fields' commands replaced by their text and separators (finish reads the rest).
//
// Conference formats (acmart.cls 2580-2588): \position, \institution and \department print
// their text and end the line (\par); \city, \state and \country go through \@ACM@addtoaddress
// (2555-2561), which puts ", " (\unskip, then the comma box) before the text unless the field
// opens a line (\ifvmode). Journal formats (2568-2578): \position, \department, \city and
// \state print nothing (\unskip\ignorespaces), \institution prints "~#1", \country ", #1".
// obeypunctuation prints each field's text alone: the author's own punctuation stands.
func (t *TexMetadata) acmAddress(value string, obey bool) string {
	reader := acmAddressReader{journal: t.acmJournal(), obey: obey, lineStart: true}
	names := append(append([]string{}, acmLineFields...), acmAddressFields...)
	position := 0
	for {
		start, end, name, ok := findCommand(value, position, names, wordEnd)
		if !ok {
			break
		}
		opening := skipSpace(value, end)
		if opening >= len(value) || (value[opening] != '{' && value[opening] != '[') {
			reader.text(value[position:end])
			position = end
			continue
		}
		reader.text(value[position:start])
		argument, ending := t.argument(value, end)
		reader.field(name, argument)
		position = ending
	}
	reader.text(value[position:])
	return strings.TrimPrefix(strings.TrimSpace(reader.out.String()), ",")
}

type acmAddressReader struct {
	out                     strings.Builder
	journal, obey           bool
	lineStart, ignoreSpaces bool
}

func (r *acmAddressReader) text(segment string) {
	if r.ignoreSpaces {
		segment = strings.TrimLeft(segment, " \t\r\n")
		r.ignoreSpaces = segment == ""
	}
	if strings.TrimSpace(segment) != "" {
		r.lineStart = false
	}
	r.out.WriteString(segment)
}

// unskip is TeX's \unskip: the space before is taken back.
func (r *acmAddressReader) unskip() {
	kept := strings.TrimRight(r.out.String(), " \t\r\n")
	r.out.Reset()
	r.out.WriteString(kept)
}

func (r *acmAddressReader) field(name, argument string) {
	switch {
	case r.journal:
		r.journalField(name, argument)
	case r.obey:
		r.text(argument)
	case slices.Contains(acmLineFields, name):
		r.text(argument)
		r.out.WriteString(" ")
		r.lineStart = true
	default:
		if !r.lineStart {
			r.unskip()
			r.out.WriteString(", ")
		}
		r.text(argument)
	}
}

func (r *acmAddressReader) journalField(name, argument string) {
	switch name {
	case "institution":
		r.unskip()
		r.out.WriteString(" ")
		r.text(argument)
	case "country":
		if !r.obey {
			r.out.WriteString(", ")
		}
		r.text(argument)
	default:
		r.unskip()
	}
	r.ignoreSpaces = true
}
