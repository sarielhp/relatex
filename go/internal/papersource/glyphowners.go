package papersource

import (
	"encoding/json"
	"errors"
	"io"
	"maps"
	"math"
	"os"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/assets"
	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/glyphowner"
	"github.com/sarielhp/relatex/internal/reading"
	"github.com/sarielhp/relatex/internal/rec"
)

// The glyph owners (decision 105, glyphowner): each glyph of the glyph map with the node it belongs
// to. A glyph of a formula belongs to the formula's math node (its occurrence on the glyph's page);
// another glyph to the node of the reading block its flow item went into (reading.SlotKey).

// unknownPosition is the glyph map's flag for a glyph inside a literal-drawn box.
const unknownPosition = 8

// leafPreference ranks the nodes made from one reading block: the one that holds its text first.
var leafPreference = map[string]int{"paragraph": 1, "heading": 2, "float_caption": 3, "caption": 3, "display_math": 5}

// ownerIndex finds the nodes glyphs belong to.
type ownerIndex struct {
	blocks   map[reading.SlotKey]string // reading block by flow item
	formulas map[[2]int64]string        // math occurrence by formula and page
	nodes    map[string]string          // node id by inspection key
	leaves   map[string]int
	owners   *glyphowner.Owners
}

// glyphMapPage is the part of a glyph map page the owners read.
type glyphMapPage struct {
	Page   int `json:"page"`
	Glyphs struct {
		Text    []string `json:"text"`
		DX      []int64  `json:"dx"`
		DY      []int64  `json:"dy"`
		Formula []int64  `json:"formula"`
		Owner   []int64  `json:"owner"`
		Slot    []int64  `json:"slot"`
		Flags   []int64  `json:"flags"`
		Font    []int64  `json:"font"`
		Char    []int64  `json:"char"`
	} `json:"glyphs"`
	// Metrics is each glyph's {width, height, depth} in sp by "font:char".
	Metrics map[string][]int64 `json:"metrics"`
	// Matrices holds the glyphs (1-based) drawn under a graphicx transform.
	Matrices map[string]any `json:"matrices"`
}

// GlyphOwners are the owners of every glyph of the final pass's glyph map; nil when the build has
// no glyph map or the collector recorded no owners (a glyph map from before decision 105).
func (s *Source) GlyphOwners(document rec.Map) *glyphowner.Owners {
	name := s.streamPath + ".glyphs.jsonl"
	if _, inventoried := s.manifest[name]; !inventoried || s.slotBlocks == nil {
		return nil
	}
	index := newOwnerIndex(document, s.retention, s.slotBlocks)
	if !s.eachGlyphPage(func(page glyphMapPage) bool {
		if len(page.Glyphs.Owner) != len(page.Glyphs.Text) {
			return false
		}
		index.page(page)
		return true
	}) {
		return nil
	}
	index.owners.Pictures = drawnPictures(document, s.pictures)
	return index.owners
}

// eachGlyphPage reads the final pass's glyph map page by page until visit says stop; false when it
// stopped.
func (s *Source) eachGlyphPage(visit func(glyphMapPage) bool) bool {
	file, err := os.Open(s.artifactPath(s.streamPath + ".glyphs.jsonl"))
	check(err)
	defer file.Close()
	decoder := json.NewDecoder(file)
	for {
		var page glyphMapPage
		err := decoder.Decode(&page)
		if errors.Is(err, io.EOF) {
			return true
		}
		if err != nil {
			rec.Fail("Invalid glyph map: %v", err)
		}
		if !visit(page) {
			return false
		}
	}
}

// drawnPictures are the document's images cropped from a drawn picture, with the picture's page
// and box: an image node takes its file's next picture, in the order of the nodes' ids.
func drawnPictures(document rec.Map, cropped map[string][]assets.DrawnPicture) []glyphowner.Picture {
	if len(cropped) == 0 {
		return nil
	}
	cropped = maps.Clone(cropped)
	var pictures []glyphowner.Picture
	nodes, _ := document["nodes"].(rec.Map)
	for _, id := range slices.Sorted(maps.Keys(nodes)) {
		for _, image := range imageItems(rec.AsMap(nodes[id])["content"], nil) {
			source, _ := image["source"].(string)
			node, _ := image["id"].(string)
			queue := cropped[source]
			if len(queue) == 0 || node == "" {
				continue
			}
			pictures = append(pictures, glyphowner.Picture{Node: node, Page: queue[0].Page, Box: queue[0].Box})
			cropped[source] = queue[1:]
		}
	}
	return pictures
}

// imageItems are the image items of a node's content, table cells' content included.
func imageItems(content any, into []rec.Map) []rec.Map {
	items, _ := content.([]any)
	for _, value := range items {
		item, _ := value.(rec.Map)
		if item["type"] == "image" {
			into = append(into, item)
		}
		if grid, ok := item["grid"].(rec.Map); ok {
			cells, _ := grid["cells"].([]any)
			for _, cell := range cells {
				into = imageItems(rec.AsMap(cell)["content"], into)
			}
		}
	}
	return into
}

