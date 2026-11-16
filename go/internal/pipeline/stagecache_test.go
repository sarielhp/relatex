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
