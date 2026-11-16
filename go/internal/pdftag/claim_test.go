package pdftag

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// kernelPacket is the XMP the LaTeX kernel writes for pdfstandard=ua-2, cut to what matters: the
// identification, the Well-Tagged PDF declarations, and the title that must stay.
const kernelPacket = `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about="" xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/"
    xmlns:pdfd="http://pdfa.org/declarations/" xmlns:dc="http://purl.org/dc/elements/1.1/">
   <pdfuaid:part>2</pdfuaid:part>
   <pdfuaid:rev>2024</pdfuaid:rev>
   <pdfd:declarations>
    <rdf:Bag>
     <rdf:li rdf:parseType="Resource">
      <pdfd:conformsTo>http://pdfa.org/declarations/wtpdf#accessibility1.0</pdfd:conformsTo>
     </rdf:li>
    </rdf:Bag>
   </pdfd:declarations>
   <dc:title><rdf:Alt><rdf:li xml:lang="x-default">Sums</rdf:li></rdf:Alt></dc:title>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`

func TestStripClaimKeepsTheRestOfThePacket(t *testing.T) {
	stripped, changed := StripClaim([]byte(kernelPacket))
	if !changed || Claims(stripped) {
		t.Fatalf("claim left in:\n%s", stripped)
	}
	for _, kept := range []string{"<dc:title>", "Sums", `xmlns:pdfuaid=`, "<?xpacket end"} {
		if !bytes.Contains(stripped, []byte(kept)) {
			t.Errorf("%q lost:\n%s", kept, stripped)
		}
	}
	attributes := `<rdf:Description rdf:about="" pdfuaid:part="2" pdfuaid:rev="2024" dc:x="1"/>`
	if got, _ := StripClaim([]byte(attributes)); string(got) != `<rdf:Description rdf:about="" dc:x="1"/>` {
		t.Errorf("attribute form: %s", got)
	}
	if _, changed := StripClaim(stripped); changed {
		t.Error("a packet without a claim changed")
	}
}

// writeTaggedFixture writes a one-page PDF whose catalog carries kernelPacket, rewritten by qpdf so
// its cross-reference table is sound.
func writeTaggedFixture(t *testing.T, qpdf, directory string) string {
	t.Helper()
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R /MarkInfo << /Marked true >> >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>",
		fmt.Sprintf("<< /Type /Metadata /Subtype /XML /Length %d >>\nstream\n%s\nendstream", len(kernelPacket)+1, kernelPacket),
	}
	var out strings.Builder
	out.WriteString("%PDF-2.0\n")
	offsets := []int{}
	for index, object := range objects {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	start := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, start)
	raw, pdf := filepath.Join(directory, "raw.pdf"), filepath.Join(directory, "paper.pdf")
	if err := os.WriteFile(raw, []byte(out.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if message, err := runQPDF(qpdf, "--object-streams=generate", raw, pdf); err != nil {
		t.Fatalf("qpdf: %v: %s", err, message)
	}
	return pdf
}

func TestWithdrawClaimTakesTheIdentificationOutOfTheFile(t *testing.T) {
	qpdf, err := exec.LookPath("qpdf")
	if err != nil {
		t.Skip("qpdf not installed")
	}
	pdf := writeTaggedFixture(t, qpdf, t.TempDir())
	before, err := Identification(pdf)
	if err != nil || !Claims(before) {
		t.Fatalf("fixture states no claim (%v):\n%s", err, before)
	}
	withdrawn, err := WithdrawClaim(pdf)
	if err != nil || !withdrawn {
		t.Fatalf("WithdrawClaim = %v, %v", withdrawn, err)
	}
	after, err := Identification(pdf)
	if err != nil || Claims(after) || !bytes.Contains(after, []byte("Sums")) {
		t.Fatalf("after withdrawal (%v):\n%s", err, after)
	}
	if again, err := WithdrawClaim(pdf); err != nil || again {
		t.Fatalf("second withdrawal = %v, %v", again, err)
	}
	if message, err := runQPDF(qpdf, "--check", pdf); err != nil {
		t.Fatalf("qpdf --check: %v: %s", err, message)
	}
}
