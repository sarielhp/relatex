// Package a11yreport is the export's report page (decision 112, written for every export since the
// accessibility run's decision 51; plan item D3): accessibility.html beside accessibility.json, a
// page an instructor can keep. For each output
// (the HTML page, the EPUB package, the tagged PDF) it states the claim reached or why not, lists
// the open items with their source lines (from the accessibility lint) and what to do, counts where
// the descriptions, the language and the table headers came from, and names the relatex version
// and the date. It is itself accessible: no script, both colour schemes, tables with headers.
package a11yreport

import (
	"fmt"
	"slices"

	"github.com/sarielhp/relatex/internal/a11ylint"
	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/pdfua"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablelayout"
)

// FileName is the report page, in the export directory beside accessibility.json.
const FileName = "accessibility.html"

// EPUBClaim is the conformance the EPUB package states with the page's (epub.ReadClaim).
const EPUBClaim = "EPUB Accessibility 1.1 - WCAG 2.2 Level AA"

// Inputs are what the page reports.
type Inputs struct {
	Title, Main, Version, Date string
	Document                   rec.Map
	// Web is accessibility.json (nil when the page was not measured).
	Web rec.Map
	// EPUB tells whether the package was written; Tagged is the pdf-ua report (nil when not built).
	EPUB   bool
	Tagged ojson.Object
	// Lint is the source lint's items; LintNote says why there are none to show, "" when it ran.
	Lint     []a11ylint.Item
	LintNote string
	// LintSummary says whether the export would earn the claim once the lint's items are done (a11ylint.ExportSummary), "" without one.
	LintSummary string
}

// Claim is one output's verdict: the standard, the claim reached ("" for none) and why not.
type Claim struct {
	Output, Target, File, Reached string
	Reasons                       []string
}

// Claims are the three outputs' verdicts, in the order the page lists them.
func Claims(in Inputs) []Claim {
	return []Claim{webClaim(in), epubClaim(in), taggedClaim(in)}
}

// webClaim is the HTML page's: accessibility.json's claim, or what keeps it from one.
func webClaim(in Inputs) Claim {
	claim := Claim{Output: "Web page (HTML)", Target: "WCAG 2.2 Level AA", File: "index.html"}
	if in.Web == nil {
		claim.Reasons = []string{"the page was not measured: convert with --accessible (or --verify-browser)"}
		return claim
	}
	if reached, _ := in.Web["claim"].(string); reached != "" {
		claim.Reached = reached
		return claim
	}
	claim.Reasons = webReasons(in.Web)
	return claim
}

// webReasons say why accessibility.json makes no claim.
func webReasons(web rec.Map) []string {
	status, _ := web["status"].(string)
	if status != "measured" {
		reason, _ := web["reason"].(string)
		return []string{fmt.Sprintf("axe-core could not measure the page (%s): %s", status, reason)}
	}
	reasons := []string{}
	for _, value := range listOf(web, "open_items") {
		item := rec.AsMap(value)
		count, _ := rec.Number(item["count"])
		reasons = append(reasons, fmt.Sprintf("%s: %d", itemName(item), int64(count)))
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "not every check ran (both themes and the structural checks are needed)")
	}
	return reasons
}

// itemName is an open item as a reader names it: the check and its WCAG criterion, or the axe rule.
func itemName(item rec.Map) string {
	id := rec.ToS(item["id"])
	if item["source"] == "axe" {
		return "axe-core rule " + id
	}
	if criterion := rec.ToS(item["criterion"]); criterion != "" {
		return fmt.Sprintf("%s (WCAG %s)", checkTitle(id), criterion)
	}
	return checkTitle(id)
}

// epubClaim is the package's: it states EPUB Accessibility 1.1 exactly when the page reaches WCAG
// 2.2 AA (decision 094).
func epubClaim(in Inputs) Claim {
	claim := Claim{Output: "EPUB package", Target: EPUBClaim, File: "the .epub beside the PDF"}
	switch {
	case !in.EPUB:
		claim.Reasons = []string{"no EPUB was written"}
	case in.Web != nil && in.Web["claim"] == htmlkit.WCAGClaim:
		claim.Reached = EPUBClaim
	default:
		claim.Reasons = []string{"the package states its conformance only with the web page's WCAG 2.2 AA claim, which is not reached"}
	}
	return claim
}