func newOwnerIndex(document, retention rec.Map, blocks map[reading.SlotKey]string) *ownerIndex {
	index := &ownerIndex{blocks: blocks, formulas: map[[2]int64]string{}, nodes: map[string]string{},
		leaves: map[string]int{}, owners: &glyphowner.Owners{SchemaVersion: glyphowner.Version, Leaves: []string{}}}
	for _, value := range rec.FetchList(retention, "occurrences") {
		occurrence := rec.AsMap(value)
		for _, page := range rec.AsList(occurrence["pages"]) {
			key := [2]int64{rec.AsInt(occurrence["formula"]), rec.AsInt(page)}
			if _, seen := index.formulas[key]; !seen {
				index.formulas[key] = rec.AsString(occurrence["id"])
			}
		}
	}
	rank := map[string]int{}
	nodes, _ := document["nodes"].(rec.Map)
	for _, id := range slices.Sorted(maps.Keys(nodes)) {
		node := rec.AsMap(nodes[id])
		key, _ := rec.AsMap(node["provenance"])["inspection_key"].(string)
		preference, ranked := leafPreference[rec.AsString(node["type"])]
		if node["type"] == "math" {
			preference, ranked = 0, true
		}
		if key == "" || !ranked {
			continue
		}
		if previous, seen := rank[key]; !seen || preference < previous {
			index.nodes[key], rank[key] = id, preference
		}
	}
	return index
}

// owner is the node of a glyph: its formula's, else its flow item's block's.
func (o *ownerIndex) owner(page int, formula, owner, slot int64) string {
	if formula > 0 {
		if node := o.nodes[o.formulas[[2]int64{formula, int64(page)}]]; node != "" {
			return node
		}
	}
	if owner <= 0 {
		return ""
	}
	block := o.blocks[reading.SlotKey{Owner: owner, Slot: slot, Page: int64(page)}]
	// A run-in title's glyphs belong to the title of their paragraph (decision 088, amendment 1).
	if base, title := strings.CutSuffix(block, reading.TitleSuffix); title {
		if node := o.nodes[base]; node != "" {
			return node + reading.TitleSuffix
		}
	}
	return o.nodes[block]
}

// page adds one glyph map page.
func (o *ownerIndex) page(page glyphMapPage) {
	g := page.Glyphs
	size := len(g.Text)
	if len(g.DX) != size || len(g.DY) != size || len(g.Formula) != size || len(g.Slot) != size || len(g.Flags) != size {
		rec.Fail("Glyph map columns differ in length")
	}
	result := glyphowner.Page{Number: page.Page, X: make([]float64, size), Y: make([]float64, size),
		Text: g.Text, Leaf: make([]int, size)}
	x, y := int64(0), int64(0)
	for at := range size {
		x, y = x+g.DX[at], y+g.DY[at]
		result.X[at], result.Y[at] = bigPoints(x), bigPoints(y)
		if g.Flags[at]&unknownPosition != 0 {
			result.Unknown = append(result.Unknown, at)
		}
		node := o.owner(page.Page, g.Formula[at], g.Owner[at], g.Slot[at])
		if node == "" {
			continue
		}
		leaf, seen := o.leaves[node]
		if !seen {
			o.owners.Leaves = append(o.owners.Leaves, node)
			leaf = len(o.owners.Leaves)
			o.leaves[node] = leaf
		}
		result.Leaf[at] = leaf
	}
	inheritBroken(result.Leaf, g.Owner, g.Formula)
	o.owners.Pages = append(o.owners.Pages, result)
}

// brokenRun bounds the glyphs without a flow item that take the node around them.
const brokenRun = 2

// inheritBroken gives the glyphs TeX made at a line break without a flow item (a discretionary's
// hyphen and post-break text: "dif-" / "ferent" from a broken ff ligature) the node of the glyphs
// on both sides when that is one node.
func inheritBroken(leaves []int, owners, formulas []int64) {
	for start := 1; start < len(leaves); start++ {
		if leaves[start] != 0 || leaves[start-1] == 0 {
			continue
		}
		end := start
		for end < len(leaves) && end-start < brokenRun && leaves[end] == 0 && owners[end] == 0 && formulas[end] == 0 {
			end++
		}
		if end > start && end < len(leaves) && leaves[end] == leaves[start-1] {
			for at := start; at < end; at++ {
				leaves[at] = leaves[start-1]
			}
		}
	}
}

// bigPoints is a length in sp as bp, to the hundredth.
func bigPoints(sp int64) float64 {
	return math.Round(geometry.SPToBP(float64(sp))*100) / 100
}
