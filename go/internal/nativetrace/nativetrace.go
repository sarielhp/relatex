// Package nativetrace is the Go port of Relatex::NativeTrace: mutool's XML traces of a PDF
// (structured text, paint traces), read as untrusted input: strict parsing, no declarations.
// It also holds the Ruby conversions the trace readers share (Kernel#Float on an attribute).
package nativetrace

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/sarielhp/relatex/internal/rec"
)

// Declared reports whether XML text holds a declaration or entity, which traces may not.
func Declared(data []byte) bool {
	return bytes.Contains(data, []byte("<!DOCTYPE")) || bytes.Contains(data, []byte("<!ENTITY"))
}

// Node is an element of a trace: its name, attributes, element children and parent.
type Node struct {
	Name     string
	Attrs    map[string]string
	Children []*Node
	Parent   *Node
}

// Parse is NativeTrace.parse with Nokogiri's strict options: the document node (its one
// child is the root element) of a trace without declarations.
func Parse(data []byte) *Node {
	if Declared(data) {
		rec.Fail("Native trace declarations forbidden")
	}
	document, err := parseByPages(data)
	if err != nil {
		rec.Fail("Invalid native trace XML: %s", FirstLine(err.Error()))
	}
	return document
}

// parallelFrom is the size from which a trace is read in pieces.
const parallelFrom = 8 << 20

var pageStart = []byte("\n<page ")

// parseByPages is ParseTree for a large trace: its pages are independent top-level elements, so the
// text is cut before page elements into pieces that are read side by side and joined under the one root.
// Anything that is not a plain document of pages (and any failure) is read whole, which reports it.
func parseByPages(data []byte) (*Node, error) {
	if len(data) < parallelFrom {
		return ParseTree(data)
	}
	return parseByPagesForced(data)
}

// Pieces cuts a large trace into complete documents of whole pages (the root element around each
// run of pages), or returns nil when the trace is small or not a plain document of pages.
func Pieces(data []byte) [][]byte {
	if len(data) < parallelFrom {
		return nil
	}
	return piecesOf(data)
}

func piecesOf(data []byte) [][]byte {
	open := bytes.Index(data, []byte("<document"))
	if open < 0 {
		return nil
	}
	end := bytes.IndexByte(data[open:], '>') + open + 1
	rootTag := data[open:end]
	closing := []byte("</document>")
	trimmed := bytes.TrimRight(data, " \r\n\t")
	if end == open || rootTag[len(rootTag)-2] == '/' || !bytes.HasSuffix(trimmed, closing) {
		return nil
	}
	body := trimmed[end : len(trimmed)-len(closing)]
	cut := cutAtPages(body, runtime.NumCPU())
	if len(cut) < 2 {
		return nil
	}
	pieces := make([][]byte, len(cut))
	for index, piece := range cut {
		pieces[index] = slices.Concat(data[:end], piece, closing)
	}
	return pieces
}

// parseByPagesForced cuts whatever its size.
func parseByPagesForced(data []byte) (*Node, error) {
	pieces := piecesOf(data)
	if pieces == nil {
		return ParseTree(data)
	}
	roots := make([]*Node, len(pieces))
	failures := make([]error, len(pieces))
	var group sync.WaitGroup
	for index, text := range pieces {
		group.Add(1)
		go func() {
			defer group.Done()
			document, err := ParseTree(text)
			if err == nil {
				roots[index] = document.Children[0]
			}
			failures[index] = err
		}()
	}
	group.Wait()
	if slices.ContainsFunc(failures, func(err error) bool { return err != nil }) {
		return ParseTree(data)
	}
	document := &Node{}
	root := roots[0]
	root.Parent = document
	document.Children = []*Node{root}
	for _, other := range roots[1:] {
		for _, child := range other.Children {
			child.Parent = root
		}
		root.Children = append(root.Children, other.Children...)
	}
	return document, nil
}

// cutAtPages cuts text before page elements into at most count pieces of about the same size.
func cutAtPages(text []byte, count int) [][]byte {
	pieces := [][]byte{}
	target := len(text)/count + 1
	start := 0
	for start < len(text) {
		stop := len(text)
		if start+target < len(text) {
			next := bytes.Index(text[start+target:], pageStart)
			if next >= 0 {
				stop = start + target + next + 1
			}
		}
		pieces = append(pieces, text[start:stop])
		start = stop
	}
	return pieces
}

// ParseTree reads XML into a document node whose children are its root elements; text is
// not kept.
func ParseTree(data []byte) (*Node, error) {
	decoder := xml.NewDecoder(bytes.NewReader(readable(data)))
	document := &Node{}
	current := document
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			child := &Node{Name: typed.Name.Local, Attrs: attributes(typed), Parent: current}
			current.Children = append(current.Children, child)
			current = child
		case xml.EndElement:
			current = current.Parent
		}
	}
	if len(document.Children) != 1 {
		return nil, errors.New("document has no single root element")
	}
	return document, nil
}

func attributes(element xml.StartElement) map[string]string {
	result := make(map[string]string, len(element.Attr))
	for _, attribute := range element.Attr {
		result[attribute.Name.Local] = attribute.Value
	}
	return result
}

// FirstLine is the first line of an error message, as Ruby's message.lines.first.
func FirstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line + "\n"
}

