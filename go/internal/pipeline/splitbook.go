package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/split"
)

// SplitDirectory is the folder of the export stage that holds a split book's files; the
// publication copies them beside the PDF, the contents page as index.html.
const SplitDirectory = "split"

// splitBook cuts the verified reading page of a book into one file per chapter and a contents
// page (decision 081), after the browser verification of the whole page. A paper stays one
// page. A split that cannot be made, or that the check finds unfaithful, leaves the whole page
// as the entry with a diagnostic.
func (c *conversion) splitBook(root string) {
	pagePath := filepath.Join(root, "html", "index.html")
	if c.options.Split == "none" || !isFile(pagePath) {
		return
	}
	data, err := os.ReadFile(pagePath)
	if err != nil {
		rec.Fail("%v", err)
	}
	page := string(data)
	files, reason := split.Write(c.document, page)
	if files == nil {
		if reason != split.NoChapters {
			c.diagnostics.Add("split.whole", reason, nil)
		}
		return
	}
	if err := split.Check(page, files); err != nil {
		c.diagnostics.Add("split.unfaithful", err.Error(), nil)
		c.degradations = append(c.degradations, ojson.Object{{"layer", "split"}, {"level", "error"}, {"reason", err.Error()},
			{"effect", "the book is published as one page"}})
		return
	}
	directory := filepath.Join(root, SplitDirectory)
	mkdirAll(directory)
	listed := make([]any, len(files))
	for index, file := range files {
		if err := os.WriteFile(filepath.Join(directory, file.Name), []byte(file.HTML), 0o644); err != nil {
			rec.Fail("%v", err)
		}
		digest := sha256.Sum256([]byte(file.HTML))
		listed[index] = ojson.Object{{"name", file.Name}, {"title", file.Title}, {"kind", file.Kind},
			{"bytes", int64(len(file.HTML))}, {"sha256", hex.EncodeToString(digest[:])}}
	}
	c.diagnostics.Add("split.chapters", fmt.Sprintf("the book is published as %d files: a contents page and one file per chapter",
		len(files)), int64(len(files)))
	pageDigest := sha256.Sum256(data)
	writeOrdered(filepath.Join(directory, "split.json"), ojson.Object{{"mode", c.options.Split}, {"status", "faithful"},
		{"page", "html/index.html"}, {"page_sha256", hex.EncodeToString(pageDigest[:])}, {"files", listed}})
}

// publishSplit copies the split files (not split.json) to the root of the publication.
func publishSplit(directory, root string) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		rec.Fail("%v", err)
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() && filepath.Ext(entry.Name()) == ".html" {
			copyFile(filepath.Join(directory, entry.Name()), filepath.Join(root, entry.Name()), 0o644)
		}
	}
}
