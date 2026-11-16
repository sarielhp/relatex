package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRlxPublishesAndWritesSynctexByDefault(t *testing.T) {
	defer func(name, root string) { Name, Root = name, root }(Name, Root)
	Root = "../../.."
	Name = "rlx"
	o := conversionOptions(map[string]any{"cache": "auto"}, ".", "out", "work", nil)
	if o.Publish == nil || !*o.Publish || o.Synctex != "word" {
		t.Fatalf("rlx: publish %v synctex %q", o.Publish, o.Synctex)
	}
	o = conversionOptions(map[string]any{"cache": "auto", "publish": false}, ".", "out", "work", nil)
	if *o.Publish || o.Synctex != "none" {
		t.Fatalf("rlx --no-publish: publish %v synctex %q", *o.Publish, o.Synctex)
	}
	o = conversionOptions(map[string]any{"cache": "auto", "synctex": "native"}, ".", "out", "work", nil)
	if o.Synctex != "native" {
		t.Fatalf("an explicit SyncTeX choice was replaced: %q", o.Synctex)
	}
}

func TestRelatexLeavesProductsInTheOutputUnlessAskedToPublish(t *testing.T) {
	defer func(name, root string) { Name, Root = name, root }(Name, Root)
	Root = "../../.."
	Name = "relatex"
	o := conversionOptions(map[string]any{"cache": "auto"}, ".", "out", "work", nil)
	if *o.Publish || o.Synctex != "none" {
		t.Fatalf("relatex: publish %v synctex %q", *o.Publish, o.Synctex)
	}
	o = conversionOptions(map[string]any{"cache": "auto", "publish": true}, ".", "out", "work", nil)
	if !*o.Publish || o.Synctex != "word" {
		t.Fatalf("relatex --publish: publish %v synctex %q", *o.Publish, o.Synctex)
	}
}

func TestFriendlySummaryNamesTheFilesRelativeToTheWorkingDirectory(t *testing.T) {
	directory, _ := os.Getwd()
	got := friendlySummary([]string{filepath.Join(directory, "a.pdf"), "/elsewhere/b.synctex.gz"}, 47400*time.Millisecond)
	if got != "wrote a.pdf, /elsewhere/b.synctex.gz (47s)" {
		t.Fatalf("summary %q", got)
	}
	if friendlySummary(nil, time.Second) != "" {
		t.Fatal("a summary for nothing written")
	}
}
