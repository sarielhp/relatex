// Package epub packages a document as an EPUB 3 publication (decision 058): the reading page as
// XHTML content documents with MathML, its figures (with their originals) and fonts as files,
// its stylesheet, the navigation document, and the package document with the EPUB
// Accessibility 1.1 discovery metadata (decision 080), in a ZIP that is the same bytes for the
// same document.
package epub

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/pagetree"
	"github.com/sarielhp/relatex/internal/rec"
)

// Options name the package and pin what the document alone does not say.
type Options struct {
	Name     string // the file's stem, as the PDF's: paper gives paper.epub; "document" when empty
	Source   string // the SHA-256 of the sources the document came from, for the identifier
	Modified string // dcterms:modified, CCYY-MM-DDThh:mm:ssZ (ModifiedDate)
	Claim    string // the conformance the accessibility report establishes; "" claims none
	Split    string // none: a book divided by its top-level sections, as a paper; else by chapter (decision 081)
	// Pages are where the print PDF's pages begin (papersource.PageList): each is marked in the text
	// and listed in the page-list navigation; PrintSource names that PDF (dc:source, pageBreakSource).
	Pages       []PageMark
	PrintSource string
	// Page writes the reading page's files beside it in a directory (figures, fonts, the accent
	// font) and returns the page: the html output's, which the export writer supplies (decision
	// 102), so the package reads the page as the export directory holds it.
	Page func(document rec.Map, directory string) string
}

// resourceDirectories are the folders beside the page that the package carries.
var resourceDirectories = []string{"figs", fontmodel.FontsDirectory}

