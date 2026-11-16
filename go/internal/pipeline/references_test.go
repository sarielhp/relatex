package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// page is a reading page that loads its script, a figure and two fonts, one of them only through
// a style attribute, and names files in places a page does not load from.
const page = `<!DOCTYPE html><html><head><style>/* see url(fonts/licenses/GUST.txt) */` +
	`@font-face{font-family:"a";src:url("fonts/a%20b.otf") format("opentype");}` +
	`@font-face{font-family:'L2D';src:url('fonts/math-accents.otf');}</style>` +
	`<script src="preview.js"></script></head><body><img src="figs/plot.svg" alt="">` +
	`<span style="background:url(&quot;figs/paint.png&quot;)">x</span>` +
	`<a href="supplement.pdf">s</a><a href="#s1">1</a><img src="data:image/png;base64,AA==" alt="">` +
	`<a href="https://example.org/x.css">e</a><p>src="nowhere.js" url(nothing.png)</p></body></html>`

func TestPageReferencesAreTheFilesThePageLoads(t *testing.T) {
	got := pageReferences(page)
	want := []string{"preview.js", "figs/plot.svg", "fonts/a b.otf", "fonts/math-accents.otf", "figs/paint.png"}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("pageReferences = %q, want %q", got, want)
	}
}

// stagedExport is a staged publication whose HTML export holds the page and everything it loads.
func stagedExport(t *testing.T) string {
	t.Helper()
	staged := t.TempDir()
	putFile(t, staged, "html/index.html", page)
	for _, name := range []string{"preview.js", "figs/plot.svg", "figs/paint.png", "fonts/a b.otf", "fonts/math-accents.otf"} {
		putFile(t, staged, "html/"+name, "x")
	}
	return staged
}

func TestPublishedRootPageFindsEveryFileItLoads(t *testing.T) {
	staged := stagedExport(t)
	publishPages(staged)
	if missing := missingReferences(staged); len(missing) > 0 {
		t.Fatalf("the root page misses %q", missing)
	}
	if !present(staged, "fonts/math-accents.otf") {
		t.Error("the root page's fonts were not published beside it")
	}
	checkPublishedReferences(staged)
}

func TestPublicationFailsWhenAPageLoadsAMissingFile(t *testing.T) {
	staged := stagedExport(t)
	publishPages(staged)
	putFile(t, staged, "chapter-01.html", `<link rel="stylesheet" href="fonts/gone.css"><img src="../outside.png">`)
	missing := missingReferences(staged)
	if !slices.Equal(missing, []string{"chapter-01.html: ../outside.png", "chapter-01.html: fonts/gone.css"}) {
		t.Errorf("missingReferences = %q", missing)
	}
	defer func() {
		failure := recover()
		if failure == nil || !strings.Contains(failure.(rec.Failure).Message, "chapter-01.html: fonts/gone.css") {
			t.Errorf("checkPublishedReferences did not fail on the missing file: %v", failure)
		}
	}()
	checkPublishedReferences(staged)
}
