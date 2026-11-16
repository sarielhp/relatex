package sourcealt

import (
	"sort"

	"github.com/sarielhp/relatex/internal/authorhints"
	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/rec"
)

// ApplyHints gives every image of a figure whose \label is keyed in hints the author's text
// alternative (decision 122); it wins over what the source says about the figure's images. The
// hint is written in the paper's own files, so it carries the source's provenance (decision 096),
// which no sidecar entry or model draft replaces. A key that names no figure of the document is
// ignored here (the lint reports it). It returns how many images it described.
func ApplyHints(document rec.Map, hints *authorhints.Hints) int {
	if hints == nil {
		return 0
	}
	alt := hints.Sections["alt"]
	if len(alt) == 0 {
		return 0
	}
	nodes := rec.FetchMap(document, "nodes")
	figures := figureOwners(nodes, rec.FetchString(document, "root"))
	labels := rec.FetchMap(document, "labels")
	keys := make([]string, 0, len(alt))
	for key := range alt {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	described := 0
	for _, key := range keys {
		text := Normalize(alt[key].Value)
		if text == "" || Placeholder(text) {
			continue
		}
		figure, ok := figures[labelTarget(labels, key)]
		if !ok {
			continue
		}
		for _, image := range figureImages(nodes, figure) {
			delete(image, "decorative")
			image["alt"], image["alt_provenance"] = text, figdesc.Source
			described++
		}
	}
	return described
}

// labelTarget is the node a label names; "" when the document has no such label.
func labelTarget(labels rec.Map, label string) string {
	entry, ok := labels[label].(rec.Map)
	if !ok {
		return ""
	}
	target, _ := entry["target"].(string)
	return target
}

// figureOwners maps each node to the innermost figure_float that holds it.
func figureOwners(nodes rec.Map, root string) map[string]string {
	owners := map[string]string{}
	var walk func(string, string)
	walk = func(id, figure string) {
		node, ok := nodes[id].(rec.Map)
		if !ok {
			return
		}
		if node["type"] == "figure_float" {
			figure = id
		}
		if figure != "" {
			owners[id] = figure
		}
		children, _ := node["children"].([]any)
		for _, child := range children {
			if name, ok := child.(string); ok {
				walk(name, figure)
			}
		}
	}
	walk(root, "")
	// A float kept outside the structure still owns its images.
	for id, value := range nodes {
		if rec.AsMap(value)["type"] == "figure_float" {
			if _, known := owners[id]; !known {
				walk(id, "")
			}
		}
	}
	return owners
}

// figureImages are the image spans under a figure, in document order.
func figureImages(nodes rec.Map, figure string) []rec.Map {
	images := []rec.Map{}
	var walk func(string)
	walk = func(id string) {
		node, ok := nodes[id].(rec.Map)
		if !ok {
			return
		}
		content, _ := node["content"].([]any)
		for _, item := range content {
			image, ok := item.(rec.Map)
			if ok && image["type"] == "image" {
				images = append(images, image)
			}
		}
		children, _ := node["children"].([]any)
		for _, child := range children {
			if name, ok := child.(string); ok {
				walk(name)
			}
		}
	}
	walk(figure)
	return images
}
