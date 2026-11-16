package typstsource

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/bibstyle"
	"github.com/sarielhp/relatex/internal/rec"
)

// Tests of the styled bibliography (decision 172): the entries Pergamon prints with sariel_biblatex.typ
// for testdata/four.bib, against a model that printed them so (styled) or otherwise
// (literal, with the differing entry).

const mathK = `<math xmlns="http://www.w3.org/1998/Math/MathML"><mi>𝑘</mi></math>`

const mathEpsilon = `<math xmlns="http://www.w3.org/1998/Math/MathML"><mi>𝜀</mi></math>`

func italicText(value string) rec.Map {
	return rec.Map{"type": "text", "text": value, "style": rec.Map{"shape": "it", "series": "m"}}
}

// threeEntries are the paper's entries as biblatex printed them with sariel_biblatex.
func threeEntries(year string) []rec.Map {
	return []rec.Map{
		{"id": "e0", "type": "paragraph", "content": []any{
			text("[AH05] P. K. Agarwal and S. Har-Peled. Approximating "), rec.Map{"type": "math", "node": "m2"},
			text("-nets of points."), italicText(" Discrete Comput. Geom."), text(", 33: 1–15, 2005.")}},
		{"id": "e1", "type": "paragraph", "content": []any{
			text("[AHV04] P. K. Agarwal, S. Har-Peled, and K. R. Varadarajan. Approximating extent measures of points."),
			italicText(" J. Assoc. Comput. Mach."), text(", 51(4): 606–635, 2004.")}},
		{"id": "e2", "type": "paragraph", "content": []any{
			text("[Dey98] T. K. Dey. Improved bounds for planar "), rec.Map{"type": "math", "node": "m1"},
			text("-sets and related problems."), italicText(" Discrete Comput. Geom."), text(", 19(3): 373–382, 1998.")}},
		{"id": "e3", "type": "paragraph", "content": []any{
			text("[Ede87] H. Edelsbrunner."), italicText(" Algorithms in Combinatorial Geometry"), text(". " + year + ".")}},
	}
}

func threeDocument(year string) rec.Map {
	cite := rec.Map{"id": "p1", "type": "paragraph", "content": []any{text("See "),
		rec.Map{"type": "text", "text": "[AHV04, Dey98]", "citation": int64(1)}, text(".")}}
	top := append([]rec.Map{cite}, threeEntries(year)...)
	document := testDocument(top, []rec.Map{{"id": "m1", "type": "math", "mathml": mathK}, {"id": "m2", "type": "math", "mathml": mathEpsilon}}, nil)
	document["citations"] = rec.Map{"calls": []any{rec.Map{"id": int64(1), "keys": []any{"a1", "c3"}}},
		"entries": []any{
			rec.Map{"id": int64(4), "key": "d4", "number": "AH05", "blocks": []any{"e0"}},
			rec.Map{"id": int64(1), "key": "a1", "number": "AHV04", "blocks": []any{"e1"}},
			rec.Map{"id": int64(2), "key": "c3", "number": "Dey98", "blocks": []any{"e2"}},
			rec.Map{"id": int64(3), "key": "b2", "number": "Ede87", "blocks": []any{"e3"}},
		}}
	return document
}

func threeBibliography(t *testing.T) *Bibliography {
	t.Helper()
	typst, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst is not on PATH")
	}
	root := setTestRoots(t)
	style, path, ok := bibstyle.Search{Root: root}.Find([]string{"sariel_biblatex"})
	if !ok {
		t.Fatal("the library has no sariel_biblatex.typ")
	}
	return &Bibliography{Processor: bibstyle.Biber, Names: []string{"sariel_biblatex"}, Style: &style, Path: path,
		Databases: []string{filepath.Join("testdata", "four.bib")}, Typst: typst, Root: root}
}

func styledExport(t *testing.T, document rec.Map, b *Bibliography) (*exporter, string) {
	t.Helper()
	e := &exporter{document: document, nodes: rec.FetchMap(document, "nodes"), directory: t.TempDir(), math: &mathWriter{}}
	e.index()
	e.styleBibliography(b)
	return e, e.render()
}

