package a11ylint

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/relatex/internal/authordir"
	"github.com/sarielhp/relatex/internal/figures"
)

// ReviewPageGenerator marks the HTML review page in its head, so the lint recognises an earlier
// page and replaces it (decision 098).
const ReviewPageGenerator = "relatex lint review"

// reviewPageTitle is the page's title and first heading.
const reviewPageTitle = "Accessibility review"

// reviewPageKinds is the order the page lists the kinds in; a kind not named here follows.
var reviewPageKinds = []string{"language", "table", "link", "colour", "figure", "draft", "drawing", "heading", "math", "long"}

// pictureExtensions are the published forms an <img> can show; a figure published only as its
// source PDF gets no picture, since a browser does not render a PDF in <img>.
var pictureExtensions = []string{".svg", ".png", ".jpg", ".jpeg", ".gif", ".webp"}

// figurePageGroup is the figure/draft items that name one file as the source names it.
type figurePageGroup struct {
	target string
	items  []pageItem
	image  string // the published file name in figs/, "" when the product holds none
}

// pageItem is an item and its index in the page's item list, which names its crop.
type pageItem struct {
	Item  Item
	Index int
}

// ReviewPage is the HTML review page for every open item (decision 098, the second phase): the
// source around each language, table, link and colour item, the figure and draft items grouped
// by the file the source names, each with its picture when the product has it, and each item's
// PDF crop when crops gives one (the phase's PDF excerpts). It carries no script and is a local
// tool's page, not part of the export. It returns the page and the figures it shows, as
// {published name => absolute path in productDir/figs}, for the caller to copy.
func ReviewPage(paper Paper, items []Item, productDir string, crops ...Crop) (string, map[string]string) {
	files := map[string]*File{}
	for _, file := range paper.Files {
		files[file.Path] = file
	}
	cropOf := map[int]Crop{}
	for _, crop := range crops {
		if crop.Item >= 0 && crop.Item < len(items) {
			cropOf[crop.Item] = crop
		}
	}
	groups := figureGroups(items)
	published := map[string]string{}
	for kind, list := range groups {
		for index := range list {
			if name, path := publishedPicture(list[index].items[0].Item.Target, productDir); name != "" {
				list[index].image = name
				published[name] = path
			}
		}
		groups[kind] = list
	}
	counts, order := pageCounts(items)
	var out strings.Builder
	out.WriteString("<!DOCTYPE html>\n<html lang=\"en\"><head><meta charset=\"utf-8\">" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" +
		"<meta name=\"generator\" content=\"" + ReviewPageGenerator + "\">" +
		"<title>" + reviewPageTitle + "</title><style>" + reviewPageStyle + "</style></head><body><main>\n")
	fmt.Fprintf(&out, "<h1>%s</h1>\n<p>%d items and %d figure groups. Each place is the source as it is; the page changes nothing.</p>\n",
		reviewPageTitle, len(items), ReviewGroups(items))
	if summary := ExportSummary(paper); summary != "" {
		out.WriteString("<p><strong>" + html.EscapeString(summary) + "</strong></p>\n")
	}
	writeContents(&out, order, counts)
	out.WriteString(reviewControls)
	for _, kind := range order {
		fmt.Fprintf(&out, "<section id=\"%s\"><h2>%s (%d)</h2>\n", html.EscapeString(kind), kindLabel(kind), counts[kind])
		if figureKind(kind) {
			writeFigureGroups(&out, groups[kind], cropOf)
		} else {
			writeItemTable(&out, kind, itemsOfKind(items, kind), files, cropOf)
		}
		out.WriteString("</section>\n")
	}
	out.WriteString("</main>\n<script>" + reviewScript + "</script></body></html>\n")
	return out.String(), published
}

// ReviewGroups is how many figure groups the page shows: figure and draft items by kind and target.
func ReviewGroups(items []Item) int {
	total := 0
	for _, list := range figureGroups(items) {
		total += len(list)
	}
	return total
}

