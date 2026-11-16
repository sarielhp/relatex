package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/split"
)

// deliveredMarker was the hidden file of the output directory that recorded which entries relatex
// wrote (decision 191). The list is kept in the project cache now (the amendment of 2026-11-16), so
// the output root holds nothing but the requested entries; an old marker is read once and removed.
const deliveredMarker = ".relatex-output.json"

// ownedRecord is the file of the output root's records directory that lists the entries relatex wrote.
const ownedRecord = "owned.json"

// delivery is one requested format's deliverable: the name it has in the output directory, and
// where it is in the product (empty for a file written beside the others).
type delivery struct {
	name, source string
}

// formatDirectories are the output directory names of the formats whose directory is not named for
// them: "typst" is the Typst source project (--typst), the layout export is "typst-layout".
var formatDirectories = map[string]string{"typst-source": "typst", "typst": "typst-layout", "json.gz": "json-gz"}

// deliveries lists what the requested formats put in the output directory. EPUB and the tagged PDF
// are single files named after the paper's PDF; every other format is a directory (HTML: the reading
// page, a book's chapter pages, figs/, fonts/ and preview.js).
func (c *conversion) deliveries(product string) []delivery {
	job := strings.TrimSuffix(filepath.Base(c.pdf), ".pdf")
	var list []delivery
	for _, format := range c.options.Formats {
		if c.exports != nil && !c.exports.Has(format) {
			continue
		}
		switch format {
		case "epub":
			list = append(list, delivery{job + ".epub", filepath.Join(product, job+".epub")})
		case "pdf-ua":
			list = append(list, delivery{job + "-tagged.pdf", filepath.Join(product, "pdf-ua", job+".pdf")})
		default:
			name := format
			if renamed, ok := formatDirectories[format]; ok {
				name = renamed
			}
			list = append(list, delivery{name, filepath.Join(product, strings.ReplaceAll(format, ".", "-"))})
		}
	}
	return list
}

// stageRecord is whether a name in a format's product directory is the stage's own record (the
// receipts and the build logs), not part of the deliverable.
func stageRecord(name string) bool {
	return name == "export.json" || name == "stage.json" || strings.HasSuffix(name, "-stage.json") || strings.HasSuffix(name, ".log")
}

// copyDeliverable copies a format's product to the staging area without the receipts at its top
// level (a layout's own support/ directory is part of it, whatever it holds).
func copyDeliverable(from, to string, top bool) {
	info, err := os.Lstat(from)
	if err != nil {
		rec.Fail("%v", err)
	}
	if !info.IsDir() {
		copyFile(from, to, info.Mode().Perm())
		return
	}
	mkdirAll(to)
	entries, err := os.ReadDir(from)
	if err != nil {
		rec.Fail("%v", err)
	}
	for _, entry := range entries {
		if !top || !stageRecord(entry.Name()) {
			copyDeliverable(filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name()), false)
		}
	}
}

// htmlDelivery adds what belongs beside the reading page: a split book's contents page and chapter
// files over the whole page, and the accessibility report.
func htmlDelivery(product, staged string) {
	if contents := filepath.Join(product, SplitDirectory, split.ContentsName); isFile(contents) {
		publishSplit(filepath.Join(product, SplitDirectory), staged)
	}
	if report := filepath.Join(product, "accessibility.html"); isFile(report) {
		copyFile(report, filepath.Join(staged, "accessibility.html"), 0o644)
	}
}

// outputKey names an output root's records: the digest of its real path.
func outputKey(output string) string {
	if resolved, err := filepath.EvalSymlinks(output); err == nil {
		output = resolved
	}
	sum := sha256.Sum256([]byte(output))
	return hex.EncodeToString(sum[:])[:12]
}

// outputRecords is the directory of the project cache that holds this output root's ownership list and
// a failed run's leftovers; "" when the run delivers nothing.
func (c *conversion) outputRecords() string {
	if c.output == "" {
		return ""
	}
	return filepath.Join(c.options.Records, outputKey(c.output))
}

// holdsInput is whether the output root is the paper's directory or one above it: the default, where
// the formats go into the directory relatex runs in. Only the entries relatex wrote there are its own.
func (c *conversion) holdsInput() bool {
	if c.output == "" {
		return false
	}
	directory := c.input
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		directory = filepath.Dir(directory)
	}
	// Both absolute: EvalSymlinks keeps a relative root relative (-o . is the paper's directory too).
	root, _ := filepath.Abs(c.output)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if absolute, err := filepath.Abs(directory); err == nil {
		directory = absolute
	}
	if resolved, err := filepath.EvalSymlinks(directory); err == nil {
		directory = resolved
	}
	relative, err := filepath.Rel(root, directory)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// ownedDeliveries are the entries of the output root an earlier conversion wrote: the project cache's
