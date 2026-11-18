package pipeline

import (
	"path/filepath"
	"reflect"
	"testing"
)

// A recorder the author shipped with the source (third cohort 781) records another machine's
// build: its inputs are not this stage's dependencies, and opening them would fail.
func TestExternalDependenciesIgnoreAShippedRecorder(t *testing.T) {
	stage := t.TempDir()
	working := filepath.Join(stage, "probe", "external", "working")
	putFile(t, stage, "probe/input/files/main.fls",
		"PWD /Users/author/paper\nINPUT /Users/author/paper/Definitions.sty\n")
	// TeX names files with "/" on every platform; on Windows an absolute name has a drive.
	article := filepath.Join(filepath.VolumeName(stage)+string(filepath.Separator), "usr", "share", "texmf", "article.cls")
	putFile(t, stage, "probe/external/output/main.fls",
		"PWD "+working+"\nINPUT "+working+"/main.tex\nINPUT "+filepath.ToSlash(article)+"\nINPUT ./Definitions.sty\n")
	got := externalDependencies(stage, stage)
	want := []string{article}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dependencies = %v, want %v", got, want)
	}
}

func TestPadLastMissRestoresTheOmittedAnswer(t *testing.T) {
	got := padLastMiss([]string{"/a.sty"}, 2)
	if len(got) != 2 || got[1] != "" {
		t.Fatalf("padLastMiss = %q, want the missing last name padded", got)
	}
	if got := padLastMiss([]string{"/a.sty", "/b.sty"}, 2); len(got) != 2 {
		t.Fatalf("padLastMiss padded a complete answer: %q", got)
	}
}

func TestLookupMismatchNamesTheReason(t *testing.T) {
	if got := lookupMismatch(nil, 1, 2, 0); got == "" {
		t.Fatal("lookupMismatch returned an empty reason")
	}
}
