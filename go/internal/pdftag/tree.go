package pdftag

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// The structure tree, the parent tree, the pages and the catalog written into the patch.

const (
	pdf2Namespace  = "http://iso.org/pdf2/ssn"
	pdf17Namespace = "http://iso.org/pdf/ssn"
)

// pdf17Roles are the roles PDF 2.0's namespace dropped: they belong to the PDF 1.7 namespace,
// which PDF/UA-2 allows beside it (8.2.4).
var pdf17Roles = map[string]bool{"TOC": true, "TOCI": true, "BlockQuote": true}

// treeWriter writes the structure.
type treeWriter struct {
	t         *tagger
	file      *pdfFile
	pages     []page
	namespace string
	legacy    string           // the PDF 1.7 namespace, once an element needs it
	parents   map[int][]string // page index → MCID → element
	annots    []string         // StructParent key − page count → element
	roles     map[string]int
	links     int
	linkRefs  []string
	title     string
	language  string
	refs      map[*selem]string
}

func newTreeWriter(t *tagger, pages []page) *treeWriter {
	return &treeWriter{t: t, file: t.file, pages: pages, parents: map[int][]string{}, roles: map[string]int{},
		refs: map[*selem]string{}}
}

// write writes everything the tagging changes.
func (w *treeWriter) write(root *selem, results []pageResult) error {
	w.language = w.t.doc.Language.Tag
	if w.language == "" {
		w.language = "en"
	}
	w.title = w.documentTitle()
	root.kids = append(root.kids, w.t.orphans...)
	for index, p := range w.pages {
		w.linkAnnotations(index, p, root)
	}
	if !prune(root) {
		return fmt.Errorf("nothing of the document was bound to the PDF")
	}
	settleContents(root)
	order(root)
	w.namespace = w.file.add(map[string]any{"/Type": "/Namespace", "/NS": "u:" + pdf2Namespace})
	w.assign(root)
	w.destinations(root)
	treeRoot := w.file.add(nil)
	w.element(root, treeRoot)
	for index, p := range w.pages {
		w.page(index, p, results[index])
	}
	w.file.set(treeRoot, map[string]any{"/Type": "/StructTreeRoot", "/K": w.refs[root],
		"/ParentTree": w.parentTree(), "/ParentTreeNextKey": len(w.pages) + len(w.annots),
		"/Namespaces": w.namespaces()})
	w.catalog(treeRoot)
	return nil
}

// prune drops elements without content; it reports whether the element stays.
func prune(element *selem) bool {
	kept := element.kids[:0]
	for _, kid := range element.kids {
		if prune(kid) {
			kept = append(kept, kid)
		}
	}
	element.kids = kept
	return len(element.mcrs) > 0 || len(element.kids) > 0 || element.objr != nil || element.role == "Document" ||
		element.role == "TD" || element.role == "TH"
}

// position is where an element's content begins, for ordering a leaf's kids. A Link that holds
// marked content is placed by its own MCRs, like any element: element.first is a layout paint
// index (links.go bindLink), while an MCR's first is a content ordinal. The two numberings
// differ wherever the layout skips or adds paints (a figure's text, form XObjects, fill and
// stroke), so only a Link without marked content (an OBJR alone) sorts by element.first.
func position(element *selem) (int, int) {
	if element.objr != nil && element.first.page > 0 && len(element.mcrs) == 0 {
		return element.first.page, element.first.index
	}
	page, first := 1<<30, 0
	for _, m := range element.mcrs {
		if m.page < page || (m.page == page && m.first < first) {
			page, first = m.page, m.first
		}
	}
	for _, kid := range element.kids {
		if kp, kf := position(kid); kp < page || (kp == page && kf < first) {
			page, first = kp, kf
		}
	}
	return page, first
}

// order sorts each leaf's marked content and inline kids by where they begin.
func order(element *selem) {
	for _, kid := range element.kids {
		order(kid)
	}
	slices.SortStableFunc(element.mcrs, func(a, b mcr) int {
		return cmp.Or(cmp.Compare(a.page, b.page), cmp.Compare(a.first, b.first))
	})
	if element.leaf == "" {
		return
	}
	slices.SortStableFunc(element.kids, func(a, b *selem) int {
		ap, af := position(a)
		bp, bf := position(b)
		return cmp.Or(cmp.Compare(ap, bp), cmp.Compare(af, bf))
	})
}

