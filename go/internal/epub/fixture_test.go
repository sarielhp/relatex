package epub

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/htmlkit"
)

// fixtureFigure is the drawing the fixture page shows.
const fixtureFigure = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120" width="120" height="120">` +
	`<circle cx="60" cy="60" r="50" fill="none" stroke="#000000" stroke-width="2"/><circle cx="60" cy="60" r="3"/></svg>` + "\n"

// TestFixturePackage builds the gate's fixture package from testdata/fixture.html: the title and two
// sections in three content documents, MathML, a described figure (with a short description and its
// details, decisions 110 and 125), a table with headers, a note, page-break
// markers with their page list, and the conformance claim a passing report earns. The gate
// (test/epub_fixture_test.rb) validates it with epubcheck and DAISY Ace; RELATEX_EPUB_FIXTURE names
// the file it is written to, else it goes to a temporary directory.
func TestFixturePackage(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("testdata", "fixture.html"))
	if err != nil {
		t.Fatal(err)
	}
	p := preparePage(string(source))
	documents, reason := splitPage(p)
	if reason != "" || len(documents) != 3 {
		t.Fatalf("documents %d, reason %q", len(documents), reason)
	}
	staging := t.TempDir()
	if err = os.MkdirAll(filepath.Join(staging, "figs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(staging, "figs", "disk.svg"), []byte(fixtureFigure), 0o644); err != nil {
		t.Fatal(err)
	}
	path := os.Getenv("RELATEX_EPUB_FIXTURE")
	if path == "" {
		path = filepath.Join(t.TempDir(), "fixture.epub")
	}
	metadata := Metadata{Title: p.title(), Creators: []string{"Ada Lovelace"}, Language: "en", Modified: fallbackModified,
		Access: evidence(p)}
	metadata.Access.Claim = htmlkit.WCAGClaim
	options := Options{Source: "fixture", PrintSource: "fixture.pdf, SHA-256 " + strings.Repeat("0", 64),
		Pages: fixturePages}
	metadata = assemble(p, documents, staging, nil, metadata, options, path)
	if metadata.Access.Pages != 3 || conformance(metadata.Access) == "" || metadata.Access.Images != 2 || metadata.Access.Described != 2 ||
		metadata.Access.Long != 1 {
		t.Fatalf("accessibility %+v", metadata.Access)
	}
	// The formula's intent and arg travel as data attributes, which epubcheck accepts (decision 109).
	if p.adjustments.IntentsAsData != 1 {
		t.Errorf("adjustments %+v", p.adjustments)
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	names := []string{}
	for _, file := range archive.File {
		names = append(names, file.Name)
	}
	want := "mimetype META-INF/container.xml EPUB/package.opf EPUB/nav.xhtml EPUB/style.css EPUB/index.xhtml EPUB/section-1.xhtml EPUB/section-2.xhtml EPUB/figs/disk.svg"
	if strings.Join(names, " ") != want {
		t.Errorf("entries %v", names)
	}
	compareGolden(t, archive)
}

// fixturePages are the fixture's three print pages (decision 130): a roman front page at the title,
// a page that begins in a table cell, and a page of floats after the text, marked at its end.
var fixturePages = []PageMark{{1, "i", "title"}, {2, "1", "cell-1"}, {3, "2", ""}}

// goldenParts are what the fixture's golden files hold: the package document, the navigation
// document, and each content document's page-break markers with what follows them.
var goldenParts = []string{"package.opf", "nav.xhtml", "markers.txt"}

// pageMarker is a page-break marker and the start of what follows it.
var pageMarker = regexp.MustCompile(`<span id="page-[^"]*" epub:type="pagebreak"[^>]*></span>.{0,40}`)

// compareGolden compares the package with testdata/golden/fixture-*; RELATEX_UPDATE_GOLDEN=1
// rewrites them.
func compareGolden(t *testing.T, archive *zip.ReadCloser) {
	t.Helper()
	files := map[string]string{}
	markers := []string{}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		name := strings.TrimPrefix(file.Name, PackageDirectory+"/")
		files[name] = string(data)
		for _, found := range pageMarker.FindAllString(string(data), -1) {
			markers = append(markers, name+": "+found)
		}
	}
	files["markers.txt"] = strings.Join(markers, "\n") + "\n"
	for _, part := range goldenParts {
		golden := filepath.Join("testdata", "golden", "fixture-"+part)
		if os.Getenv("RELATEX_UPDATE_GOLDEN") == "1" {
			if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(golden, []byte(files[part]), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != files[part] {
			t.Errorf("%s differs from %s (RELATEX_UPDATE_GOLDEN=1 rewrites it):\n%s", part, golden, files[part])
		}
	}
}
