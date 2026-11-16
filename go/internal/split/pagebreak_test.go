package split

import (
	"strings"
	"testing"
)

// A page opens each section with its page marker (decision 081): the marker comes before the
// part's and the chapters' headings, and the split still reads those headings.
func TestPageMarkersDoNotHideHeadings(t *testing.T) {
	page := strings.NewReplacer(
		`<section id="s1" class="section"><h2 id="h1">`,
		`<section id="s1" class="section"><hr id="page-1" role="doc-pagebreak" aria-label="Page 1"/><h2 id="h1">`,
		`<section id="s2" class="section"><h2 id="h2a">`,
		`<section id="s2" class="section"><hr id="page-2" role="doc-pagebreak" aria-label="Page 2"/><h2 id="h2a">`,
		`<section id="s3" class="section"><h2 id="h3a">`,
		`<section id="s3" class="section"><hr id="page-3" role="doc-pagebreak" aria-label="Page 3"/><h2 id="h3a">`,
	).Replace(bookPage)
	files, reason := Write(bookDocument(bookCounters), page)
	if files == nil {
		t.Fatalf("no split: %s", reason)
	}
	names := []string{}
	for _, file := range files {
		names = append(names, file.Name+"|"+file.Title)
	}
	want := "index.html|Contents front.html|Front matter preface.html|Preface part-1.html|Part I chapter-01.html|1 Grids " +
		"chapter-02.html|2 Packing bibliography.html|Bibliography"
	if strings.Join(names, " ") != want {
		t.Fatalf("files: %v", names)
	}
	if err := Check(page, files); err != nil {
		t.Fatalf("check: %v", err)
	}
}

// An element other than a heading still ends the run: an hr that is no page marker leaves the
// section without a leading heading, as before.
func TestARuleWithoutThePageMarkerEndsTheHeadings(t *testing.T) {
	page := strings.Replace(bookPage, `<section id="s3" class="section"><h2 id="h3a">2</h2>`,
		`<section id="s3" class="section"><hr/><h2 id="h3a">2</h2>`, 1)
	files, reason := Write(bookDocument(bookCounters), page)
	if files == nil {
		t.Fatalf("no split: %s", reason)
	}
	if files[5].Name != "untitled.html" || files[5].Title != "Untitled" {
		t.Errorf("the chapter that opens with a rule is %s|%s", files[5].Name, files[5].Title)
	}
	if err := Check(page, files); err != nil {
		t.Fatalf("check: %v", err)
	}
}
