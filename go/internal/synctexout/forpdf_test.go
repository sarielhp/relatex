package synctexout

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/command"
)

func TestSynctexNamesTheSourcesThatChangedWhileItBuilt(t *testing.T) {
	directory := t.TempDir()
	for name, text := range map[string]string{"paper.tex": "x", "paper.bbl": "y"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sources := map[string]string{"paper.tex": command.FileDigest(filepath.Join(directory, "paper.tex")), "paper.bbl": "stale"}
	if got := ChangedSources(directory, sources); !slices.Equal(got, []string{"paper.bbl"}) {
		t.Fatalf("changed %q", got)
	}
	built := Built{Input: directory, Sources: sources}
	if reason := built.difference(filepath.Join(directory, "paper.pdf")); !strings.Contains(reason, "paper.bbl") ||
		!strings.Contains(reason, "bibliography") {
		t.Fatalf("reason %q", reason)
	}
	if Path("/a/paper.pdf") != "/a/paper.synctex.gz" {
		t.Fatal("the SyncTeX file sits beside the PDF")
	}
}

func TestSynctexRefusesAnotherPDFAndTakesTheSameBuild(t *testing.T) {
	directory := t.TempDir()
	pdf := filepath.Join(directory, "paper.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-theirs"), 0o644); err != nil {
		t.Fatal(err)
	}
	built := Built{Input: directory, PDF: []byte("%PDF-ours"), Scratch: func() string { return directory }}
	var refused *Refused
	err := built.Verify(pdf)
	if !errors.As(err, &refused) || !strings.HasPrefix(err.Error(), "refused: paper.pdf is not the PDF relatex built (") ||
		!strings.HasSuffix(err.Error(), "; paper.synctex.gz left as it was") {
		t.Fatalf("refusal %v", err)
	}
	built.PDF = []byte("%PDF-theirs")
	if err := built.Verify(pdf); err != nil {
		t.Fatalf("the same build is taken: %v", err)
	}
}

func TestSynctexTimesOutWithoutANewPDF(t *testing.T) {
	err := Await(filepath.Join(t.TempDir(), "paper.pdf"), time.Now(), 0)
	var timedOut *TimedOut
	if !errors.As(err, &timedOut) || err.Error() != "timed out: no paper.pdf written within 0s of the start" {
		t.Fatalf("timeout %v", err)
	}
}

func TestSynctexInstallTellsThePartialFile(t *testing.T) {
	directory := t.TempDir()
	told := []string{}
	name, err := Install(filepath.Join(directory, "paper.pdf"), []byte("word"), func(path string) { told = append(told, path) })
	if err != nil || name != "paper.synctex.gz" {
		t.Fatalf("installed %q, %v", name, err)
	}
	if len(told) != 2 || filepath.Base(told[0]) != "paper.synctex.gz.tmp" || told[1] != "" {
		t.Fatalf("partial files %q", told)
	}
}
