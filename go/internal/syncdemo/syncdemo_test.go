package syncdemo

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

func TestAlignSkipsWordsOnlyOneSideHas(t *testing.T) {
	cases := []struct {
		name      string
		html, pdf []string
		want      []int
	}{
		{"equal", []string{"a", "b"}, []string{"a", "b"}, []int{0, 1}},
		{"html extra", []string{"a", "x", "b"}, []string{"a", "b"}, []int{0, -1, 1}},
		{"pdf extra", []string{"a", "b"}, []string{"a", "x", "b"}, []int{0, 2}},
		{"footnote mark glued to the word", []string{"very", "weak"}, []string{"very1", "weak"}, []int{0, 1}},
		{"digits are not marks", []string{"1"}, []string{"12"}, []int{-1}},
		{"empty", nil, []string{"a"}, []int{}},
	}
	for _, c := range cases {
		if got := align(c.html, c.pdf); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAlignGreedyWhenTheTableWouldBeHuge(t *testing.T) {
	got := greedy([]string{"a", "c"}, []string{"a", "b", "c"}, []int{-1, -1})
	if !reflect.DeepEqual(got, []int{0, 2}) {
		t.Errorf("greedy: %v", got)
	}
}

func TestNormalizeKeepsLettersAndDigits(t *testing.T) {
	for in, want := range map[string]string{"Markov’s": "markovs", "ﬁne,": "fine", "Har-Peled∗": "harpeled", "(3/4)": "34", "—": ""} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTokenizeKeepsQuotesAndRawText(t *testing.T) {
	tokens := tokenize(`<p id="a>b" class='x'>Hi <b>there</b></p><script>if (a<b) {}</script><br/>`)
	var names []string
	for _, tok := range tokens {
		names = append(names, tok.name+map[bool]string{true: "/", false: ""}[tok.close])
	}
	if got := strings.Join(names, ","); got != "p,,b,,b/,p/,script,raw,script/,br" {
		t.Errorf("tokens %q", got)
	}
	if idOf(tokens[0]) != "a>b" {
		t.Errorf("id %q", idOf(tokens[0]))
	}
	if !tokens[len(tokens)-1].void {
		t.Error("<br/> is void")
	}
}

func TestWithAttributesKeepsSelfClosing(t *testing.T) {
	if got := withAttributes(parseTag(`<math id='x'>`), `class="sx"`); got != `<math id='x' class="sx">` {
		t.Error(got)
	}
	if got := withAttributes(parseTag(`<img src="a"/>`), `class="sx"`); got != `<img src="a" class="sx"/>` {
		t.Error(got)
	}
}

// fixture is a one-paragraph paper: "An easy proof" and a formula, from a source of two lines.
func fixture() *Data {
	return &Data{
		Sources: []Source{{Name: "main.tex", Text: "An easy proof of $x^2$ here\nsecond\n"}},
		Leaves: map[string][]Word{
			"n-p": {
				{Norm: "an", Origins: []Origin{{File: 0, Line: 1, Start: 0, End: 2}}},
				{Norm: "easy", Origins: []Origin{{File: 0, Line: 1, Start: 3, End: 7}}},
				{Norm: "proof", Origins: []Origin{{File: 0, Line: 1, Start: -1, End: -1}}},
				{Norm: "here", Origins: []Origin{{File: 0, Line: 1, Start: 23, End: 27}}},
			},
			"n-m": {{Norm: "x", Origins: []Origin{{File: 0, Line: 1, Start: 18, End: 19}}}, {Norm: "2", Origins: []Origin{{File: 0, Line: 1, Start: 20, End: 21}}}},
		},
	}
}

const fixtureHTML = `<html><head><title>t</title></head><body><main><p id="n-p">An easy <i>proof</i> of <span><math id='n-m'><mi>x</mi></math></span> here &amp; there</p>` +
	`<p>Chrome words</p></main></body></html>`

func TestAnnotateWrapsMappedWordsAndMarksFormulas(t *testing.T) {
	page, stats := Annotate(fixtureHTML, fixture())
	for _, want := range []string{
		`<span class="w" tabindex="0" data-o="0:1:0:2">An</span> <span class="w" tabindex="0" data-o="0:1:3:7">easy</span>`,
		`<i><span class="w" tabindex="0" data-o="0:1">proof</span></i>`,
		`<math id='n-m' class="sx" tabindex="0" data-o="0:1:17:22">`,
		`<span class="w" tabindex="0" data-o="0:1:23:27">here</span> &amp; there`,
		`<p>Chrome words</p>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %s in\n%s", want, page)
		}
	}
	// "of", "there" and the two chrome words are prose words without a source range.
	if stats.Words != 8 || stats.Owned != 6 || stats.Matched != 4 || stats.Exact != 3 || stats.LineOnly != 1 {
		t.Errorf("stats %+v", stats)
	}
	if stats.Formulas != 1 || stats.FormulasMapped != 1 {
		t.Errorf("formulas %+v", stats)
	}
}

func TestAnnotateLeavesTheTextOfTheHTMLUntouched(t *testing.T) {
	page, _ := Annotate(fixtureHTML, fixture())
	strip := func(s string) string {
		var out strings.Builder
		inTag := false
		for _, r := range s {
			switch {
			case r == '<':
				inTag = true
			case r == '>':
				inTag = false
			case !inTag:
				out.WriteRune(r)
			}
		}
		return out.String()
	}
	if strip(page) != strip(fixtureHTML) {
		t.Errorf("text changed:\n%q\n%q", strip(page), strip(fixtureHTML))
	}
}

func TestFormulaRangesMergePerLineAndWidenOverDelimiters(t *testing.T) {
	data := &Data{Sources: []Source{{Name: "a.tex", Text: "a $x+y$ b\n\\(z\\)\nplain"}}}
	got := data.formulaRanges([]Origin{{0, 1, 3, 4}, {0, 1, 5, 6}, {0, 2, 2, 3}, {0, 3, -1, -1}})
	want := []Origin{{0, 1, 2, 7}, {0, 2, 0, 5}, {0, 3, -1, -1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%v, want %v", got, want)
	}
}

func TestSourceLineGivesEachWordItsColumn(t *testing.T) {
	got := sourceLine(`a \cmd{é<b>} 12`)
	for _, want := range []string{`data-c="0">a<`, `data-c="2">\cmd<`, `data-c="7">é<`, `data-c="9">b<`, `&lt;`, `data-c="13">12<`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}

func TestBuildJoinsHyphenatedWordsAndFindsOwners(t *testing.T) {
	owners := &glyphowner.Owners{Leaves: []string{"n-p"}, Pages: []glyphowner.Page{{Number: 1, Leaf: []int{1, 1, 1, 0}}}}
	sourceMap := &rawSourceMap{
		Words: []rawWord{
			{ID: "g1-0", Text: "con-", Origins: []rawOrigin{{File: "b.tex", Line: 3, Characters: []int{4, 7}}}},
			{ID: "g1-1", Text: "sider", Origins: []rawOrigin{{File: "b.tex", Line: 3, Characters: []int{7, 12}}}},
			{ID: "g1-2", Text: "now", Origins: []rawOrigin{{File: "a.tex", Line: 1}}},
			{ID: "g1-3", Text: "unowned", Origins: []rawOrigin{{File: "a.tex", Line: 9}}},
		},
		LogicalWords: []rawLogical{{Normalized: "consider", WordIDs: []string{"g1-0", "g1-1"},
			Origins: []rawOrigin{{File: "b.tex", Line: 3, Characters: []int{4, 12}}}}},
		Sources: map[string]rawSource{"a.tex": {Text: "now"}, "b.tex": {Text: "xx\nyy\n    consider"}},
	}
	data := build(sourceMap, owners)
	want := []Word{{Norm: "consider", Origins: []Origin{{File: 0, Line: 3, Start: 4, End: 12}}}, {Norm: "now", Origins: []Origin{{File: 1, Line: 1, Start: -1, End: -1}}}}
	if !reflect.DeepEqual(data.Leaves["n-p"], want) {
		t.Errorf("%+v, want %+v", data.Leaves["n-p"], want)
	}
	if len(data.Sources) != 2 || data.Sources[0].Name != "b.tex" || data.Sources[1].Text != "now" {
		t.Errorf("sources %+v", data.Sources)
	}
}

func TestPageSwapsThePolicyAddsScriptAndSource(t *testing.T) {
	index := `<html><head><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'self' file:"/>` +
		`<script src="preview.js" defer="defer"></script></head><body class="x">` + `<main><p id="n-p">An easy</p></main></body></html>`
	page, _, err := Page(index, fixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"'unsafe-inline'", `<body class="x"><div id="demo"><div id="paper">`, `<aside id="source"`, `data-f="0"`,
		"<script>\n(() => {", `role="tab"`, `<span class="no">2</span>`, `<style>`} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Count(page, "</body>") != 1 || strings.Count(page, "Content-Security-Policy") != 1 {
		t.Error("body or policy duplicated")
	}
	if _, _, err := Page("<html></html>", fixture()); err == nil {
		t.Error("a page without a body is refused")
	}
}

// writeProduct is a product directory with the files Write reads: the source map, the owners, the html directory.
func writeProduct(t *testing.T) string {
	t.Helper()
	product := t.TempDir()
	document := map[string]any{"payload": map[string]any{"source_map": map[string]any{
		"words": []map[string]any{
			{"id": "g1-0", "text": "An", "origins": []map[string]any{{"file": "main.tex", "line": 1, "characters": []int{0, 2}}}},
			{"id": "g1-1", "text": "easy", "origins": []map[string]any{{"file": "main.tex", "line": 1, "characters": []int{3, 7}}}},
		},
		"sources": map[string]any{"main.tex": map[string]any{"text": "An easy\n"}},
	}}}
	writeGzip(t, filepath.Join(product, DocumentFile), document)
	writeGzip(t, filepath.Join(product, glyphowner.File), map[string]any{"schema_version": glyphowner.Version, "leaves": []string{"n-p"},
		"pages": []map[string]any{{"number": 1, "x": []float64{1, 2}, "y": []float64{1, 1}, "text": []string{"A", "e"}, "leaf": []int{1, 1}}}})
	exported := filepath.Join(product, "html")
	if err := os.MkdirAll(filepath.Join(exported, "figs"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"index.html": `<html><head></head><body><p id="n-p">An easy</p></body></html>`, "figs/a.svg": "<svg/>",
		"preview.js": "//", "export.json": "{}", "stage.json": "{}"}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(exported, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return product
}

func writeGzip(t *testing.T, path string, value any) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer := gzip.NewWriter(file)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWriteMakesASelfContainedDirectoryAndLeavesTheProduct(t *testing.T) {
	product := writeProduct(t)
	before, _ := os.ReadFile(filepath.Join(product, "html", "index.html"))
	out := filepath.Join(t.TempDir(), "demo")
	stats, err := Write(product, out)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Words != 2 || stats.Matched != 2 || stats.Exact != 2 {
		t.Errorf("stats %+v", stats)
	}
	page, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if !strings.Contains(string(page), `data-o="0:1:3:7">easy</span>`) || !strings.Contains(string(page), `<aside id="source"`) {
		t.Errorf("page:\n%s", page)
	}
	for _, name := range []string{"figs/a.svg", "preview.js"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("%s not copied: %v", name, err)
		}
	}
	for _, name := range []string{"export.json", "stage.json"} {
		if _, err := os.Stat(filepath.Join(out, name)); err == nil {
			t.Errorf("%s copied", name)
		}
	}
	after, _ := os.ReadFile(filepath.Join(product, "html", "index.html"))
	if string(before) != string(after) || strings.Contains(string(after), "<script") {
		t.Error("the export's own page changed")
	}
}

func TestWriteNeedsTheFullProduct(t *testing.T) {
	product := writeProduct(t)
	if err := os.Remove(filepath.Join(product, glyphowner.File)); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(product, filepath.Join(t.TempDir(), "demo")); err == nil || !strings.Contains(err.Error(), "--product") {
		t.Errorf("error %v", err)
	}
}

// A demo shows no email address: masked to the same length in the source (columns stay true), and a
// mailto link loses its target in the paper.
func TestDemoShowsNoEmailAddress(t *testing.T) {
	at := "someone" + "@" + "example.edu" // built here: the public leak scan refuses a literal address
	line := `\href{mailto:` + at + `}{` + at + `}; x`
	masked := maskEmails(line)
	if strings.Contains(masked, "example") || len([]rune(masked)) != len([]rune(line)) {
		t.Errorf("masked %q", masked)
	}
	short := "a" + "@" + "b.org"
	page := redactPage(`<a class="x" href="mailto:` + short + `" rel="n">` + short + `</a>`)
	if strings.Contains(page, "mailto") || strings.Contains(page, short) {
		t.Errorf("page %q", page)
	}
}
