package pipeline

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/pdfua"
)

// The identification stays only in a conforming file (decision 112); a failed build has no file to
// change; a file whose identification cannot be taken out is reported as claiming what it did not
// earn (the withdrawal itself is tested on a PDF in pdftag).
func TestEarnedClaimKeepsTheIdentificationOnlyForAConformingFile(t *testing.T) {
	if _, err := exec.LookPath("qpdf"); err != nil {
		t.Skip("qpdf not installed")
	}
	directory := t.TempDir()
	pdf := filepath.Join(directory, "paper.pdf")
	if err := os.WriteFile(pdf, []byte("not a PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	conforming := EarnedClaim(ojson.Object{{"status", "conforming"}}, pdf, "WCAG 2.2 Level AA")
	if kept, _ := conforming.Get("identification").(ojson.Object); kept.Get("pdfuaid") != "kept" {
		t.Fatalf("conforming: %v", conforming)
	}
	// A conforming file beside a web page that claims nothing claims nothing either.
	alone := EarnedClaim(ojson.Object{{"status", "conforming"}}, pdf, "")
	if kept, _ := alone.Get("identification").(ojson.Object); kept.Get("pdfuaid") != "unearned" || alone.Get("status") != "conforming" {
		t.Fatalf("conforming without the web claim: %v", alone)
	}
	if failed := EarnedClaim(ojson.Object{{"status", "failed"}}, pdf, ""); failed.Has("identification") {
		t.Fatalf("failed: %v", failed)
	}
	if absent := EarnedClaim(reportWith(), filepath.Join(directory, "none.pdf"), ""); absent.Has("identification") {
		t.Fatalf("no file: %v", absent)
	}
	tagged := EarnedClaim(reportWith("pdfua.undescribed"), pdf, "WCAG 2.2 Level AA")
	identification, _ := tagged.Get("identification").(ojson.Object)
	found := pdfua.Diagnostics(tagged)
	if identification.Get("pdfuaid") != "unearned" || len(found) != 2 || found[1].Code != "pdfua.claimed" {
		t.Fatalf("unreadable tagged file: %v", tagged)
	}
}
