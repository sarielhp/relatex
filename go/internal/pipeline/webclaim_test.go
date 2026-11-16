package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/ojson"
)

const tagPacket = `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/">` +
	`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` +
	`<rdf:Description rdf:about="" xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/">` +
	`<pdfuaid:part>2</pdfuaid:part></rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`

// taggedFixture writes a one-page PDF whose catalog carries tagPacket; qpdf reconstructs the xref.
func taggedFixture(t *testing.T, directory string) string {
	qpdf, err := exec.LookPath("qpdf")
	if err != nil {
		t.Skip("qpdf not installed")
	}
	raw, pdf := filepath.Join(directory, "raw.pdf"), filepath.Join(directory, "paper.pdf")
	body := fmt.Sprintf("%%PDF-2.0\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>\nendobj\n"+
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"+
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>\nendobj\n"+
		"4 0 obj\n<< /Type /Metadata /Subtype /XML /Length %d >>\nstream\n%s\nendstream\nendobj\n"+
		"trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n0\n%%%%EOF\n", len(tagPacket)+1, tagPacket)
	if err := os.WriteFile(raw, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if message, err := exec.Command(qpdf, "--warning-exit-0", "--object-streams=generate", raw, pdf).CombinedOutput(); err != nil {
		t.Fatalf("qpdf: %v: %s", err, message)
	}
	return pdf
}

func writeClaim(t *testing.T, path string, claim any) string {
	data, err := json.Marshal(map[string]any{"claim": claim})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The export root's own report wins; else --web-report's is read. Missing/unreadable gives "".
func TestWebClaimReadsTheEarlierConversionsReport(t *testing.T) {
	root := t.TempDir()
	earlier := writeClaim(t, filepath.Join(t.TempDir(), "accessibility.json"), "WCAG 2.2 Level AA")
	if claim := webClaim(root, Options{WebReport: earlier}); claim != "WCAG 2.2 Level AA" {
		t.Fatalf("from --web-report: %q", claim)
	}
	writeClaim(t, filepath.Join(root, "accessibility.json"), "WCAG 2.1 Level A")
	if claim := webClaim(root, Options{WebReport: earlier}); claim != "WCAG 2.1 Level A" {
		t.Fatalf("the export root's own report did not win: %q", claim)
	}
	for _, absent := range []string{filepath.Join(root, "absent.json"), root} {
		if claim := webClaim(t.TempDir(), Options{WebReport: absent}); claim != "" {
			t.Fatalf("unreadable report %s: %q", absent, claim)
		}
	}
}

// A conforming file keeps the identification only beside a claim; null/missing and tagged withdraw.
func TestEarnedClaimUsesTheWebReport(t *testing.T) {
	directory := t.TempDir()
	claiming := writeClaim(t, filepath.Join(directory, "claim.json"), "WCAG 2.2 Level AA")
	null := writeClaim(t, filepath.Join(directory, "null.json"), nil)
	cases := []struct{ status, claim, want string }{
		{"conforming", webClaim(directory, Options{WebReport: claiming}), "kept"},
		{"conforming", webClaim(directory, Options{WebReport: null}), "withdrawn"},
		{"conforming", webClaim(directory, Options{WebReport: filepath.Join(directory, "absent.json")}), "withdrawn"},
		{"tagged", "WCAG 2.2 Level AA", "withdrawn"},
	}
	for _, test := range cases {
		pdf := taggedFixture(t, t.TempDir())
		report := EarnedClaim(ojson.Object{{"status", test.status}}, pdf, test.claim)
		identification, _ := report.Get("identification").(ojson.Object)
		if got := identification.Get("pdfuaid"); got != test.want {
			t.Errorf("%s %q: %v", test.status, test.claim, got)
		}
	}
}

// The claim is part of the pdf-ua stage configuration, so another claim never reuses its cache entry.
func TestTaggedPDFCacheKeyIncludesTheWebClaim(t *testing.T) {
	tagged, inputs, identity := ojson.Object{{"toolchain", "one"}}, ojson.Object{}, ojson.Object{}
	key := func(claim string) string {
		return stageKey("export.pdf-ua", inputs, taggedStageConfiguration(tagged, claim), identity)
	}
	if key("WCAG 2.2 Level AA") == key("WCAG 2.1 Level A") {
		t.Fatal("a different web claim reused the tagged PDF's cache entry")
	}
}
