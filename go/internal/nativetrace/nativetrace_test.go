package nativetrace

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// The expectations are Ruby's Kernel#Float.
func TestFloatIsRubysKernelFloat(t *testing.T) {
	for text, want := range map[string]float64{"1": 1, " 2.5 ": 2.5, "-.5": -0.5, "1e3": 1000, "+4": 4} {
		if got := Float(text); got != want {
			t.Errorf("Float(%q) = %v, want %v", text, got, want)
		}
	}
	for _, text := range []string{"abc", "1.5.2", "inf", "nan", ""} {
		func() {
			defer func() {
				if _, ok := recover().(rec.ArgumentError); !ok {
					t.Errorf("Float(%q) did not raise an ArgumentError", text)
				}
			}()
			Float(text)
		}()
	}
}

func TestParseKeepsElementsInOrder(t *testing.T) {
	document := Parse([]byte(`<document><page n="1"><line><char c="a"/></line></page><page n="2"/></document>`))
	pages := document.Descendants("page")
	if len(pages) != 2 || pages[0].Attrs["n"] != "1" || len(pages[0].Descendants("char")) != 1 || pages[0].Parent.Name != "document" {
		t.Fatalf("unexpected tree: %+v", pages)
	}
}
