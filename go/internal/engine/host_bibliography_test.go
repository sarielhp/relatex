package engine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestWithBibliographyReadsTheCallersBibinputs(t *testing.T) {
	shared := t.TempDir()
	host := WithBibliography(nil, shared+":", "", t.TempDir())
	if host == nil || !slices.Contains(host.BibPath, shared) {
		t.Fatalf("BibPath = %v, want it to hold %s", host, shared)
	}
	if mounts := host.mounts(); !slices.Contains(mounts, shared) {
		t.Fatalf("mounts = %v, want the BIBINPUTS directory shown to the sandbox", mounts)
	}
	if WithBibliography(nil, "", "", "") != nil {
		t.Fatal("no BIBINPUTS and no BSTINPUTS must leave the host nil")
	}
}

func TestBibliographyPathKeepsOwnDirectoriesFirst(t *testing.T) {
	got := bibliographyPath([]string{"/shared/bib", "", "."}, "/work", "/shared/bib")
	if !strings.HasPrefix(got, "/work:/shared/bib") || !strings.HasSuffix(got, ":") {
		t.Fatalf("bibliographyPath = %q", got)
	}
	if strings.Count(got, "/shared/bib") != 1 {
		t.Fatalf("bibliographyPath repeats an entry: %q", got)
	}
}

func TestFingerprintCoversBibliographyDirectories(t *testing.T) {
	shared := t.TempDir()
	host := WithBibliography(nil, shared, "", shared)
	before := host.Fingerprint()
	if err := os.WriteFile(filepath.Join(shared, "x.bib"), []byte("@misc{a}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if host.Fingerprint() == before {
		t.Fatal("a new database in BIBINPUTS must change the host's fingerprint")
	}
}
