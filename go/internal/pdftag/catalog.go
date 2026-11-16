package pdftag

import (
	"fmt"
	"html"
	"slices"
	"strings"
)

// The catalog's tagging entries, the XMP identification and the link annotations' structure.

// catalog marks the document tagged, names its language and title and identifies PDF/UA-2.
func (w *treeWriter) catalog(treeRoot string) {
	ref := w.file.root()
	dict := copyDict(w.file.dict(ref))
	dict["/StructTreeRoot"] = treeRoot
	dict["/MarkInfo"] = map[string]any{"/Marked": true}
	dict["/Lang"] = "u:" + w.language
	preferences := copyDict(w.file.dict(dict["/ViewerPreferences"]))
	preferences["/DisplayDocTitle"] = true
	dict["/ViewerPreferences"] = preferences
	if properties := w.withoutAutoStates(dict["/OCProperties"]); properties != nil {
		dict["/OCProperties"] = properties
	}
	dict["/Metadata"] = w.file.addStream(map[string]any{"/Type": "/Metadata", "/Subtype": "/XML"},
		[]byte(xmp(w.title, w.language)))
	w.file.set(ref, dict)
}

// withoutAutoStates is the optional content properties without the AS key of any configuration
// (PDF/UA-2 8.7-2: pdfTeX's ocg package writes one with a View, a Print and an Export entry), nil
// when there are none to change. The View entry is folded into the configuration first: an OCG whose
// Usage says its View state is OFF (ON) is listed OFF (taken off the list), as a viewer applies the
// entry, so the pages show what they showed. Print and Export say nothing about the screen.
func (w *treeWriter) withoutAutoStates(value any) map[string]any {
	properties := w.file.dict(value)
	if properties == nil {
		return nil
	}
	result, changed := copyDict(properties), false
	list := slices.Clone(w.file.array(properties["/Configs"]))
	configs := append([]any{properties["/D"]}, list...)
	for index, config := range configs {
		dict := w.file.dict(config)
		if dict == nil || dict["/AS"] == nil {
			continue
		}
		dict = w.foldViewState(dict)
		delete(dict, "/AS")
		changed = true
		if index == 0 {
			result["/D"] = dict
		} else {
			list[index-1] = dict
		}
	}
	if !changed {
		return nil
	}
	if len(list) > 0 {
		result["/Configs"] = list
	}
	return result
}

// foldViewState returns a copy of the configuration with its AS array's View entries applied to
// its OFF list.
func (w *treeWriter) foldViewState(config map[string]any) map[string]any {
	result := copyDict(config)
	off := slices.Clone(w.file.array(config["/OFF"]))
	for _, entry := range w.file.array(config["/AS"]) {
		state := w.file.dict(entry)
		if state["/Event"] != "/View" {
			continue
		}
		for _, group := range w.file.array(state["/OCGs"]) {
			view := w.file.dict(w.file.dict(w.file.dict(group)["/Usage"])["/View"])
			switch view["/ViewState"] {
			case "/OFF":
				if !slices.Contains(off, group) {
					off = append(off, group)
				}
			case "/ON":
				off = slices.DeleteFunc(off, func(item any) bool { return item == group })
			}
		}
	}
	delete(result, "/OFF")
	if len(off) > 0 {
		result["/OFF"] = off
	}
	return result
}

// byteOrderMark begins an XMP packet's begin attribute.
const byteOrderMark = string(rune(0xfeff))

// xmp is the metadata packet: title, language and the PDF/UA-2 identification.
func xmp(title, language string) string {
	title, language = html.EscapeString(title), html.EscapeString(language)
	return `<?xpacket begin="` + byteOrderMark + `" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/"
    xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/" xmlns:pdf="http://ns.adobe.com/pdf/1.3/"
    xmlns:xmp="http://ns.adobe.com/xap/1.0/">
   <dc:title><rdf:Alt><rdf:li xml:lang="x-default">` + title + `</rdf:li></rdf:Alt></dc:title>
   <dc:language><rdf:Bag><rdf:li>` + language + `</rdf:li></rdf:Bag></dc:language>
   <dc:format>application/pdf</dc:format>
   <pdfuaid:part>2</pdfuaid:part>
   <pdfuaid:rev>2024</pdfuaid:rev>
   <pdf:Producer>relatex (tagged from the model, route 2)</pdf:Producer>
   <xmp:CreatorTool>relatex</xmp:CreatorTool>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`
}

