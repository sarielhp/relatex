package typstsource

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

func setTestRoots(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	Root, tablemodel.Root, fontmodel.Root, paperdoc.Root = root, root, root, root
	return root
}

func TestPaperWithoutChaptersIsUnchangedByteForByte(t *testing.T) {
	root := setTestRoots(t)
	doc := decodeFile(t, filepath.Join(root, "test", "fixtures", "typst", "sections.json")).(rec.Map)
	outDir := t.TempDir()
	Write(doc, outDir)

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"main.typ", "prefix.typ"}) {
		t.Fatalf("unexpected files for non-chapter paper: %v", names)
	}

	preamble, readErr := os.ReadFile(filepath.Join(root, "lib", "relatex", "typst_source_preamble.typ"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	e := &exporter{document: doc, nodes: rec.FetchMap(doc, "nodes"), directory: t.TempDir(),
		math: &mathWriter{}, preamble: string(preamble)}
	e.index()
	single := e.render()
	got, gotErr := os.ReadFile(filepath.Join(outDir, "main.typ"))
	if gotErr != nil {
		t.Fatal(gotErr)
	}
	if string(got) != single {
		t.Errorf("paper main.typ changed when no chapters present")
	}
}

func sampleBookDocument(t *testing.T, root string) rec.Map {
	t.Helper()
	doc := decodeFile(t, filepath.Join(root, "test", "fixtures", "typst", "sections.json")).(rec.Map)
	nid := func(n int) string { return fmt.Sprintf("n-%064x", n) }
	thmSem := func(entity int64) rec.Map {
		return rec.Map{"kind": "theorem", "entity": entity, "environment": "theorem", "title": "Theorem",
			"counter": "theorem", "numbered": true, "counter_register": 333745}
	}
	headingSem := func(counter string, level int64) rec.Map {
		return rec.Map{"kind": "heading", "counter": counter, "level": level, "runin": false}
	}
	chapterHead := func(id, title string) rec.Map {
		return rec.Map{"id": id, "type": "heading", "content": []any{text(title)}, "semantics": headingSem("chapter", 1)}
	}
	paragraph := func(id string, spans ...any) rec.Map {
		return rec.Map{"id": id, "type": "paragraph", "content": spans}
	}
	ref := func(targetLabel, targetID, printed string) rec.Map {
		return rec.Map{"type": "reference", "label": targetLabel, "target": targetID, "text": printed}
	}
	mathN := rec.Map{"id": nid(13), "type": "math", "mathml": blockX, "tex": rec.Map{"status": "unavailable"}}
	nodes := rec.Map{
		nid(1):  rec.Map{"id": nid(1), "type": "document", "children": []any{nid(2), nid(3), nid(24), nid(29)}},
		nid(2):  paragraph(nid(2), text("Front note before "), ref("sec:two", nid(19), "2"), text(".")),
		nid(3):  rec.Map{"id": nid(3), "type": "section", "semantics": headingSem("part", 1), "children": []any{nid(4), nid(5), nid(18)}},
		nid(4):  rec.Map{"id": nid(4), "type": "heading", "content": []any{text("Part One")}, "semantics": headingSem("part", 1)},
		nid(5):  rec.Map{"id": nid(5), "type": "section", "semantics": headingSem("chapter", 1), "children": []any{nid(6), nid(7), nid(9), nid(10), nid(14), nid(17)}},
		nid(6):  chapterHead(nid(6), "First"),
		nid(7):  rec.Map{"id": nid(7), "type": "theorem", "semantics": thmSem(1), "children": []any{nid(8)}},
		nid(8):  paragraph(nid(8), text("Theorem 1.1."), text(" Theorem statement.")),
		nid(9):  rec.Map{"id": nid(9), "type": "display_math", "children": []any{nid(13)}, "semantics": rec.Map{"kind": "equation", "entity": int64(3), "environment": "equation"}, "content": []any{rec.Map{"type": "math", "node": nid(13)}, text("(1)")}},
		nid(13): mathN,
		nid(10): rec.Map{"id": nid(10), "type": "figure_float", "children": []any{nid(11), nid(12)}},
		nid(11): paragraph(nid(11), text("Fig one")),
		nid(12): rec.Map{"id": nid(12), "type": "float_caption", "children": []any{nid(15)}},
		nid(15): paragraph(nid(15), text("Figure 1: Captioned.")),
		nid(14): rec.Map{"id": nid(14), "type": "figure_float", "children": []any{nid(16)}},
		nid(16): paragraph(nid(16), text("Uncaptioned")),
		nid(17): paragraph(nid(17), text("End of ch 1.")),
		nid(18): rec.Map{"id": nid(18), "type": "section", "semantics": headingSem("chapter", 1), "children": []any{nid(19), nid(20), nid(22), nid(23)}},
		nid(19): chapterHead(nid(19), "Second"),
		nid(20): rec.Map{"id": nid(20), "type": "theorem", "semantics": thmSem(2), "children": []any{nid(21)}},
		nid(21): paragraph(nid(21), text("Theorem 2.1."), text(" Another theorem.")),
		nid(22): paragraph(nid(22), text("See "), ref("thm:1", nid(8), "1.1"), text(", "), ref("eq:1", nid(9), "(1)"), text(", "), ref("fig:1", nid(15), "1"), text(".")),
		nid(23): rec.Map{"id": nid(23), "type": "figure_float", "children": []any{nid(30), nid(31)}},
		nid(30): paragraph(nid(30), text("Fig two")),
		nid(31): rec.Map{"id": nid(31), "type": "float_caption", "children": []any{nid(32)}},
		nid(32): paragraph(nid(32), text("Figure 2: Second.")),
		nid(24): rec.Map{"id": nid(24), "type": "section", "semantics": headingSem("chapter", 1), "children": []any{nid(25), nid(26)}},
		nid(25): chapterHead(nid(25), "Appendix"),
		nid(26): rec.Map{"id": nid(26), "type": "section", "semantics": headingSem("section", 2), "children": []any{nid(27), nid(28)}},
		nid(27): rec.Map{"id": nid(27), "type": "heading", "content": []any{text("Details")}, "semantics": headingSem("section", 2)},
		nid(28): paragraph(nid(28), text("Appendix body.")),
		nid(29): paragraph(nid(29), text("Afterword referencing "), ref("sec:one", nid(6), "1"), text(".")),
	}
	prov := rec.Map{"inspection_key": "main", "source_hint": "unavailable"}
	for _, entry := range nodes {
		nodeMap := entry.(rec.Map)
		if _, ok := nodeMap["children"]; !ok {
			nodeMap["children"] = []any{}
		}
		if _, ok := nodeMap["provenance"]; !ok {
			nodeMap["provenance"] = prov
		}
	}
	labels := rec.Map{
		"sec:one": rec.Map{"target": nid(6), "number": "1", "status": "verified"},
		"sec:two": rec.Map{"target": nid(19), "number": "2", "status": "verified"},
		"sec:app": rec.Map{"target": nid(25), "number": "A", "status": "verified"},
		"sec:sub": rec.Map{"target": nid(27), "number": "A.1", "status": "verified"},
		"thm:1":   rec.Map{"target": nid(8), "number": "1.1", "status": "verified"},
		"eq:1":    rec.Map{"target": nid(9), "number": "1", "status": "verified"},
		"fig:1":   rec.Map{"target": nid(15), "number": "1", "status": "verified"},
	}
	blockOrder := []any{}
	var collectBlocks func(id string)
	collectBlocks = func(id string) {
		nodeMap := nodes[id].(rec.Map)
		kind := toS(nodeMap["type"])
		if kind == "heading" || kind == "paragraph" || kind == "display_math" {
			blockOrder = append(blockOrder, id)
		}
		for _, child := range nodeMap["children"].([]any) {
			collectBlocks(toS(child))
		}
	}
	collectBlocks(nid(1))
	doc["root"] = nid(1)
	doc["nodes"] = nodes
	doc["block_order"] = blockOrder
	doc["main_block_order"] = blockOrder
	doc["note_order"] = []any{}
	doc["labels"] = labels
	frontmatter := doc["frontmatter"].(rec.Map)
	frontmatter["title"] = "Sample Book"
	delete(frontmatter, "abstract_blocks")
	delete(frontmatter, "abstract_source")
	delete(frontmatter, "abstract")
	return doc
}

func TestBookSplitsIntoMainAndChapterFiles(t *testing.T) {
	root := setTestRoots(t)
	doc := sampleBookDocument(t, root)
	outDir := t.TempDir()
	Write(doc, outDir)

	chapterEntries, err := os.ReadDir(filepath.Join(outDir, "chapters"))
	if err != nil {
		t.Fatal(err)
	}
	gotChapters := make([]string, 0, len(chapterEntries))
	for _, entry := range chapterEntries {
		gotChapters = append(gotChapters, entry.Name())
	}
	slices.Sort(gotChapters)
	wantChapters := []string{"01.typ", "02.typ", "03.typ", "04.typ", "05.typ", "06.typ"}
	if !slices.Equal(gotChapters, wantChapters) {
		t.Fatalf("chapter files = %v, want %v", gotChapters, wantChapters)
	}

	preamble, preambleErr := os.ReadFile(filepath.Join(root, "lib", "relatex", "typst_source_preamble.typ"))
	if preambleErr != nil {
		t.Fatal(preambleErr)
	}
	e := &exporter{document: doc, nodes: rec.FetchMap(doc, "nodes"), directory: t.TempDir(),
		math: &mathWriter{}, preamble: string(preamble)}
	e.index()
	single := e.render()
	mainBytes, mainErr := os.ReadFile(filepath.Join(outDir, "main.typ"))
	if mainErr != nil {
		t.Fatal(mainErr)
	}
	mainText := string(mainBytes)
	if !strings.HasPrefix(mainText, header) {
		t.Errorf("split main.typ missing the header")
	}
	lastPos := -1
	for _, name := range wantChapters {
		directive := `#include "chapters/` + name + `"`
		pos := strings.Index(mainText, directive)
		if pos <= lastPos {
			t.Fatalf("main.typ missing or out-of-order %s:\n%s", directive, mainText)
		}
		lastPos = pos
	}

	probeEnd := strings.Index(single, "#let ltd-probe(")
	closeLine := strings.Index(single[probeEnd:], "\n}\n\n")
	singleBody := single[probeEnd+closeLine+4:]

	bodies := make([]string, 0, len(wantChapters))
	seenLets := map[string]bool{}
	for _, name := range wantChapters {
		raw, readErr := os.ReadFile(filepath.Join(outDir, "chapters", name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		textBody := strings.TrimPrefix(string(raw), "#import \"../prefix.typ\": *\n\n")
		textBody = strings.TrimSuffix(textBody, "\n")
		filtered := []string{}
		for _, line := range strings.Split(textBody, "\n") {
			if strings.HasPrefix(line, "#let ") && seenLets[line] {
				continue
			}
			if strings.HasPrefix(line, "#let ") {
				seenLets[line] = true
			}
			filtered = append(filtered, line)
		}
		bodies = append(bodies, strings.Join(filtered, "\n"))
	}
	concatenated := strings.Join(bodies, "\n\n") + "\n"
	if concatenated != singleBody {
		t.Errorf("concatenated chapters differ from single-file body:\ngot:\n%s\nwant:\n%s", concatenated, singleBody)
	}

	typstBin, typstErr := exec.LookPath("typst")
	if typstErr != nil {
		t.Skip("typst not installed")
	}
	singleDir := t.TempDir()
	singleMain := filepath.Join(singleDir, "main.typ")
	if writeErr := os.WriteFile(singleMain, []byte(single), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	writeSupportTest(t, singleDir)
	expr := `query(selector(heading).or(math.equation).or(figure).or(ref)).map(el => (func: repr(el.func()), heading: counter(heading).at(el.location()), fig: counter(figure.where(kind: image)).at(el.location()), eq: counter(math.equation).at(el.location())))`
	singleOut, sErr := exec.Command(typstBin, "eval", "--ignore-system-fonts", "--format", "json", "--in", singleMain, expr).CombinedOutput()
	if sErr != nil {
		t.Fatalf("single typst eval failed: %v\n%s", sErr, singleOut)
	}
	splitOut, pErr := exec.Command(typstBin, "eval", "--ignore-system-fonts", "--format", "json", "--in", filepath.Join(outDir, "main.typ"), expr).CombinedOutput()
	if pErr != nil {
		t.Fatalf("split typst eval failed: %v\n%s", pErr, splitOut)
	}
	if string(splitOut) != string(singleOut) {
		t.Errorf("cross-file counters differ from single-file:\nsplit:  %s\nsingle: %s", splitOut, singleOut)
	}
}

// writeSupportTest writes relatex's Typst where an export's main.typ imports it from.
func writeSupportTest(t *testing.T, directory string) {
	t.Helper()
	preamble, err := os.ReadFile(filepath.Join(setTestRoots(t), "lib", "relatex", "typst_source_preamble.typ"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(filepath.Join(directory, preambleFile), string(preamble))
}
