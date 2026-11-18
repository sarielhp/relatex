package browser

import (
	"bytes"
	"github.com/sarielhp/relatex/internal/bookindex"
	"github.com/sarielhp/relatex/internal/htmlkit"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/contrast"
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/mathexport"
	"github.com/sarielhp/relatex/internal/mathintent"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stylemodel"
)

// Root is the repository root: the inventory scripts are read from lib/relatex.
var Root string

// The inventories PaperBrowserCheck adds to BrowserCheck's (FLOATS, LISTS, PAGE_TOTALS,
// IMAGES, NOTES).
const (
	floatsScript = `[...document.querySelectorAll('main [data-structure]')]
  .filter(e => ['figure_float','table_float','float_caption'].includes(e.dataset.structure))
  .map(e => ({id:e.id,type:e.dataset.structure,tag:e.localName,
             caption:e.getAttribute('aria-labelledby'), role:e.getAttribute('role'),
             float_parent:e.dataset.structure === 'float_caption' ? e.parentElement.id : null}))`
	listsScript = `[...document.querySelectorAll('main [data-structure]')]
  .filter(e => ['ordered_list','unordered_list','list_item'].includes(e.dataset.structure))
  .map(e => ({id:e.id,type:e.dataset.structure,tag:e.localName,list_style:getComputedStyle(e).listStyleType}))`
	pageTotalsScript = `[...document.querySelectorAll('main [data-document-page-total]')].map(e => ({
  id: e.id, count: Number(e.dataset.documentPageTotal), source_label: e.dataset.sourceLabel,
  pdf_sha256: e.dataset.pdfSha256, anchor: e.dataset.sourceAnchor, text: e.textContent,
  visible: e.getBoundingClientRect().width > 0 && e.getBoundingClientRect().height > 0 && getComputedStyle(e).visibility === 'visible'
}))`
	// imagesScript measures each image with its top scrolled to the viewport's: Chromium's client
	// rects are single floats, so far down a long page (a book, 2^18 px and more) a box's edges
	// round to 1/32 px or coarser, and its height with them.
	// A figure shown as inline SVG (decision 146) reads back as the file it names (data-src), its
	// root's attributes and its whole markup: each element's name, attributes and children, blank text
	// left out.
	imagesScript = `(() => {
  const [x, y] = [window.scrollX, window.scrollY];
  const kept = c => c.nodeType === Node.ELEMENT_NODE || (c.nodeType === Node.TEXT_NODE && c.textContent.trim() !== '');
  const markup = n => n.nodeType === Node.ELEMENT_NODE ? {tag: n.localName,
    attributes: Object.fromEntries([...n.attributes].map(a => [a.name, a.value])),
    children: [...n.childNodes].filter(kept).map(markup)} : n.textContent;
  const images = [...document.querySelectorAll('main img[data-kind="image"], main svg[data-kind="image"]')].map(e => {
    window.scrollBy(0, e.getBoundingClientRect().top);
    const box = e.getBoundingClientRect();
    const visible = getComputedStyle(e).visibility === 'visible' && box.width > 0 && box.height > 0;
    if (e.localName === 'svg') return {
      id: e.id, asset: e.dataset.asset, src: e.dataset.src, inline: true,
      width: box.width, height: box.height,
      style: e.getAttribute('style'),
      attributes: [...e.attributes].map(a => a.name).sort(), visible,
      root: Object.fromEntries([...e.attributes].map(a => [a.name, a.value])),
      content: [...e.childNodes].filter(kept).map(markup)
    };
    return {
      id: e.id, asset: e.dataset.asset, src: e.getAttribute('src'), current: e.currentSrc,
      complete: e.complete, natural_width: e.naturalWidth, natural_height: e.naturalHeight,
      width: box.width, height: box.height,
      style: e.getAttribute('style'),
      attributes: [...e.attributes].map(a => a.name).sort(), visible
    };
  });
  window.scrollTo(x, y);
  return images;
})()`
	notesScript = `[...document.querySelectorAll('main [data-note]')].map(e => ({
  id: e.id, number: e.dataset.note,
  children: [...e.children].filter(c => c.hasAttribute('data-block') || c.hasAttribute('data-structure')).map(c => c.id),
  labels: [...e.querySelectorAll('[data-note-label]')].map(c => {
    const styled = c.querySelector('[data-text-style]');
    return {text:c.textContent, block:c.dataset.blockTarget,
            ...(styled ? {style:JSON.parse(styled.dataset.textStyle)} : {})};
  }),
  backlinks: [...e.querySelectorAll('[data-note-backref]')].map(c => c.getAttribute('href').slice(1))
}))`
	// textPaintScript is PDFTextPaint::INVENTORY.
	textPaintScript = `[...document.querySelectorAll('main [data-kind="text"],main [data-kind="text"] *,main [data-kind="reference"],main [data-kind="reference"] *,main [data-kind="note_reference"],main math,main math *,main [data-page-decoration]')]
  .map(e => {const s=getComputedStyle(e);return {color:s.color,opacity:s.opacity}})`
	// continuationsScript flags the styled nodes that are a continuation piece (data-join-cont), in node order.
	continuationsScript = "[...document.querySelectorAll('main [data-text-style]')].map(e => e.hasAttribute('data-join-cont'))"
	platformFontsScript = "[...document.querySelectorAll('main [data-text-style]:not([data-join-cont])')]." +
		"some(e => getComputedStyle(e).fontFamily.startsWith('ltd-'))"
)