func TestMatchingEntriesShipAStyledBibliography(t *testing.T) {
	e, output := styledExport(t, threeDocument("1987"), threeBibliography(t))
	if e.bibReport["status"] != statusStyled {
		t.Fatalf("report %v", e.bibReport)
	}
	for _, want := range []string{`#ltd-bibliography()`, `#pergamon.citen("a1");`, `#pergamon.citen("c3");`,
		`#show: pergamon.refsection.with(style: ltd-bib-style)`} {
		if !strings.Contains(output, want) {
			t.Errorf("main.typ lacks %s:\n%s", want, output)
		}
	}
	if strings.Contains(output, "#bibitem(") {
		t.Errorf("literal entries in a styled export:\n%s", output)
	}
	references, err := os.ReadFile(filepath.Join(e.directory, referencesFile))
	if err != nil || !strings.Contains(string(references), "Planar $k$-Sets") {
		t.Errorf("references.bib does not hold the model's formula: %s %v", references, err)
	}
	for _, file := range []string{"support/" + styleFile, "support/" + bibliographyFile, "support/vendor/pergamon/lib.typ"} {
		if _, err := os.Stat(filepath.Join(e.directory, file)); err != nil {
			t.Errorf("missing %s", file)
		}
	}
	if strings.Contains(string(references), `\varepsilon`) {
		t.Errorf("references.bib keeps the TeX formula: %s", references)
	}
	wantPlaced(t, e.placed, map[string]string{"cite:d4": element, "cite:a1": element, "cite:b2": element, "cite:c3": element})
}

func TestDifferingEntryKeepsLiteralEntries(t *testing.T) {
	e, output := styledExport(t, threeDocument("1988"), threeBibliography(t))
	if e.bibReport["status"] != statusLiteral || !strings.Contains(toS(e.bibReport["reason"]), "1 of 4 entries differ") {
		t.Fatalf("report %v", e.bibReport)
	}
	differing := rec.AsMap(list(e.bibReport["differing"])[0])
	if differing["key"] != "b2" || differing["field"] != "text" {
		t.Errorf("differing %v", differing)
	}
	if !strings.Contains(output, `#bibitem("Ede87")`) || strings.Contains(output, "pergamon") {
		t.Errorf("literal export:\n%s", output)
	}
	for _, file := range []string{styleFile, bibliographyFile, referencesFile, "vendor"} {
		if _, err := os.Stat(filepath.Join(e.directory, file)); err == nil {
			t.Errorf("a literal export keeps %s", file)
		}
	}
}

func TestMissingStyleIsLiteralWithItsReason(t *testing.T) {
	b := &Bibliography{Processor: bibstyle.BibTeX, Names: []string{"plain"}, Path: ".:texmf"}
	e, _ := styledExport(t, threeDocument("1987"), b)
	if e.bibReport["status"] != statusLiteral || !strings.Contains(toS(e.bibReport["reason"]), "no Typst style plain.typ") {
		t.Errorf("report %v", e.bibReport)
	}
}

func TestEntriesMustBeOneRun(t *testing.T) {
	document := threeDocument("1987")
	section := rec.AsMap(rec.FetchMap(document, "nodes")["section"])
	section["children"] = []any{"e1", "p1", "e2", "e3"}
	e := &exporter{document: document, nodes: rec.FetchMap(document, "nodes"), directory: t.TempDir(), math: &mathWriter{}}
	e.index()
	if _, reason := e.bibliographyEntries(); !strings.Contains(reason, "one run") {
		t.Errorf("reason %q", reason)
	}
}

