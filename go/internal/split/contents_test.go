package split

import (
	"strings"
	"testing"
)

// sectionedPage is bookPage with a section in chapter 1 that its contents entry lists.
var sectionedPage = strings.Replace(strings.Replace(bookPage, `<p id="q2">Second.</p>`,
	`<section id="s2s" class="section"><h3 id="h2s">1.1 Cells</h3><p id="q2">Second.</p></section>`, 1),
	`<li><a href="#h2a">1 Grids</a></li>`, `<li><a href="#h2a">1 Grids</a><ol><li><a href="#h2s">1.1 Cells</a></li></ol></li>`, 1)

func TestContentsPageListsTheFilesAndChaptersTheirSections(t *testing.T) {
	files, reason := Write(bookDocument(bookCounters), sectionedPage)
	if files == nil {
		t.Fatalf("no split: %s", reason)
	}
	contents, chapter := files[0].HTML, files[4].HTML
	if strings.Contains(contents, "1.1 Cells") || !strings.Contains(contents, `<a href="chapter-01.html#h2a">1 Grids</a></li>`) {
		t.Errorf("the contents page keeps the chapter's sections: %s", contents)
	}
	if !strings.Contains(contents, `<a href="part-1.html#h1">Part I</a><ol>`) {
		t.Errorf("the contents page lost the part's chapters")
	}
	if !strings.Contains(chapter, `data-split-navigation="sections" aria-label="Sections"><ol><li><a href="#h2s">1.1 Cells</a>`) {
		t.Errorf("chapter 1 lacks its sections list: %s", chapter)
	}
	if err := Check(sectionedPage, files); err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, tamper := range [][2]string{
		{`1 Grids</a></li>`, `1 Grids</a><ol><li><a href="chapter-01.html#h2s">1.1 Cells</a></li></ol></li>`},
		{`<li><a href="part-1.html#h1">Part I</a><ol><li><a href="chapter-01.html#h2a">1 Grids</a></li></ol></li>`,
			`<li><a href="part-1.html#h1">Part I</a></li>`},
	} {
		changed := append([]File(nil), files...)
		changed[0].HTML = strings.Replace(contents, tamper[0], tamper[1], 1)
		if changed[0].HTML == contents {
			t.Fatalf("tamper %q did not apply", tamper[0])
		}
		if err := Check(sectionedPage, changed); err == nil {
			t.Errorf("a contents page changed to %q passed the check", tamper[1])
		}
	}
}