// Checker is PaperBrowserCheck: the checks on one page.
type Checker struct{ browser *Browser }

// CheckPaper is PaperBrowserCheck.open { |browser| browser.check(html_root, document) }: the check
// on a fresh page of the configured browser, its session in the report. A browser that times out
// or cannot be reached on every attempt is returned as *Unavailable; a failed check is raised.
func CheckPaper(htmlRoot string, document rec.Map) (rec.Map, error) {
	report, session, err := withPage(htmlRoot, func(browser *Browser) rec.Map {
		return (&Checker{browser: browser}).Check(htmlRoot, document)
	})
	if err != nil {
		return nil, err
	}
	return rec.Merge(report, session.Evidence()), nil
}

func script(name string) string {
	bytes, err := os.ReadFile(filepath.Join(Root, "lib", "relatex", name))
	if err != nil {
		rec.Fail("%v", err)
	}
	return string(bytes)
}

// Check is PaperBrowserCheck#check (BrowserCheck#check underneath): the HTML in directory
// loaded offline, its DOM inventory written as dom.json and verified against the document.
func (c *Checker) Check(directory string, document rec.Map) rec.Map {
	shrunkCells = []any{}
	verifyImageFiles(document, directory)
	c.browser.GoTo(pageURL("index.html"))
	inventory := c.inventory()
	writeOrdered(filepath.Join(directory, "dom.json"), inventory)
	// A recorded ink adjustment (plan accessibility §7.4 decision 1) reads as the PDF ink it stands for.
	plain := contrast.Restore(inventory.Plain(), document)
	observedMath, _ := inventory.Get("math").([]any)
	verifyPaper(plain, observedMath, document)
	result := report(plain, document)
	// Phase 0 measures accessibility: axe-core's verdict is recorded, and a failed run only leaves it unmeasured.
	accessibility, err := c.Accessibility()
	if err != nil {
		accessibility = rec.Map{"status": "unmeasured", "reason": err.Error()}
	}
	result["accessibility"] = accessibility
	return result
}

func writeOrdered(path string, value any) {
	if err := ojson.WriteFile(path, value); err != nil {
		rec.Fail("%v", err)
	}
}

// evaluateJSON is BrowserCheck#evaluate_json: the script's value serialized in the browser
// (CDP drops -0 array entries), read in its key order.
func (c *Checker) evaluateJSON(source string) any {
	expression := strings.TrimSuffix(strings.TrimSpace(source), ";")
	text, ok := c.browser.Evaluate("JSON.stringify(" + expression + ")").(string)
	if !ok {
		rec.Fail("Browser inventory is not JSON")
	}
	value, err := ojson.Decode(bytes.NewReader([]byte(text)))
	if err != nil {
		rec.Fail("%v", err)
	}
	return value
}

