package bundle

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// The writer stores a layout page by page as storeLayout stores the whole one: the same reference,
// the same part files byte for byte (a book's export is the same either way).
func TestLayoutWriterWritesWhatStoreLayoutWrites(t *testing.T) {
	whole, streamed := t.TempDir(), t.TempDir()
	layout, reference := storedBook(t, whole)
	page := rec.AsMap(rec.FetchList(mapLayoutPages(rec.Map{"pages": rec.FetchList(layout, "pages")[:1]}, compactGlyph).(rec.Map), "pages")[0])
	previous := partBudget
	partBudget = 2 * len(canonical(t, page))
	defer func() { partBudget = previous }()

	writer := NewLayoutWriter(streamed, 7, []int64{3, 6})
	for _, value := range rec.FetchList(layout, "pages") {
		writer.Add(writer.Prepare(rec.AsMap(value)))
	}
	got := writer.Finish(layout)
	if !bytes.Equal(canonical(t, got), canonical(t, reference)) {
		t.Fatalf("reference differs:\n%s\n%s", canonical(t, got), canonical(t, reference))
	}
	for _, part := range rec.FetchList(reference, "parts") {
		name := filepath.FromSlash(rec.FetchString(rec.AsMap(part), "file"))
		want, _ := os.ReadFile(filepath.Join(whole, name))
		if have, err := os.ReadFile(filepath.Join(streamed, name)); err != nil || !bytes.Equal(have, want) {
			t.Fatalf("part %s differs: %v", name, err)
		}
	}
}

func TestLayoutWriterRefusesAMissingPage(t *testing.T) {
	writer := NewLayoutWriter(t.TempDir(), 3, nil)
	writer.Add(writer.Prepare(rec.AsMap(rec.FetchList(bookLayout(t, 1), "pages")[0])))
	if failure := catchFailure(func() { writer.Finish(bookLayout(t, 1)) }); failure == nil {
		t.Fatal("a layout short of its pages was accepted")
	}
}

func TestWholeLayoutJoinsTheParts(t *testing.T) {
	directory := t.TempDir()
	layout, reference := storedBook(t, directory)
	read := AttachLayout(throughJSON(t, reference), directory)
	whole := WholeLayout(read)
	if Stored(whole) || !bytes.Equal(canonical(t, whole["pages"]), canonical(t, layout["pages"])) {
		t.Fatal("the joined pages differ from the layout stored")
	}
	if got := WithoutDirectory(read); got["directory"] != nil || !Stored(got) {
		t.Fatalf("directory kept: %v", got)
	}
}
