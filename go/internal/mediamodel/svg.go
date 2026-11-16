package mediamodel

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/xmltree"
)

// maxSVGBytes is DocumentBundle::MAX_BYTES.
const maxSVGBytes = 512 * 1024 * 1024

var (
	validatedSVG     = map[string]bool{}
	validatedSVGLock sync.Mutex
)

// ValidateSVG is PDFLayout.validate_svg!: a page or figure SVG without declarations, active
// content or external resources. Each distinct SVG is checked once per process.
func ValidateSVG(data []byte) []byte {
	key := hexDigest(data)
	validatedSVGLock.Lock()
	seen := validatedSVG[key]
	validatedSVGLock.Unlock()
	if seen {
		return data
	}
	parseSVG(data)
	validatedSVGLock.Lock()
	defer validatedSVGLock.Unlock()
	if len(validatedSVG) >= 256 {
		validatedSVG = map[string]bool{}
	}
	validatedSVG[key] = true
	return data
}

// parseSVG is PDFLayout.parse_svg!'s checks, on a strict streaming parse.
func parseSVG(data []byte) {
	if len(data) > maxSVGBytes {
		rec.Fail("SVG page exceeds size limit")
	}
	if bytes.Contains(data, []byte("<!DOCTYPE")) || bytes.Contains(data, []byte("<!ENTITY")) {
		rec.Fail("SVG declarations/entities are forbidden")
	}
	decoder := xml.NewDecoder(bytes.NewReader(xmltree.Characters(data)))
	depth := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			rec.Fail("Invalid native trace XML: %s", err.Error())
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if depth == 0 && typed.Name.Local != "svg" {
				rec.Fail("Expected an SVG page")
			}
			depth++
			checkSVGElement(typed)
		case xml.EndElement:
			depth--
		}
	}
	if depth != 0 {
		rec.Fail("Invalid native trace XML: unexpected end of document")
	}
}

func checkSVGElement(element xml.StartElement) {
	if slices.Contains([]string{"script", "foreignObject", "animate", "set"}, element.Name.Local) {
		rec.Fail("Active SVG content is forbidden")
	}
	for _, attribute := range element.Attr {
		if attribute.Name.Space == "xmlns" || (attribute.Name.Space == "" && attribute.Name.Local == "xmlns") {
			continue // A namespace declaration is no attribute node in libxml2.
		}
		value := attribute.Value
		if strings.HasPrefix(strings.ToLower(attribute.Name.Local), "on") {
			rec.Fail("Active SVG attribute is forbidden")
		}
		if (attribute.Name.Local == "href" || attribute.Name.Local == "src") && !strings.HasPrefix(value, "#") &&
			!strings.HasPrefix(value, "data:image/") {
			rec.Fail("External SVG resource is forbidden")
		}
		if externalURL(value) {
			rec.Fail("External SVG style resource is forbidden")
		}
	}
}

// externalURL is /url\(\s*['"]?(?!#)/i: with backtracking it matches any "url(" not
// directly followed by "#".
func externalURL(value string) bool {
	lower := strings.ToLower(value)
	for start := 0; ; {
		index := strings.Index(lower[start:], "url(")
		if index < 0 {
			return false
		}
		next := start + index + 4
		if next >= len(lower) || lower[next] != '#' {
			return true
		}
		start = next
	}
}
