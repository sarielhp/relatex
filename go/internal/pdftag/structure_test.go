package pdftag

import (
	"fmt"
	"runtime"
	"slices"
	"testing"
)

func roles(elements []*selem) []string {
	result := []string{}
	for _, element := range elements {
		result = append(result, element.role)
	}
	return result
}

// pieces must split every block once: a block cut at many tables shares one content slice with all
// its runs, so splitting it again for each run copied the whole slice (papers4 0950: gigabytes).
func TestPiecesDoNotSplitBlocksAgain(t *testing.T) {
	const blocks, pairs = 8, 250
	nodes := map[string]node{}
	children, order := []string{}, []string{}
	for block := range blocks {
		id := fmt.Sprintf("p%d", block)
		content := make([]contentItem, 0, pairs*2)
		for range pairs {
			content = append(content, contentItem{Type: "text", Text: "word"},
				contentItem{Type: "table_grid", Grid: &tableGrid{}})
		}
		nodes[id] = node{ID: id, Type: "paragraph", Content: content}
		children, order = append(children, id), append(order, id)
	}
	nodes["d"] = node{ID: "d", Type: "document", Children: children}
	b := newBuilder(&document{Root: "d", Nodes: nodes, BlockOrder: order})
	b.build("d")
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	b.pieces()
	runtime.ReadMemStats(&after)
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8<<20 {
		t.Fatalf("pieces allocated %d bytes for %d blocks, want under 8 MiB: blocks were split again", allocated, blocks)
	}
}

// A row that covers fewer columns than the table ends with empty cells (a row listed apart is one
// row); a row span counts in the rows below.
func TestTableRowsArePadded(t *testing.T) {
	grid := &tableGrid{Cells: []tableCell{{ID: "a", Row: 1, Column: 1, Rowspan: 2}, {ID: "b", Row: 1, Column: 2},
		{ID: "c", Row: 1, Column: 3}, {ID: "e", Row: 3, Column: 1}, {ID: "d", Row: 2, Column: 2}}}
	table := newBuilder(&document{}).table(grid)
	widths := []int{}
	for _, row := range table.kids {
		widths = append(widths, len(row.kids))
	}
	if !slices.Equal(widths, []int{3, 2, 3}) {
		t.Fatalf("cells per row %v", widths)
	}
}

// A Figure's /Alt is its short text; its long description is an associated file supplementing it
// (decision 110), plain text with a /Desc, as PDF/UA-2 asks of embedded files.
func TestFigureLongDescriptionIsAnAssociatedFile(t *testing.T) {
	b := newBuilder(&document{})
	figure := b.figure("p", 0, contentItem{Type: "image", ID: "i", Alt: "A plot.", LongDescription: "The error falls fast."})
	if figure.alt != "A plot." || figure.long != "The error falls fast." {
		t.Fatalf("figure alt %q long %q", figure.alt, figure.long)
	}
	file := &pdfFile{objects: map[string]any{}, patch: map[string]any{}}
	w := &treeWriter{file: file}
	dict := map[string]any{}
	w.attributes(figure, dict)
	if dict["/Alt"] != "u:A plot." {
		t.Fatalf("alt %v", dict["/Alt"])
	}
	files, _ := dict["/AF"].([]any)
	if len(files) != 1 {
		t.Fatalf("associated files %v", dict["/AF"])
	}
	spec, _ := file.patch[objectKey(files[0].(string))].(map[string]any)["value"].(map[string]any)
	if spec["/AFRelationship"] != "/Supplement" || spec["/Desc"] != "u:"+LongDescriptionDesc {
		t.Fatalf("file specification %v", spec)
	}
	only := b.figure("p", 1, contentItem{Type: "image", ID: "j", LongDescription: "Only a long text."})
	if only.alt != "Only a long text." || only.long != "" {
		t.Fatalf("a figure with only a long text: alt %q long %q", only.alt, only.long)
	}
}

// A Formula's associated files are its MathML and, when the round trip proved one, its TeX
// (decision 141): the author's verified TeX wins over the formula TeX written from MathML.
func TestFormulaTeXIsASecondAssociatedFile(t *testing.T) {
	mathml := `<math xmlns="http://www.w3.org/1998/Math/MathML"><mi>𝑥</mi></math>`
	doc := &document{Nodes: map[string]node{
		"m": {ID: "m", Type: "math", MathML: mathml, FormulaTeX: &formulaTeXField{Text: "x"}},
		"n": {ID: "n", Type: "math", MathML: mathml},
		"a": {ID: "a", Type: "math", MathML: mathml, TeX: &authorTeX{Status: "verified", Text: `\x`}, FormulaTeX: &formulaTeXField{Text: "x"}},
	}}
	b := newBuilder(doc)
	file := &pdfFile{objects: map[string]any{}, patch: map[string]any{}}
	w := &treeWriter{file: file}
	cases := map[string][]string{"m": {"u:MathML", "u:LaTeX"}, "n": {"u:MathML"}, "a": {"u:MathML", "u:LaTeX"}}
	texts := map[string]string{"m": "x", "a": `\x`}
	for id, want := range cases {
		dict := map[string]any{}
		w.attributes(b.formula(id), dict)
		files, _ := dict["/AF"].([]any)
		descriptions := []string{}
		for _, ref := range files {
			spec, _ := file.patch[objectKey(ref.(string))].(map[string]any)["value"].(map[string]any)
			descriptions = append(descriptions, spec["/Desc"].(string))
		}
		if !slices.Equal(descriptions, want) {
			t.Fatalf("%s: associated files %v, want %v", id, descriptions, want)
		}
		if got := b.formula(id).tex; got != texts[id] {
			t.Fatalf("%s: TeX %q, want %q", id, got, texts[id])
		}
	}
}

