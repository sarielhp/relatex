package bundle

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/sourcemap"
	"github.com/sarielhp/relatex/internal/zgzip"
)

// partName is the only name a part file may have: no other path is read beside a bundle.
var partName = regexp.MustCompile(`\Alayout/pages-[0-9]{4,}-[0-9]{4,}\.json\.gz\z`)

// Stored is whether a layout is a bundle's reference to its part files rather than the whole
// layout in memory.
func Stored(layout rec.Map) bool {
	_, parted := layout["parts"]
	return parted
}

// LayoutPageCount is the number of pages of a layout, whole or stored.
func LayoutPageCount(layout rec.Map) int {
	if Stored(layout) {
		return int(rec.FetchInt(layout, "page_count"))
	}
	return len(rec.FetchList(layout, "pages"))
}

// EachLayoutPages calls visit with the layout's pages and the index of the first: a whole
// layout's at once, a stored layout's part by part, each read, checked and expanded when its
// turn comes and released after. Every reader of a stored layout goes through it.
func EachLayoutPages(layout rec.Map, visit func(first int, pages []any)) {
	if !Stored(layout) {
		visit(0, rec.FetchList(layout, "pages"))
		return
	}
	directory := rec.FetchString(layout, directoryKey)
	for _, value := range rec.FetchList(layout, "parts") {
		part := rec.AsMap(value)
		visit(int(rec.FetchInt(part, "first_page"))-1, loadPart(directory, part))
	}
}

// attachLayout checks a stored layout reference as Read finds it and records the directory its
// parts are read from; any other layout is returned as read.
func attachLayout(layout any, directory string) any {
	source, ok := layout.(rec.Map)
	if !ok || source == nil || !Stored(source) {
		return layout
	}
	if _, present := source[directoryKey]; present || source["storage"] != storedVersion {
		rec.Fail("Unsupported layout storage")
	}
	if _, whole := source["pages"]; whole {
		rec.Fail("Layout stored both whole and in parts")
	}
	next := int64(1)
	for _, value := range rec.FetchList(source, "parts") {
		part := rec.AsMap(value)
		first, last := rec.FetchInt(part, "first_page"), rec.FetchInt(part, "last_page")
		if first != next || last < first || !partName.MatchString(rec.FetchString(part, "file")) {
			rec.Fail("Layout parts do not cover the pages in order")
		}
		sourcemap.ValidateDigest(rec.Fetch(part, "sha256"))
		next = last + 1
	}
	if next-1 != rec.FetchInt(source, "page_count") || next == 1 {
		rec.Fail("Layout parts do not cover the pages in order")
	}
	result := without(source)
	result[directoryKey] = directory
	return result
}

// readPartFile is a part file's bytes as stored, checked against its reference.
func readPartFile(directory string, part rec.Map) []byte {
	path := filepath.Join(directory, filepath.FromSlash(rec.FetchString(part, "file")))
	info, err := os.Stat(path)
	check(err)
	if info.Size() > maxBytes || info.Size() != rec.FetchInt(part, "bytes") {
		rec.Fail("Layout part size differs: %s", rec.FetchString(part, "file"))
	}
	data, err := os.ReadFile(path)
	check(err)
	if hexSum(data) != rec.Fetch(part, "sha256") {
		rec.Fail("Layout part hash differs: %s", rec.FetchString(part, "file"))
	}
	return data
}

// loadPart is a part's pages, whole: read, checked, expanded (decision 085).
func loadPart(directory string, part rec.Map) []any {
	expanded, err := zgzip.Decompress(readPartFile(directory, part), maxBytes)
	if err != nil {
		panic(bundleError{err})
	}
	if int64(len(expanded)) != rec.FetchInt(part, "expanded_bytes") {
		rec.Fail("Layout part size differs: %s", rec.FetchString(part, "file"))
	}
	parsed, err := canonjson.Decode(bytes.NewReader(expanded))
	if err != nil {
		panic(bundleError{err})
	}
	stored := rec.AsMap(parsed)
	first, last := rec.FetchInt(part, "first_page"), rec.FetchInt(part, "last_page")
	if stored["schema_version"] != partVersion || !rec.Equal(stored["first_page"], first) || !rec.Equal(stored["last_page"], last) {
		rec.Fail("Layout part differs from its reference: %s", rec.FetchString(part, "file"))
	}
	pages := rec.FetchList(rec.AsMap(expandLayout(without(stored, "schema_version", "first_page", "last_page"))), "pages")
	if int64(len(pages)) != last-first+1 {
		rec.Fail("Layout part page count differs: %s", rec.FetchString(part, "file"))
	}
	return pages
}
