package html

import (
	"slices"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
)

// blockStyle is the stylesheet of beamer blocks (decision 083, amendment 1): a block is a theorem
// box whose border names its family, alerted or example; its text keeps the PDF's own styles.
const blockStyle = `.block-alerted { border-left-color: var(--alert); }
.block-example { border-left-color: var(--proof); }
`

var blockFamilies = []string{"block", "alerted", "example"}

// BlockFamily is a beamer block's template family (block, alerted, example), or "" for a node
// that is no block. A family relatex does not know reads as a plain block.
func BlockFamily(node rec.Map) string {
	semantics, _ := node["semantics"].(rec.Map)
	family, isBlock := semantics["beamer_block"].(string)
	switch {
	case !isBlock || node["type"] != "theorem":
		return ""
	case slices.Contains(blockFamilies, family):
		return family
	}
	return "block"
}

// blockCSS is blockStyle for a document holding a block, else nothing: a paper's page is unchanged.
func (p *Paper) blockCSS() string {
	for _, value := range p.nodes {
		if node, ok := value.(rec.Map); ok && BlockFamily(node) != "" {
			return blockStyle
		}
	}
	return ""
}

// blockUntitled: the block's title box was empty, so its first paragraph is body text.
func blockUntitled(node rec.Map) bool {
	semantics, _ := node["semantics"].(rec.Map)
	untitled, _ := semantics["untitled"].(bool)
	return untitled
}

// blockFamilyNames are the names of an untitled block by its family (decision 167 amendment 4).
var blockFamilyNames = map[string]string{"block": "Block", "alerted": "Alert block", "example": "Example block"}

// blockNode renders a beamer block: a group named by its first paragraph, the block's title; an
// untitled block in an English page is named by its family.
func (p *Paper) blockNode(identifier string, node rec.Map, family string) string {
	classes := "theorem block"
	if family != "block" {
		classes += " block-" + family
	}
	label := ""
	if blockUntitled(node) && p.english() {
		label = " role=\"group\" aria-label=\"" + blockFamilyNames[family] + "\""
	}
	if children := rec.FetchList(node, "children"); len(children) > 0 && !blockUntitled(node) {
		if first := p.node(children[0]); first["type"] == "paragraph" {
			label = " role=\"group\" aria-labelledby=\"" + htmlkit.Escape(rec.AsString(children[0])) + "\""
		}
	}
	return "<div id=\"" + identifier + "\" data-structure=\"theorem\" data-block-family=\"" + family +
		"\" class=\"" + classes + "\"" + label + ">" + p.children(node) + "</div>"
}
