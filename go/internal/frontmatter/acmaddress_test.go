package frontmatter

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// An acmart affiliation reads as acmart.cls prints it (papers/1174 "Tokyo, Japan", was
// "Tokyo Japan"): conference formats end a line after the institution and join the address
// fields with commas, journal formats print the institution and ", country" only, and
// obeypunctuation keeps the author's own punctuation. The expectations match
// PaperTexMetadata (test/acm_address_test.rb).
func TestAcmAffiliationSeparators(t *testing.T) {
	affiliation := `\affiliation{
    \institution{The University of Hinode}
    \department{Dept}
    \city{Tokyo}
    \state{Kanto}
    \country{Japan}
}`
	cases := []struct{ class, affiliation, want string }{
		{"sigplan,10pt", affiliation, "The University of Hinode Dept Tokyo, Kanto, Japan"},
		{"format=sigconf", `\affiliation{\institution{U}\city{Tokyo}\country{Japan}}`, "U Tokyo, Japan"},
		{"acmsmall", affiliation, "The University of Hinode, Japan"},
		{"", `\affiliation{\position{Prof}\institution{U}\country{Japan}}`, "U, Japan"},
		{"sigconf", `\affiliation[obeypunctuation=true]{\institution{U},\\ \city{Tokyo} \country{Japan}}`, "U, Tokyo Japan"},
		{"acmsmall", `\affiliation[obeypunctuation]{\institution{U}; \country{Japan}}`, "U; Japan"},
		{"sigconf", `\affiliation[obeypunctuation=false]{\city{Tokyo}\country{Japan}}`, "Tokyo, Japan"},
	}
	for _, item := range cases {
		source := "\\documentclass[" + item.class + "]{acmart}\n\\title{T}\\author{Ann}" + item.affiliation +
			"\n\\begin{abstract}Text.\\end{abstract}\n\\maketitle\n"
		authors := rec.FetchList(NewTexMetadata(source).Project(), "authors")
		got := marshal(authors[0].(rec.Map)["affiliations"])
		if want := marshal([]any{item.want}); got != want {
			t.Errorf("%s %s: affiliations %s, want %s", item.class, item.affiliation, got, want)
		}
	}
}
