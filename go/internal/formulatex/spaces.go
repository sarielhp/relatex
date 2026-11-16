package formulatex

import (
	"math"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/xmltree"
)

// namedSpaces are TeX's spaces in math units, as luamml writes them.
var namedSpaces = map[string]string{"0.167em": "\\,", "0.222em": "\\:", "0.278em": "\\;", "-0.167em": "\\!"}

// space writes an mspace: a named space, \quad and \qquad, a space of its width, or a strut
// (a box of its height and depth, as \vphantom and \phantom leave). luamml's pt is the PDF's big
// point, so lengths are written in bp.
func space(node *xmltree.Node) string {
	width := node.Attrs["width"]
	if named, ok := namedSpaces[width]; ok && len(node.Attrs) == 1 {
		return named
	}
	if _, colored := node.Attrs["mathbackground"]; colored {
		fail("mspace rule")
	}
	points := length(width)
	height, depth := length(node.Attrs["height"]), length(node.Attrs["depth"])
	if height == 0 && depth == 0 {
		return horizontalSpace(points)
	}
	strut := "\\rule[" + bigPoints(-depth) + "]{0bp}" + braced(bigPoints(height+depth))
	if points == 0 {
		return strut
	}
	return "\\phantom" + braced("\\rule["+bigPoints(-depth)+"]"+braced(bigPoints(points))+braced(bigPoints(height+depth)))
}

// horizontalSpace is a space of a width in big points: \quad and \qquad at a 10 pt font's size
// and near it, else \hspace.
func horizontalSpace(points float64) string {
	switch {
	case points > 8 && points < 13:
		return "\\quad"
	case points > 16 && points < 26:
		return "\\qquad"
	}
	return "\\hspace" + braced(bigPoints(points))
}

// length is an mspace attribute in points; absent is 0.
func length(text string) float64 {
	if text == "" {
		return 0
	}
	if !strings.HasSuffix(text, "pt") {
		fail("mspace length %s", text)
	}
	value, err := strconv.ParseFloat(strings.TrimSuffix(text, "pt"), 64)
	if err != nil {
		fail("mspace length %s", text)
	}
	return value
}

func bigPoints(value float64) string {
	return strconv.FormatFloat(math.Round(value*1e6)/1e6, 'f', -1, 64) + "bp"
}

// delimiting: a row's first and last elements are a \left...\right pair, at most one of them an
// empty delimiter.
func delimiting(first, last *xmltree.Node) bool {
	return isFence(first) && (isFence(last) || isNullDelimiter(last)) || isNullDelimiter(first) && isFence(last)
}

// isNullDelimiter: the kern TeX sets for an empty \left. or \right. delimiter.
func isNullDelimiter(node *xmltree.Node) bool {
	return node.Name == "mspace" && len(node.Attrs) == 1 && node.Attrs["width"] == "1.196pt"
}
