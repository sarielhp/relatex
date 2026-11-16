package reading

import (
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
)

const totPagesPackage = "7979159e363fabfc2f9e9680313683de0207157c2d5f46145f18c03eaa6c78c3"

var (
	pageTotalFields = []string{"seq", "kind", "code", "source", "package_sha256", "label", "total", "cached", "shipouts", "anchor"}
	hexDigest       = regexp.MustCompile(`\A[0-9a-f]{64}\z`)
)

// pageEvidence is DocumentPageEvidence: the document's page total (totpages), when the
// physical PDF page count proves it.
type pageEvidence struct {
	records     []rec.Map
	proof       any
	diagnostics []rec.Map
}

// target is the document page total target, or nil; an unprovable total is a diagnostic.
func (p *pageEvidence) target() (result rec.Map) {
	rows := []rec.Map{}
	for _, row := range p.records {
		if row["kind"] == "diagnostic" && row["code"] == "document_page_total" {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	defer p.unsupported(&result)
	if !stream.Enabled(p.records[0], "page-totals-0.1") {
		rec.Fail("Document totals require their extension")
	}
	if len(rows) != 1 {
		rec.Fail("Ambiguous document page total")
	}
	row := rows[0]
	p.validateSource(row)
	p.validatePDF(row)
	return rec.Map{"kind": "document_page_total", "label": rec.Fetch(row, "label"), "number": toS(rec.Fetch(row, "total")),
		"count": rec.Fetch(row, "total"), "anchor": rec.Fetch(row, "anchor"), "pdf_sha256": rec.Fetch(rec.AsMap(p.proof), "pdf_sha256")}
}

// unsupported turns a failure of the total into its diagnostic and no target.
func (p *pageEvidence) unsupported(result *rec.Map) {
	recovered := recover()
	if recovered == nil {
		return
	}
	failure, ok := recovered.(rec.Failure)
	if !ok {
		panic(recovered)
	}
	p.diagnostics = append(p.diagnostics, rec.Map{"code": "unsupported_document_page_total", "reason": failure.Message})
	*result = nil
}

func (p *pageEvidence) validateSource(row rec.Map) {
	_, anchor := row["anchor"].(string)
	if !rec.SameKeys(row, pageTotalFields) || row["source"] != "totpages-2.00-hyperref" || row["package_sha256"] != totPagesPackage ||
		row["label"] != "TotPages" || !anchor {
		rec.Fail("Unknown document-total writer")
	}
	for _, entry := range p.records {
		if entry["kind"] == "label" && rec.Equal(entry["name"], row["label"]) {
			rec.Fail("Conflicting ordinary document-total label")
		}
	}
	count, isInt := row["total"].(int64)
	last := p.records[len(p.records)-1]
	if !isInt || count < 1 || count > 100_000 || !rec.Equal(row["cached"], count) || !rec.Equal(row["shipouts"], count) ||
		!rec.Equal(rec.Fetch(last, "pages"), count) || !less(rec.Fetch(row, "seq"), rec.Fetch(last, "seq")) {
		rec.Fail("Unstable source document page count")
	}
}

func (p *pageEvidence) validatePDF(row rec.Map) {
	p.validateHelper()
	proof, _ := p.proof.(rec.Map)
	digest, isString := proof["pdf_sha256"].(string)
	stage, isMap := proof["stage"].(rec.Map)
	valid := proof != nil && rec.SameKeys(proof, []string{"helper", "pages", "pdf_sha256", "stage"}) && isString &&
		hexDigest.MatchString(digest) && rec.Equal(proof["pages"], rec.Fetch(row, "total")) && isMap
	if valid {
		exit, ok := rec.Number(stage["exit"])
		if !ok {
			rec.Fail("undefined method 'zero?' for %T", stage["exit"])
		}
		valid = exit == 0 && stage["timeout"] == false
	}
	if !valid {
		rec.Fail("Independent physical PDF page count is missing or differs")
	}
}

func (p *pageEvidence) validateHelper() {
	proof, _ := p.proof.(rec.Map)
	helper, ok := proof["helper"].(rec.Map)
	valid := ok && rec.SameKeys(helper, []string{"path", "realpath", "sha256", "version"})
	for _, value := range helper {
		text, isString := value.(string)
		valid = valid && isString && text != ""
	}
	if !valid || !hexDigest.MatchString(helper["sha256"].(string)) || !strings.HasPrefix(helper["version"].(string), "pdfinfo version ") {
		rec.Fail("Missing independent PDF helper identity")
	}
}
