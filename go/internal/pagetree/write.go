package pagetree

import "strings"

var (
	textEscaper      = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	attributeEscaper = strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;")
)

// WriteHTML writes the node as HTML: quoted attributes, escaped text, void elements without an
// end tag, the text of style and script as it stands. Read back with Parse it is the same tree.
func WriteHTML(builder *strings.Builder, n *Node) {
	writeHTML(builder, n, false)
}

func writeHTML(builder *strings.Builder, n *Node, raw bool) {
	if n.Name == "" {
		if raw {
			builder.WriteString(n.Text)
		} else {
			builder.WriteString(textEscaper.Replace(n.Text))
		}
		return
	}
	builder.WriteString("<" + n.Name)
	for _, a := range n.Attrs {
		builder.WriteString(" " + a.Name + `="` + attributeEscaper.Replace(a.Value) + `"`)
	}
	builder.WriteString(">")
	if voidElements[n.Name] && len(n.Children) == 0 {
		return
	}
	for _, child := range n.Children {
		writeHTML(builder, child, rawTextElements[n.Name])
	}
	builder.WriteString("</" + n.Name + ">")
}

// HTML is the node written as HTML (WriteHTML).
func HTML(n *Node) string {
	var builder strings.Builder
	WriteHTML(&builder, n)
	return builder.String()
}

// Clone is a deep copy of the node.
func Clone(n *Node) *Node {
	copied := &Node{Name: n.Name, Text: n.Text, Attrs: append([]Attribute(nil), n.Attrs...)}
	if len(n.Children) > 0 {
		copied.Children = make([]*Node, len(n.Children))
		for index, child := range n.Children {
			copied.Children[index] = Clone(child)
		}
	}
	return copied
}

// Child is the first child element with the name, and its index; -1 and nil when there is none.
func Child(parent *Node, name string) (int, *Node) {
	for index, child := range parent.Children {
		if child.Name == name {
			return index, child
		}
	}
	return -1, nil
}
