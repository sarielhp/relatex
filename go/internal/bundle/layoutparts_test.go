package bundle

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

func TestChapterSpansCutAtTheChapters(t *testing.T) {
	got := chapterSpans(10, []int64{7, 3, 3, 1, 11})
	want := []span{{0, 2}, {2, 6}, {6, 10}}
	if !slices.Equal(got, want) {
		t.Fatalf("spans %v, want %v", got, want)
	}
	if got := chapterSpans(4, nil); !slices.Equal(got, []span{{0, 4}}) {
		t.Fatalf("a paper is one span: %v", got)
	}
}

func TestBudgetSpansKeepEachPartWithinTheBudget(t *testing.T) {
	got := budgetSpans([]int{4, 4, 4, 9, 1, 1}, 8)
	want := []span{{0, 2}, {2, 3}, {3, 4}, {4, 6}}
	if !slices.Equal(got, want) {
		t.Fatalf("spans %v, want %v", got, want)
	}
}

// bookLayout is codecLayout with count numbered pages.
func bookLayout(t *testing.T, count int) rec.Map {
	t.Helper()
	layout := codecLayout(t)
	template := rec.AsMap(rec.FetchList(layout, "pages")[0])
	pages := make([]any, count)
	for index := range pages {
		page := maps.Clone(template)
		page["number"] = int64(index + 1)
		pages[index] = page
	}
	layout["pages"] = pages
	layout["pdf_sha256"] = strings.Repeat("a", 64)
	return layout
}

// throughJSON is a value as a reader decodes it.
func throughJSON(t *testing.T, value any) rec.Map {
	t.Helper()
	decoded, err := canonjson.Decode(bytes.NewReader(canonical(t, value)))
	if err != nil {
		t.Fatal(err)
	}
	return rec.AsMap(decoded)
}

// storedBook writes a seven-page book with chapters on pages 3 and 6, the part budget small
// enough that the first chapter (pages 3 to 5) takes two parts.
func storedBook(t *testing.T, directory string) (rec.Map, rec.Map) {
	t.Helper()
	layout := bookLayout(t, 7)
	page := rec.AsMap(rec.FetchList(mapLayoutPages(rec.Map{"pages": rec.FetchList(layout, "pages")[:1]}, compactGlyph).(rec.Map), "pages")[0])
	previous := partBudget
	partBudget = 2 * len(canonical(t, page))
	defer func() { partBudget = previous }()
	return layout, rec.AsMap(storeLayout(layout, directory, []int64{3, 6}))
}

func TestStoredLayoutIsReadBackPartByPart(t *testing.T) {
	directory := t.TempDir()
	layout, reference := storedBook(t, directory)
	if _, whole := reference["pages"]; whole || reference["storage"] != storedVersion || reference["page_count"] != int64(7) {
		t.Fatalf("reference %v", reference)
	}
	files := []string{}
	for _, part := range rec.FetchList(reference, "parts") {
		files = append(files, rec.FetchString(rec.AsMap(part), "file"))
	}
	want := []string{"layout/pages-0001-0002.json.gz", "layout/pages-0003-0004.json.gz", "layout/pages-0005-0005.json.gz",
		"layout/pages-0006-0007.json.gz"}
	if !slices.Equal(files, want) {
		t.Fatalf("parts %v, want %v", files, want)
	}
	read := rec.AsMap(attachLayout(throughJSON(t, reference), directory))
	if !Stored(read) || LayoutPageCount(read) != 7 {
		t.Fatalf("read %v", read)
	}
	pages, visits := []any{}, 0
	EachLayoutPages(read, func(first int, part []any) {
		if first != len(pages) {
			t.Fatalf("part begins at %d after %d pages", first, len(pages))
		}
		visits++
		pages = append(pages, part...)
	})
	if visits != 4 || !bytes.Equal(canonical(t, pages), canonical(t, layout["pages"])) {
		t.Fatalf("%d visits; pages differ from the layout written", visits)
	}
}

func TestStoredLayoutPartsAreCopiedBesideAnotherBundle(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	_, reference := storedBook(t, first)
	read := rec.AsMap(attachLayout(throughJSON(t, reference), first))
	copied := storeLayout(read, second, nil)
	if !bytes.Equal(canonical(t, copied), canonical(t, reference)) {
		t.Fatalf("copied reference differs:\n%s\n%s", canonical(t, copied), canonical(t, reference))
	}
	for _, part := range rec.FetchList(reference, "parts") {
		name := filepath.FromSlash(rec.FetchString(rec.AsMap(part), "file"))
		original, _ := os.ReadFile(filepath.Join(first, name))
		if copied, err := os.ReadFile(filepath.Join(second, name)); err != nil || !bytes.Equal(copied, original) {
			t.Fatalf("part %s not copied: %v", name, err)
		}
	}
}

func TestAChangedPartIsRefused(t *testing.T) {
	directory := t.TempDir()
	_, reference := storedBook(t, directory)
	part := rec.AsMap(rec.FetchList(reference, "parts")[1])
	path := filepath.Join(directory, filepath.FromSlash(rec.FetchString(part, "file")))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)/2] ^= 1
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	read := rec.AsMap(attachLayout(throughJSON(t, reference), directory))
	failure := catchFailure(func() { EachLayoutPages(read, func(int, []any) {}) })
	if failure == nil || !strings.HasPrefix(failure.Message, "Layout part hash differs") {
		t.Fatalf("got %v", failure)
	}
}

func TestPartsMustCoverThePagesInOrder(t *testing.T) {
	_, reference := storedBook(t, t.TempDir())
	cases := map[string]func(rec.Map){
		"gap":       func(r rec.Map) { r["parts"] = slices.Delete(rec.FetchList(r, "parts"), 1, 2) },
		"count":     func(r rec.Map) { r["page_count"] = int64(8) },
		"escape":    func(r rec.Map) { rec.AsMap(rec.FetchList(r, "parts")[0])["file"] = "../pages-0001-0002.json.gz" },
		"directory": func(r rec.Map) { r[directoryKey] = "/tmp" },
	}
	for name, change := range cases {
		changed := throughJSON(t, reference)
		change(changed)
		if failure := catchFailure(func() { attachLayout(changed, "/nonexistent") }); failure == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// catchFailure runs run and returns its Failure, nil when it returns; other panics go on.
func catchFailure(run func()) (caught *rec.Failure) {
	defer func() {
		if value := recover(); value != nil {
			failure, ok := value.(rec.Failure)
			if !ok {
				panic(value)
			}
			caught = &failure
		}
	}()
	run()
	return nil
}
