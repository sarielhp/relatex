package tablelayout

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablelayout/tablefixture"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

type cell = tablefixture.Cell
type rule = tablefixture.Rule

// headerCase is a table made for one header rule of decisions 088, 111 and 131, and the header
// rows and columns the rules give it.
type headerCase struct {
	name          string
	rows          [][]cell
	rules         []rule
	author        rec.Map // author_header_rows / author_header_columns keys, as tablehints writes them
	headerRows    []int
	rowSource     string
	headerColumns []int
	columnSource  string
}

// plain is a table of regular text: a first row of labels, then body rows of a name and words.
func plain(body int, columns int) [][]cell { return filled(body, columns, "yes") }

// numeric is plain with numbers in the body (text over numbers).
func numeric(body int, columns int) [][]cell { return filled(body, columns, "12") }

func filled(body int, columns int, value string) [][]cell {
	rows := [][]cell{labels(columns)}
	for row := range body {
		cells := []cell{{Text: "name" + string(rune('a'+row))}}
		for range columns - 1 {
			cells = append(cells, cell{Text: value})
		}
		rows = append(rows, cells)
	}
	return rows
}

func labels(columns int) []cell {
	cells := []cell{}
	for column := range columns {
		cells = append(cells, cell{Text: "Label " + string(rune('A'+column))})
	}
	return cells
}

func horizontal(thickness int64, boundaries ...int) []rule {
	rules := []rule{}
	for _, at := range boundaries {
		rules = append(rules, rule{At: at, Thickness: thickness})
	}
	return rules
}

func vertical(thickness int64, boundaries ...int) []rule {
	rules := []rule{}
	for _, at := range boundaries {
		rules = append(rules, rule{Vertical: true, At: at, Thickness: thickness})
	}
	return rules
}

func emboldenRow(rows [][]cell, row int) [][]cell {
	for index := range rows[row] {
		rows[row][index].Bold = true
	}
	return rows
}

func emboldenFirstColumn(rows [][]cell, from int) [][]cell {
	for row := from; row < len(rows); row++ {
		rows[row][0].Bold = true
	}
	return rows
}

// corner empties the top-left cell.
func corner(rows [][]cell) [][]cell {
	rows[0][0] = cell{}
	return rows
}

func blankLabel(rows [][]cell) [][]cell {
	rows[0][2] = cell{}
	return rows
}

func unnamedRows(rows [][]cell) [][]cell {
	for row := 1; row < len(rows); row++ {
		rows[row][0] = cell{}
	}
	return rows
}

// numericFirstRow is a grid of numbers whose first row is data, not labels.
func numericFirstRow() [][]cell {
	rows := numeric(3, 3)
	rows[0] = []cell{{Text: "3"}, {Text: "4"}, {Text: "5"}}
	return rows
}

// withBody is a head row of labels over body rows of the given first cell and then the value, in two columns.
func twoColumns(head []cell, first string, value string) [][]cell {
	rows := [][]cell{head}
	for range 3 {
		rows = append(rows, []cell{{Text: first}, {Text: value}})
	}
	return rows
}

// formulaRow is a first row of formulas over a body of numbers (decision 186, amendment 1).
func formulaRow(body int, columns int) [][]cell {
	rows := numeric(body, columns)
	for column := range columns {
		rows[0][column] = cell{Math: true}
	}
	return rows
}

// mathAndWords is a first row of one formula and words over numbers: only the words are header cells.
func mathAndWords() [][]cell {
	rows := numeric(3, 3)
	rows[0][0] = cell{Math: true}
	return rows
}

// formulasOver is a table of formulas in the first row over the given body cells.
func formulasOver(body cell) [][]cell {
	rows := [][]cell{{{Math: true}, {Math: true}}}
	for range 3 {
		rows = append(rows, []cell{body, body})
	}
	return rows
}

func groupedHead() [][]cell {
	return [][]cell{
		{{Text: "Method", Rowspan: 2}, {Text: "Accuracy", Colspan: 2}, {Text: "Time", Rowspan: 2}},
		{{Text: "train"}, {Text: "test"}},
		{{Text: "ours"}, {Text: "91"}, {Text: "88"}, {Text: "3"}},
		{{Text: "base"}, {Text: "85"}, {Text: "80"}, {Text: "4"}},
	}
}