// textRoles are the leaves a link inside their text becomes a kid of.
var textRoles = map[string]bool{"P": true, "H1": true, "H2": true, "H3": true, "H4": true, "H5": true,
	"H6": true, "Caption": true, "TD": true, "TH": true}

// linkAnnotations gives each link annotation of a page a Link element, inside the text it covers
// or at the end of the document, and the annotation its structure key and description.
func (w *treeWriter) linkAnnotations(index int, p page, root *selem) {
	annots := w.file.array(p.dict["/Annots"])
	if len(annots) == 0 {
		return
	}
	frame := w.file.frameOf(p)
	for _, value := range annots {
		ref, _ := value.(string)
		annot := w.file.dict(ref)
		if annot == nil || !isRef(ref) {
			continue
		}
		role := "Annot"
		switch annot["/Subtype"] {
		case "/Link":
			role = "Link"
		case "/Widget":
			role = "Form" // PDF/UA-2 8.10.1: a widget belongs to a Form element
		}
		key := len(w.pages) + len(w.annots)
		w.annots = append(w.annots, "")
		element := w.t.linkElements[ref]
		if element == nil {
			element = &selem{role: role}
			w.placeLink(element, index+1, w.file.array(annot["/Rect"]), frame, root)
		}
		element.key, element.objr = key, map[string]any{"/Type": "/OBJR", "/Obj": ref, "/Pg": p.ref}
		dict := copyDict(annot)
		dict["/StructParent"] = key
		if _, has := dict["/Contents"]; !has {
			dict["/Contents"] = "u:" + w.linkText(annot)
		}
		w.file.set(ref, dict)
		w.links++
		w.linkRefs = append(w.linkRefs, ref)
	}
}

// placeLink puts the Link inside the first text leaf whose glyph lies in the annotation's
// rectangle, else at the end of the document.
func (w *treeWriter) placeLink(element *selem, number int, rect []any, frame pageFrame, root *selem) {
	if len(rect) == 4 {
		left, top := frame.layout(numberOf(rect[0]), numberOf(rect[1]))
		right, bottom := frame.layout(numberOf(rect[2]), numberOf(rect[3]))
		x0, x1, y0, y1 := min(left, right), max(left, right), min(top, bottom), max(top, bottom)
		for ordinal, glyph := range w.t.painted[number] {
			at := glyph.baseline
			if at[0] < x0 || at[0] > x1 || at[1] < y0 || at[1] > y1+2 {
				continue
			}
			leaf := w.t.binding.leafOf[glyphRef{number, ordinal}]
			if linked := w.t.linkHosts[leaf]; linked != nil {
				leaf = linked.leaf
			}
			if host := w.t.builder.leaves[leaf]; host != nil && textRoles[host.role] {
				element.first = glyphRef{number, ordinal}
				host.kids = append(host.kids, element)
				return
			}
		}
	}
	root.kids = append(root.kids, element)
}

// linkText describes a link annotation that has no Contents.
func (w *treeWriter) linkText(annot map[string]any) string {
	action := w.file.dict(annot["/A"])
	switch action["/S"] {
	case "/URI":
		return "Link to " + pdfText(action["/URI"])
	case "/Named":
		return "Go to the " + strings.ToLower(strings.TrimPrefix(fmt.Sprint(action["/N"]), "/")) + " page"
	case "/GoTo":
		return "Go to " + pdfText(action["/D"])
	}
	if dest, ok := annot["/Dest"]; ok {
		return "Go to " + pdfText(dest)
	}
	return "Link"
}

// pdfText is a qpdf JSON string or name as text.
func pdfText(value any) string {
	text, ok := value.(string)
	if !ok {
		return "a place in the document"
	}
	for _, prefix := range []string{"u:", "/"} {
		if strings.HasPrefix(text, prefix) {
			return strings.TrimPrefix(text, prefix)
		}
	}
	if strings.HasPrefix(text, "b:") {
		return "a place in the document"
	}
	return text
}
