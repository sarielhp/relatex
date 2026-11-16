package mathexport

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// inherited is MathPresentation::INHERITED.
var inherited = []string{"mathvariant", "displaystyle", "scriptlevel", "mathsize", "mathcolor", "dir"}

// accentGlyphs is MathPresentation::ACCENT_GLYPHS.
var accentGlyphs = map[string]string{"accent": "~˜¯−^ˆ→˙¨ˇ˘‾", "accentunder": "_¯‾−⏟⎵→"}

var absoluteUnits = []string{"pt", "px", "pc", "in", "cm", "mm"}

var fenceBoxZeroFields = map[string][]string{"mpadded": {"width", "height", "depth", "lspace", "voffset"}, "mspace": {"width"},
	"mo": {"lspace", "rspace"}, "mrow": {}, "mstyle": {}}

var scriptLevel = regexp.MustCompile(`\A[+-]?\d+\z`)

// Style is a node's effective presentation style: inherited attributes by name.
type Style map[string]string

// Presentation is MathPresentation: one lossless presentation view of a formula's MathML.
type Presentation struct {
	Original string
	SHA256   string
	Root     *Node
	paths    map[*Node]string
	styles   map[*Node]Style
}

// NewPresentation is MathPresentation.new(xml).
func NewPresentation(xml string) *Presentation {
	sum := sha256.Sum256([]byte(xml))
	p := &Presentation{Original: xml, SHA256: hex.EncodeToString(sum[:]), Root: ParsePaper(xml),
		paths: map[*Node]string{}, styles: map[*Node]Style{}}
	display := "false"
	if p.Root.Get("display") == "block" {
		display = "true"
	}
	p.index(p.Root, "/math", Style{"displaystyle": display, "scriptlevel": "0"})
	return p
}

// Path is the node's location in the formula (/math/mrow[1]/mi[2]).
func (p *Presentation) Path(node *Node) string {
	path, ok := p.paths[node]
	if !ok {
		panic(rec.Failure{Message: "key not found: node", Lookup: true})
	}
	return path
}

// Style is the node's effective style.
func (p *Presentation) Style(node *Node) Style {
	style, ok := p.styles[node]
	if !ok {
		panic(rec.Failure{Message: "key not found: node", Lookup: true})
	}
	return style
}

func (p *Presentation) index(node *Node, location string, outer Style) {
	// attributes.to_h.transform_values(&:to_s): the raw values.
	script, hasScript := node.RawAttribute("scriptlevel")
	if hasScript && !scriptLevel.MatchString(script) {
		rec.Fail("%s @scriptlevel: expected an integer", location)
	}
	effective := Style{}
	for name, value := range outer {
		effective[name] = value
	}
	for _, name := range inherited {
		if value, ok := node.RawAttribute(name); ok {
			effective[name] = value
		}
	}
	if hasScript && (strings.HasPrefix(script, "+") || strings.HasPrefix(script, "-")) {
		base := 0
		if value, ok := outer["scriptlevel"]; ok {
			base = rubyToI(value)
		}
		effective["scriptlevel"] = strconv.Itoa(max(base+rubyInteger(script), 0))
	}
	p.paths[node] = location
	p.styles[node] = effective
	counts := map[string]int{}
	for position, child := range node.Elements() {
		counts[child.Name]++
		p.index(child, fmt.Sprintf("%s/%s[%d]", location, child.Name, counts[child.Name]), p.childStyle(node, effective, position))
	}
}

// rubyToI is String#to_i: the leading integer, 0 without one.
func rubyToI(text string) int {
	match := regexp.MustCompile(`\A\s*[+-]?\d+`).FindString(text)
	value, _ := strconv.Atoi(strings.TrimSpace(match))
	return value
}

// rubyInteger is Integer(text, 10) on a validated script level.
func rubyInteger(text string) int {
	value, err := strconv.Atoi(text)
	if err != nil {
		panic(fmt.Errorf("%w: %q", errArgument, text))
	}
	return value
}

var scriptParents = []string{"msub", "msup", "msubsup", "mmultiscripts", "mover", "munder", "munderover"}

