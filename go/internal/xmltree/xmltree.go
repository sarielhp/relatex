// Package xmltree reads an XML document into a small element tree, for the places the Ruby
// reference walks a parsed document (REXML or Nokogiri): element names are local names,
// attributes are looked up by local name, text keeps its whitespace with entities expanded.
package xmltree

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// Node is an element or, with an empty Name, a text node (character data or CDATA).
type Node struct {
	Name     string
	Attrs    map[string]string
	Children []*Node
	Text     string
}

// Parse reads a document and returns its root element.
func Parse(text string) (*Node, error) {
	decoder := xml.NewDecoder(strings.NewReader(text))
	document := &Node{}
	stack := []*Node{document}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		parent := stack[len(stack)-1]
		switch typed := token.(type) {
		case xml.StartElement:
			node := &Node{Name: typed.Name.Local, Attrs: map[string]string{}}
			for _, attribute := range typed.Attr {
				node.Attrs[attribute.Name.Local] = attribute.Value
			}
			parent.Children = append(parent.Children, node)
			stack = append(stack, node)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 1 {
				parent.Children = append(parent.Children, &Node{Text: string(typed)})
			}
		}
	}
	elements := document.Elements()
	if len(elements) != 1 {
		return nil, errors.New("xmltree: no single root element")
	}
	return elements[0], nil
}

// Elements is the element children, in order (REXML's elements).
func (n *Node) Elements() []*Node {
	result := []*Node{}
	for _, child := range n.Children {
		if child.Name != "" {
			result = append(result, child)
		}
	}
	return result
}

// Named is the element children with a name (REXML's get_elements(name)).
func (n *Node) Named(name string) []*Node {
	result := []*Node{}
	for _, child := range n.Children {
		if child.Name == name {
			result = append(result, child)
		}
	}
	return result
}

// Content is the text of every descendant text node, in document order (Nokogiri's text,
// REXML's .//text() joined).
func (n *Node) Content() string {
	var builder strings.Builder
	n.collect(&builder)
	return builder.String()
}

func (n *Node) collect(builder *strings.Builder) {
	if n.Name == "" {
		builder.WriteString(n.Text)
		return
	}
	for _, child := range n.Children {
		child.collect(builder)
	}
}