func TestNormalizedTextAndRuns(t *testing.T) {
	segments := []segment{{text: "[A]  \u201cOﬃce\u00A0"}, {text: "work", italic: true}, {text: " "}, {text: "plan", italic: true},
		{text: "\uE000x\uE000 it\uE001s "}, {text: foldMath("𝑘 − 1"), math: true}}
	runes := normalize(segments)
	if got := plainText(runes); got != "[A] “Office work plan”x” it’s -1k" {
		t.Errorf("text %q", got)
	}
	if got := runs(runes, italic); len(got) != 1 || got[0] != "work plan" {
		t.Errorf("italic runs %q", got)
	}
}

func TestPrintedEntrySegmentsAndPlaceholders(t *testing.T) {
	entry := printedEntry{Key: "k", Cells: [][][3]any{{{"[K]", false, false}},
		{{"A " + placeholder(1) + " and \uE002x\u2212y\uE003 " + placeholder(0), true, false}}}}
	if got := entry.placeholders(); len(got) != 2 || got[0] != 1 || got[1] != 0 {
		t.Errorf("placeholders %v", got)
	}
	segments := entry.segments()
	if !segments[len(segments)-2].math || segments[len(segments)-2].text != "-xy" {
		t.Errorf("segments %+v", segments)
	}
}

// A file in the sources is recorded by its path there, not by the run's work directory; a library
// style by its resource path.
func TestRecordedPathsNameTheSources(t *testing.T) {
	sources := t.TempDir()
	b := &Bibliography{Sources: sources, Databases: []string{filepath.Join(sources, "sub", "refs.bib")},
		Style: &bibstyle.Style{Name: "house", Path: "/elsewhere/house.typ", Origin: bibstyle.OriginLibrary}}
	if path := b.databaseRecords()[0].(rec.Map)["path"]; path != "sub/refs.bib" {
		t.Errorf("database recorded as %v", path)
	}
	if path := b.styleRecord().(rec.Map)["path"]; path != bibstyle.Library+"/house.typ" {
		t.Errorf("style recorded as %v", path)
	}
	if path := b.recorded("/outside/refs.bib"); path != "/outside/refs.bib" {
		t.Errorf("outside file recorded as %v", path)
	}
}

// Decision 173: an address url.sty set in math mode (an eprint inside its arXiv link, a URL) is the
// entry's text, not one of its formulas; a title's formula inside the title's DOI link stays a formula.
func TestURLSetInMathIsText(t *testing.T) {
	digits := `<math xmlns="http://www.w3.org/1998/Math/MathML"><mn>2001.00001</mn></math>`
	address := `<math xmlns="http://www.w3.org/1998/Math/MathML"><mi mathvariant="normal">http://a.org/x</mi></math>`
	entry := rec.Map{"id": "e0", "type": "paragraph", "content": []any{
		text("[GSW20] A. Gee. On "), rec.Map{"type": "math", "node": "m1", "external_link": "l1"},
		text("-sets. arXiv: "), rec.Map{"type": "math", "node": "m2", "external_link": "l2"},
		text(". "), rec.Map{"type": "math", "node": "m3"}, text(".")}}
	document := testDocument([]rec.Map{entry}, []rec.Map{{"id": "m1", "type": "math", "mathml": mathK},
		{"id": "m2", "type": "math", "mathml": digits}, {"id": "m3", "type": "math", "mathml": address}}, nil)
	// The model's links, by id (paperdoc's "items").
	document["external_links"] = rec.Map{"items": rec.Map{"l1": rec.Map{"href": "https://doi.org/10.1/k"},
		"l2": rec.Map{"href": "https://arxiv.org/abs/2001.00001"}}}
	e := &exporter{document: document, nodes: rec.FetchMap(document, "nodes"), math: &mathWriter{}}
	if formulas := e.entryFormulas(entry); len(formulas) != 1 || formulas[0]["id"] != "m1" {
		t.Errorf("formulas %v", formulas)
	}
	texts := e.modelTexts([]bibEntry{{key: "g20", node: entry}})
	if got := plainText(texts[0].text); got != "[GSW20] A. Gee. On k-sets. arXiv: 2001.00001. http://a.org/x." {
		t.Errorf("model text %q", got)
	}
}
