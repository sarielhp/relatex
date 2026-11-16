package nativetrace

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func sameTree(a, b *Node) bool {
	if a.Name != b.Name || !reflect.DeepEqual(a.Attrs, b.Attrs) || len(a.Children) != len(b.Children) {
		return false
	}
	for index := range a.Children {
		if a.Children[index].Parent != a || b.Children[index].Parent != b || !sameTree(a.Children[index], b.Children[index]) {
			return false
		}
	}
	return true
}

// A trace large enough to be read in pieces gives the tree the whole read gives.
func TestParseByPagesMatchesTheWholeRead(t *testing.T) {
	var text strings.Builder
	text.WriteString("<?xml version=\"1.0\"?>\n<document filename=\"x.pdf\">\n")
	for page := 1; page <= 3000; page++ {
		fmt.Fprintf(&text, "<page id=\"page%d\" width=\"595\">\n<block bbox=\"1 2 3 4\">\n<char c=\"a&amp;b\" quad=\"1 2 3 4 5 6 7 8\"/>\n<g/>\n</block>\n</page>\n", page)
	}
	text.WriteString("</document>\n")
	data := []byte(text.String())
	whole, err := ParseTree(data)
	if err != nil {
		t.Fatal(err)
	}
	pieces := cutAtPages(data[strings.Index(string(data), "<document"):], 4)
	if len(pieces) < 2 {
		t.Fatalf("cut into %d pieces", len(pieces))
	}
	got, err := parseByPagesForced(data)
	if err != nil {
		t.Fatal(err)
	}
	if !sameTree(whole, got) {
		t.Fatal("pieces differ from the whole read")
	}
	if os.Getenv("TRACE_FILE") != "" {
		traceData, _ := os.ReadFile(os.Getenv("TRACE_FILE"))
		a, errA := ParseTree(traceData)
		b, errB := parseByPages(traceData)
		if errA != nil || errB != nil || !sameTree(a, b) {
			t.Fatalf("real trace differs: %v %v", errA, errB)
		}
	}
}