// inventory is PaperBrowserCheck#browser_inventory, in Ruby's key order.
func (c *Checker) inventory() ojson.Object {
	// The checks measure the light theme: its ink is the PDF's text paint. Without a choice the page follows the system.
	c.browser.Page("Runtime.evaluate", rec.Map{"expression": "document.documentElement.setAttribute('data-theme', 'light')"})
	c.browser.Page("Runtime.evaluate", rec.Map{"expression": "document.fonts.ready.then(() => true)", "awaitPromise": true})
	base, ok := c.evaluateJSON(script("browser_inventory.js")).(ojson.Object)
	if !ok {
		rec.Fail("Browser inventory is not an object")
	}
	extra := []struct {
		key    string
		source string
	}{{"lists", listsScript}, {"floats", floatsScript}, {"notes", notesScript}, {"images", imagesScript},
		{"tables", script("table_inventory.js")}, {"page_totals", pageTotalsScript}, {"text_styles", textStylesScript},
		{"citations", citationsScript}, {"frontmatter", frontmatterScript}}
	for _, entry := range extra {
		base = base.Set(entry.key, c.evaluateJSON(entry.source))
	}
	base = base.Set("platform_fonts", c.platformFonts())
	return base.Set("text_paint", c.evaluateJSON(textPaintScript))
}

// platformFonts is PaperBrowserCheck#source_platform_fonts: the faces Chromium used for each
// styled span, when a linked font family is in use.
func (c *Checker) platformFonts() any {
	if present, _ := c.browser.Evaluate(platformFontsScript).(bool); !present {
		return []any{}
	}
	c.browser.Page("DOM.enable", nil)
	c.browser.Page("CSS.enable", nil)
	document := rec.AsMap(ojson.Plain(c.browser.Page("DOM.getDocument", nil)))
	root := rec.FetchMap(document, "root")["nodeId"]
	nodes := rec.AsMap(ojson.Plain(c.browser.Page("DOM.querySelectorAll",
		rec.Map{"nodeId": root, "selector": "main [data-text-style]"})))
	continuations, _ := c.browser.Evaluate(continuationsScript).([]any)
	result := []any{}
	for index, node := range rec.FetchList(nodes, "nodeIds") {
		fonts, _ := c.browser.Page("CSS.getPlatformFontsForNode", rec.Map{"nodeId": node}).(ojson.Object)
		faces := rec.AsList(ojson.Plain(fonts.Get("fonts")))
		if joined, _ := continuations[index].(bool); joined && len(result) > 0 {
			// a piece cut after a formula (data-join-cont) is part of the run before it
			result[len(result)-1] = append(rec.AsList(result[len(result)-1]), faces...)
			continue
		}
		result = append(result, faces)
	}
	return result
}

// report is BrowserCheck#check's report as PaperBrowserCheck#check rewrites it.
func report(inventory, document rec.Map) rec.Map {
	nodes := rec.FetchMap(document, "nodes")
	references := int64(0)
	for _, id := range sortedKeys(nodes) {
		content, _ := rec.AsMap(nodes[id])["content"].([]any)
		for _, span := range content {
			if rec.AsMap(span)["type"] == "reference" {
				references++
			}
		}
	}
	rows := int64(0)
	for _, math := range rec.FetchList(inventory, "math") {
		rows += int64(len(rec.FetchList(rec.AsMap(math), "rows")))
	}
	assets := rec.Map{"count": int64(len(imageSpans(document)))}
	if status, ok := rec.FetchMap(document, "assets")["status"]; ok {
		assets["status"] = status
	}
	return rec.Map{"status": "diagnostic", "ferrum": ferrumVersion, "offline": true, "browser_sandbox": true,
		"blocks": int64(len(rec.FetchList(inventory, "blocks"))), "math": int64(len(rec.FetchList(inventory, "math"))),
		"rows": rows, "references": references, "labels": int64(len(rec.FetchMap(document, "labels"))),
		"qualified": false, "scope": "DOM retention and browser rendering only",
		"notes": int64(len(rec.FetchList(document, "note_order"))), "dom_status": "verified", "assets": assets,
		"unverified_requirements": rec.Fetch(document, "unverified_requirements"),
		"shrunk_cells":            shrunkCells}
}