// labelledGroups is a head of two rows: a group heading over two columns and the row of their labels.
func labelledGroups(labelOne string, labelTwo string) [][]cell {
	return [][]cell{
		{{Text: "Method"}, {Text: "Accuracy", Colspan: 2}, {Text: "Time"}},
		{{Text: "Name"}, {Text: labelOne}, {Text: labelTwo}, {Text: "Unit"}},
		{{Text: "ours"}, {Text: "91"}, {Text: "88"}, {Text: "3"}},
		{{Text: "base"}, {Text: "85"}, {Text: "80"}, {Text: "4"}},
	}
}

// rowGroups is a table whose first column names groups of rows with a \multirow cell.
func rowGroups(first string) [][]cell {
	return [][]cell{
		labels(3),
		{{Text: first, Rowspan: 2}, {Text: "p"}, {Text: "12"}},
		{{Text: "q"}, {Text: "12"}},
		{{Text: "Group B", Rowspan: 2}, {Text: "r"}, {Text: "12"}},
		{{Text: "s"}, {Text: "12"}},
	}
}

func headerCases() []headerCase {
	booktabs := append(horizontal(tablefixture.Heavy, 0, 4), horizontal(tablefixture.Light, 1)...)
	everyMidrule := append(horizontal(tablefixture.Heavy, 0, 4), horizontal(tablefixture.Light, 1, 2, 3)...)
	twoRowHead := append(horizontal(tablefixture.Heavy, 0, 5), horizontal(tablefixture.Light, 2)...)
	deepMidrule := append(horizontal(tablefixture.Heavy, 0, 6), horizontal(tablefixture.Light, 4)...)
	noBodyColumn := groupedHead()
	noBodyColumn[1][1].Text = ""
	bodyGroup := append(groupedHead(), []cell{{Text: "sum"}, {Text: "176", Colspan: 2}, {Text: "7"}})
	title := plain(3, 3)
	title[0] = []cell{{Text: "Results", Colspan: 3}}
	return []headerCase{
		{name: "booktabs_head", rows: plain(3, 3), rules: booktabs, headerRows: []int{0}, rowSource: HeaderBooktabs},
		{name: "booktabs_every_midrule", rows: plain(3, 3), rules: everyMidrule, headerRows: []int{0}, rowSource: HeaderBooktabs},
		{name: "booktabs_two_row_head", rows: append(plain(3, 3), labels(3)), rules: twoRowHead, headerRows: []int{0, 1}, rowSource: HeaderBooktabs},
		{name: "booktabs_midrule_too_deep", rows: plain(5, 3), rules: deepMidrule},
		{name: "hline_every_row", rows: plain(3, 3), rules: horizontal(tablefixture.Array, 0, 1, 2, 3, 4)},
		{name: "hline_under_first_row", rows: plain(3, 3), rules: horizontal(tablefixture.Array, 0, 1, 4), headerRows: []int{0}, rowSource: HeaderRule},
		{name: "unruled_plain", rows: plain(3, 3)},
		{name: "numeric_body_under_words", rows: numeric(3, 3), headerRows: []int{0}, rowSource: HeaderNumeric,
			headerColumns: []int{0}, columnSource: HeaderFirstColumnInferred},
		{name: "numeric_body_under_every_hline", rows: numeric(3, 3), rules: horizontal(tablefixture.Array, 0, 1, 2, 3, 4),
			headerRows: []int{0}, rowSource: HeaderNumeric,
			headerColumns: []int{0}, columnSource: HeaderFirstColumnInferred},
		{name: "numeric_first_row_is_data", rows: numericFirstRow()},
		{name: "corner_over_labelled_columns", rows: corner(plain(3, 3)), headerRows: []int{0}, rowSource: HeaderCorner},
		{name: "corner_with_a_blank_label", rows: blankLabel(corner(plain(3, 3)))},
		{name: "corner_without_row_labels", rows: unnamedRows(corner(plain(3, 3)))},
		{name: "booktabs_group_head_label_row", rows: labelledGroups("train", "test"),
			rules: append(horizontal(tablefixture.Heavy, 0, 4), horizontal(tablefixture.Light, 1, 2)...), headerRows: []int{0, 1}, rowSource: HeaderBooktabs,
			headerColumns: []int{0}, columnSource: HeaderFirstColumnInferred},
		{name: "booktabs_group_head_numeric_labels", rows: labelledGroups("96", "192"),
			rules: append(horizontal(tablefixture.Heavy, 0, 4), horizontal(tablefixture.Light, 1, 2)...), headerRows: []int{0}, rowSource: HeaderBooktabs,
			headerColumns: []int{0}, columnSource: HeaderFirstColumnInferred},
		{name: "ruled_group_head_label_row", rows: labelledGroups("train", "test"), rules: horizontal(tablefixture.Array, 0, 1, 4),
			headerRows: []int{0, 1}, rowSource: HeaderRule,
			headerColumns: []int{0}, columnSource: HeaderFirstColumnInferred},
		{name: "multirow_first_column", rows: rowGroups("Group A"), rules: append(horizontal(tablefixture.Heavy, 0, 5), horizontal(tablefixture.Light, 1)...),
			headerRows: []int{0}, rowSource: HeaderBooktabs, headerColumns: []int{0}, columnSource: HeaderFirstColumnSpan},
		{name: "multirow_first_column_numbers", rows: rowGroups("12"), rules: append(horizontal(tablefixture.Heavy, 0, 5), horizontal(tablefixture.Light, 1)...),
			headerRows: []int{0}, rowSource: HeaderBooktabs},
		{name: "inferred_citations_over_numbers", rows: twoColumns(labels(2), "[5]", "80.40"), headerRows: []int{0}, rowSource: HeaderInferred,
			headerColumns: []int{0}, columnSource: HeaderFirstColumnInferred},
		{name: "inferred_formulas_over_numbers", rows: formulaRow(3, 3), headerRows: []int{0}, rowSource: HeaderInferred,
			headerColumns: []int{0}, columnSource: HeaderFirstColumnInferred},
		{name: "inferred_formula_and_words_over_numbers", rows: mathAndWords(), headerRows: []int{0}, rowSource: HeaderInferred,
			headerColumns: []int{0}, columnSource: HeaderFirstColumnInferred},
		{name: "inferred_words_over_words", rows: plain(3, 3)},
		{name: "inferred_glossary", rows: twoColumns(labels(2), "p(u)", "spline value")},
		{name: "inferred_definition_pairs", rows: twoColumns([]cell{{Text: "senc/2"}, {Text: "encrypt a message"}}, "sdec/2", "decrypt a message")},
		{name: "inferred_formulas_over_formulas", rows: formulasOver(cell{Math: true})},
		{name: "inferred_formulas_over_words", rows: formulasOver(cell{Text: "same"})},
		{name: "inferred_layout_of_equal_cells", rows: [][]cell{{{Math: true}, {Math: true}}, {{Math: true}, {Math: true}}}},
		{name: "inferred_numbers_in_the_first_row", rows: twoColumns([]cell{{Text: "3"}, {Text: "4"}}, "name", "12")},
		{name: "inferred_column_beside_words", rows: [][]cell{labels(3), {{Text: "a"}, {Text: "12"}, {Text: "yes"}}, {{Text: "b"}, {Text: "13"}, {Text: "yes"}},
			{{Text: "c"}, {Text: "14"}, {Text: "yes"}}}},
		{name: "inferred_column_of_numbers", rows: [][]cell{labels(3), {{Text: "1"}, {Text: "12"}, {Text: "2"}}, {{Text: "2"}, {Text: "13"}, {Text: "3"}},
			{{Text: "3"}, {Text: "14"}, {Text: "4"}}}, headerRows: []int{0}, rowSource: HeaderNumeric},
		{name: "multicolumn_groups", rows: groupedHead(), headerRows: []int{0, 1}, rowSource: HeaderSpan,
			headerColumns: []int{0}, columnSource: HeaderFirstColumnInferred},
		{name: "multicolumn_without_labels", rows: noBodyColumn},
		{name: "multicolumn_in_body", rows: bodyGroup},
		{name: "multicolumn_title_row", rows: title},
		{name: "bold_first_row", rows: emboldenRow(plain(3, 3), 0), headerRows: []int{0}, rowSource: HeaderBold},
		{name: "bold_first_column", rows: emboldenFirstColumn(plain(3, 3), 1), rules: booktabs, headerRows: []int{0}, rowSource: HeaderBooktabs,
			headerColumns: []int{0}, columnSource: HeaderFirstColumnBold},
		{name: "bold_first_column_under_plain_labels", rows: emboldenFirstColumn(plain(3, 3), 1)},
		{name: "bold_first_row_and_column", rows: emboldenFirstColumn(emboldenRow(plain(3, 3), 0), 1), headerRows: []int{0}, rowSource: HeaderBold,
			headerColumns: []int{0}, columnSource: HeaderFirstColumnBold},
		{name: "bold_everywhere", rows: emboldenRow(emboldenRow(emboldenRow(emboldenRow(plain(3, 3), 0), 1), 2), 3)},
		{name: "bold_first_column_and_more", rows: func() [][]cell {
			rows := emboldenFirstColumn(plain(3, 3), 1)
			rows[1][1].Bold, rows[2][1].Bold, rows[3][1].Bold = true, true, true
			return rows
		}()},
		{name: "rule_after_first_column", rows: plain(3, 3), rules: vertical(tablefixture.Array, 1), headerColumns: []int{0}, columnSource: HeaderFirstColumnRule},
		{name: "rule_after_first_column_framed", rows: plain(3, 3), rules: vertical(tablefixture.Array, 0, 1, 3), headerColumns: []int{0},
			columnSource: HeaderFirstColumnRule},
		{name: "rule_after_every_column", rows: plain(3, 3), rules: vertical(tablefixture.Array, 0, 1, 2, 3)},
		{name: "double_rule_after_first_column", rows: plain(3, 3),
			rules: append(vertical(tablefixture.Array, 0, 2, 3), rule{Vertical: true, At: 1, Thickness: tablefixture.Array},
				rule{Vertical: true, At: 1, Thickness: tablefixture.Array * 2}), headerColumns: []int{0}, columnSource: HeaderFirstColumnRule},
		{name: "two_columns_ruled_between", rows: plain(3, 2), rules: vertical(tablefixture.Array, 1), headerColumns: []int{0},
			columnSource: HeaderFirstColumnRule},
		{name: "two_columns_framed", rows: plain(3, 2), rules: vertical(tablefixture.Array, 0, 1, 2)},
		{name: "author_rows_and_no_columns", rows: emboldenFirstColumn(plain(3, 3), 1),
			author:     rec.Map{AuthorHeaderRows: []any{int64(0), int64(1)}, AuthorHeaderColumns: []any{}},
			headerRows: []int{0, 1}, rowSource: HeaderAuthor},
		{name: "author_no_rows_over_booktabs", rows: plain(3, 3), rules: booktabs, author: rec.Map{AuthorHeaderRows: []any{}}},
		{name: "author_columns", rows: plain(3, 3), author: rec.Map{AuthorHeaderColumns: []any{int64(0)}},
			headerColumns: []int{0}, columnSource: HeaderAuthor},
	}
}

