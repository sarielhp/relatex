package syncdemo

import (
	"regexp"
	"strings"
)

// A demo page is made to be shared, so it shows no email address: an address in the paper or its
// source is masked character for character (the source's columns stay true), and a mailto link loses
// its target.

var (
	emailAddress = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	mailtoTarget = regexp.MustCompile(`\s+href="mailto:[^"]*"`)
)

// maskEmails replaces each address's letters and digits with a middle dot, keeping its length.
func maskEmails(text string) string {
	return emailAddress.ReplaceAllStringFunc(text, func(address string) string {
		return strings.Map(func(r rune) rune {
			if r == '@' || r == '.' {
				return r
			}
			return '·'
		}, address)
	})
}

// redactPage drops mailto targets from the paper's HTML and masks the addresses it shows.
func redactPage(page string) string {
	return maskEmails(mailtoTarget.ReplaceAllString(page, ""))
}