func sortedKeys(m rec.Map) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// nodeValues is document.fetch('nodes').values, in the order a parsed document holds them.
func nodeValues(document rec.Map) []rec.Map {
	nodes := rec.FetchMap(document, "nodes")
	result := []rec.Map{}
	for _, id := range sortedKeys(nodes) {
		result = append(result, rec.AsMap(nodes[id]))
	}
	return result
}

// verifyPaper is PaperBrowserCheck#verify!.
// observedMath is the inventory's math in the browser's key order, for the failure message.
func verifyPaper(inventory rec.Map, observedMath []any, document rec.Map) {
	paperdoc.Validate(document)
	verifyTextStyles(inventory, document)
	verifyPlatformFonts(inventory, document)
	verifyTextPaint(inventory, document)
	verifyCitations(inventory, document)
	verifyFrontmatter(inventory, document)
	nodes := rec.FetchMap(document, "nodes")
	blocks := []rec.Map{}
	hidden, backdrops := contrast.Hidden(document), contrast.DocumentBackdrops(document)
	for _, id := range rec.FetchList(document, "block_order") {
		block := rec.Pick(rec.Node(nodes, id), "id", "type", "content")
		blocks = append(blocks, rec.Merge(block, rec.Map{"content": hiddenMarks(rec.FetchList(block, "content"), hidden, backdrops)}))
	}
	structures := orderedStructures(document)
	validateLists(inventory, structures, nodes)
	validateFloats(inventory, structures, nodes)
	verifyBlocksAndMath(inventory, observedMath, blocks, structures, nodes, mathintent.MacrosOf(document), stylemodel.BaseSize(document))
	notes := verifyNotes(inventory, document)
	verifyPageTotals(inventory, document)
	callsites := []string{}
	for _, note := range notes {
		for _, id := range rec.FetchList(note, "backlinks") {
			callsites = append(callsites, rec.ToS(id))
		}
	}
	imageIDs, shownImages := validateImages(inventory, document)
	tableIDs := validateTables(inventory, document)
	ids := append(append(append(append(sortedKeys(nodes), callsites...), imageIDs...), tableIDs...), pageAnchors(document)...)
	validateRendering(safeInventory(inventory, document, shownImages), ids)
}

// hiddenMarks marks the spans of one content list that the page hides from screen readers, as the
// HTML writer does (html.Paper.hiddenText): text in an ink the author hid (contrast.Hidden), off
// an author's fill. The inventory reads the mark back with the span's lang.
func hiddenMarks(content []any, hidden map[string]bool, backdrops contrast.Backdrops) []any {
	fills := contrast.BehindOn(content, "", backdrops)
	marked := make([]any, len(content))
	for index, value := range content {
		span := rec.AsMap(value)
		color, _ := span["color"].(string)
		if span["type"] != "rule" && fills[index] == "" && hidden[color] {
			span = rec.Merge(span, rec.Map{"aria_hidden": "true"})
		}
		marked[index] = span
	}
	return marked
}

