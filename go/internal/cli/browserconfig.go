package cli

import (
	"net/url"
	"slices"

	"github.com/sarielhp/relatex/internal/browser"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// browserCheck is one setting of the "browser" section: null, or a value its test accepts.
type browserCheck struct {
	name, expected string
	test           func(any) bool
}

// httpAddress accepts an http:// address with a host.
func httpAddress(value any) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	parsed, err := url.Parse(text)
	return err == nil && parsed.Scheme == "http" && parsed.Host != ""
}

// browserChecks: ace_endpoint is read by the development tools only (tools/ace_remote.rb, DAISY
// Ace as a service); the product validates it so the shared file stays checked.
var browserChecks = []browserCheck{
	{"endpoint", "null or an http:// address", httpAddress},
	{"sessions", "null or a number from 1 to 64", func(value any) bool {
		number, ok := value.(int64)
		return ok && number >= 1 && number <= 64
	}},
	{"token_file", "null or a file name", func(value any) bool { text, ok := value.(string); return ok && text != "" }},
	{"ace_endpoint", "null or an http:// address", httpAddress},
}

// validateBrowser checks the "browser" section: only its known settings, each null or valid.
func validateBrowser(value any, path string) {
	section, ok := value.(ojson.Object)
	if !ok {
		rec.Fail("%s: \"browser\" must be an object", path)
	}
	for _, pair := range section {
		index := slices.IndexFunc(browserChecks, func(check browserCheck) bool { return check.name == pair.Key() })
		if index < 0 {
			rec.Fail("%s: unknown setting \"browser.%s\"", path, pair.Key())
		}
		if check := browserChecks[index]; pair.Value() != nil && !check.test(pair.Value()) {
			rec.Fail("%s: \"browser.%s\" must be %s", path, check.name, check.expected)
		}
	}
}

// configureBrowser hands the merged "browser" section to the browser checks.
func configureBrowser(settings ojson.Object) {
	section, _ := settings.Get("browser").(ojson.Object)
	endpoint, _ := section.Get("endpoint").(string)
	sessions, _ := section.Get("sessions").(int64)
	tokenFile, _ := section.Get("token_file").(string)
	browser.Configure(browser.Settings{Endpoint: endpoint, Sessions: int(sessions), TokenFile: tokenFile})
}
