package typstsource

import (
	"github.com/sarielhp/relatex/internal/rec"
)

// beamerBlock renders a beamer block (decision 083, amendment 1) as a titled box: its first
// paragraph is the title, set strong, and the rest is the body. A block is no numbered theorem:
// its title is not a theorem name. ok is false for a node that is no block.
func (e *exporter) beamerBlock(node rec.Map) (string, bool) {
	if _, isBlock := dig(node, "semantics", "beamer_block").(string); !isBlock {
		return "", false
	}
	parts := e.nodesOf(node)
	outputs := e.nestedList(func() []string {
		outputs := []string{}
		for index, part := range parts {
			if index == 0 && part["type"] == "paragraph" && dig(node, "semantics", "untitled") != true {
				if title := rec.Strip(e.inline(content(part), part, 0, false)); title != "" {
					outputs = append(outputs, "#strong["+title+"]")
				}
				continue
			}
			outputs = append(outputs, e.renderNode(toS(part["id"])))
		}
		return outputs
	})
	return "#block(width: 100%, inset: 0.6em, stroke: (left: 0.25em + luma(160)))[" + joinPresent("\n\n", outputs...) + "]", true
}