func verifyBlocksAndMath(inventory rec.Map, observedMath []any, blocks, structures []rec.Map, nodes rec.Map, macros mathintent.Macros, base int64) {
	expectedMath := []any{}
	projected := []any{}
	for _, block := range blocks {
		for _, span := range inline.Flat(rec.FetchList(block, "content"), true) {
			if span["type"] == "math" {
				expectedMath = append(expectedMath, mathInventory(rec.Node(nodes, span["node"]), macros, base))
			}
		}
		content := []any{}
		for _, span := range rec.FetchList(block, "content") {
			content = append(content, withoutSources(browserSpan(rec.AsMap(span))))
		}
		projected = append(projected, rec.Merge(block, rec.Map{"content": content}))
	}
	if !rec.Equal(rec.FetchList(inventory, "blocks"), projected) {
		rec.Fail("Browser changed paper blocks")
	}
	if !rec.Equal(rec.FetchList(inventory, "structures"), maps(structures)) {
		rec.Fail("Browser changed paper structures")
	}
	observed := rec.FetchList(inventory, "math")
	if rec.Equal(observed, plainList(expectedMath)) {
		return
	}
	index := 0
	for index < len(observed) && index < len(expectedMath) && rec.Equal(observed[index], ojson.Plain(expectedMath[index])) {
		index++
	}
	// Ruby's zip(...).index finds no pair when the inventory is a prefix: nil.to_i is 0.
	if index == len(observed) {
		index = 0
	}
	rec.Fail("Browser changed paper MathML (formula %d): %s for %s", index, prefix(generate(at(observedMath, index)), 300),
		prefix(generate(at(expectedMath, index)), 300))
}

func at(list []any, index int) any {
	if index < len(list) {
		return list[index]
	}
	return nil
}

func plainList(list []any) []any {
	result := make([]any, len(list))
	for index, item := range list {
		result[index] = ojson.Plain(item)
	}
	return result
}

func maps(list []rec.Map) []any {
	result := make([]any, len(list))
	for index, item := range list {
		result[index] = item
	}
	return result
}

// generate is JSON.generate of a value (objects in their order).
func generate(value any) string { return string(ojson.Marshal(value)) }

func prefix(text string, count int) string {
	runes := []rune(text)
	if len(runes) <= count {
		return text
	}
	return string(runes[:count])
}

// mathInventory is BrowserCheck#math_inventory with PaperMathML: the formula's projected
// MathML as the browser inventory reads a tree.
func mathInventory(node rec.Map, macros mathintent.Macros, base int64) ojson.Object {
	root := mathexport.ParsePaper(rec.FetchString(node, "mathml"))
	if alt := mathexport.AltText(root); alt != "" {
		root.SetAttribute("alttext", alt)
	}
	mathexport.FitToPage(root, base)
	projected := mathexport.ParseRaw(mathexport.ProjectPage(root, node, macros))
	return ojson.Object{{"id", rec.Fetch(node, "id")}, {"tree", mathTree(projected)},
		{"rows", rec.Fetch(node, "children")}}
}

// mathTree is BrowserCheck#math_tree: as the browser inventory's trim(), a text of Unicode
// space (the no-break space included) is dropped.
func mathTree(element *mathexport.Node) ojson.Object {
	children := []any{}
	for _, child := range element.Children {
		if !child.Text {
			children = append(children, mathTree(child))
			continue
		}
		if value := child.Value(); strings.TrimFunc(value, rubySpaceOrBOM) != "" {
			children = append(children, value)
		}
	}
	attributes := ojson.Object{}
	for _, attribute := range element.Attrs {
		attributes = attributes.Set(attribute.Expanded(), attribute.Value())
	}
	return ojson.Object{{"tag", element.Name}, {"attributes", attributes},
		{"children", children}}
}

func validateRendering(inventory rec.Map, ids []string) {
	observed := []string{}
	for _, id := range rec.FetchList(inventory, "ids") {
		observed = append(observed, rec.ToS(id))
	}
	sort.Strings(observed)
	sort.Strings(ids)
	if !slices.Equal(observed, ids) {
		rec.Fail("Browser changed or duplicated anchors")
	}
	verifyPageBreaks(inventory, ids)
	if !rec.Equal(rec.Fetch(inventory, "brokenLinks"), int64(0)) {
		rec.Fail("Broken browser references")
	}
	if len(rec.FetchList(inventory, "invisible")) > 0 {
		rec.Fail("Invisible browser content")
	}
	if !truthy(rec.Fetch(inventory, "nativeMath")) || !truthy(rec.Fetch(inventory, "mathNamespaces")) {
		rec.Fail("Native MathML unavailable or invalid")
	}
	for _, fraction := range rec.FetchList(inventory, "fractions") {
		if !truthy(fraction) {
			rec.Fail("Fraction layout failed")
		}
	}
	if len(rec.FetchList(inventory, "resources")) == 0 && rec.Equal(rec.Fetch(inventory, "unsafeNodes"), int64(0)) {
		return
	}
	rec.Fail("Unexpected resources or unsafe browser markup")
}

