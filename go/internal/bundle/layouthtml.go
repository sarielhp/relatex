package bundle

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/sourcemap"
)

// htmlEscape is CGI.escapeHTML.
var htmlEscape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")

// toS is Ruby's to_s on a JSON number or string.
func toS(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return string(marshal(value))
}

// difference is Ruby's a - b on JSON numbers: Integer when both are, else Float.
func difference(left, right any) any {
	a, aInt := left.(int64)
	b, bInt := right.(int64)
	if aInt && bInt {
		return a - b
	}
	x, okLeft := rec.Number(left)
	y, okRight := rec.Number(right)
	if !okLeft || !okRight {
		rec.Fail("undefined method '-' for %T", left)
	}
	return x - y
}

// WriteLayoutHTML is LayoutHTML.write: layout.html (the page graphics with a button per
// word), viewer.js and viewer-data.js.
func WriteLayoutHTML(layout, sourceMap rec.Map, directory string) {
	selections := map[string]rec.Map{}
	if sourceMap != nil {
		selections = sourcemap.Membership(sourceMap)
	}
	var pages strings.Builder
	for _, value := range rec.FetchList(layout, "pages") {
		page := rec.AsMap(value)
		graphics := decodeSVG(rec.FetchString(page, "svg"))
		mediamodel.ValidateSVG(graphics)
		uri := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(graphics)
		var words strings.Builder
		for _, entry := range pageWords(page, sourceMap, selections) {
			words.WriteString(wordButton(entry.word, rec.Fetch(entry.selected, "origins"), rec.FetchString(entry.selected, "id")))
		}
		pages.WriteString(`<section class="page" aria-label="Page ` + toS(rec.Fetch(page, "number")) + `" style="width:` +
			toS(rec.Fetch(page, "width")) + "pt;height:" + toS(rec.Fetch(page, "height")) + `pt">` +
			`<img alt="Original page graphics" src="` + uri + `"/>` + words.String() + "</section>")
	}
	html := `<!doctype html><html lang="en"><meta charset="utf-8">` +
		`<meta http-equiv="Content-Security-Policy" content="default-src &#39;none&#39;; img-src data:; ` +
		`style-src &#39;unsafe-inline&#39;; script-src &#39;self&#39;">` +
		`<title>Source synchronization</title><style>body{background:#ddd}.page{position:relative;background:white;margin:1em auto}` +
		`.page img{width:100%;height:100%}.word{position:absolute;border:0;padding:0;background:transparent;cursor:pointer}` +
		`.word:focus,.word:hover{background:#ff04;outline:1px solid blue}` +
		`.word.selected{background:#ff06;outline:1px solid #369}` +
		`#source-panel{position:sticky;top:0;background:white;z-index:1;padding:.5em}` +
		`#source-selection{white-space:pre-wrap}#source-text{max-height:14em;overflow:auto;text-align:left}</style>` +
		`<aside id="source-panel"><div id="source-selection" role="status">Select a rendered word or a source range.</div>` +
		sourceForm() + `<details><summary>Source snapshot</summary><pre id="source-text"></pre></details></aside>` + pages.String() +
		`<script src="viewer-data.js"></script><script src="viewer.js"></script></html>`
	check(os.WriteFile(filepath.Join(directory, "layout.html"), []byte(html), 0o644))
	script, err := os.ReadFile(filepath.Join(Root, "lib", "relatex", "source_viewer.js"))
	check(err)
	check(os.WriteFile(filepath.Join(directory, "viewer.js"), script, 0o644))
	check(os.WriteFile(filepath.Join(directory, "viewer-data.js"), viewerData(layout, sourceMap), 0o644))
}

func viewerData(layout, sourceMap rec.Map) []byte {
	m := sourceMap
	if m == nil {
		words := []any{}
		for _, page := range rec.FetchList(layout, "pages") {
			for _, value := range rec.FetchList(rec.AsMap(page), "words") {
				word := rec.Map{}
				for key, field := range rec.AsMap(value) {
					word[key] = field
				}
				word["origins"] = []any{}
				words = append(words, word)
			}
		}
		m = rec.Map{"words": words}
	}
	data := rec.Map{}
	if sources, ok := m["sources"]; ok {
		data["sources"] = sources
	}
	selected := []any{}
	for _, word := range sourcemap.Selection(m) {
		selected = append(selected, word)
	}
	data["words"] = selected
	if index, ok := m["index"]; ok {
		data["index"] = index
	} else {
		data["index"] = sourcemap.Index(m)
	}
	return append(append([]byte("window.RelatexData="), marshal(data)...), ";\n"...)
}

type pageWord struct{ word, selected rec.Map }

// pageWords: the page's word buttons with their selections: from a source map 2 its own
// words, placed by TeX in the same page coordinates (decision 066); otherwise the layout's.
func pageWords(page, sourceMap rec.Map, selections map[string]rec.Map) []pageWord {
	result := []pageWord{}
	number := rec.Fetch(page, "number")
	if sourceMap != nil && sourceMap["schema_version"] == sourcemap.Schema {
		for _, value := range rec.FetchList(sourceMap, "words") {
			word := rec.AsMap(value)
			if !rec.Equal(rec.Fetch(word, "page"), number) {
				continue
			}
			selected, ok := selections[rec.FetchString(word, "id")]
			if !ok {
				selected = word
			}
			result = append(result, pageWord{word, selected})
		}
		return result
	}
	for _, value := range rec.FetchList(page, "words") {
		word := rec.AsMap(value)
		selected, ok := selections[rec.FetchString(word, "id")]
		if !ok {
			selected = rec.Map{}
			for key, field := range word {
				selected[key] = field
			}
			selected["origins"] = []any{}
		}
		result = append(result, pageWord{word, selected})
	}
	return result
}

func sourceForm() string {
	fields := []struct{ name, label string }{{"line", "Line"}, {"column", "Column"}, {"end_line", "End line"}, {"end_column", "End column"}}
	inputs := []string{}
	for _, field := range fields {
		value := ""
		if field.name == "line" {
			value = ` value="1" required`
		}
		inputs = append(inputs, "<label>"+field.label+` <input id="source-`+field.name+`" type="number" min="1"`+value+"/></label>")
	}
	return `<form id="source-form"><label>File <select id="source-file"></select></label> ` + strings.Join(inputs, " ") +
		` <button type="submit">Find words</button></form>`
}

func wordButton(word rec.Map, origins any, selectionID string) string {
	text := htmlEscape.Replace(rec.FetchString(word, "text"))
	var buttons strings.Builder
	for _, value := range rec.FetchList(word, "boxes") {
		box := rec.AsList(value)
		css := "left:" + toS(box[0]) + "pt;top:" + toS(box[1]) + "pt;width:" + toS(difference(box[2], box[0])) +
			"pt;height:" + toS(difference(box[3], box[1])) + "pt"
		buttons.WriteString(`<button class="word" data-word="` + htmlEscape.Replace(selectionID) + `" data-fragment="` +
			htmlEscape.Replace(rec.FetchString(word, "id")) + `" data-origins="` + htmlEscape.Replace(string(marshal(origins))) +
			`" style="` + css + `" aria-label="` + text + `" title="` + text + `"></button>`)
	}
	return buttons.String()
}
