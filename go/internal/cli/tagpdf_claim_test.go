package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

const claimTestPacket = `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/">` +
	`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` +
	`<rdf:Description rdf:about="" xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/">` +
	`<pdfuaid:part>2</pdfuaid:part></rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`

// claimTestPDF writes a one-page PDF whose catalog carries claimTestPacket; qpdf reconstructs the xref.
func claimTestPDF(t *testing.T) string {
	t.Helper()
	qpdf, err := exec.LookPath("qpdf")
	if err != nil {
		t.Skip("qpdf not installed")
	}
	directory := t.TempDir()
	raw, pdf := filepath.Join(directory, "raw.pdf"), filepath.Join(directory, "paper.pdf")
	body := fmt.Sprintf("%%PDF-2.0\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>\nendobj\n"+
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"+
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>\nendobj\n"+
		"4 0 obj\n<< /Type /Metadata /Subtype /XML /Length %d >>\nstream\n%s\nendstream\nendobj\n"+
		"trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n0\n%%%%EOF\n", len(claimTestPacket)+1, claimTestPacket)
	if err := os.WriteFile(raw, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if message, err := exec.Command(qpdf, "--warning-exit-0", "--object-streams=generate", raw, pdf).CombinedOutput(); err != nil {
		t.Fatalf("qpdf: %v: %s", err, message)
	}
	return pdf
}

// claimTestReport is a TagOrdinary-shaped report naming pdf as the tagged PDF.
func claimTestReport(pdf, status string) ojson.Object {
	return ojson.Object{{"status", status}, {"tagging", ojson.Object{{"pdf", pdf}}}}
}

func claimTestFile(t *testing.T, claim any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"claim": claim})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "accessibility.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func identificationOf(report ojson.Object) any {
	identification, _ := report.Get("identification").(ojson.Object)
	return identification.Get("pdfuaid")
}

// --web-report needs its value, as --pdf and --target do: a usage error (status 2), nothing run.
func TestTagPDFWebReportNeedsAValue(t *testing.T) {
	if status, _, stderr := run(t, "tag-pdf", "--web-report"); status != 2 || stderr == "" {
		t.Fatalf("--web-report without a file: status %d, stderr %q", status, stderr)
	}
}

// The tagged PDF keeps its PDF/UA-2 identification only when it conforms and the web claim is
// reached; a tagged (not conforming) file withdraws it whatever the claim (decision 112).
func TestTagPDFClaimKeepsTheIdentificationOnlyWhenEarned(t *testing.T) {
	for _, test := range []struct {
		name   string
		status string
		claim  bool
		want   string
	}{
		{"conforming with a claim", "conforming", true, "kept"},
		{"conforming without a claim", "conforming", false, "withdrawn"},
		{"tagged with a claim", "tagged", true, "withdrawn"},
	} {
		t.Run(test.name, func(t *testing.T) {
			pdf := claimTestPDF(t)
			webReport := ""
			if test.claim {
				webReport = claimTestFile(t, "WCAG 2.2 Level AA")
			}
			report := tagPDFClaim(claimTestReport(pdf, test.status), t.TempDir(), webReport)
			if got := identificationOf(report); got != test.want {
				t.Fatalf("identification %v, want %s", got, test.want)
			}
			data, err := os.ReadFile(pdf)
			if err != nil {
				t.Fatal(err)
			}
			if declared := strings.Contains(string(data), "pdfuaid:part"); declared != (test.want == "kept") {
				t.Fatalf("the file %s its PDF/UA-2 identification", map[bool]string{true: "still declares", false: "no longer declares"}[declared])
			}
		})
	}
}

// The export root's own accessibility.json is the claim tag-pdf judges against, as it is for a
// conversion: --web-report only stands in when the root reaches none.
func TestTagPDFClaimReadsTheExportRootFirst(t *testing.T) {
	pdf := claimTestPDF(t)
	root := t.TempDir()
	data, err := json.Marshal(map[string]any{"claim": "WCAG 2.2 Level AA"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "accessibility.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	report := tagPDFClaim(claimTestReport(pdf, "conforming"), root, claimTestFile(t, "WCAG 2.1 Level A"))
	identification, _ := report.Get("identification").(ojson.Object)
	if identification.Get("pdfuaid") != "kept" || !strings.Contains(rec.ToS(identification.Get("reason")), "WCAG 2.2 Level AA") {
		t.Fatalf("identification %v, want kept against the export root's claim", identification)
	}
}
