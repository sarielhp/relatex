package nativetrace

import (
	"fmt"
	"strings"
	"testing"
)

// A large trace read page by page gives each page the tree the whole read gives it; a small or odd
// trace is left to the whole read.
func TestSplitPagesGiveTheWholeReadsPages(t *testing.T) {
	var text strings.Builder
	text.WriteString("<?xml version=\"1.0\"?>\n<document filename=\"x.pdf\">\n")
	for page := 1; page <= 3000; page++ {
		fmt.Fprintf(&text, "<page id=\"page%d\" width=\"595\">\n<block bbox=\"1 2 3 4\">\n<char c=\"a&amp;b\" quad=\"1 2 3 4 5 6 7 8\"/>\n</block>\n</page>\n", page)
	}
	text.WriteString("</document>\n")
	data := []byte(text.String())
	whole, err := ParseTree(data)
	if err != nil {
		t.Fatal(err)
	}
	wanted := whole.Descendants("page")
	split := splitPages(data)
	if split == nil || split.Len() != len(wanted) {
		t.Fatalf("split %v", split)
	}
	for _, index := range []int{0, 1, 1499, 2999} {
		got := split.Page(index)
		if got.Name != "page" || got.Attrs["id"] != wanted[index].Attrs["id"] || len(got.Descendants("char")) != 1 {
			t.Fatalf("page %d differs", index+1)
		}
		if char := got.Descendants("char")[0]; char.Attrs["c"] != "a&b" {
			t.Fatalf("page %d glyph %v", index+1, char.Attrs)
		}
	}
	if SplitPages(data) != nil || splitPages([]byte("<document>"+strings.Repeat(" ", 100)+"</document>")) != nil {
		t.Fatal("a small trace or one without pages was split")
	}
}
