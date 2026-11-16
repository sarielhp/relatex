package split

import (
	"strings"
	"testing"
)

// TestStandardClassChapterNames: the book class prints "Chapter 1" above the title and
// "Appendix A" above an appendix's; the files are chapter-01.html and appendix-a.html.
func TestStandardClassChapterNames(t *testing.T) {
	page := strings.NewReplacer(`<h2 id="h2a"> 1 </h2>`, `<h2 id="h2a">Chapter 1</h2>`,
		`<h2 id="h3a">2</h2>`, `<h2 id="h3a">Appendix A</h2>`).Replace(bookPage)
	files, reason := Write(bookDocument(bookCounters), page)
	if files == nil {
		t.Fatalf("no split: %s", reason)
	}
	if files[4].Name != "chapter-01.html" || files[5].Name != "appendix-a.html" {
		t.Errorf("files %s and %s", files[4].Name, files[5].Name)
	}
	for heading, want := range map[string]string{"Chapter 12": "12", "Kapitel 3": "3", "Appendix B": "B", "1 Introduction": "1",
		"Chapter 1 First Steps": "1", "Further Reading": "", "A Survey of Methods": "", "Chapter One": ""} {
		if got := chapterNumber(heading); got != want {
			t.Errorf("chapterNumber(%q) = %q, want %q", heading, got, want)
		}
	}
}