// assign gives every element its object.
func (w *treeWriter) assign(element *selem) {
	w.refs[element] = w.file.add(nil)
	for _, kid := range element.kids {
		w.assign(kid)
	}
}

// element writes an element and its kids.
func (w *treeWriter) element(element *selem, parent string) {
	ref := w.refs[element]
	w.roles[element.role]++
	dict := map[string]any{"/Type": "/StructElem", "/S": "/" + element.role, "/P": parent, "/NS": w.namespaceOf(element.role),
		"/K": w.kids(element)}
	w.attributes(element, dict)
	if target := w.refs[element.ref]; element.ref != nil && target != "" {
		dict["/Ref"] = []any{target}
	}
	w.file.set(ref, dict)
	for _, kid := range element.kids {
		w.element(kid, ref)
	}
}

// namespaceOf is the namespace a role belongs to.
func (w *treeWriter) namespaceOf(role string) string {
	if !pdf17Roles[role] {
		return w.namespace
	}
	if w.legacy == "" {
		w.legacy = w.file.add(map[string]any{"/Type": "/Namespace", "/NS": "u:" + pdf17Namespace})
	}
	return w.legacy
}

// namespaces are the namespaces the tree uses.
func (w *treeWriter) namespaces() []any {
	if w.legacy == "" {
		return []any{w.namespace}
	}
	return []any{w.namespace, w.legacy}
}

// kids is an element's /K: marked content, kid elements in their order, then links.
func (w *treeWriter) kids(element *selem) []any {
	type entry struct {
		page, first int
		value       any
	}
	var entries []entry
	for _, m := range element.mcrs {
		pageRef := w.pages[m.page-1].ref
		entries = append(entries, entry{m.page, m.first, map[string]any{"/Type": "/MCR", "/Pg": pageRef, "/MCID": m.mcid}})
		w.parents[m.page-1] = setAt(w.parents[m.page-1], m.mcid, w.refs[element])
	}
	for _, kid := range element.kids {
		page, first := position(kid)
		entries = append(entries, entry{page, first, w.refs[kid]})
	}
	if element.leaf != "" {
		slices.SortStableFunc(entries, func(a, b entry) int {
			return cmp.Or(cmp.Compare(a.page, b.page), cmp.Compare(a.first, b.first))
		})
	}
	values := make([]any, 0, len(entries)+1)
	for _, e := range entries {
		values = append(values, e.value)
	}
	if element.objr != nil {
		values = append(values, element.objr)
		w.annots[element.key-len(w.pages)] = w.refs[element]
	}
	return values
}

func setAt(list []string, index int, value string) []string {
	for len(list) <= index {
		list = append(list, "")
	}
	list[index] = value
	return list
}

// attributes adds an element's alternative text, MathML, language and list or table attributes.
func (w *treeWriter) attributes(element *selem, dict map[string]any) {
	if element.alt != "" {
		dict["/Alt"] = "u:" + element.alt
	}
	if element.mathml != "" {
		files := []any{w.mathFile(element)}
		if element.tex != "" {
			files = append(files, w.texFile(element))
		}
		dict["/AF"] = files
	}
	if element.long != "" {
		dict["/AF"] = []any{w.descriptionFile(element)}
	}
	switch element.role {
	case "Document":
		dict["/Lang"] = "u:" + w.language
	case "L":
		dict["/A"] = map[string]any{"/O": "/List", "/ListNumbering": "/Disc"}
	case "TH", "TD":
		dict["/A"] = cellAttributes(element)
	}
}

// cellAttributes are a table cell's spans and, for a header, its scope.
func cellAttributes(element *selem) map[string]any {
	attributes := map[string]any{"/O": "/Table"}
	if element.role == "TH" {
		attributes["/Scope"] = cmp.Or(element.scope, "/Column")
	}
	if element.span[0] > 1 {
		attributes["/ColSpan"] = element.span[0]
	}
	if element.span[1] > 1 {
		attributes["/RowSpan"] = element.span[1]
	}
	return attributes
}

// mathFile is a formula's MathML as an associated file.
func (w *treeWriter) mathFile(element *selem) string {
	return w.formulaFile("mml", "/application/mathml+xml", "MathML", element.mathml)
}

