package mathexport

import (
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// MathProjectionSupport: an exporter contract, not a restriction on retained MathML. A TeX
// limitation selects native MathML in Markdown; Typst reports it explicitly.
var (
	supportMetadata   = []string{"xmlns", "class", "intent"}
	supportStyle      = []string{"mathvariant", "displaystyle", "scriptlevel", "mathsize", "mathcolor", "mathbackground", "dir"}
	supportAttributes = map[string][]string{
		"math":  {"display"},
		"mo":    {"lspace", "rspace", "stretchy", "fence", "symmetric", "movablelimits", "minsize", "maxsize"},
		"mfrac": {"linethickness", "bevelled"}, "mover": {"accent"}, "munder": {"accentunder"},
		"munderover": {"accent", "accentunder"}, "mspace": {"width", "height", "depth"},
		"mpadded": {"width", "height", "depth", "lspace", "voffset"},
		"mtable":  {"columnalign", "columnspacing", "rowalign", "rowspacing"},
		"mtr":     {"columnalign", "rowalign"}, "mtd": {"columnalign", "rowalign", "columnspan", "rowspan"},
	}
	supportBoolean = []string{"displaystyle", "stretchy", "fence", "symmetric", "movablelimits", "accent", "accentunder", "bevelled"}
	texMathML      = []string{"mathsize", "mathcolor", "mathbackground", "minsize", "maxsize", "height", "depth", "voffset"}
	pandocNative   = map[string][]string{"mfrac": {"linethickness"}, "mo": {"movablelimits"}, "mtable": {"columnalign"},
		"mtr": {"columnalign"}, "mtd": {"columnalign"}, "mspace": {"width"}}
	supportDefaults = map[string]string{"dir": "ltr", "bevelled": "false", "columnspan": "1", "rowspan": "1",
		"frame": "none", "rowlines": "none", "columnlines": "none"}
)

// validateSupport is MathProjectionSupport.validate!(presentation, target:).
func validateSupport(p *Presentation, target string) {
	walk(p.Root, func(node *Node) {
		for _, attribute := range node.Attrs {
			name, value := attribute.Expanded(), attribute.Value()
			if issue := attributeIssue(node, name, value, target); issue != "" {
				rec.Fail("%s %s @%s=%s: %s", target, p.Path(node), name, rubyInspect(value), issue)
			}
		}
	})
}

func walk(node *Node, visit func(*Node)) {
	visit(node)
	for _, child := range node.Elements() {
		walk(child, visit)
	}
}

func attributeIssue(node *Node, name, value, target string) string {
	if slices.Contains(supportMetadata, name) {
		return ""
	}
	if slices.Contains(supportBoolean, name) && value != "true" && value != "false" {
		return "Expected a MathML boolean"
	}
	if defaultValue, ok := supportDefaults[name]; ok {
		if defaultValue == value {
			return ""
		}
		return "Only the declared default is implemented"
	}
	if name == "rowalign" && slices.Contains(strings.Fields(value), "axis") {
		return "Axis-aligned matrix cells need an axis metric adapter"
	}
	if !slices.Contains(supportStyle, name) && !slices.Contains(supportAttributes[node.Name], name) {
		return "No native projection for this meaningful attribute"
	}
	if name == "symmetric" && value == "false" {
		return "Asymmetric stretchy operators require a native projection"
	}
	if name == "display" && value != "block" && value != "inline" {
		return "Unimplemented MathML display mode"
	}
	if name == "scriptlevel" && !scriptLevel.MatchString(value) {
		return "Malformed mathematical script level"
	}
	return dialectIssue(node, name, value, target)
}

func dialectIssue(node *Node, name, value, target string) string {
	if target == "pandoc" && slices.Contains(pandocNative[node.Name], name) {
		return "Native MathML preserves this attribute outside the Markdown TeX dialect"
	}
	if target != "tex" && target != "pandoc" {
		return ""
	}
	if slices.Contains(texMathML, name) {
		return "Native MathML preserves this geometry/style in Markdown"
	}
	if name == "rowalign" || name == "rowspacing" || name == "columnspacing" {
		return "Native MathML preserves per-row and per-column spacing in Markdown"
	}
	if node.Name == "mpadded" {
		return "Native MathML preserves adjusted box geometry in Markdown"
	}
	if node.Name == "mo" && (name == "stretchy" || name == "fence" || name == "symmetric") && value == "true" {
		return "Native MathML preserves variable fence sizing in Markdown"
	}
	return ""
}

// rubyInspect is String#inspect for attribute values (printable text).
func rubyInspect(value string) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for index, character := range value {
		switch {
		case character == '"' || character == '\\':
			builder.WriteByte('\\')
			builder.WriteRune(character)
		case character == '#' && index+1 < len(value) && strings.IndexByte("{$@", value[index+1]) >= 0:
			builder.WriteString(`\#`)
		case character == '\n':
			builder.WriteString(`\n`)
		case character == '\t':
			builder.WriteString(`\t`)
		case character == '\r':
			builder.WriteString(`\r`)
		case character == 0x1b:
			builder.WriteString(`\e`)
		case character < 0x20 || character == 0x7f:
			builder.WriteString(`\x` + strings.ToUpper(string("0123456789abcdef"[character>>4])+string("0123456789abcdef"[character&15])))
		default:
			builder.WriteRune(character)
		}
	}
	builder.WriteByte('"')
	return builder.String()
}
