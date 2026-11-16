package figscan

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// element is a generic XML element of an Ipe document.
type element struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Text     string     `xml:",chardata"`
	Children []element  `xml:",any"`
}

func (e *element) attr(name string) string {
	for _, a := range e.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (e *element) has(name string) bool {
	for _, a := range e.Attrs {
		if a.Name.Local == name {
			return true
		}
	}
	return false
}

// parseIpe decodes an Ipe XML document. Ipe 7 writes no DOCTYPE or entities beyond XML's own.
func parseIpe(source []byte) (*element, error) {
	decoder := xml.NewDecoder(bytes.NewReader(source))
	decoder.Strict = false
	root := &element{}
	if err := decoder.Decode(root); err != nil {
		return nil, fmt.Errorf("ipe xml: %w", err)
	}
	if root.XMLName.Local != "ipe" {
		return nil, fmt.Errorf("ipe xml: root is <%s>", root.XMLName.Local)
	}
	return root, nil
}

// styles are the symbolic values of an Ipe document's stylesheets; a later stylesheet overrides
// an earlier one, as in Ipe's cascade (the file lists the bottom sheet first).
type styles struct {
	colors    map[string]string
	pens      map[string]float64
	opacities map[string]float64
}

// ipeDefaultColors are the standard sheet's colours, for names a file uses without defining.
var ipeDefaultColors = map[string]string{
	"black": "0", "white": "1", "red": "1 0 0", "green": "0 1 0", "blue": "0 0 1", "yellow": "1 1 0",
	"orange": "1 0.647 0", "gold": "1 0.843 0", "purple": "0.627 0.125 0.941", "gray": "0.745",
	"brown": "0.647 0.165 0.165", "navy": "0 0 0.502", "pink": "1 0.753 0.796", "seagreen": "0.18 0.545 0.341",
	"turquoise": "0.251 0.878 0.816", "violet": "0.933 0.51 0.933", "darkblue": "0 0 0.545",
	"darkcyan": "0 0.545 0.545", "darkgray": "0.663", "darkgreen": "0 0.392 0", "darkmagenta": "0.545 0 0.545",
	"darkorange": "1 0.549 0", "darkred": "0.545 0 0", "lightblue": "0.678 0.847 0.902",
	"lightcyan": "0.878 1 1", "lightgray": "0.827", "lightgreen": "0.565 0.933 0.565",
	"lightyellow": "1 1 0.878",
}

func readStyles(root *element) styles {
	s := styles{colors: map[string]string{}, pens: map[string]float64{"normal": 0.4, "heavier": 0.8, "fat": 1.2, "ultrafat": 2},
		opacities: map[string]float64{"opaque": 1}}
	for name, value := range ipeDefaultColors {
		s.colors[name] = value
	}
	for i := range root.Children {
		sheet := &root.Children[i]
		if sheet.XMLName.Local != "ipestyle" {
			continue
		}
		s.readSheet(sheet)
	}
	return s
}

func (s styles) readSheet(sheet *element) {
	for i := range sheet.Children {
		entry := &sheet.Children[i]
		name, value := entry.attr("name"), entry.attr("value")
		switch entry.XMLName.Local {
		case "color":
			s.colors[name] = value
		case "pen":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				s.pens[name] = v
			}
		case "opacity":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				s.opacities[name] = v
			}
		}
	}
}

// colorName is a colour as the author named it, or its numeric value written readably.
func (s styles) colorName(value string) string {
	if value == "" {
		return ""
	}
	if _, ok := s.colors[value]; ok || numbers(value) == nil {
		return value
	}
	return numericColorName(numbers(value))
}

func numericColorName(values []float64) string {
	switch len(values) {
	case 1:
		switch values[0] {
		case 0:
			return "black"
		case 1:
			return "white"
		}
		return fmt.Sprintf("gray %.0f%%", values[0]*100)
	case 3:
		return fmt.Sprintf("#%02x%02x%02x", channel(values[0]), channel(values[1]), channel(values[2]))
	}
	return "unknown"
}

func channel(value float64) int { return int(math.Round(math.Max(0, math.Min(1, value)) * 255)) }

// rgb resolves a colour attribute to its components; ok is false for an unknown name.
func (s styles) rgb(value string) ([3]float64, bool) {
	if named, ok := s.colors[value]; ok {
		value = named
	}
	values := numbers(value)
	switch len(values) {
	case 1:
		return [3]float64{values[0], values[0], values[0]}, true
	case 3:
		return [3]float64{values[0], values[1], values[2]}, true
	}
	return [3]float64{}, false
}

// contrast is a colour's contrast against white paper, scaled to (0, 1]: black is 1. An unknown
// colour counts as fully contrasting.
func (s styles) contrast(value string) float64 {
	color, ok := s.rgb(value)
	if !ok {
		return 1
	}
	return contrastOf(color)
}

// contrastOf is an RGB colour's contrast against white paper, scaled to (0, 1]: black is 1.
func contrastOf(color [3]float64) float64 {
	luminance := float64(0.2126*linear(color[0])) + float64(0.7152*linear(color[1])) + float64(0.0722*linear(color[2]))
	return (1.05/(luminance+0.05) - 1) / 20
}

func linear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// pen is a pen attribute's width in points (Ipe's normal pen when absent or unknown).
func (s styles) pen(value string) float64 {
	if value == "" {
		return s.pens["normal"]
	}
	if v, err := strconv.ParseFloat(value, 64); err == nil {
		return v
	}
	if v, ok := s.pens[value]; ok {
		return v
	}
	return s.pens["normal"]
}

// opacity is an opacity attribute's value, 1 when absent or unknown.
func (s styles) opacity(value string) float64 {
	if v, ok := s.opacities[value]; ok {
		return v
	}
	if v, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64); err == nil && strings.HasSuffix(value, "%") {
		return v / 100
	}
	return 1
}