// fixtureDirectory holds each case's grid and expected reading for the acceptance's own reading
// (test/table_header_rules_test.rb). RELATEX_UPDATE_FIXTURES=1 rewrites them.
var fixtureDirectory = filepath.Join("..", "..", "..", "test", "fixtures", "table_headers")

func TestHeaderRules(t *testing.T) {
	tablemodel.Root = filepath.Join("..", "..", "..")
	for _, test := range headerCases() {
		t.Run(test.name, func(t *testing.T) {
			grid := tablefixture.Grid(test.rows, test.rules)
			for key, value := range test.author {
				grid[key] = value
			}
			layout := NewLayout(NewTable(grid, nil))
			rows, columns := layout.HeaderRows(), layout.HeaderColumns()
			if !slices.Equal(rows, orEmpty(test.headerRows)) || layout.HeaderSource() != test.rowSource {
				t.Errorf("header rows %v (%q), want %v (%q)", rows, layout.HeaderSource(), test.headerRows, test.rowSource)
			}
			if !slices.Equal(columns, orEmpty(test.headerColumns)) || layout.HeaderColumnSource() != test.columnSource {
				t.Errorf("header columns %v (%q), want %v (%q)", columns, layout.HeaderColumnSource(), test.headerColumns, test.columnSource)
			}
			checkFixture(t, test.name, grid, rec.Map{"rows": ints(rows), "row_source": test.rowSource, "columns": ints(columns),
				"column_source": test.columnSource, "scopes": scopes(layout, grid)})
		})
	}
}

