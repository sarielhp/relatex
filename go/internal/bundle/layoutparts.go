package bundle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/zgzip"
)

// The bundle (schema bundle-1.2) keeps the layout's pages out of itself, in part files beside it
// (decision 087): layout/pages-FIRST-LAST.json.gz, each the compact pages (decision 085) of one
// part, chapter or page range of the document, gzip-compressed. The bundle's layout is a
// reference: the layout without its pages, the page count and, per part, its file, page range,
// SHA-256 and sizes. A reader loads one part at a time; no reader holds the whole layout.

const (
	// LayoutDirectory is the directory beside a bundle that holds its layout parts.
	LayoutDirectory = "layout"
	// storedVersion names the reference form of a stored layout (its "storage" field).
	storedVersion = "layout-parts-1"
	// partVersion is the schema of a part file.
	partVersion = "layout-part-1"
	// directoryKey holds, in a layout read from a bundle, the directory its parts are read from;
	// it is never stored.
	directoryKey = "directory"
)

// partBudget bounds a part's stored JSON: a chapter larger than it is cut into page ranges each
// at most this size (a single page may exceed it). A variable so tests can cut small layouts.
var partBudget = 64 << 20

// span is a range of page indexes, first included, last excluded.
type span struct{ first, last int }

// chapterSpans cuts count pages at the pages the chapters begin on (1-based, any order): the
// front matter, then one span per part or chapter; a document without chapters is one span.
func chapterSpans(count int, chapters []int64) []span {
	starts := []int{0}
	for _, page := range chapters {
		if page > 1 && page <= int64(count) {
			starts = append(starts, int(page-1))
		}
	}
	slices.Sort(starts)
	starts = slices.Compact(starts)
	spans := make([]span, len(starts))
	for index, start := range starts {
		last := count
		if index+1 < len(starts) {
			last = starts[index+1]
		}
		spans[index] = span{start, last}
	}
	return spans
}

// budgetSpans cuts pages into consecutive spans whose stored size stays within budget, each at
// least one page: a page goes into the current span unless that would exceed the budget.
func budgetSpans(sizes []int, budget int) []span {
	spans := []span{}
	start, total := 0, 0
	for index, size := range sizes {
		if index > start && total+size > budget {
			spans = append(spans, span{start, index})
			start, total = index, 0
		}
		total += size
	}
	if len(sizes) > start {
		spans = append(spans, span{start, len(sizes)})
	}
	return spans
}

// storeLayout writes the layout's pages as part files into directory/layout and returns the
// reference the bundle stores; a layout read from a bundle has its part files copied, each
// checked. The layout itself is not changed (the export formats read it meanwhile).
func storeLayout(layout any, directory string, chapters []int64) any {
	source, ok := layout.(rec.Map)
	if !ok || source == nil {
		return layout
	}
	target := filepath.Join(directory, LayoutDirectory)
	if _, err := os.Lstat(target); err == nil {
		rec.Fail("Layout parts destination exists")
	}
	check(os.MkdirAll(target, 0o755))
	if Stored(source) {
		return copyParts(source, directory)
	}
	pages := rec.FetchList(source, "pages")
	parts := []any{}
	for _, chapter := range chapterSpans(len(pages), chapters) {
		compacted := rec.FetchList(rec.AsMap(mapLayoutPages(rec.Map{"pages": pages[chapter.first:chapter.last]}, compactGlyph)), "pages")
		sizes := make([]int, len(compacted))
		for index, page := range compacted {
			sizes[index] = len(marshal(page))
		}
		for _, cut := range budgetSpans(sizes, partBudget) {
			parts = append(parts, writePart(directory, compacted[cut.first:cut.last], chapter.first+cut.first+1))
		}
	}
	reference := without(source, "pages")
	reference["storage"], reference["page_count"], reference["parts"] = storedVersion, int64(len(pages)), parts
	return reference
}

// writePart writes one part file, its pages numbered from first, and returns its reference.
func writePart(directory string, pages []any, first int) rec.Map {
	last := first + len(pages) - 1
	stored := rec.AsMap(shareLayoutSVG(rec.Map{"pages": pages}))
	stored["schema_version"], stored["first_page"], stored["last_page"] = partVersion, int64(first), int64(last)
	expanded := append(marshal(stored), '\n')
	if len(expanded) > maxBytes {
		rec.Fail("Layout part exceeds expanded byte limit: pages %d-%d, %d bytes", first, last, len(expanded))
	}
	compressed, err := zgzip.Compress(expanded)
	check(err)
	name := fmt.Sprintf("%s/pages-%04d-%04d.json.gz", LayoutDirectory, first, last)
	writeNew(filepath.Join(directory, filepath.FromSlash(name)), compressed)
	return rec.Map{"file": name, "first_page": int64(first), "last_page": int64(last), "sha256": hexSum(compressed),
		"bytes": int64(len(compressed)), "expanded_bytes": int64(len(expanded))}
}

// copyParts copies the part files of a layout read from a bundle beside another bundle, each
// checked against its reference, and returns the reference to store.
func copyParts(layout rec.Map, directory string) rec.Map {
	from := rec.FetchString(layout, directoryKey)
	for _, value := range rec.FetchList(layout, "parts") {
		part := rec.AsMap(value)
		name := rec.FetchString(part, "file")
		writeNew(filepath.Join(directory, filepath.FromSlash(name)), readPartFile(from, part))
	}
	return without(layout, directoryKey)
}

// writeNew writes data to a new file, through a temporary file linked into place.
func writeNew(path string, data []byte) {
	if _, err := os.Lstat(path); err == nil {
		rec.Fail("Bundle destination exists")
	}
	temporary := fmt.Sprintf("%s.tmp-%d", path, os.Getpid())
	defer os.Remove(temporary)
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	check(err)
	_, err = file.Write(data)
	check(errors.Join(err, file.Close()))
	if err := os.Link(temporary, path); err != nil {
		// FAT and exFAT have no hard links: the file is renamed into place instead, unless the
		// destination appeared meanwhile.
		if _, statErr := os.Lstat(path); statErr == nil {
			check(err)
		}
		check(os.Rename(temporary, path))
	}
}
