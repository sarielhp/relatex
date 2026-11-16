package fontmodel

import (
	"os"
	"path/filepath"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
)

// FontsDirectory is the folder beside the page that holds the linked font files.
const FontsDirectory = "fonts"

// WriteFiles is SourceFonts.write_files!: every linked font file of the document, once, as
// directory/fonts/<name> beside the page, its bytes checked against the digest the document
// records. The fonts are the originals (decision 058), never converted or embedded.
func WriteFiles(document rec.Map, directory string) {
	fonts, _ := document["fonts"].(rec.Map)
	items, _ := fonts["items"].([]any)
	written := map[string]bool{}
	for _, value := range items {
		file, linked := rec.AsMap(value)["file"].(rec.Map)
		if !linked || written[rec.FetchString(file, "name")] {
			continue
		}
		written[rec.FetchString(file, "name")] = true
		copyFont(file, filepath.Join(directory, FontsDirectory))
	}
}

func readSourcePath(source string, fromSlash func(string) string) string {
	return fromSlash(source)
}

func copyFont(file rec.Map, directory string) {
	source := readSourcePath(rec.FetchString(file, "source"), filepath.FromSlash)
	data, err := os.ReadFile(source)
	if err != nil || command.FileDigest(source) != file["sha256"] {
		rec.Fail("Linked font changed: %s", filepath.Base(source))
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		rec.Fail("%v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, rec.FetchString(file, "name")), data, 0o644); err != nil {
		rec.Fail("%v", err)
	}
}
