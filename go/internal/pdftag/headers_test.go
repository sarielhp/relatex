package pdftag

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/tablemodel"
)

// headerFixture is a table of test/fixtures/table_headers (tablelayout's header rule cases) as the
// model reads it from document.json.
func headerFixture(t *testing.T, name string) *tableGrid {
	t.Helper()
	tablemodel.Root = filepath.Join("..", "..", "..")
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "test", "fixtures", "table_headers", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Grid *tableGrid `json:"grid"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture.Grid
}

// tableOutline is a table's structure as rows of cells, each its role and attributes.
func tableOutline(table *selem) string {
	var builder strings.Builder
	for _, row := range table.kids {
		cells := []string{}
		for _, cell := range row.kids {
			attributes := cellAttributes(cell)
			keys := []string{}
			for key := range attributes {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			parts := []string{cell.role}
			for _, key := range keys {
				parts = append(parts, fmt.Sprintf("%s=%v", key, attributes[key]))
			}
			cells = append(cells, strings.Join(parts, " "))
		}
		builder.WriteString("TR: " + strings.Join(cells, " | ") + "\n")
	}
	return builder.String()
}

// The tagged PDF's TH cells and their /Scope follow the page's header cells (decision 131): a
// booktabs head row of column headers and a bold first column of row headers; a table without a
// marked header has TD cells only. RELATEX_UPDATE_GOLDEN=1 rewrites the golden.
func TestTableHeaderTags(t *testing.T) {
	var outline strings.Builder
	for _, name := range []string{"bold_first_column", "multicolumn_groups", "rule_after_every_column"} {
		table := newBuilder(&document{}).table(headerFixture(t, name))
		outline.WriteString("# " + name + "\n" + tableOutline(table))
	}
	path := filepath.Join("testdata", "table-headers.golden")
	if os.Getenv("RELATEX_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(outline.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	golden, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if outline.String() != string(golden) {
		t.Fatalf("table tags differ from %s:\n%s", path, outline.String())
	}
}
