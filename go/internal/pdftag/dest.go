package pdftag

import (
	"slices"
	"strings"
)

// Structure destinations (PDF/UA-2 8.8): every link and outline item that goes to a place in the
// document also names the structure element there, the first one at or below the destination.

// placedElement is an element's first content on a page, by its glyph's distance from the top.
type placedElement struct {
	top float64
	ref string
}

// placements lists, per page, where each element's marked content begins.
func (w *treeWriter) placements(root *selem) map[int][]placedElement {
	placed := map[int][]placedElement{}
	var walk func(element *selem)
	walk = func(element *selem) {
		for _, m := range element.mcrs {
			// An MCR's first is a content ordinal; the painted glyphs are indexed by the
			// layout's paint index, which differs where the layout skips or adds paints.
			if m.page < 1 || m.page > len(w.t.prepared) {
				continue
			}
			prepared := w.t.prepared[m.page-1]
			if prepared.mapper == nil || prepared.failure != "" || m.first >= len(prepared.mapper.mapping) {
				continue
			}
			index := prepared.mapper.mapping[m.first]
			glyphs := w.t.painted[m.page]
			if index < 0 || index >= len(glyphs) {
				continue
			}
			placed[m.page] = append(placed[m.page], placedElement{glyphs[index].baseline[1], w.refs[element]})
		}
		for _, kid := range element.kids {
			walk(kid)
		}
	}
	walk(root)
	for _, list := range placed {
		slices.SortStableFunc(list, func(a, b placedElement) int {
			switch {
			case a.top < b.top:
				return -1
			case a.top > b.top:
				return 1
			}
			return 0
		})
	}
	return placed
}

// destinations adds structure destinations to the links and the outline.
func (w *treeWriter) destinations(root *selem) {
	resolver := &destResolver{w: w, placed: w.placements(root), root: w.refs[root], pageIndex: map[string]int{}}
	for index, p := range w.pages {
		resolver.pageIndex[p.ref] = index
	}
	for _, ref := range w.linkRefs {
		dict := copyDict(w.file.dict(ref))
		if resolver.retarget(dict) {
			w.file.set(ref, dict)
		}
	}
	catalog := copyDict(w.file.dict(w.file.root()))
	outlines := w.file.dict(catalog["/Outlines"])
	resolver.outline(outlines["/First"], 0)
	if open, ok := catalog["/OpenAction"]; ok {
		wrapper := map[string]any{"/A": open}
		if _, isArray := w.file.resolve(open).([]any); isArray {
			wrapper = map[string]any{"/Dest": open}
		}
		if resolver.retarget(wrapper) {
			catalog["/OpenAction"] = wrapper["/A"]
			w.file.set(w.file.root(), catalog)
		}
	}
}

// destResolver finds the structure element a destination names.
type destResolver struct {
	w         *treeWriter
	placed    map[int][]placedElement
	root      string
	pageIndex map[string]int
}

// outline retargets the outline items from first on, and their kids.
func (r *destResolver) outline(item any, depth int) {
	for count := 0; item != nil && count < 100000 && depth < 64; count++ {
		ref, _ := item.(string)
		dict := r.w.file.dict(ref)
		if dict == nil {
			return
		}
		changed := copyDict(dict)
		if r.retarget(changed) {
			r.w.file.set(ref, changed)
		}
		r.outline(dict["/First"], depth+1)
		item = dict["/Next"]
	}
}

// retarget turns a /Dest or a GoTo action into a GoTo action with a structure destination.
func (r *destResolver) retarget(dict map[string]any) bool {
	action := r.w.file.dict(dict["/A"])
	dest, hasDest := dict["/Dest"]
	switch {
	case action != nil && action["/S"] == "/GoTo":
		dest = action["/D"]
	case action == nil && hasDest:
		action = map[string]any{"/S": "/GoTo", "/D": dest}
	default:
		return false
	}
	target := r.element(dest)
	if target == nil {
		return false
	}
	changed := copyDict(action)
	changed["/SD"] = target
	delete(dict, "/Dest")
	dict["/A"] = changed
	return true
}

// element is the structure destination for a destination: [element /XYZ left top zoom] or
// [element /Fit].
func (r *destResolver) element(dest any) []any {
	explicit := r.explicit(dest, 0)
	if len(explicit) < 2 {
		return nil
	}
	pageRef, _ := explicit[0].(string)
	index, ok := r.pageIndex[pageRef]
	if !ok {
		return nil
	}
	top := 0.0
	if explicit[1] == "/XYZ" && len(explicit) > 3 {
		if value := numberOf(explicit[3]); value != 0 {
			_, top = r.w.file.frameOf(r.w.pages[index]).layout(numberOf(explicit[2]), value)
		}
	}
	target := r.at(index+1, top)
	return append([]any{target}, explicit[1:]...)
}

// at is the first element whose content begins at or below top on the page, else the page's last,
// else the document.
func (r *destResolver) at(number int, top float64) string {
	list := r.placed[number]
	for _, entry := range list {
		if entry.top >= top-1 {
			return entry.ref
		}
	}
	if len(list) > 0 {
		return list[len(list)-1].ref
	}
	return r.root
}

// explicit resolves a named destination to its array.
func (r *destResolver) explicit(dest any, depth int) []any {
	if depth > 4 {
		return nil
	}
	switch value := r.w.file.resolve(dest).(type) {
	case []any:
		return value
	case map[string]any:
		return r.explicit(value["/D"], depth+1)
	case string:
		if strings.HasPrefix(value, "/") {
			catalogDests := r.w.file.dict(r.w.file.dict(r.w.file.root())["/Dests"])
			return r.explicit(catalogDests[value], depth+1)
		}
		names := r.w.file.dict(r.w.file.dict(r.w.file.root())["/Names"])
		return r.explicit(r.lookup(names["/Dests"], value, 0), depth+1)
	}
	return nil
}

// lookup finds a key in a name tree.
func (r *destResolver) lookup(node any, key string, depth int) any {
	dict := r.w.file.dict(node)
	if dict == nil || depth > 32 {
		return nil
	}
	names := r.w.file.array(dict["/Names"])
	for i := 0; i+1 < len(names); i += 2 {
		if names[i] == key {
			return names[i+1]
		}
	}
	for _, kid := range r.w.file.array(dict["/Kids"]) {
		if found := r.lookup(kid, key, depth+1); found != nil {
			return found
		}
	}
	return nil
}
