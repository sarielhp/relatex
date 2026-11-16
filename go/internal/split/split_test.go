package split

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// bookPage is a small book: front matter, an unnumbered chapter, a part with two chapters (the
// second refers to the first and to a note), the bibliography, notes after the document, and the
// contents after main.
const bookPage = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"/><title>A Book</title><style>nav > ol { margin: 0 }</style><script src="preview.js" defer="defer"></script></head><body><a class="skip-link" href="#main">Skip to main content</a><p role="status">Reading copy.</p><p class="page-links"><a href="#contents">Contents</a></p><main id="main"><div id="d" data-structure="document" class="document"><p id="t">Title page &amp; more</p>
<section id="s0" class="section"><h2 id="h0">Preface</h2><p id="p0">Words.</p></section>
<section id="s1" class="section"><h2 id="h1">Part I</h2><p id="p1">Part opening.</p><section id="s2" class="section"><h2 id="h2a"> 1 </h2><h2 id="h2b">Grids</h2><p id="p2">See <a href="#n1">1</a>.</p><p id="q2">Second.</p></section><section id="s3" class="section"><h2 id="h3a">2</h2><h2 id="h3b">Packing</h2><p id="p3">By <a href="#h2b">Chapter 1</a>, <a href="#p0">the preface</a> and <a href="#p3">here</a>.</p></section></section>
<section id="s4" class="section"><h2 id="h4">Bibliography</h2><ol><li id="b1">An entry.</li></ol></section>
</div><aside id="n1" data-note="1.1"><a href="#p2" data-note-backref="">↩</a> A note.</aside></main><nav id="contents" aria-label="Contents"><h2>Contents</h2><ol><li><a href="#h0">Preface</a></li><li><a href="#h1">Part I</a><ol><li><a href="#h2a">1 Grids</a></li></ol></li></ol></nav></body></html>
`

func bookDocument(counters map[string]string) rec.Map {
	nodes := rec.Map{}
	order := []any{}
	for _, id := range []string{"h0", "h1", "h2a", "h2b", "h3a", "h3b", "h4"} {
		nodes[id] = rec.Map{"type": "heading", "semantics": rec.Map{"counter": counters[id]}}
		order = append(order, id)
	}
	return rec.Map{"nodes": nodes, "main_block_order": order}
}

var bookCounters = map[string]string{"h0": "chapter", "h1": "part", "h2a": "chapter", "h2b": "chapter", "h3a": "chapter",
	"h3b": "chapter", "h4": "chapter"}

func TestWriteSplitsABookByChapter(t *testing.T) {
	files, reason := Write(bookDocument(bookCounters), bookPage)
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
	if err := Check(bookPage, files); err != nil {
		t.Fatalf("check: %v", err)
	}
	chapter2 := files[5].HTML
	for _, expected := range []string{`href="chapter-01.html#h2b"`, `href="preface.html#p0"`, `href="#p3"`,
		`data-preview-for="h2b"`, `<h2>Grids</h2><p>See`, `data-split-continued="s1"`, `data-split-continued="d"`,
		`<title>2 Packing — A Book</title>`, `href="index.html#contents"`, `href="chapter-01.html" rel="prev"`} {
		if !strings.Contains(chapter2, expected) {
			t.Errorf("chapter 2 lacks %s", expected)
		}
	}
	if !strings.Contains(files[4].HTML, `<aside id="n1"`) || strings.Contains(files[0].HTML, "<main") {
		t.Errorf("the note belongs with its chapter, and the contents page has no main")
	}
	if !strings.Contains(files[0].HTML, `<a href="chapter-01.html#h2a">`) {
		t.Errorf("contents links point into the files")
	}
	if !strings.Contains(files[0].HTML, `nav > ol { margin: 0 }`) {
		t.Errorf("the stylesheet keeps its text")
	}
	// The skip link leads to each file's own main; the contents page has none, so to the contents.
	if !strings.Contains(files[0].HTML, `<a class="skip-link" href="#contents">`) {
		t.Errorf("the contents page's skip link leads to the contents")
	}
	for _, file := range files[1:] {
		if !strings.Contains(file.HTML, `<a class="skip-link" href="#main">`) || !strings.Contains(file.HTML, `<main id="main">`) {
			t.Errorf("%s: the skip link leads to the file's main", file.Name)
		}
	}
}

// A page whose notice is its banner (decision 095): the contents page opens that one header with
// the book's title, so it has one banner landmark, and the check still proves it faithful.
func TestContentsPageTitleJoinsTheBanner(t *testing.T) {
	page := strings.Replace(bookPage, `<p role="status">Reading copy.</p><p class="page-links"><a href="#contents">Contents</a></p>`,
		`<header><p role="status">Reading copy.</p><p class="page-links"><a href="#contents">Contents</a></p></header>`, 1)
	files, reason := Write(bookDocument(bookCounters), page)
	if files == nil {
		t.Fatalf("no split: %s", reason)
	}
	if err := Check(page, files); err != nil {
		t.Fatalf("check: %v", err)
	}
	contents := files[0].HTML
	if strings.Count(contents, "<header") != 1 || !strings.Contains(contents, `<header><h1 data-split-navigation="title">A Book</h1><p role="status">`) {
		t.Errorf("contents page banner:\n%s", contents)
	}
	if strings.Count(files[4].HTML, "<header>") != 1 {
		t.Errorf("chapter banner:\n%s", files[4].HTML)
	}
}

func TestCheckRefusesAStraySkipLink(t *testing.T) {
	files, _ := Write(bookDocument(bookCounters), bookPage)
	files[0].HTML = strings.Replace(files[0].HTML, `class="skip-link" href="#contents"`, `class="skip-link" href="#h0"`, 1)
	if err := Check(bookPage, files); err == nil {
		t.Fatalf("a contents page skip link elsewhere passed")
	}
}

func TestCheckFindsAnUnfaithfulSplit(t *testing.T) {
	files, _ := Write(bookDocument(bookCounters), bookPage)
	for _, tamper := range []struct {
		index    int
		from, to string
	}{
		{4, "Second.", "Changed."},
		{5, `href="chapter-01.html#h2b"`, `href="chapter-01.html#missing"`},
		{6, `<li id="b1">An entry.</li>`, ""},
		{0, `<p role="status">Reading copy.</p>`, ""},
	} {
		changed := append([]File(nil), files...)
		changed[tamper.index].HTML = strings.Replace(changed[tamper.index].HTML, tamper.from, tamper.to, 1)
		if changed[tamper.index].HTML == files[tamper.index].HTML {
			t.Fatalf("tamper %q did not apply", tamper.from)
		}
		if err := Check(bookPage, changed); err == nil {
			t.Errorf("tampering %q passed the check", tamper.from)
		}
	}
	if err := Check(bookPage, append(append([]File(nil), files[:4]...), files[5:]...)); err == nil {
		t.Errorf("a missing chapter passed the check")
	}
}

func TestWriteLeavesAPaperWhole(t *testing.T) {
	counters := map[string]string{}
	for id := range bookCounters {
		counters[id] = "section"
	}
	if files, reason := Write(bookDocument(counters), bookPage); files != nil || reason != "the document has no chapters" {
		t.Fatalf("a paper was split: %v %q", len(files), reason)
	}
}

func TestWriteKeepsCrossFileIDReferencesWhole(t *testing.T) {
	page := strings.Replace(bookPage, `<p id="q2">`, `<p id="q2" aria-describedby="p0">`, 1)
	if files, reason := Write(bookDocument(bookCounters), page); files != nil || !strings.Contains(reason, "#p0") {
		t.Fatalf("a cross-file id reference was split: %q", reason)
	}
}

func TestNames(t *testing.T) {
	for title, want := range map[string]string{"Errata for Geometric Approximation Algorithms": "errata-for-geometric-approximation",
		"Index": "book-index", "∑": "section"} {
		if got := slug(title); got != want {
			t.Errorf("slug(%q) = %q, want %q", title, got, want)
		}
	}
	if got := chapterNumber("1. Introduction") + chapterNumber("A. Proofs") + chapterNumber("A Survey"); got != "1A" {
		t.Errorf("chapter numbers %q", got)
	}
}
