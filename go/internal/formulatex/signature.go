package formulatex

import (
	"strings"

	"github.com/sarielhp/relatex/internal/xmltree"
)

// longestComposite is the most elements a composite (composites.go) spans: a command's own
// elements and the spaces TeX puts on both sides.
const longestComposite = 7

// compositeAt is the command whose elements start at index (\longrightarrow's bar, kern and arrow,
// with the spaces around it), the longest first, and how many elements it takes.
func compositeAt(elements []*xmltree.Node, index int) (string, int) {
	for width := min(longestComposite, len(elements)-index); width > 0; width-- {
		if tex, ok := composites[signature(elements[index:index+width])]; ok {
			return tex, width
		}
	}
	return "", 0
}

// signature is what a run of elements is as far as typesetting it again shows: names, texts,
// variants and the widths of spaces; not the boxes' measures, which follow the size.
func signature(elements []*xmltree.Node) string {
	var builder strings.Builder
	for _, node := range elements {
		writeSignature(&builder, node)
	}
	return builder.String()
}

func writeSignature(builder *strings.Builder, node *xmltree.Node) {
	builder.WriteString(node.Name)
	if variant := node.Attrs["mathvariant"]; variant != "" {
		builder.WriteString("/" + variant)
	}
	if node.Name == "mspace" {
		builder.WriteString("=" + node.Attrs["width"])
	}
	children := node.Elements()
	if len(children) == 0 {
		builder.WriteString(":" + node.Content() + ";")
		return
	}
	builder.WriteString("(")
	for _, child := range children {
		writeSignature(builder, child)
	}
	builder.WriteString(")")
}