// texFile is a formula's TeX as an associated file beside its MathML (decision 141), the
// application/x-tex the HTML annotation carries too.
func (w *treeWriter) texFile(element *selem) string {
	return w.formulaFile("tex", "/application/x-tex", "LaTeX", element.tex)
}

// formulaFile writes one supplementary associated file of a formula: its stream and its Filespec.
func (w *treeWriter) formulaFile(extension, subtype, desc, text string) string {
	name := fmt.Sprintf("formula-%d.%s", len(w.file.patch), extension)
	stream := w.file.addStream(map[string]any{"/Type": "/EmbeddedFile", "/Subtype": subtype}, []byte(text))
	return w.file.add(map[string]any{"/Type": "/Filespec", "/F": "u:" + name, "/UF": "u:" + name,
		"/AFRelationship": "/Supplement", "/Desc": "u:" + desc, "/EF": map[string]any{"/F": stream, "/UF": stream}})
}

// LongDescriptionDesc is the /Desc of a Figure's long description file (decision 110), as the
// tagged PDF of route 1 (removed on 2026-11-03) wrote it too.
const LongDescriptionDesc = "Long description"

// descriptionFile is a Figure's long description as an associated file (ISO 32000-2 14.13): plain
// UTF-8 text supplementing the Figure, whose /Alt stays the short text (decision 110).
func (w *treeWriter) descriptionFile(element *selem) string {
	name := fmt.Sprintf("figure-description-%d.txt", len(w.file.patch))
	stream := w.file.addStream(map[string]any{"/Type": "/EmbeddedFile", "/Subtype": "/text/plain"}, []byte(element.long))
	return w.file.add(map[string]any{"/Type": "/Filespec", "/F": "u:" + name, "/UF": "u:" + name,
		"/AFRelationship": "/Supplement", "/Desc": "u:" + LongDescriptionDesc, "/EF": map[string]any{"/F": stream, "/UF": stream}})
}

// parentTree maps each page's MCIDs and each annotation to their elements.
func (w *treeWriter) parentTree() map[string]any {
	var nums []any
	for index := range w.pages {
		list := make([]any, 0, len(w.parents[index]))
		for _, ref := range w.parents[index] {
			if ref == "" {
				list = append(list, nil)
				continue
			}
			list = append(list, ref)
		}
		nums = append(nums, index, list)
	}
	for index, ref := range w.annots {
		nums = append(nums, len(w.pages)+index, ref)
	}
	return map[string]any{"/Nums": nums}
}

// page writes a page's new content and its structure keys. The content replaces the page's
// content stream in place (qpdf's JSON update kept a page's old content when /Contents named a
// new stream, slides 0005 page 10); the page's further streams, if any, are emptied.
func (w *treeWriter) page(index int, p page, result pageResult) {
	dict := copyDict(p.dict)
	refs := []any{dict["/Contents"]}
	if list, ok := w.file.resolve(dict["/Contents"]).([]any); ok && len(list) > 0 {
		refs = list
	}
	for at, value := range refs {
		ref, _ := value.(string)
		if result.content == nil {
			break // the page could not be read: its content stays as it was, reported
		}
		data := result.content
		if at > 0 {
			data = nil
		}
		w.file.setStream(ref, map[string]any{}, data)
	}
	dict["/Contents"] = refs[0]
	dict["/StructParents"] = index
	dict["/Tabs"] = "/S"
	w.file.set(p.ref, dict)
}

func copyDict(dict map[string]any) map[string]any {
	clone := make(map[string]any, len(dict))
	for key, value := range dict {
		clone[key] = value
	}
	return clone
}

// documentTitle is the front matter's title, else the first heading, else the job name.
func (w *treeWriter) documentTitle() string {
	if title, ok := w.t.doc.Frontmatter.Title.(string); ok && strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	for _, id := range w.t.doc.BlockOrder {
		n := w.t.doc.Nodes[id]
		if n.Type != "heading" {
			continue
		}
		var text strings.Builder
		for _, item := range n.Content {
			text.WriteString(item.Text)
		}
		if title := strings.TrimSpace(text.String()); title != "" {
			return title
		}
	}
	return jobName(w.t.options.PDF)
}
