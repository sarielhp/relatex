package nativetrace

import (
	"testing"

	"github.com/sarielhp/relatex/internal/xmltree"
)

// mutool writes a glyph mapped to a control character as a reference XML cannot hold (fifth
// cohort 1457: &#x15; in a trace, &#x0015; in a figure SVG), or a lone surrogate half (third
// cohort 802). Each reads as U+FFFD; legal references and malformed XML are left to the parser.
func TestUnreadableCharactersReadAsReplacement(t *testing.T) {
	cases := map[string]string{
		`<g unicode="&#x15;"/>`:                        "<g unicode=\"\uFFFD\"/>",
		`<use data-text="&#x0015;"/>`:                  "<use data-text=\"\uFFFD\"/>",
		`<g a="&#21;&#xD800;&#xFFFE;&#99999999999;"/>`: "<g a=\"\uFFFD\uFFFD\uFFFD\uFFFD\"/>",
		"<g a=\"\x01\" b=\"\xed\xa0\x80\"/>":           "<g a=\"\uFFFD\" b=\"\uFFFD\"/>",
		`<g a="&#x9;&#x2019;&#65;&amp;"/>`:             `<g a="&#x9;&#x2019;&#65;&amp;"/>`,
		"<g>\t\r\n</g>":                                "<g>\t\r\n</g>",
	}
	for input, want := range cases {
		if got := string(xmltree.Characters([]byte(input))); got != want {
			t.Errorf("Characters(%q) = %q, want %q", input, got, want)
		}
	}
	document := Parse([]byte(`<document><page><g unicode="&#x15;"/></page></document>`))
	if got := document.Descendants("g")[0].Attrs["unicode"]; got != "\uFFFD" {
		t.Fatalf("parsed unicode %q", got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("a malformed reference was read")
		}
	}()
	Parse([]byte(`<document a="&#xZZ;"/>`))
}

// MuPDF's trace writes a marked content's text into metatext's txt unescaped (third cohort 0639:
// an /Alt of "<exemplar demonstrations>"); the trace reads with the text as written and the
// elements around it unchanged.
func TestMetatextTextReadsAsWritten(t *testing.T) {
	trace := "<document>\n<page>\n<metatext type=\"alt\" txt=\"<a & \"b\">\">\n<fill_text/>\n</metatext>\n" +
		"<metatext type=\"alt\" txt=\"plain\">\n</metatext>\n</page>\n</document>\n"
	document := Parse([]byte(trace))
	texts := document.Descendants("metatext")
	if len(texts) != 2 || texts[0].Attrs["txt"] != `<a & "b">` || texts[1].Attrs["txt"] != "plain" {
		t.Fatalf("metatexts %v", texts)
	}
	if len(texts[0].Named("fill_text")) != 1 {
		t.Fatalf("fill_text not inside the metatext")
	}
	names := []string{}
	EachElement([]byte(trace), func(name string, _ map[string]string, _ []*Node) { names = append(names, name) })
	if len(names) != 5 {
		t.Fatalf("elements %v", names)
	}
	clean := []byte("<document><metatext type=\"alt\" txt=\"x\">\n</metatext></document>")
	if got := metatexts(clean); &got[0] != &clean[0] {
		t.Fatalf("clean trace copied")
	}
}
