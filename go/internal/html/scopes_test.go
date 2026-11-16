package html

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablelayout"
	"github.com/sarielhp/relatex/internal/tablelayout/tablefixture"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// scopeFixture is a grid of test/fixtures/table_headers (tablelayout's header rule cases) as a
// table_grid span.
func scopeFixture(t *testing.T, name string) rec.Map {
	t.Helper()
	tablemodel.Root = filepath.Join("..", "..", "..")
	file, err := os.Open(filepath.Join("..", "..", "..", "test", "fixtures", "table_headers", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	value, err := canonjson.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return tablefixture.Span("t-"+name, rec.FetchMap(rec.AsMap(value), "grid"))
}

// A classic projection's cells take the same scopes by row and column.
func TestClassicScopes(t *testing.T) {
	grid := rec.FetchMap(scopeFixture(t, "bold_first_column"), "grid")
	layout := tablelayout.NewLayout(tablelayout.NewTable(grid, nil))
	head, body := classicScopes(layout, 0), classicScopes(layout, 2)
	if head(0) != "col" || head(2) != "col" || body(0) != "row" || body(1) != "" {
		t.Fatalf("scopes %q %q %q %q", head(0), head(2), body(0), body(1))
	}
}

// cellOpener is a cell's start tag up to its scope, and the text of its first span.
var cellOpener = regexp.MustCompile(`<(t[hd]) (?:(scope="[a-z]+") )?[^>]*data-rich-cell="([^"]+)"[^>]*>`)

// The page's header cells (decision 131): booktabs' head row is <th scope="col">, a bold first
// column below it <th scope="row">, \multicolumn group headings and their label row <th
// scope="col">, and a table ruled throughout has none. RELATEX_UPDATE_GOLDEN=1 rewrites the golden.
func TestTableHeaderScopes(t *testing.T) {
	var outline strings.Builder
	for _, name := range []string{"bold_first_column", "multicolumn_groups", "rule_after_first_column", "rule_after_every_column"} {
		page := (&Paper{document: rec.Map{}}).TableGridSpan(scopeFixture(t, name))
		outline.WriteString("# " + name + "\n")
		for _, match := range cellOpener.FindAllStringSubmatch(page, -1) {
			outline.WriteString(match[3] + " " + match[1] + " " + match[2] + "\n")
		}
	}
	path := filepath.Join("testdata", "table-scopes.golden")
	if os.Getenv("RELATEX_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(outline.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	golden, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if outline.String() != string(golden) {
		t.Fatalf("header cells differ from %s:\n%s", path, outline.String())
	}
}
