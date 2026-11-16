package epub

import (
	"fmt"
	"path"
	"strings"
)

// PackageDirectory holds the package document and the publication's files inside the ZIP.
const (
	PackageDirectory = "EPUB"
	PackageName      = "package.opf"
	containerPath    = "META-INF/container.xml"
)

// container is META-INF/container.xml: where the package document is.
func container() string {
	return xmlDeclaration + `<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">` +
		`<rootfiles><rootfile full-path="` + PackageDirectory + "/" + PackageName +
		`" media-type="application/oebps-package+xml"/></rootfiles></container>` + "\n"
}

// Metadata is what the package document says about the publication.
type Metadata struct {
	Identifier string // a URN
	Title      string
	Creators   []string
	Language   string // a BCP 47 tag
	Modified   string // CCYY-MM-DDThh:mm:ssZ
	Source     string // dc:source: the print edition the page list follows; "" without a page list
	// PageBreakSource is the source of the page breaks (EPUB Accessibility 1.1 pageBreakSource):
	// the PDF's identifier, else a statement naming the PDF by its title; "" without a page list.
	PageBreakSource string
	Access          Accessibility
}

// item is one manifest entry: a file of the package, its media type and properties.
type item struct {
	id, href, mediaType string
	properties          []string
}

// mediaTypes are the core media types of EPUB 3 by extension; anything else is a foreign resource.
var mediaTypes = map[string]string{".xhtml": "application/xhtml+xml", ".css": "text/css", ".svg": "image/svg+xml",
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp",
	".otf": "font/otf", ".ttf": "font/ttf", ".woff": "font/woff", ".woff2": "font/woff2", ".pdf": "application/pdf"}

func mediaType(name string) string {
	if known, ok := mediaTypes[strings.ToLower(path.Ext(name))]; ok {
		return known
	}
	return "application/octet-stream"
}

// itemID is a manifest id for a file: its path made an XML name, unique among the ids given.
func itemID(href string, taken map[string]bool) string {
	var builder strings.Builder
	builder.WriteString("i-")
	for _, r := range href {
		if r < 128 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_') {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('_')
		}
	}
	id := builder.String()
	for suffix := 2; taken[id]; suffix++ {
		id = fmt.Sprintf("%s-%d", builder.String(), suffix)
	}
	taken[id] = true
	return id
}

// packageDocument is package.opf: the metadata, the manifest of every file and the spine of
// the content documents in reading order.
func packageDocument(metadata Metadata, items []item, spine []string) string {
	var builder strings.Builder
	builder.WriteString(xmlDeclaration + `<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="pub-id"`)
	if metadata.Language != "" {
		builder.WriteString(` xml:lang="` + attributeEscaper.Replace(metadata.Language) + `"`)
	}
	builder.WriteString(">\n<metadata xmlns:dc=\"http://purl.org/dc/elements/1.1/\">\n")
	writeMetadata(&builder, metadata)
	builder.WriteString("</metadata>\n<manifest>\n")
	ids := map[string]string{}
	for _, entry := range items {
		ids[entry.href] = entry.id
		builder.WriteString(`<item id="` + entry.id + `" href="` + attributeEscaper.Replace(entry.href) + `" media-type="` + entry.mediaType + `"`)
		if len(entry.properties) > 0 {
			builder.WriteString(` properties="` + strings.Join(entry.properties, " ") + `"`)
		}
		builder.WriteString("/>\n")
	}
	builder.WriteString("</manifest>\n<spine>\n")
	for _, href := range spine {
		builder.WriteString(`<itemref idref="` + ids[href] + `"/>` + "\n")
	}
	builder.WriteString("</spine>\n</package>\n")
	return builder.String()
}

func writeMetadata(builder *strings.Builder, metadata Metadata) {
	element := func(name, value string) {
		builder.WriteString("<" + name + ">" + textEscaper.Replace(value) + "</" + name + ">\n")
	}
	builder.WriteString(`<dc:identifier id="pub-id">` + textEscaper.Replace(metadata.Identifier) + "</dc:identifier>\n")
	title := metadata.Title
	if title == "" {
		title = "Untitled document"
	}
	element("dc:title", title)
	for _, creator := range metadata.Creators {
		element("dc:creator", creator)
	}
	language := metadata.Language
	if language == "" {
		language = "und"
	}
	element("dc:language", language)
	if metadata.Source != "" {
		// The source of the page list (EPUB Accessibility Techniques 1.1, "Identify the source of the
		// pagination").
		builder.WriteString(`<dc:source id="print-source">` + textEscaper.Replace(metadata.Source) + "</dc:source>\n" +
			`<meta refines="#print-source" property="source-of">pagination</meta>` + "\n")
	}
	if metadata.PageBreakSource != "" {
		// EPUB Accessibility 1.1, section 4.2 (the property that replaces the refinement above;
		// both are written for readers of either).
		builder.WriteString(`<meta property="pageBreakSource">` + textEscaper.Replace(metadata.PageBreakSource) + "</meta>\n")
	}
	builder.WriteString(`<meta property="dcterms:modified">` + metadata.Modified + "</meta>\n")
	for _, pair := range accessibilityMetadata(metadata.Access) {
		builder.WriteString(`<meta property="` + pair[0] + `">` + textEscaper.Replace(pair[1]) + "</meta>\n")
	}
}
