package browser

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/xmltree"
)

// inlineAttributes are the attributes the page gives a figure shown as inline SVG (decision 146),
// beside those of its file's root but its size.
var inlineAttributes = []string{"aria-label", "data-asset", "data-height-sp", "data-kind", "data-src", "data-width-sp", "id", "role", "style"}

var urlReference = regexp.MustCompile(`url\(\s*(['"]?)#`)

// pictureMarkup is a figure file as the page must hold it inline (decision 146): its root's
// attributes but width and height, and its content as the inventory reads it back (each element's
// name, attributes and children, blank text left out), every id prefixed and every local
// reference with it; and the ids it holds.
type pictureMarkup struct {
	root    rec.Map
	content []any
	ids     []string
}

// readPictureMarkup reads an SVG file into its inline markup with ids prefixed by prefix.
func readPictureMarkup(data []byte, prefix string) (pictureMarkup, error) {
	decoder := xml.NewDecoder(bytes.NewReader(xmltree.AttributeWhitespace(xmltree.Characters(data))))
	result := pictureMarkup{root: rec.Map{}}
	stack, top := []rec.Map{}, rec.Map(nil)
	for {
		token, err := decoder.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return pictureMarkup{}, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if len(stack) == 0 && result.content != nil {
				return pictureMarkup{}, errors.New("more than one root")
			}
			element := rec.Map{"tag": typed.Name.Local, "attributes": rec.Map{}, "children": []any{}}
			for _, attribute := range typed.Attr {
				name, value := spelled(attribute.Name), attribute.Value
				if len(stack) == 0 {
					if name != "width" && name != "height" {
						result.root[name] = value
					}
					continue
				}
				value = withPrefix(name, value, prefix)
				if name == "id" {
					result.ids = append(result.ids, value)
				}
				rec.AsMap(element["attributes"])[name] = value
			}
			if len(stack) == 0 {
				result.content = []any{}
				top = element
			} else {
				appendChild(stack[len(stack)-1], element)
			}
			stack = append(stack, element)
		case xml.EndElement:
			if len(stack) == 0 {
				return pictureMarkup{}, errors.New("unbalanced end tag")
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				appendText(stack[len(stack)-1], string(typed))
			}
		}
	}
	if len(stack) == 0 && top != nil {
		result.content = top["children"].([]any)
		return result, nil
	}
	return pictureMarkup{}, errors.New("no SVG root")
}

// appendText adds text to an element, joined to the text before it as a page's parser joins it.
func appendText(element rec.Map, text string) {
	children := element["children"].([]any)
	if count := len(children); count > 0 {
		if previous, ok := children[count-1].(string); ok {
			children[count-1] = previous + text
			element["children"] = children
			return
		}
	}
	element["children"] = append(children, text)
}

func appendChild(parent, element rec.Map) {
	parent["children"] = append(parent["children"].([]any), element)
}

// withoutBlankText drops the blank text the inventory leaves out, at every level.
func withoutBlankText(children []any) []any {
	kept := []any{}
	for _, child := range children {
		switch typed := child.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				kept = append(kept, typed)
			}
		case rec.Map:
			typed["children"] = withoutBlankText(typed["children"].([]any))
			kept = append(kept, typed)
		}
	}
	return kept
}

func spelled(name xml.Name) string {
	if name.Space == "" {
		return name.Local
	}
	return name.Space + ":" + name.Local
}

// withPrefix is an attribute's value with the figure's ids prefixed: an id, a local href, url(#…).
func withPrefix(name, value, prefix string) string {
	switch {
	case name == "id":
		return prefix + value
	case (name == "href" || strings.HasSuffix(name, ":href")) && strings.HasPrefix(value, "#"):
		return "#" + prefix + value[1:]
	}
	return urlReference.ReplaceAllString(value, "url(${1}#"+prefix)
}

// validateInlinePicture checks a figure shown as inline SVG (decision 146) against the bytes the
// document displays for it: the file's root attributes and its whole markup, ids prefixed by the
// figure's id, at the file's intrinsic size. It returns the ids the markup holds.
func validateInlinePicture(image, span, document rec.Map) []string {
	data, _ := figures.Displayed(document, span)
	markup, err := readPictureMarkup(data, rec.ToS(span["id"])+"-")
	if err != nil {
		rec.Fail("Inline figure file is not SVG: %v", err)
	}
	root := rec.FetchMap(image, "root")
	for name, value := range markup.root {
		if !rec.Equal(root[name], value) {
			rec.Fail("Browser changed inline figure root %s", name)
		}
	}
	if !rec.Equal(rec.Fetch(image, "content"), withoutBlankText(markup.content)) {
		rec.Fail("Browser changed inline figure markup")
	}
	box := strings.Fields(strings.ReplaceAll(rec.ToS(markup.root["viewBox"]), ",", " "))
	if len(box) != 4 {
		rec.Fail("Inline figure has no viewBox")
	}
	for index, dimension := range []string{"width", "height"} {
		points, err := strconv.ParseFloat(box[2+index], 64)
		if err != nil || !(points > 0) || math.Abs(points*96/72-pixels(rec.Fetch(span, dimension+"_sp"))) >= 1.0 {
			rec.Fail("Browser changed SVG intrinsic dimensions")
		}
	}
	return markup.ids
}

// rootAttributeNames are the attribute names of the file's root the page keeps on the inline svg.
func rootAttributeNames(span, document rec.Map) []string {
	data, _ := figures.Displayed(document, span)
	markup, err := readPictureMarkup(data, "")
	if err != nil {
		rec.Fail("Inline figure file is not SVG: %v", err)
	}
	names := make([]string, 0, len(markup.root))
	for name := range markup.root {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
