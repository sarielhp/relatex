package pipeline

import (
	"path/filepath"

	"github.com/sarielhp/relatex/internal/epub"
	"github.com/sarielhp/relatex/internal/ojson"
)

// webClaim is the WCAG claim the tagged PDF is judged against: the export root's own accessibility.json
// wins; a conversion that builds no html reads --web-report. "" when neither claims.
func webClaim(root string, options Options) string {
	claim := epub.ReadClaim(filepath.Join(root, "accessibility.json"))
	if claim == "" && options.WebReport != "" {
		claim = epub.ReadClaim(options.WebReport)
	}
	return claim
}

// WebClaim is webClaim for callers outside the pipeline (tag-pdf, which judges its tagged PDF's
// identification against the same report a conversion would): the export root's own
// accessibility.json wins, else webReport's, else "" when neither claims.
func WebClaim(root, webReport string) string {
	return webClaim(root, Options{WebReport: webReport})
}

// taggedStageConfiguration is the pdf-ua export stage's configuration: the tagged build and its
// checks, and the web claim the identification is judged against, so a different claim never reuses
// a cached tagged PDF with another identification.
func taggedStageConfiguration(tagged ojson.Object, claim string) ojson.Object {
	return ojson.Object{{"pdf_ua", tagged}, {"web_claim", claim}}
}