// EachElement is NativeTrace.each_element: each element's name, attributes and its open
// ancestors, outermost first, in document order.
func EachElement(data []byte, visit func(name string, attrs map[string]string, open []*Node)) {
	if Declared(data) {
		rec.Fail("Native trace declarations forbidden")
	}
	decoder := xml.NewDecoder(bytes.NewReader(readable(data)))
	open := []*Node{}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			rec.Fail("Invalid native trace XML: %s", FirstLine(err.Error()))
		}
		switch typed := token.(type) {
		case xml.StartElement:
			element := &Node{Name: typed.Name.Local, Attrs: attributes(typed)}
			visit(element.Name, element.Attrs, open)
			open = append(open, element)
		case xml.EndElement:
			open = open[:len(open)-1]
		}
	}
	if len(open) != 0 {
		rec.Fail("Truncated native trace XML")
	}
}

// Descendants is .//name below n, in document order.
func (n *Node) Descendants(name string) []*Node {
	result := []*Node{}
	var walk func(*Node)
	walk = func(current *Node) {
		for _, child := range current.Children {
			if child.Name == name {
				result = append(result, child)
			}
			walk(child)
		}
	}
	walk(n)
	return result
}

// Named are the element children with a name.
func (n *Node) Named(name string) []*Node {
	result := []*Node{}
	for _, child := range n.Children {
		if child.Name == name {
			result = append(result, child)
		}
	}
	return result
}

// Names are the element children's names.
func (n *Node) Names() []string {
	result := make([]string, len(n.Children))
	for index, child := range n.Children {
		result[index] = child.Name
	}
	return result
}

// Attr is node[name]: the value and whether the attribute exists.
func (n *Node) Attr(name string) (string, bool) {
	value, ok := n.Attrs[name]
	return value, ok
}

// FetchAttr is node.attributes.fetch(name): a KeyError (an ArgumentError here) when it is
// missing.
func (n *Node) FetchAttr(name string) string {
	value, ok := n.Attrs[name]
	if !ok {
		rec.RaiseArgument("key not found: %q", name)
	}
	return value
}

// Float is Kernel#Float on a string (surrounding whitespace allowed); an ArgumentError for
// anything else.
func Float(text string) float64 {
	trimmed := strings.TrimSpace(text)
	value, err := strconv.ParseFloat(trimmed, 64)
	bare := strings.TrimLeft(trimmed, "+-")
	if err != nil || trimmed == "" || strings.ContainsAny(text, "xXpP") || strings.EqualFold(bare, "inf") ||
		strings.EqualFold(bare, "infinity") || strings.EqualFold(trimmed, "nan") {
		rec.RaiseArgument("invalid value for Float(): %q", text)
	}
	return value
}

// Pages is a large trace read page by page: the text stays as bytes and each page is parsed when it
// is asked for, so a document of hundreds of pages never holds all their elements at once.
type Pages struct {
	data           []byte
	prefix, suffix []byte
	starts, stops  []int
}

// SplitPages cuts a large plain document of pages (the root element around page elements only) at
// its pages, without parsing them; it returns nil for a small trace and for anything else, which
// Parse reads whole.
func SplitPages(data []byte) *Pages {
	if len(data) < parallelFrom {
		return nil
	}
	return splitPages(data)
}

// splitPages cuts whatever its size.
func splitPages(data []byte) *Pages {
	if Declared(data) {
		return nil
	}
	open := bytes.Index(data, []byte("<document"))
	if open < 0 {
		return nil
	}
	end := bytes.IndexByte(data[open:], '>') + open + 1
	closing := []byte("</document>")
	trimmed := bytes.TrimRight(data, " \r\n\t")
	if end == open || data[end-2] == '/' || !bytes.HasSuffix(trimmed, closing) {
		return nil
	}
	bodyEnd := len(trimmed) - len(closing)
	pages := &Pages{data: data, prefix: data[:end], suffix: closing}
	for at := end; at < bodyEnd; {
		next := bytes.Index(data[at:bodyEnd], pageStart)
		if next < 0 {
			break
		}
		pages.starts = append(pages.starts, at+next+1)
		at += next + 1
	}
	if len(pages.starts) == 0 || len(bytes.TrimSpace(data[end:pages.starts[0]])) != 0 {
		return nil
	}
	for index := range pages.starts {
		stop := bodyEnd
		if index+1 < len(pages.starts) {
			stop = pages.starts[index+1]
		}
		pages.stops = append(pages.stops, stop)
	}
	return pages
}

// Len is the number of pages.
func (p *Pages) Len() int { return len(p.starts) }

// Page parses page index (from 0): the page element, below a root of its own.
func (p *Pages) Page(index int) *Node {
	text := slices.Concat(p.prefix, p.data[p.starts[index]:p.stops[index]], p.suffix)
	document, err := ParseTree(text)
	if err != nil {
		rec.Fail("Invalid native trace XML: %s", FirstLine(err.Error()))
	}
	root := document.Children[0]
	if len(root.Children) != 1 || root.Children[0].Name != "page" {
		rec.Fail("Invalid native trace XML: page %d is not one page element\n", index+1)
	}
	return root.Children[0]
}

// Floats is text.split.map { Float(_1) }.
func Floats(text string) []float64 {
	fields := strings.Fields(text)
	result := make([]float64, len(fields))
	for index, field := range fields {
		result[index] = Float(field)
	}
	return result
}
