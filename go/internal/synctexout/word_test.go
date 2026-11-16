package synctexout

import (
	"strings"
	"testing"
)

func TestColumnMarkersSitJustInsideBothEdges(t *testing.T) {
	got := columnMarkers("1,54,784", 1000, 5000, 700)
	if len(got) != 2 || string(got[0]) != "$1,54,784:1001,700\n" || string(got[1]) != "$1,54,784:4999,700\n" {
		t.Fatalf("markers %q", got)
	}
}

func TestColumnMarkersSkipABoxTooNarrowToHoldThem(t *testing.T) {
	if got := columnMarkers("1,54,784", 1000, 1003, 700); got != nil {
		t.Fatalf("markers %q for a 3 unit box", got)
	}
}

func TestInputNamesDefaultToThePackagedSources(t *testing.T) {
	w := &wordSynctex{inputName: func(name string) string { return "sources/" + name }}
	if got := w.inputName("a/b.tex"); !strings.HasPrefix(got, "sources/") {
		t.Fatalf("input name %q", got)
	}
}
