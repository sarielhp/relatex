package pipeline

import (
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/pdftag"
	"github.com/sarielhp/relatex/internal/pdfua"
	"github.com/sarielhp/relatex/internal/rec"
)

// EarnedClaim keeps the PDF/UA-2 identification in the tagged PDF's metadata only when the report
// calls the file conforming (veraPDF passes and relatex's own checks hold) and the web page of the
// same document reaches its WCAG claim (webClaim, "" for none): the HTML is the more important output,
// so the PDF never claims more than it (the author, 2026-11-02; decision 112). The status stays the
// file's own verdict. Route 2's XMP (and an author's \DocumentMetadata{pdfstandard=ua-2}) write
// the identification into the file, so a file that does not earn it has it taken out again,
// with the conformance declarations; the report's "identification" says which. A file whose
// identification cannot be taken out gets the pdfua.claimed diagnostic: it states a conformance it
// did not earn.
func EarnedClaim(report ojson.Object, pdf, webClaim string) ojson.Object {
	status := rec.ToS(report.Get("status"))
	if status == "failed" || !isFile(pdf) {
		return report
	}
	if status == "conforming" && webClaim != "" {
		return report.Set("identification", ojson.Object{{"pdfuaid", "kept"},
			{"reason", "veraPDF and relatex's checks pass, and the web page claims " + webClaim}})
	}
	why := "the file is " + status + ", not conforming"
	if status == "conforming" {
		why = "the file conforms, but the web page of the same document reaches no WCAG claim"
	}
	withdrawn, err := pdftag.WithdrawClaim(pdf)
	switch {
	case err != nil:
		found := append(pdfua.Diagnostics(report), pdfua.Diagnostic{Code: "pdfua.claimed",
			Message: "the tagged PDF has not earned its claim (" + why + "), but its metadata still declares PDF/UA-2: " + err.Error()})
		return report.Set("identification", ojson.Object{{"pdfuaid", "unearned"}, {"reason", err.Error()}}).
			Set("diagnostics", pdfua.DiagnosticList(found))
	case withdrawn:
		return report.Set("identification", ojson.Object{{"pdfuaid", "withdrawn"},
			{"reason", why + ": the PDF/UA-2 identification and the conformance declarations were removed from its metadata"}})
	}
	return report.Set("identification", ojson.Object{{"pdfuaid", "absent"}, {"reason", why + "; the file states no conformance"}})
}