// A float is an Aside with its caption a Caption kid holding the caption's paragraphs (ISO 32005:
// never a Caption of Document, which a Div would make it), and its other text as it is.
func TestFloatIsAnAsideWithItsCaption(t *testing.T) {
	text := []contentItem{{Type: "text", Text: "words"}}
	doc := &document{Root: "d", Nodes: map[string]node{
		"d":   {ID: "d", Type: "document", Children: []string{"f"}},
		"f":   {ID: "f", Type: "figure_float", Children: []string{"p1", "c", "p2", "p3"}},
		"p1":  {ID: "p1", Type: "paragraph", Content: text},
		"c":   {ID: "c", Type: "float_caption", Children: []string{"cp"}},
		"cp":  {ID: "cp", Type: "paragraph", Content: text},
		"p2":  {ID: "p2", Type: "paragraph", Content: text},
		"p3":  {ID: "p3", Type: "paragraph", Content: text},
		"ref": {ID: "ref", Type: "paragraph", Content: []contentItem{{Type: "reference", Text: "2"}}},
	}}
	built := newBuilder(doc).build("d")
	float := built[0].kids[0]
	if float.role != "Aside" {
		t.Fatalf("float is %s", float.role)
	}
	if got := roles(float.kids); !slices.Equal(got, []string{"Caption", "P", "P", "P"}) {
		t.Fatalf("the caption between paragraphs moves to the nearer end: %v", got)
	}
	if caption := float.kids[0]; len(caption.kids) != 1 || caption.kids[0].leaf != "cp" {
		t.Fatalf("caption kids %v", roles(caption.kids))
	}
	pieces := newBuilder(doc).itemPieces("ref", doc.Nodes["ref"].Content)
	if len(pieces) != 1 || pieces[0].kind != pieceText || pieces[0].text != "2" {
		t.Fatalf("a reference's text is the paragraph's: %+v", pieces)
	}
}

// A row that the row spans above cover wholly has no cells in the grid and is no row of the
// structure: veraPDF counts it as a row of no cells (8.2.5.26-3/-4, paper 1182), so the spans
// shrink to the rows that remain.
func TestCoveredRowsAreDropped(t *testing.T) {
	grid := &tableGrid{Cells: []tableCell{{ID: "a", Row: 0, Column: 0, Rowspan: 3}, {ID: "b", Row: 0, Column: 1, Rowspan: 2},
		{ID: "c", Row: 1, Column: 2}, {ID: "d", Row: 3, Column: 0}, {ID: "e", Row: 3, Column: 1}}}
	table := newBuilder(&document{}).table(grid)
	widths, spans := []int{}, []int{}
	for _, row := range table.kids {
		widths = append(widths, len(row.kids))
		spans = append(spans, row.kids[0].span[1])
	}
	if !slices.Equal(widths, []int{3, 1, 3}) || !slices.Equal(spans, []int{2, 1, 1}) {
		t.Fatalf("cells per row %v, row spans of the first cells %v", widths, spans)
	}
}

// The AS key of the optional content configuration goes (8.7-2), its View entry folded into the OFF
// list first; a Print or Export entry changes nothing on the screen.
func TestOptionalContentAutoStatesAreFolded(t *testing.T) {
	entry := func(value any) map[string]any { return map[string]any{"value": value} }
	file := &pdfFile{objects: map[string]any{
		"obj:6 0 R": entry(map[string]any{"/Type": "/OCG", "/Usage": map[string]any{"/View": map[string]any{"/ViewState": "/OFF"}}}),
		"obj:7 0 R": entry(map[string]any{"/Type": "/OCG", "/Usage": map[string]any{"/Print": map[string]any{"/PrintState": "/ON"}}}),
		"obj:8 0 R": entry([]any{"6 0 R", "7 0 R"}),
	}, patch: map[string]any{}}
	w := &treeWriter{file: file}
	view := map[string]any{"/Event": "/View", "/OCGs": "8 0 R"}
	printing := map[string]any{"/Event": "/Print", "/OCGs": "8 0 R"}
	properties := map[string]any{"/OCGs": "8 0 R", "/D": map[string]any{"/AS": []any{view, printing}, "/BaseState": "/ON"}}
	got := w.withoutAutoStates(properties)
	config, _ := got["/D"].(map[string]any)
	if config["/AS"] != nil || config["/BaseState"] != "/ON" || !slices.Equal(config["/OFF"].([]any), []any{"6 0 R"}) {
		t.Fatalf("configuration %v", config)
	}
	if w.withoutAutoStates(map[string]any{"/D": map[string]any{"/BaseState": "/ON"}}) != nil {
		t.Fatal("a configuration without AS changed")
	}
}