// list, else (a root of the earlier layouts, which is not the paper's directory) the old hidden marker,
// else every entry of a directory that has a manifest.
func ownedDeliveries(records, output string, legacy bool) []string {
	if data, err := os.ReadFile(filepath.Join(records, ownedRecord)); err == nil {
		return decodeEntries(data)
	}
	if !legacy {
		return nil
	}
	if data, err := os.ReadFile(filepath.Join(output, deliveredMarker)); err == nil {
		return decodeEntries(data)
	}
	if !isFile(filepath.Join(output, "manifest.json")) {
		return nil
	}
	entries, _ := os.ReadDir(output)
	var names []string
	for _, entry := range entries {
		if ownedEntry(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	return names
}

// decodeEntries reads a list of entry names, keeping only plain names.
func decodeEntries(data []byte) []string {
	var marker struct {
		Entries []string `json:"entries"`
	}
	if json.Unmarshal(data, &marker) != nil {
		return nil
	}
	return slices.DeleteFunc(marker.Entries, func(name string) bool { return name == "" || filepath.Base(name) != name || name == ".." })
}

// owned is the entries of the output root that this root's records say relatex wrote.
func (c *conversion) owned() []string {
	return ownedDeliveries(c.outputRecords(), c.output, !c.holdsInput())
}

// outputExclusions are the paths of the output root the source snapshot leaves out: all of it, unless
// it holds the paper, where only what relatex wrote (and an earlier layout's output/) is its own.
func (c *conversion) outputExclusions() []string {
	if c.output == "" {
		return nil
	}
	if !c.holdsInput() {
		return []string{c.output}
	}
	names := c.owned()
	for _, item := range c.deliveries(c.product) {
		names = append(names, item.name)
	}
	if earlier := filepath.Join(c.output, "output"); isFile(filepath.Join(earlier, "manifest.json")) || isFile(filepath.Join(earlier, deliveredMarker)) {
		names = append(names, "output")
	}
	paths := make([]string, 0, len(names))
	for _, name := range names {
		paths = append(paths, filepath.Join(c.output, name))
	}
	return paths
}

// checkDeliveries refuses to replace an entry relatex did not write.
func checkDeliveries(output string, list []delivery, owned []string) {
	for _, item := range list {
		if _, err := os.Lstat(filepath.Join(output, item.name)); err == nil && !slices.Contains(owned, item.name) {
			rec.Fail("%s exists in %s and relatex did not write it; move it away or choose another output directory with -o DIR",
				item.name, output)
		}
	}
}

// checkOutputEarly refuses an entry of the requested formats that is not relatex's before the work
// starts. The names that depend on the job (the EPUB and the tagged PDF) are checked at delivery.
func (c *conversion) checkOutputEarly() {
	if c.output == "" || c.options.ProductsOnly {
		return
	}
	var list []delivery
	for _, item := range (&conversion{options: c.options, pdf: "job.pdf"}).deliveries("") {
		if !strings.HasSuffix(item.name, ".epub") && !strings.HasSuffix(item.name, "-tagged.pdf") {
			list = append(list, item)
		}
	}
	checkDeliveries(c.output, list, c.owned())
}

// writeOwned records the entries relatex wrote in the output root, and removes the hidden marker an
// earlier layout kept in the output root itself.
func (c *conversion) writeOwned(names []any) {
	records := c.outputRecords()
	mkdirAll(records)
	writeOrdered(filepath.Join(records, ownedRecord), ojson.Object{{"root", c.output}, {"entries", names}})
	removeFile(filepath.Join(c.output, deliveredMarker))
}

// deliver puts the requested formats into the output directory: each in its own entry with a plain
// name, the entries an earlier conversion wrote replaced, everything else left alone. It returns the
// paths written.
func (c *conversion) deliver() []string {
	if c.output == "" {
		return nil
	}
	list := c.deliveries(c.product)
	if report := filepath.Join(c.product, "accessibility.html"); isFile(report) && !slices.ContainsFunc(list, func(item delivery) bool {
		return item.name == "html"
	}) {
		// Without the HTML page the report is a deliverable of its own, named for the paper.
		job := strings.TrimSuffix(filepath.Base(c.pdf), ".pdf")
		list = append(list, delivery{job + "-accessibility.html", report})
	}
	owned := c.owned()
	checkDeliveries(c.output, list, owned)
	mkdirAll(c.output)
	staged := command.MakeTemp(c.output, ".relatex-deliver-")
	defer removeAll(staged)
	for _, item := range list {
		copyDeliverable(item.source, filepath.Join(staged, item.name), true)
		if item.name == "html" {
			htmlDelivery(c.product, filepath.Join(staged, item.name))
		}
	}
	clearStale(c.outputRecords())
	if !c.holdsInput() {
		clearStale(c.output)
	}
	for _, name := range owned {
		removeAll(filepath.Join(c.output, name))
	}
	paths := make([]string, 0, len(list))
	names := make([]any, 0, len(list))
	for _, item := range list {
		rename(filepath.Join(staged, item.name), filepath.Join(c.output, item.name))
		paths = append(paths, filepath.Join(c.output, item.name))
		names = append(names, item.name)
	}
	c.writeOwned(names)
	return paths
}