func orEmpty(values []int) []int {
	if values == nil {
		return []int{}
	}
	return values
}

func ints(values []int) []any {
	result := []any{}
	for _, value := range values {
		result = append(result, int64(value))
	}
	return result
}

func scopes(layout *Layout, grid rec.Map) rec.Map {
	result := rec.Map{}
	for _, value := range rec.FetchList(grid, "cells") {
		cell := rec.AsMap(value)
		if scope := layout.CellScope(cell); scope != "" {
			result[rec.FetchString(cell, "id")] = scope
		}
	}
	return result
}

// checkFixture compares the case's stored fixture with this reading, or rewrites it.
func checkFixture(t *testing.T, name string, grid, expect rec.Map) {
	t.Helper()
	path := filepath.Join(fixtureDirectory, name+".json")
	want, err := canonjson.Marshal(rec.Map{"grid": grid, "expect": expect})
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("RELATEX_UPDATE_FIXTURES") == "1" {
		if made := os.MkdirAll(fixtureDirectory, 0o755); made != nil {
			t.Fatal(made)
		}
		if written := os.WriteFile(path, append(want, '\n'), 0o644); written != nil {
			t.Fatal(written)
		}
		return
	}
	stored, err := os.ReadFile(path)
	if err != nil || string(stored) != string(want)+"\n" {
		t.Fatalf("%s is stale or missing (RELATEX_UPDATE_FIXTURES=1 rewrites it): %v", path, err)
	}
}