func truthy(value any) bool { return value != nil && value != false }

// pageBreakID is a page-break marker's id (epub.MarkWebPages).
var pageBreakID = regexp.MustCompile(`\Apage-[1-9][0-9]*(?:-[1-9][0-9]*)?\z`)

// verifyPageBreaks: the print pages' markers (plan accessibility A8) are empty spans in the main
// text, out of the flow and at most a pixel, each named "Page " and its label, with ids of their
// own (ids are the page's other anchors).
func verifyPageBreaks(inventory rec.Map, ids []string) {
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	breaks, _ := inventory["pageBreaks"].([]any)
	for _, value := range breaks {
		row := rec.AsMap(value)
		id, _ := row["id"].(string)
		label, _ := row["label"].(string)
		size, _ := rec.Number(row["size"])
		rest, named := strings.CutPrefix(label, "Page ")
		if seen[id] || !pageBreakID.MatchString(id) || !named || strings.TrimSpace(rest) == "" || !truthy(row["empty"]) ||
			!truthy(row["main"]) || !truthy(row["flow"]) || size > 1 {
			rec.Fail("Invalid page-break markers")
		}
		seen[id] = true
	}
}

// orderedStructures is PaperBrowserCheck#ordered_structures: the structural nodes in reading
// order, notes' own roots left out.
func orderedStructures(document rec.Map) []rec.Map {
	nodes := rec.FetchMap(document, "nodes")
	var walk func(id any) []rec.Map
	walk = func(id any) []rec.Map {
		node := rec.Node(nodes, id)
		if !slices.Contains(paperdoc.Structures, rec.FetchString(node, "type")) {
			return nil
		}
		own := []rec.Map{}
		if node["type"] != "note" {
			own = append(own, rec.Pick(node, "id", "type", "children"))
		}
		for _, child := range rec.FetchList(node, "children") {
			own = append(own, walk(child)...)
		}
		return own
	}
	result := walk(rec.Fetch(document, "root"))
	for _, root := range rec.FetchList(document, "note_order") {
		result = append(result, walk(root)...)
	}
	return result
}

// pageAnchors are the page's own anchors beyond the document's nodes: the table of contents when the
// main text has headings, the main element and the index anchors.
func pageAnchors(document rec.Map) []string {
	nodes := rec.FetchMap(document, "nodes")
	// The main element, which the skip link targets (decision 088).
	anchors := []string{htmlkit.MainID}
	for _, id := range rec.FetchList(document, "main_block_order") {
		if rec.Node(nodes, id)["type"] == "heading" {
			anchors = append(anchors, htmlkit.ContentsID)
			break
		}
	}
	// The linked index's anchors (decision 084).
	for _, list := range bookindex.Anchors(document) {
		for _, anchor := range list {
			anchors = append(anchors, rec.FetchString(anchor, "id"))
		}
	}
	// The long descriptions of figures (decisions 096, 110): each a disclosure and its text.
	for _, span := range imageSpans(document) {
		if _, described := htmlkit.LongDescription(span); described {
			anchors = append(anchors, htmlkit.DescriptionID(span), htmlkit.DetailsID(span))
		}
	}
	// The bibliography run's wrapper (decision 111), when the page writes it: the English page's
	// doc-bibliography div around the entries.
	if id := bibliographyWrapperID(document); id != "" {
		anchors = append(anchors, id)
	}
	return anchors
}

// bibliographyWrapperID is the run's wrapper id, or "" when no run applies or the page is not
// English (the wrapper's name is the English word).
func bibliographyWrapperID(document rec.Map) string {
	if !htmlkit.English(document) {
		return ""
	}
	if _, _, _, ok := htmlkit.BibliographyRun(document); !ok {
		return ""
	}
	return htmlkit.BibliographyID(document)
}
