// Package links is the Go port of Relatex::PaperExternalLinks' catalog and of
// PaperFrontmatter's href checks: the external links hyperref executed, each with a target a
// reader can follow or the reason it cannot.
//
// It is in the model layer (decision 102): it imports only utilities, and the model's link
// graph (extlinks) and the outputs read its href checks and ORCID names.
package links

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/rubyuri"
	"github.com/sarielhp/relatex/internal/stream"
)

var email = regexp.MustCompile(`\A[A-Za-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\z`)
var unsafeCharacters = regexp.MustCompile(`[\x00-\x20\x7f]`)

// Catalog is PaperExternalLinks.catalog: link id to its definition.
func Catalog(records []rec.Map) map[int64]rec.Map {
	rows := []rec.Map{}
	for _, row := range records {
		if row["kind"] == "diagnostic" && row["code"] == "external_link" {
			rows = append(rows, row)
		}
	}
	if len(rows) > 0 && !stream.Enabled(records[0], "external-links-0.1") {
		rec.Fail("External links require their declared extension")
	}
	return Definitions(rows)
}

// Definitions is PaperExternalLinks.definitions. A URL copied with spaces keeps them,
// percent-encoded; spaces around it are dropped, as browsers do. A target no reader can
// follow (no scheme: "github.com/x") is kept, marked unsupported with the reason.
func Definitions(rows []rec.Map) map[int64]rec.Map {
	result := map[int64]rec.Map{}
	for _, row := range rows {
		link, isInt := row["link"].(int64)
		_, seen := result[link]
		keys := []string{}
		for key := range row {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		if !slices.Equal(keys, []string{"code", "href", "kind", "link", "seq", "source"}) || row["source"] != "hyperref-execution" ||
			!isInt || link <= 0 || seen {
			rec.Fail("Malformed or duplicate external-link execution")
		}
		copied := rec.Map{}
		for key, value := range row {
			copied[key] = value
		}
		href := strings.NewReplacer("[", "%5B", "]", "%5D").Replace(EncodedHref(rec.Strip(rec.FetchString(row, "href"))))
		copied["href"] = href
		reason, err := SafeHref(href)
		if err != nil {
			panic(err)
		}
		if reason != "" {
			copied["unsupported"] = reason
		}
		result[link] = copied
	}
	return result
}

// EncodedHref is PaperFrontmatter.encoded_href: control characters, space and DEL
// percent-encoded.
func EncodedHref(href string) string {
	return unsafeCharacters.ReplaceAllStringFunc(href, func(character string) string {
		return fmt.Sprintf("%%%02X", character[0])
	})
}

// SafeHref is PaperFrontmatter.safe_href!: "" when the link is safe to follow, else the
// reason no reader can follow it. A malformed mailto target (\href{mailto:}{...}, third
// cohort 718) is malformed like any other unparsable link.
func SafeHref(href string) (string, error) {
	if unsafeCharacters.MatchString(href) {
		return "Unsafe external link characters", nil
	}
	uri, err := rubyuri.Parse(href)
	var invalid rubyuri.InvalidURIError
	if errors.As(err, &invalid) {
		return "Malformed external link: " + invalid.Message, nil
	}
	var component rubyuri.InvalidComponentError
	if errors.As(err, &component) {
		return "Malformed external link: " + component.Message, nil
	}
	if err != nil {
		return "", err
	}
	scheme := ""
	if uri.Scheme != nil {
		scheme = *uri.Scheme
	}
	if (scheme == "http" || scheme == "https") && uri.Host != nil && uri.Userinfo == nil {
		return "", nil
	}
	opaque := ""
	if uri.Opaque != nil {
		opaque = *uri.Opaque
	}
	if scheme == "mailto" && email.MatchString(opaque) {
		return "", nil
	}
	return "Unsupported external link scheme or target", nil
}