func (p *Presentation) childStyle(parent *Node, style Style, index int) Style {
	if index == 0 && slices.Contains(scriptParents, parent.Name) {
		return style
	}
	increment, ok := p.scriptIncrement(parent, style, index)
	if !ok {
		return style
	}
	merged := Style{}
	for name, value := range style {
		merged[name] = value
	}
	level, has := style["scriptlevel"]
	if !has {
		panic(rec.Failure{Message: `key not found: "scriptlevel"`, Lookup: true})
	}
	merged["displaystyle"] = "false"
	merged["scriptlevel"] = strconv.Itoa(rubyInteger(level) + increment)
	return merged
}

func (p *Presentation) scriptIncrement(parent *Node, style Style, index int) (int, bool) {
	switch parent.Name {
	case "msub", "msup", "msubsup", "mmultiscripts":
		return 1, true
	case "mfrac":
		if style["displaystyle"] == "true" {
			return 0, true
		}
		return 1, true
	case "mroot":
		if index == 1 {
			return 2, true
		}
		return 0, false
	case "mover", "munder", "munderover":
	default:
		return 0, false
	}
	name := "accent"
	if parent.Name == "munder" || (parent.Name == "munderover" && index == 1) {
		name = "accentunder"
	}
	decoration := parent.Element(index + 1)
	if p.Accent(parent, name, decoration) {
		return 0, true
	}
	return 1, true
}

// Accent is MathPresentation#accent?.
func (p *Presentation) Accent(owner *Node, name string, decoration *Node) bool {
	if explicit, ok := owner.Attribute(name); ok {
		return explicit == "true"
	}
	if decoration == nil {
		crash("undefined method 'name' for nil")
	}
	text, ok := decoration.TextValue()
	return decoration.Name == "mo" && ok && strings.Contains(accentGlyphs[name], text)
}

// FixedBoxMetrics is MathPresentation#fixed_box_metrics: exact declared box metrics, or
// nil when the box needs measurement.
func (p *Presentation) FixedBoxMetrics(node *Node) []NamedLength {
	if slices.Contains([]string{"math", "mrow", "mstyle"}, node.Name) && len(node.Elements()) == 1 {
		return p.FixedBoxMetrics(node.Element(1))
	}
	if node.Name != "mpadded" {
		return nil
	}
	for _, name := range []string{"width", "height", "depth"} {
		if !node.Has(name) {
			return nil
		}
	}
	metrics := []NamedLength{}
	for _, name := range []string{"width", "height", "depth"} {
		metrics = append(metrics, NamedLength{name, NewLength(node.Get(name), true)})
	}
	for _, metric := range metrics {
		length := metric.Length
		if length.Adjustment || !slices.Contains(absoluteUnits, length.Unit) || length.Value < 0 {
			return nil
		}
	}
	return metrics
}

// NamedLength is one entry of an ordered length table.
type NamedLength struct {
	Name   string
	Length Length
}

// FenceBox is MathPresentation.fence_box: a retained fixed-size TeX delimiter box (one
// delimiter and struts) and its stretch target; fence is nil when the node is no such box.
func FenceBox(node *Node) (fence *Node, size Length) {
	if node.Name != "mpadded" || !node.Has("width") || !node.Has("height") || !node.Has("depth") {
		return nil, Length{}
	}
	height, depth := NewLength(node.Get("height"), true), NewLength(node.Get("depth"), true)
	if height.Adjustment || depth.Adjustment || height.Unit != depth.Unit {
		return nil, Length{}
	}
	if !slices.Contains(absoluteUnits, height.Unit) || !(height.Value+depth.Value > 0) {
		return nil, Length{}
	}
	descendants := node.Descendants()
	operators := []*Node{}
	for _, child := range descendants {
		if !fenceBoxChild(child) {
			return nil, Length{}
		}
		if child.Name == "mo" {
			operators = append(operators, child)
		}
	}
	if len(operators) != 1 || operators[0].Get("fence") != "true" || operators[0].Has("minsize") || operators[0].Has("maxsize") {
		return nil, Length{}
	}
	return operators[0], NewLength(FormatG(height.Value+depth.Value, 10)+height.Unit, false)
}

func fenceBoxChild(node *Node) bool {
	dimensions, known := fenceBoxZeroFields[node.Name]
	if !known {
		return false
	}
	for _, name := range append(slices.Clone(inherited), "mathbackground") {
		if node.Has(name) {
			return false
		}
	}
	if node.Name != "mo" {
		for _, text := range node.Texts() {
			if rec.Strip(text) != "" {
				return false
			}
		}
	}
	for _, name := range dimensions {
		if node.Has(name) && NewLength(node.Get(name), false).Value != 0 {
			return false
		}
	}
	return true
}
