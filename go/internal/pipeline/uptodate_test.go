package pipeline

import (
	"path/filepath"
	"testing"
)

func TestProductFilesAreBesideTheMainFile(t *testing.T) {
	got := productFiles("/papers/a", "sub/paper.tex")
	want := []string{"/papers/a/sub/paper.pdf", "/papers/a/sub/paper.synctex.gz", "/papers/a/sub/paper.bbl"}
	for index := range want {
		if filepath.ToSlash(got[index]) != want[index] {
			t.Fatalf("product %d is %q, want %q", index, got[index], want[index])
		}
	}
}

// A build pinned to another date prints another \today: the date is part of the engine stage's
// identity (cohort papers 15 and 42 reused a smoke build made on the run date).
func TestSourceDateIsPartOfTheBuildIdentity(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1792357709")
	t.Setenv("FORCE_SOURCE_DATE", "1")
	pinned := sourceDate()
	if len(pinned) != 2 || pinned.Get("SOURCE_DATE_EPOCH") != "1792357709" || pinned.Get("FORCE_SOURCE_DATE") != "1" {
		t.Fatalf("pinned date is %v", pinned)
	}
	t.Setenv("SOURCE_DATE_EPOCH", "1793232000")
	if digest(sourceDate()) == digest(pinned) {
		t.Fatal("two build dates share an identity")
	}
}