// figureGroups groups the figure and draft items by kind and the file the source names, preserving
// the order of their first place.
func figureGroups(items []Item) map[string][]figurePageGroup {
	groups := map[string][]figurePageGroup{}
	first := map[string]int{}
	for position, item := range items {
		if !figureKind(item.Kind) {
			continue
		}
		id := item.Kind + "\x00" + item.Target
		place := pageItem{Item: item, Index: position}
		if at, found := first[id]; found {
			groups[item.Kind][at].items = append(groups[item.Kind][at].items, place)
			continue
		}
		first[id] = len(groups[item.Kind])
		groups[item.Kind] = append(groups[item.Kind], figurePageGroup{target: item.Target, items: []pageItem{place}})
	}
	return groups
}

// publishedPicture is a group's picture in productDir/figs: its source name's stem with a derived
// form first, or "" when the product does not hold it.
func publishedPicture(target, productDir string) (string, string) {
	if target == "" || productDir == "" {
		return "", ""
	}
	stem := figures.SourceName(target)
	if stem == "" {
		return "", ""
	}
	for _, extension := range pictureExtensions {
		name := stem + extension
		path := filepath.Join(productDir, "figs", name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return name, path
		}
	}
	return "", ""
}

// pageCounts are the items by kind and the kinds in the page's order.
func pageCounts(items []Item) (map[string]int, []string) {
	counts, order := map[string]int{}, []string{}
	for _, item := range items {
		if counts[item.Kind] == 0 {
			order = append(order, item.Kind)
		}
		counts[item.Kind]++
	}
	ordered := []string{}
	for _, kind := range reviewPageKinds {
		if counts[kind] > 0 {
			ordered = append(ordered, kind)
		}
	}
	for _, kind := range order {
		if !contains(reviewPageKinds, kind) {
			ordered = append(ordered, kind)
		}
	}
	return counts, ordered
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func itemsOfKind(items []Item, kind string) []pageItem {
	result := []pageItem{}
	for index, item := range items {
		if item.Kind == kind {
			result = append(result, pageItem{Item: item, Index: index})
		}
	}
	return result
}

func figureKind(kind string) bool {
	return kind == "figure" || kind == "draft"
}

func kindLabel(kind string) string {
	if kind == "" {
		return "Other"
	}
	return strings.ToUpper(kind[:1]) + kind[1:]
}

func writeContents(out *strings.Builder, order []string, counts map[string]int) {
	out.WriteString("<nav aria-label=\"Contents\"><h2>Contents</h2><ul>\n")
	for _, kind := range order {
		fmt.Fprintf(out, "<li><a href=\"#%s\">%s</a> (%d)</li>\n", html.EscapeString(kind), kindLabel(kind), counts[kind])
	}
	out.WriteString("</ul></nav>\n")
}

// writeItemTable is one non-figure kind: each item's place with its PDF crop, message, fix,
// suggestion and the source excerpt around its line.
func writeItemTable(out *strings.Builder, kind string, items []pageItem, files map[string]*File, crops map[int]Crop) {
	fmt.Fprintf(out, "<table><caption>%d %s items, with the source around each place</caption>\n", len(items), kindLabel(kind))
	out.WriteString("<thead><tr><th>Place</th><th>Message and fix</th><th>Source</th></tr></thead><tbody>\n")
	for _, place := range items {
		writeItemRow(out, place, files[place.Item.File], crops)
	}
	out.WriteString("</tbody></table>\n")
}

func writeItemRow(out *strings.Builder, place pageItem, file *File, crops map[int]Crop) {
	item := place.Item
	location := item.File
	if item.Line > 0 {
		location += fmt.Sprintf(":%d", item.Line)
	}
	fmt.Fprintf(out, "<tr><td><code>%s</code>\n", html.EscapeString(location))
	writeCrop(out, place, crops)
	out.WriteString("</td><td>\n")
	fmt.Fprintf(out, "<p>%s</p>\n", html.EscapeString(item.Message))
	if item.Fix != "" {
		fmt.Fprintf(out, "<p class=\"fix\">%s</p>\n", html.EscapeString(item.Fix))
	}
	if item.Suggestion != "" {
		fmt.Fprintf(out, "<p class=\"muted\">Suggestion</p><pre class=\"suggestion\">%s</pre>\n", html.EscapeString(item.Suggestion))
	}
	if patchable(item) {
		fmt.Fprintf(out, "<p class=\"accept\"><label><input type=\"checkbox\" class=\"accept\" data-item=\"%s\" data-sha=\"%s\"> Apply this suggestion</label></p>\n",
			html.EscapeString(decisionID(item)), html.EscapeString(decisionSHA(item, file)))
	}
	out.WriteString("</td><td>\n")
	if text := sourceExcerpt(file, item); text != "" {
		fmt.Fprintf(out, "<pre class=\"source\">%s</pre>\n", text)
	} else {
		out.WriteString("<p class=\"muted\">No source line for this item.</p>\n")
	}
	out.WriteString("</td></tr>\n")
}

// sourceExcerpt is the item's line and three lines either side from the original source, numbered,
// the item's line marked. It stays inside the file at its first and last line.
func sourceExcerpt(file *File, item Item) string {
	if file == nil || file.Source == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(file.Source, "\n"), "\n")
	line := item.NoteLine
	if line <= 0 {
		line = item.Line
	}
	if line < 1 || line > len(lines) {
		return ""
	}
	start, end := max(1, line-3), min(len(lines), line+3)
	var out strings.Builder
	for number := start; number <= end; number++ {
		text := html.EscapeString(strings.TrimSuffix(lines[number-1], "\r"))
		if number == line {
			fmt.Fprintf(&out, "<span class=\"hit\">%5d  %s</span>\n", number, text)
			continue
		}
		fmt.Fprintf(&out, "%5d  %s\n", number, text)
	}
	return out.String()
}

