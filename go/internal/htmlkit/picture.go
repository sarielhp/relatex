package htmlkit

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/xmltree"
)

// PictureSource is the prefix of the source a picture TeX drew carries: its crop of the PDF
// (decision 137).
const PictureSource = "rendered/"

const svgNamespace = "http://www.w3.org/2000/svg"

// Picture is a figure the page shows as inline SVG (decision 146): the root attributes of its file
// but its size (the page sizes it as it sizes an image), and the file's content with every id
// prefixed by the figure's own.
type Picture struct {
	Attributes []xml.Attr
	Content    string
}

// PictureForbidden are the SVG elements a figure shown inline may not hold: a style sheet would
// apply to the whole page (and name ids the prefix renames), script and foreign content run or
// hold markup the page does not check.
var PictureForbidden = map[string]bool{"style": true, "script": true, "foreignObject": true}

var pictureReference = regexp.MustCompile(`url\(\s*(['"]?)#`)

// InlinePicture is the inline form of an image span (decision 146): a picture TeX drew (its source
// a crop, decision 137) that is neither decorative nor a link, whose displayed form is an SVG holding
// text (its labels passed decision 137's check) and no forbidden element. Any other image is an img.
func InlinePicture(document, span rec.Map) (Picture, bool) {
	source, _ := span["source"].(string)
	if !strings.HasPrefix(source, PictureSource) || span["decorative"] == true || span["external_link"] != nil {
		return Picture{}, false
	}
	data, mime := figures.Displayed(document, span)
	if mime != "image/svg+xml" {
		return Picture{}, false
	}
	picture, err := inlineSVG(data, rec.ToS(span["id"])+"-")
	if err != nil {
		return Picture{}, false
	}
	return picture, true
}

// inlineSVG rewrites an SVG file as the content of an inline svg element whose ids carry prefix;
// an error when it is not an SVG with text and without forbidden elements.
func inlineSVG(data []byte, prefix string) (Picture, error) {
	decoder := xml.NewDecoder(bytes.NewReader(xmltree.AttributeWhitespace(xmltree.Characters(data))))
	var content strings.Builder
	picture := Picture{}
	depth, text := 0, false
	open := false // the last start tag is not closed yet: an element without content closes as />
	for {
		token, err := decoder.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Picture{}, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			name := qualified(typed.Name)
			if depth == 0 {
				picture.Attributes = rootAttributes(typed.Attr)
				if name != "svg" || !slices.Contains(picture.Attributes, xml.Attr{Name: xml.Name{Local: "xmlns"}, Value: svgNamespace}) {
					return Picture{}, errors.New("not an SVG")
				}
				depth++
				continue
			}
			if PictureForbidden[name] {
				return Picture{}, errors.New("forbidden element " + name)
			}
			text = text || name == "text"
			closeTag(&content, &open)
			content.WriteString("<" + name)
			for _, attribute := range typed.Attr {
				content.WriteString(" " + qualified(attribute.Name) + `="` + attributeEscaper.Replace(prefixed(attribute, prefix)) + `"`)
			}
			open = true
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 {
				continue
			}
			if open {
				content.WriteString("/>")
				open = false
				continue
			}
			content.WriteString("</" + qualified(typed.Name) + ">")
		case xml.CharData:
			if depth > 0 {
				closeTag(&content, &open)
				content.WriteString(textEscaper.Replace(string(typed)))
			}
		}
	}
	if !text {
		return Picture{}, errors.New("no text")
	}
	picture.Content = content.String()
	return picture, nil
}

var textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// attributeEscaper writes an attribute value so the HTML and the XHTML parser read it alike: white
// space a character reference put in the file's value (literal white space is a space by then,
// xmltree.AttributeWhitespace) is a character reference again, which an XML parser does not normalize.
var attributeEscaper = strings.NewReplacer("'", "&#39;", "&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;",
	"\t", "&#9;", "\n", "&#10;", "\r", "&#13;")

func closeTag(content *strings.Builder, open *bool) {
	if *open {
		content.WriteString(">")
		*open = false
	}
}

// rootAttributes are the file's root attributes but its width and height.
func rootAttributes(attributes []xml.Attr) []xml.Attr {
	kept := []xml.Attr{}
	for _, attribute := range attributes {
		if attribute.Name.Space == "" && (attribute.Name.Local == "width" || attribute.Name.Local == "height") {
			continue
		}
		kept = append(kept, attribute)
	}
	return kept
}

// prefixed is an attribute's value with the figure's ids prefixed: an id, a local href, a url(#…)
// reference.
func prefixed(attribute xml.Attr, prefix string) string {
	switch {
	case attribute.Name.Space == "" && attribute.Name.Local == "id":
		return prefix + attribute.Value
	case attribute.Name.Local == "href" && strings.HasPrefix(attribute.Value, "#"):
		return "#" + prefix + attribute.Value[1:]
	}
	return pictureReference.ReplaceAllString(attribute.Value, "url(${1}#"+prefix)
}

// qualified is a name as the file spells it, its prefix kept.
func qualified(name xml.Name) string {
	if name.Space == "" {
		return name.Local
	}
	return name.Space + ":" + name.Local
}

// PictureAttributes is the root attributes written as HTML attributes.
func (p Picture) PictureAttributes() string {
	var builder strings.Builder
	for _, attribute := range p.Attributes {
		builder.WriteString(" " + qualified(attribute.Name) + `="` + attributeEscaper.Replace(attribute.Value) + `"`)
	}
	return builder.String()
}