// taggedClaim is the tagged PDF's: PDF/UA-2 when its report calls it conforming and kept its
// identification, which it does only beside the web page's claim (decision 112).
func taggedClaim(in Inputs) Claim {
	claim := Claim{Output: "Tagged PDF", Target: "PDF/UA-2", File: "pdf-ua/" + rec.ToS(in.Tagged.Get("file"))}
	if in.Tagged == nil {
		claim.File = ""
		claim.Reasons = []string{"no tagged PDF was built: convert with --accessible (or --pdf-ua)"}
		return claim
	}
	if in.Tagged.Get("status") == "conforming" {
		identification, _ := in.Tagged.Get("identification").(ojson.Object)
		if identification.Get("pdfuaid") == "kept" {
			claim.Reached = "PDF/UA-2"
		} else {
			claim.Reasons = []string{"the tagged PDF conforms, but it claims PDF/UA-2 only beside the web page's WCAG 2.2 AA claim, which is not reached"}
		}
		return claim
	}
	claim.Reasons = []string{fmt.Sprintf("the tagged PDF is %s, not conforming", rec.ToS(in.Tagged.Get("status")))}
	for _, found := range pdfua.Diagnostics(in.Tagged) {
		claim.Reasons = append(claim.Reasons, found.Code+": "+found.Message)
	}
	return claim
}

// Provenance is one row of the provenance table: what was counted and how many came from where.
type Provenance struct {
	Subject string
	Counts  map[string]int
}

// Origins are the provenance table's columns: the source (alt keys, \relatexalt, \Description, babel, rules
// under header rows), the sidecar (the author's text), reviewed and unreviewed model drafts,
// inferred (table headers, decorative images) and detected (the language), and none.
var Origins = []string{"source", "sidecar", "reviewed", "model", "inferred", "detected", "none"}

// ProvenanceRows count, for the figures, the document language and the data tables' header rows,
// where each came from.
func ProvenanceRows(in Inputs) []Provenance {
	figureCounts := map[string]int{}
	// A conversion without HTML/PDF has no document: the page then counts no figures.
	if in.Document != nil {
		for _, figure := range figdesc.Figures(in.Document) {
			figureCounts[figureOrigin(figure.Provenance())]++
		}
	}
	return []Provenance{{"Figure descriptions", figureCounts}, {"Document language", languageCounts(in.Document)},
		{"Table header rows", tableCounts(in)}}
}

// figureOrigin is a figure description's column: the sidecar for the author's own entries.
func figureOrigin(provenance string) string {
	switch provenance {
	case "":
		return "none"
	case "author":
		return "sidecar"
	case figdesc.SourceMacro: // \relatexalt (decision 125, amendment 2)
		return "source"
	}
	if slices.Contains(Origins, provenance) {
		return provenance
	}
	return "inferred"
}

// languageCounts is the document language's column: declared in the source (babel, polyglossia, a
// class option, \DocumentMetadata), detected from the text, or none (relatex's default).
func languageCounts(document rec.Map) map[string]int {
	language, _ := document["language"].(rec.Map)
	source := rec.ToS(language["source"])
	switch source {
	case "", "default":
		return map[string]int{"none": 1}
	case "detected", "inferred":
		return map[string]int{source: 1}
	}
	return map[string]int{"source": 1}
}

// tableCounts are the data tables by where their header row came from, as the structural check's
// table_headers row records it (decision 111): none for the tables it lists without header cells,
// source for headers the author declared (a table hint, the tagging project's keys, a longtable head)
// and inferred for headers read from the typesetting (decisions 111 and 131: booktabs, a rule, group
// headings, bold type in the first row or column).
func tableCounts(in Inputs) map[string]int {
	counts := map[string]int{"none": 0, "inferred": 0, "source": 0}
	for _, value := range listOf(in.Web, "structural") {
		row := rec.AsMap(value)
		if row["check"] != "table_headers" {
			continue
		}
		count, _ := rec.Number(row["count"])
		counts["none"] = int(count)
		headers, _ := row["headers"].(rec.Map)
		for source, value := range headers {
			number, _ := rec.Number(value)
			switch source {
			case tablelayout.HeaderAuthor, tablelayout.HeaderTagging, tablelayout.HeaderLongtable:
				counts["source"] += int(number)
			case tablelayout.HeaderBooktabs, tablelayout.HeaderRule, tablelayout.HeaderSpan, tablelayout.HeaderBold,
				tablelayout.HeaderCorner, tablelayout.HeaderNumeric, tablelayout.HeaderInferred, tablelayout.HeaderFirstColumnInferred,
				tablelayout.HeaderFirstColumnRule, tablelayout.HeaderFirstColumnBold, tablelayout.HeaderFirstColumnSpan:
				counts["inferred"] += int(number)
			}
		}
	}
	return counts
}

// listOf is a list field of a report, nil when it has none.
func listOf(report rec.Map, key string) []any {
	list, _ := report[key].([]any)
	return list
}