// writeFigureGroups is one figure kind: each group with the file as the source names it, its count,
// its picture when the product has it, and each place with its PDF crop and the sidecar key to
// describe.
func writeFigureGroups(out *strings.Builder, groups []figurePageGroup, crops map[int]Crop) {
	for _, group := range groups {
		target := group.target
		if target == "" {
			target = "a figure the source does not name"
		}
		fmt.Fprintf(out, "<div class=\"group\"><h3>%s <span class=\"badge\">%d places</span></h3>\n",
			html.EscapeString(target), len(group.items))
		if group.image != "" {
			fmt.Fprintf(out, "<img src=\"figs/%s\" alt=\"\">\n<p class=\"muted\">Picture: <code>figs/%s</code></p>\n",
				html.EscapeString(group.image), html.EscapeString(group.image))
		} else {
			out.WriteString("<p class=\"muted\">No picture of this figure is in the product.</p>\n")
		}
		writeMessages(out, group.items)
		out.WriteString("<ul class=\"places\">\n")
		for _, place := range group.items {
			item := place.Item
			location := item.File
			if item.Line > 0 {
				location += fmt.Sprintf(":%d", item.Line)
			}
			fmt.Fprintf(out, "<li><code>%s</code>", html.EscapeString(location))
			if item.Key != "" {
				fmt.Fprintf(out, " key <code>%s</code>: <code>relatex alt set %s --alt \"…\"</code>",
					html.EscapeString(item.Key), html.EscapeString(item.Key))
			}
			writeCrop(out, place, crops)
			out.WriteString("</li>\n")
		}
		out.WriteString("</ul></div>\n")
	}
}

// writeCrop is an item's PDF excerpt, or what the page says instead of one: a document-level item
// has no page (language, the --math note); any other without a crop has no box in the source map
// or came after the crop limit.
func writeCrop(out *strings.Builder, place pageItem, crops map[int]Crop) {
	if crop, ok := crops[place.Index]; ok {
		fmt.Fprintf(out, "<figure><img src=\"crops/%s\" alt=\"Excerpt of the PDF page %d around this item\"><figcaption>Page %d</figcaption></figure>\n",
			html.EscapeString(crop.PNG), crop.Page, crop.Page)
		return
	}
	if documentLevel(place.Item) {
		out.WriteString("<p class=\"muted\">document-level</p>\n")
	}
}