// Write packages the document into directory/<name>.epub and returns the export report.
func Write(document rec.Map, directory string, options Options) ojson.Object {
	staging, err := os.MkdirTemp(directory, ".package-")
	if err != nil {
		rec.Fail("%v", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	p := preparePage(options.Page(document, staging))
	documents, whole, chapters := splitBookPage(p, document, options.Split)
	name := options.Name
	if name == "" {
		name = "document"
	}
	file := name + ".epub"
	metadata := assemble(p, documents, staging, unshownFiles(document, p), publicationMetadata(document, p, options), options,
		filepath.Join(directory, file))
	split := "sections"
	if chapters {
		split = "chapters"
	}
	if whole != "" {
		split = "whole: " + whole
	}
	return ojson.Object{{"status", "success"}, {"file", file}, {"losses", []any{}}, {"content_documents", int64(len(documents))},
		{"split", split}, {"identifier", metadata.Identifier}, {"modified", metadata.Modified},
		{"accessibility", accessibilityReport(metadata.Access)},
		{"adjustments", ojson.Object{{"intents_as_data", int64(p.adjustments.IntentsAsData)},
			{"colors_as_style", int64(p.adjustments.ColorsAsStyle)}, {"fences_as_data", int64(p.adjustments.FencesAsData)},
			{"captions_as_divisions", int64(p.adjustments.CaptionsAsDivisions)}}}}
}

// assemble writes the package of the content documents to path: the page-break markers and the
// page list, the navigation, the files beside the page (staging) but those left out, and the
// package document with the metadata completed by what the files establish (identifier, pages).
func assemble(p *page, documents []contentDocument, staging string, left map[string]bool, metadata Metadata, options Options,
	path string) Metadata {
	pages := markPages(documents, options.Pages)
	files, items, spine := packageFiles(p, documents, pages, staging, left)
	metadata.Identifier = identifier(options.Source, files)
	metadata.Access.Pages = len(pages)
	if len(pages) > 0 {
		metadata.Source = options.PrintSource
		metadata.PageBreakSource = pageBreakSource(options.PrintSource, metadata.Title)
	}
	entries := []entry{{containerPath, []byte(container())},
		{PackageDirectory + "/" + PackageName, []byte(packageDocument(metadata, items, spine))}}
	for _, it := range items {
		entries = append(entries, entry{PackageDirectory + "/" + it.href, files[it.href]})
	}
	writeArchive(path, entries)
	return metadata
}

func accessibilityReport(access Accessibility) ojson.Object {
	claim := any(nil)
	if conformsTo := conformance(access); conformsTo != "" {
		claim = conformsTo
	}
	return ojson.Object{{"images", int64(access.Images)}, {"described_images", int64(access.Described)},
		{"drafted_descriptions", int64(access.Drafts)}, {"long_descriptions", int64(access.Long)}, {"formulas", int64(access.Formulas)}, {"headings", int64(access.Headings)},
		{"page_breaks", int64(access.Pages)}, {"conforms_to", claim}, {"summary", summary(access)}}
}

// packageFiles are the publication's files by their path in the package, their manifest entries
// (navigation, stylesheet, content documents in reading order, then figures and fonts by path)
// and the spine.
func packageFiles(p *page, documents []contentDocument, pages []pageEntry, staging string, left map[string]bool) (map[string][]byte, []item, []string) {
	files := map[string][]byte{}
	taken := map[string]bool{}
	files[NavigationName] = []byte(navigation(p, documents, pages, p.title()))
	files[StylesheetName] = []byte(p.style)
	items := []item{{itemID(NavigationName, taken), NavigationName, mediaType(NavigationName), []string{"nav"}},
		{itemID(StylesheetName, taken), StylesheetName, mediaType(StylesheetName), nil}}
	spine := []string{}
	for _, document := range documents {
		files[document.name] = []byte(p.xhtml(document))
		items = append(items, item{itemID(document.name, taken), document.name, mediaType(document.name), properties(document)})
		spine = append(spine, document.name)
	}
	for _, relative := range resourceFiles(staging) {
		if left[relative] {
			continue
		}
		data, err := os.ReadFile(filepath.Join(staging, filepath.FromSlash(relative)))
		if err != nil {
			rec.Fail("%v", err)
		}
		files[relative] = data
		items = append(items, item{itemID(relative, taken), relative, mediaType(relative), nil})
	}
	return files, items, spine
}

// unshownFiles are the figure files no content document shows with an img: an original the page
// does not show (a source PDF beside its SVG), which the export directory keeps beside the derived
// forms (decision 058) and a reading system cannot use, and the file of a figure shown as inline SVG
// (decision 146), whose markup the content document holds. The package leaves them out.
func unshownFiles(document rec.Map, p *page) map[string]bool {
	displayed, originals := figures.Plan(document)
	shown := map[string]bool{}
	p.body.Walk(func(n *pagetree.Node) {
		if source, ok := n.Attr("src"); ok {
			shown[source] = true
		}
	})
	left := map[string]bool{}
	for _, files := range []map[string]string{displayed, originals} {
		for _, relative := range files {
			if !shown[relative] {
				left[relative] = true
			}
		}
	}
	return left
}

// resourceFiles are the figures and fonts written beside the page, by path.
func resourceFiles(staging string) []string {
	result := []string{}
	for _, folder := range resourceDirectories {
		err := filepath.WalkDir(filepath.Join(staging, folder), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type().IsRegular() {
				relative, _ := filepath.Rel(staging, path)
				result = append(result, filepath.ToSlash(relative))
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			rec.Fail("%v", err)
		}
	}
	sort.Strings(result)
	return result
}

// publicationMetadata is the package's metadata from the document: its title, authors and
// language, and the accessibility evidence (assemble adds the identifier and the pages).
func publicationMetadata(document rec.Map, p *page, options Options) Metadata {
	metadata := Metadata{Title: p.title(), Language: htmlkit.DocumentLanguage(document), Modified: options.Modified,
		Access: evidence(p)}
	metadata.Access.Claim = options.Claim
	metadata.Access.Drafts = draftedImages(rec.FetchMap(document, "nodes"), 0)
	if metadata.Modified == "" {
		metadata.Modified = ModifiedDate("")
	}
	if front, ok := document["frontmatter"].(rec.Map); ok {
		authors, _ := front["authors"].([]any)
		for _, author := range authors {
			if name, ok := rec.AsMap(author)["name"].(string); ok && strings.TrimSpace(name) != "" {
				metadata.Creators = append(metadata.Creators, strings.Join(strings.Fields(name), " "))
			}
		}
	}
	return metadata
}

// evidence counts what the accessibility metadata rests on: figures and their alternatives,
// formulas, headings.
func evidence(p *page) Accessibility {
	access := Accessibility{}
	p.body.Walk(func(n *pagetree.Node) {
		switch n.Name {
		case "img":
			access.Images++
			if alt, ok := n.Attr("alt"); ok && described(n, alt) {
				access.Described++
			}
		case "svg":
			// A figure shown as inline SVG (decision 146) is an image named by its aria-label.
			if kind, _ := n.Attr("data-kind"); kind == "image" {
				access.Images++
				if label, ok := n.Attr("aria-label"); ok && described(n, label) {
					access.Described++
				}
			}
		case "details":
			if kind, _ := n.Attr("data-kind"); kind == "long-description" {
				access.Long++
			}
		case "math":
			access.Formulas++
		case "h1", "h2", "h3", "h4", "h5", "h6":
			access.Headings++
		}
	})
	return access
}

// draftedImages counts the image spans below value (table cells included) whose description is a
// model's draft nobody has reviewed (decision 086, alt_provenance "model").
func draftedImages(value any, depth int) int {
	if depth > 200 {
		return 0
	}
	count := 0
	switch typed := value.(type) {
	case rec.Map:
		if typed["type"] == "image" && typed["alt_provenance"] == "model" {
			count++
		}
		for _, child := range typed {
			count += draftedImages(child, depth+1)
		}
	case []any:
		for _, child := range typed {
			count += draftedImages(child, depth+1)
		}
	}
	return count
}

// described tells whether an image has a text alternative: a real description, or an empty
// one on an image marked decorative. The page flags a placeholder alternative with
// data-undescribed, which is not a description however it reads.
func described(image *pagetree.Node, alt string) bool {
	role, _ := image.Attr("role")
	if strings.TrimSpace(alt) == "" {
		return role == "presentation" || role == "none"
	}
	_, placeholder := image.Attr(htmlkit.UndescribedAttribute)
	return !placeholder
}

// identifier is a URN that names the publication: a name-based UUID of the sources' digest alone,
// so a corrected package of the same sources is the same publication with a newer modified date.
// A package without known sources (exported from a bundle) is named by its files' digests.
func identifier(source string, files map[string][]byte) string {
	hash := sha256.New()
	hash.Write([]byte("relatex-epub\n" + source + "\n"))
	if source == "" {
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			digest := sha256.Sum256(files[name])
			hash.Write([]byte(name + " " + hex.EncodeToString(digest[:]) + "\n"))
		}
	}
	sum := hash.Sum(nil)
	sum[6] = sum[6]&0x0f | 0x50
	sum[8] = sum[8]&0x3f | 0x80
	text := hex.EncodeToString(sum[:16])
	return "urn:uuid:" + text[0:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:32]
}

// fallbackModified is the date of a package whose build date is unknown: fixed, so the package
// stays reproducible.
const fallbackModified = "1980-01-01T00:00:00Z"

var pdfDate = regexp.MustCompile(`/CreationDate\s*\(D:(\d{14})(Z|[+-]\d{2}'?\d{2}'?)?`)

// ModifiedDate is the publication's dcterms:modified, pinned so the same build gives the same
// package: SOURCE_DATE_EPOCH when set, else the creation date of the PDF the document was read
// from (which TeX takes from SOURCE_DATE_EPOCH too), else a fixed date.
func ModifiedDate(pdf string) string {
	if epoch, err := strconv.ParseInt(os.Getenv("SOURCE_DATE_EPOCH"), 10, 64); err == nil {
		return time.Unix(epoch, 0).UTC().Format(time.RFC3339)
	}
	if pdf == "" {
		return fallbackModified
	}
	data, err := os.ReadFile(pdf)
	if err != nil {
		return fallbackModified
	}
	match := pdfDate.FindSubmatch(data)
	if match == nil {
		return fallbackModified
	}
	stamp, err := time.Parse("20060102150405 -0700", string(match[1])+" "+zoneOffset(string(match[2])))
	if err != nil {
		return fallbackModified
	}
	return stamp.UTC().Format(time.RFC3339)
}

// zoneOffset is a PDF date's zone (Z, +hh'mm') as -0700 layout text.
func zoneOffset(zone string) string {
	digits := strings.ReplaceAll(zone, "'", "")
	if len(digits) != 5 {
		return "+0000"
	}
	return digits
}

// ReadClaim is the conformance claim of an accessibility report (accessibility.json): its
// claim when the report makes one, "" when it makes none or does not exist.
func ReadClaim(path string) string {
	value, err := ojson.ReadFile(path)
	if err != nil {
		return ""
	}
	report, ok := ojson.Plain(value).(rec.Map)
	if !ok {
		return ""
	}
	claim, _ := report["claim"].(string)
	return claim
}
