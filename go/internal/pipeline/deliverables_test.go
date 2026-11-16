package pipeline

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/ojson"
)

func TestDeliveriesNameEachRequestedFormat(t *testing.T) {
	c := &conversion{pdf: "/x/paper.pdf", options: Options{Formats: []string{"typst-source", "html", "epub", "pdf-ua", "typst"}},
		exports: ojson.Object{{"typst-source", 1}, {"html", 1}, {"epub", 1}, {"pdf-ua", 1}}}
	names := []string{}
	for _, item := range c.deliveries("/p") {
		names = append(names, item.name)
	}
	if want := []string{"typst", "html", "paper.epub", "paper-tagged.pdf"}; !slices.Equal(names, want) {
		t.Errorf("deliveries %v, want %v (a format that was not written is left out)", names, want)
	}
}

func TestOwnedDeliveriesComeFromTheMarkerOrAnEarlierLayout(t *testing.T) {
	output := t.TempDir()
	if owned := ownedDeliveries(t.TempDir(), output, true); len(owned) != 0 {
		t.Errorf("an empty directory owns %v", owned)
	}
	putFile(t, output, "notes.txt", "mine")
	putFile(t, output, deliveredMarker, `{"entries":["html"]}`) // an earlier layout's marker
	if owned := ownedDeliveries(t.TempDir(), output, true); !slices.Equal(owned, []string{"html"}) {
		t.Errorf("owned %v", owned)
	}
	earlier := t.TempDir()
	putFile(t, earlier, "manifest.json", "{}")
	putFile(t, earlier, "index.html", "old")
	putFile(t, earlier, "_work/x", "x")
	if owned := ownedDeliveries(t.TempDir(), earlier, true); !slices.Contains(owned, "index.html") || slices.Contains(owned, "_work") {
		t.Errorf("an earlier layout owns %v", owned)
	}
}

func TestCheckDeliveriesRefusesAnEntryRelatexDidNotWrite(t *testing.T) {
	output := t.TempDir()
	putFile(t, output, "typst/mine.typ", "x")
	list := []delivery{{name: "typst"}}
	refused := func(owned []string) (refused bool) {
		defer func() { refused = recover() != nil }()
		checkDeliveries(output, list, owned)
		return false
	}
	if !refused(nil) {
		t.Error("a user's typst directory was accepted")
	}
	if refused([]string{"typst"}) {
		t.Error("relatex's own typst directory was refused")
	}
	if !present(output, filepath.Join("typst", "mine.typ")) {
		t.Error("the check changed the directory")
	}
}
