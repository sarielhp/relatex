package contrast

import (
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/rec"
)

// blockInks are the inks of text inside a beamer block (decision 083, amendment 1). A block paints
// its title and body boxes behind its text, as a table cell may: white on a block's coloured title
// reads in the PDF, so it is no ink the author hid.
func blockInks(document rec.Map) map[string]bool {
	nodes := rec.FetchMap(document, "nodes")
	found := map[string]bool{}
	var walk func(id any, depth int)
	walk = func(id any, depth int) {
		node, ok := nodes[rec.AsString(id)].(rec.Map)
		if !ok || depth > 64 {
			return
		}
		content, _ := node["content"].([]any)
		for _, span := range inline.Deep(content) {
			if color, has := span["color"].(string); has {
				found[color] = true
			}
		}
		children, _ := node["children"].([]any)
		for _, child := range children {
			walk(child, depth+1)
		}
	}
	for id, value := range nodes {
		node, _ := value.(rec.Map)
		semantics, _ := node["semantics"].(rec.Map)
		if _, isBlock := semantics["beamer_block"].(string); isBlock && node["type"] == "theorem" {
			walk(id, 0)
		}
	}
	return found
}