// documentLevel reports whether an item concerns the document rather than one place: the language
// line and the math note name no page.
func documentLevel(item Item) bool {
	return item.Line <= 0 || item.Kind == "language" || item.Kind == "math"
}

// writeMessages writes each distinct message of a group once, so a repeated figure says it once.
func writeMessages(out *strings.Builder, items []pageItem) {
	seen := map[string]bool{}
	for _, place := range items {
		message := place.Item.Message
		if message == "" || seen[message] {
			continue
		}
		seen[message] = true
		fmt.Fprintf(out, "<p>%s</p>\n", html.EscapeString(message))
	}
}

// reviewPageStyle keeps the page readable in the browser's light or dark scheme (the review page's
// style, copied so a11ylint need not import alttext).
const reviewPageStyle = `:root{color-scheme:light dark;--fg:#1b1b1b;--bg:#fff;--muted:#555;--line:#ccc;--hit:#8a4b00}
@media (prefers-color-scheme:dark){:root{--fg:#e8e8e8;--bg:#161616;--muted:#aaa;--line:#444;--hit:#f0b45c}}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.5 system-ui,sans-serif}
main{max-width:72rem;margin:0 auto;padding:1rem}
section{border-top:1px solid var(--line);padding:1rem 0}
table{border-collapse:collapse;width:100%}caption{text-align:left;color:var(--muted);padding:.4rem 0}
th,td{border-top:1px solid var(--line);padding:.4rem;vertical-align:top;text-align:left}
pre{overflow:auto;background:rgba(127,127,127,.12);padding:.5rem;font-size:.85rem}
.hit{color:var(--hit);font-weight:600}
.badge{font-size:.8rem;font-weight:600;border:1px solid currentColor;border-radius:.3rem;padding:0 .4rem}
img{max-width:100%;max-height:24rem;background:#fff;border:1px solid var(--line)}
code{font-size:.9em}.muted{color:var(--muted)}.fix{font-weight:500}
.places li{margin:.2rem 0}:focus-visible{outline:2px solid currentColor;outline-offset:2px}
.controls{display:flex;gap:.6rem;align-items:center;margin:1rem 0}.accept{margin:.4rem 0}`

// reviewControls are the page's buttons; without scripts it says how to make a decisions file.
const reviewControls = `<div class="controls"><button type="button" class="tick-all">Tick every suggestion</button>
<button type="button" class="save-review">Save the review file</button> <span class="saved" role="status"></span>
<noscript><p>Scripts are off: the page is read-only. To keep the items you accept, write a ` + DecisionsFile +
	` with a [review] table, then one [[accept]] per item holding its id and sha (both beside its checkbox), and run relatex lint --accessibility --patch FILE --accept ` + DecisionsFile + `.</p></noscript></div>
`

// reviewScript saves the ticked items as the decisions file: [review] first (so a sidecar reader
// refuses it), then one [[accept]] per ticked item with the id and sha the page showed.
const reviewScript = `(function(){
var q=JSON.stringify;
function boxes(){return Array.prototype.slice.call(document.querySelectorAll('input.accept'));}
document.querySelectorAll('button.tick-all').forEach(function(b){b.addEventListener('click',function(){
boxes().forEach(function(box){box.checked=true;});});});
document.querySelectorAll('button.save-review').forEach(function(b){b.addEventListener('click',function(){
var lines=['# Accessibility decisions saved from the review page; take them in with:',
'# relatex lint --accessibility --patch PATCH --accept ` + DecisionsFile + `','[review]',
'saved = '+q(new Date().toISOString().slice(0,10)),''];
var count=0;
boxes().forEach(function(box){if(!box.checked){return;}
lines.push('[[accept]]','id = '+q(box.dataset.item),'sha = '+q(box.dataset.sha),'');count++;});
var link=document.createElement('a');
link.href=URL.createObjectURL(new Blob([lines.join('\n')],{type:'application/toml'}));
link.download='` + authordir.LintName + `';document.body.appendChild(link);link.click();link.remove();
document.querySelectorAll('span.saved').forEach(function(s){s.textContent=count+' items saved in ` + authordir.LintName + ` (keep it as ` + DecisionsFile + `)';});});});
})();`
