package bundle

import (
	"os"
	"path/filepath"

	"github.com/sarielhp/relatex/internal/rec"
)

// LayoutWriter stores a layout's pages as part files while they are bound, one page at a time, as
// storeLayout cuts a whole layout: the same chapter spans, the same part budget, the same bytes. A
// book's layout never has to be in memory (a 440-page book's pages were 5 GB of maps).
type LayoutWriter struct {
	directory string
	starts    map[int]bool // the pages a chapter begins on (0-based)
	count     int
	next      int
	pending   []any
	total     int
	first     int // 1-based number of the first pending page
	parts     []any
}

// PreparedPage is a page in its stored (compact) form with the size it counts for in a part.
type PreparedPage struct {
	page any
	size int
}

// NewLayoutWriter writes the part files of a layout of count pages into directory/layout, cut at the
// pages the chapters begin on (1-based, as storeLayout takes them).
func NewLayoutWriter(directory string, count int, chapters []int64) *LayoutWriter {
	if err := os.MkdirAll(filepath.Join(directory, LayoutDirectory), 0o755); err != nil {
		check(err)
	}
	starts := map[int]bool{}
	for _, chapter := range chapterSpans(count, chapters) {
		starts[chapter.first] = true
	}
	return &LayoutWriter{directory: directory, starts: starts, count: count, first: 1}
}

// Prepare compacts a page and measures it; it may run for several pages at once.
func (w *LayoutWriter) Prepare(page rec.Map) any {
	compacted, failure := mapPage(page, compactGlyph)
	if failure != nil {
		panic(failure)
	}
	return PreparedPage{page: compacted, size: len(marshal(compacted))}
}

// Add takes the prepared pages in page order.
func (w *LayoutWriter) Add(prepared any) {
	item := prepared.(PreparedPage)
	if len(w.pending) > 0 && (w.starts[w.next] || w.total+item.size > partBudget) {
		w.flush()
	}
	w.pending = append(w.pending, item.page)
	w.total += item.size
	w.next++
}

func (w *LayoutWriter) flush() {
	if len(w.pending) == 0 {
		return
	}
	w.parts = append(w.parts, writePart(w.directory, w.pending, w.first))
	w.first += len(w.pending)
	w.pending, w.total = nil, 0
}

// Finish writes the last part and returns the reference to the stored layout: layout (its data
// without pages) with its page count and parts. The directory is not in it; AttachLayout adds it.
func (w *LayoutWriter) Finish(layout rec.Map) rec.Map {
	w.flush()
	if w.next != w.count {
		rec.Fail("Layout pages stored %d of %d", w.next, w.count)
	}
	reference := without(layout, "pages")
	reference["storage"], reference["page_count"], reference["parts"] = storedVersion, int64(w.count), w.parts
	return reference
}

// AttachLayout checks the reference to a stored layout as a bundle read finds it and records the
// directory (holding layout/pages-*.json.gz) its parts are read from.
func AttachLayout(layout rec.Map, directory string) rec.Map {
	attached, _ := attachLayout(layout, directory).(rec.Map)
	return attached
}

// WithoutDirectory is a stored layout's reference as a bundle keeps it, without the directory its
// parts are read from here; a layout held whole is returned as it is.
func WithoutDirectory(layout rec.Map) rec.Map {
	if !Stored(layout) {
		return layout
	}
	return without(layout, directoryKey)
}

// WholeLayout is a layout with its pages in it: a stored layout's parts read and joined (the memory
// the parts exist to spare, for the few readers that need every page at once); any other is returned
// as it is.
func WholeLayout(layout rec.Map) rec.Map {
	if layout == nil || !Stored(layout) {
		return layout
	}
	pages := make([]any, 0, LayoutPageCount(layout))
	EachLayoutPages(layout, func(_ int, part []any) { pages = append(pages, part...) })
	whole := without(layout, directoryKey, "parts", "storage", "page_count")
	whole["pages"] = pages
	return whole
}