// Every fixture on disk is a case of the table (none left behind by a renamed case).
func TestHeaderFixturesAreCases(t *testing.T) {
	names := []string{}
	for _, test := range headerCases() {
		names = append(names, test.name+".json")
	}
	entries, err := os.ReadDir(fixtureDirectory)
	if err != nil {
		t.Fatal(err)
	}
	stored := []string{}
	for _, entry := range entries {
		stored = append(stored, entry.Name())
	}
	sort.Strings(names)
	if !slices.Equal(stored, names) {
		t.Fatalf("fixtures %v, cases %v", stored, names)
	}
}

// A cell of the header rows is a column header, one of the header columns below them a row header,
// and the corner a column header; a blank first cell is no header.
func TestCellScopes(t *testing.T) {
	tablemodel.Root = filepath.Join("..", "..", "..")
	rows := emboldenFirstColumn(emboldenRow(plain(2, 3), 0), 1)
	rows[0][0] = cell{}
	layout := NewLayout(NewTable(tablefixture.Grid(rows, nil), nil))
	got := scopes(layout, layout.Model.Data)
	want := rec.Map{"c0-1": ScopeColumn, "c0-2": ScopeColumn, "c1-0": ScopeRow, "c2-0": ScopeRow}
	if !rec.Equal(got, want) {
		t.Fatalf("scopes %v, want %v", got, want)
	}
	if source := HeaderSourceOf(layout.Model.Data, nil); source != HeaderBold {
		t.Fatalf("source %q", source)
	}
	scoped := HeaderScopes(layout.Model.Data, nil)
	if len(scoped) != 4 || scoped["c1-0"] != ScopeRow {
		t.Fatalf("HeaderScopes %v", scoped)
	}
}
