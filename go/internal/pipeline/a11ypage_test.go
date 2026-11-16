package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/a11yreport"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/input"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// A document without a title block has no frontmatter key: the title falls back to the file name
// (it used to fail with `key not found: "frontmatter"`).
func TestDocumentTitleWithoutTitleBlock(t *testing.T) {
	cases := []struct {
		name     string
		document rec.Map
		want     string
	}{
		{"no frontmatter", rec.Map{}, "hw_01.tex"},
		{"empty title", rec.Map{"frontmatter": rec.Map{"title": "  "}}, "hw_01.tex"},
		{"title", rec.Map{"frontmatter": rec.Map{"title": " Sums "}}, "Sums"},
	}
	for _, tc := range cases {
		c := &conversion{document: tc.document}
		if got := c.documentTitle("dir/hw_01.tex"); got != tc.want {
			t.Errorf("%s: title %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Conversions without html or pdf-ua (such as markdown-only) do not write accessibility.html.
func TestAccessibilityPageRequiresHTMLOrPDFUA(t *testing.T) {
	root := t.TempDir()
	c := &conversion{archive: &input.Archive{Manifest: rec.Map{"main": "paper.tex"}}, options: Options{Root: t.TempDir()}}
	c.writeAccessibilityPage(root, ojson.Object{{"markdown", ojson.Object{}}})
	if isFile(filepath.Join(root, a11yreport.FileName)) {
		t.Fatalf("%s was written without html or pdf-ua", a11yreport.FileName)
	}
}

// Exports with html or pdf-ua carry the report page (decision 112): missing web measurement,
// EPUB, or tagged PDF gives honest reasons rather than failing on the nil document and lint.
func TestAccessibilityPageForHTMLOrPDFUA(t *testing.T) {
	for _, format := range []string{"html", "pdf-ua"} {
		root := t.TempDir()
		c := &conversion{archive: &input.Archive{Manifest: rec.Map{"main": "paper.tex"}}, options: Options{Root: t.TempDir()}}
		c.writeAccessibilityPage(root, ojson.Object{{format, ojson.Object{}}})
		data, err := os.ReadFile(filepath.Join(root, a11yreport.FileName))
		if err != nil {
			t.Fatalf("%s: the export has no %s: %v", format, a11yreport.FileName, err)
		}
		page := string(data)
		for _, want := range []string{"the page was not measured", "no EPUB was written"} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: page lacks %q", format, want)
			}
		}
		if format == "html" && !strings.Contains(page, "no tagged PDF was built") {
			t.Errorf("%s: page lacks %q", format, "no tagged PDF was built")
		}
		if strings.Contains(page, "<script") {
			t.Errorf("%s: the report page holds a script", format)
		}
	}
}

// A lint that cannot read the source is a note on the page, returned as a reason, never a panic
// that fails the conversion.
func TestSourceLintReportsAnUnreadableSource(t *testing.T) {
	c := &conversion{input: t.TempDir()}
	items, note, _ := c.sourceLint("missing.tex", nil)
	if len(items) != 0 || !strings.Contains(note, "The source could not be read for the lint") {
		t.Errorf("lint of a missing source: %d items, note %q", len(items), note)
	}
}

// accessibility.html is a sibling file in the export root, so the manifest lists it with its digest.
func TestOutputInventoryListsTheAccessibilityPage(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, a11yreport.FileName)
	if err := os.WriteFile(path, []byte("<html>report</html>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inventory := outputInventory(root)
	if got, want := rec.ToS(inventory[a11yreport.FileName]), command.FileDigest(path); got != want {
		t.Errorf("manifest %s = %q, want %q", a11yreport.FileName, got, want)
	}
}
