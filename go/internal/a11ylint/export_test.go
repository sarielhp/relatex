package a11ylint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

const exportSource = `\documentclass{article}
\begin{document}
\begin{figure}
  \begin{tabular}{cc}
    a & b \\
    c & d
  \end{tabular}
  \caption{A red and blue chart of the results.}
\end{figure}

\begin{table}
  \begin{tabular}{ll}
    Name & Count \\
    Cats & 3 \\
    Dogs & 4
  \end{tabular}
  \caption{Pets in the street.}
  \label{tab:pets}
\end{table}
\end{document}
`

func textSpan(text string) rec.Map { return rec.Map{"type": "text", "text": text} }

// exportCell is one cell of a plain text grid.
func exportCell(row, column int64, text string) any {
	return rec.Map{"row": row, "column": column, "content": []any{textSpan(text)}}
}

func exportGrid(id string, cells ...any) rec.Map {
	return rec.Map{"type": "table_grid", "id": id, "grid": rec.Map{"row_count": int64(2), "column_count": int64(2), "cells": cells}}
}

// exportDocument is a figure holding a table and a table float, each with a caption in a paragraph
// inside a float_caption, as the export writes them.
func exportDocument() rec.Map {
	nodes := rec.Map{
		"fig": rec.Map{"id": "fig", "type": "figure_float", "children": []any{"figp", "figcap"}},
		"figp": rec.Map{"id": "figp", "type": "paragraph", "children": []any{}, "content": []any{
			exportGrid("g-fig", exportCell(0, 0, "a"), exportCell(0, 1, "b"), exportCell(1, 0, "c"), exportCell(1, 1, "d"))}},
		"figcap":  rec.Map{"id": "figcap", "type": "float_caption", "children": []any{"figcapp"}},
		"figcapp": rec.Map{"id": "figcapp", "type": "paragraph", "children": []any{}, "content": []any{textSpan("Figure 1.1: A red and blue chart of the results.")}},
		"tab":     rec.Map{"id": "tab", "type": "table_float", "children": []any{"tabp", "tabcap"}},
		"tabp": rec.Map{"id": "tabp", "type": "paragraph", "children": []any{}, "content": []any{
			exportGrid("g-tab", exportCell(0, 0, "Name"), exportCell(0, 1, "Count"), exportCell(1, 0, "Cats"), exportCell(1, 1, "3"))}},
		"tabcap":  rec.Map{"id": "tabcap", "type": "float_caption", "children": []any{"tabcapp"}},
		"tabcapp": rec.Map{"id": "tabcapp", "type": "paragraph", "children": []any{}, "content": []any{textSpan("Table 2.1: Pets in the street.")}},
	}
	return rec.Map{"nodes": nodes, "labels": rec.Map{"tab:pets": rec.Map{"target": "tabp", "number": "2.1"}}}
}

// exportWords are the source map's words of the two captions, each with its line.
func exportWords() rec.Map {
	words := []any{}
	add := func(line int64, texts ...string) {
		for _, text := range texts {
			words = append(words, rec.Map{"text": text, "origins": []any{rec.Map{"file": "main.tex", "line": line}}})
		}
	}
	add(8, "A", "red", "and", "blue", "chart", "of", "the", "results.")
	add(17, "Pets", "in", "the", "street.")
	return rec.Map{"words": words}
}

func exportReport() rec.Map {
	return rec.Map{
		"claim":      nil,
		"open_items": []any{rec.Map{"id": "table_headers", "count": int64(2)}, rec.Map{"id": "use_of_color", "count": int64(1)}},
		"structural": []any{
			rec.Map{"check": "table_headers", "status": "gap", "count": int64(2), "items": []any{
				rec.Map{"id": "g-fig", "cells": int64(4)}, rec.Map{"id": "g-tab", "cells": int64(4)}}},
			rec.Map{"check": "use_of_color", "status": "gap", "count": int64(1), "items": []any{
				rec.Map{"block": "figcapp", "colours": []any{"red", "blue"}, "text": "Figure 1.1: A red and blue chart"}}},
		},
	}
}

func exportPaper(t *testing.T, report rec.Map) Paper {
	t.Helper()
	paper := Paper{Files: oneFile(t, "main.tex", exportSource), SourceMap: exportWords()}
	return paper.WithExport(report, exportDocument())
}

// The export's open items become lint items at their source lines: each table at its \begin{tabular},
// found through its float's caption, with the fix for its float's kind.
func TestExportTablesAreLocatedAtTheirTabular(t *testing.T) {
	paper := exportPaper(t, exportReport())
	items := Lint(paper, Options{})
	byLine := map[int]Item{}
	for _, item := range items {
		if item.Kind == "table" {
			byLine[item.Line] = item
		}
	}
	figure, table := byLine[4], byLine[12]
	if !strings.Contains(figure.Fix, "in Figure 1.1") || !strings.Contains(figure.Fix, `relatex tables: 1.1#1 = "header_rows=1"`) {
		t.Errorf("figure table item: %+v", figure)
	}
	if !strings.Contains(table.Fix, "in Table 2.1") || !strings.Contains(table.Fix, `relatex tables: tab:pets = "header_rows=1"`) {
		t.Errorf("table float item: %+v", table)
	}
}

// A caption the source scan already lists is marked, not listed twice, and its fix says what closes
// the export's check.
func TestExportColourMergesWithTheSourceItem(t *testing.T) {
	items := Lint(exportPaper(t, exportReport()), Options{})
	colours := []Item{}
	for _, item := range items {
		if item.Kind == "colour" {
			colours = append(colours, item)
		}
	}
	if len(colours) != 1 || colours[0].Line != 8 {
		t.Fatalf("colour items: %+v", colours)
	}
	if !strings.Contains(colours[0].Message, "use_of_color check lists it") || !strings.Contains(colours[0].Fix, "closes when the caption names none") {
		t.Errorf("merged item: %+v", colours[0])
	}
}

// The summary names what is left: nothing but the listed items gives the claim; another open item or
// an item without a place does not.
func TestExportSummary(t *testing.T) {
	if got := ExportSummary(exportPaper(t, exportReport())); !strings.Contains(got, "earns the WCAG claim") || !strings.Contains(got, "3 table_headers and use_of_color") {
		t.Errorf("summary: %q", got)
	}
	report := exportReport()
	report["open_items"] = append(report["open_items"].([]any), rec.Map{"id": "color-contrast", "count": int64(2), "source": "axe"})
	if got := ExportSummary(exportPaper(t, report)); !strings.Contains(got, "stays withdrawn") || !strings.Contains(got, "color-contrast (2)") {
		t.Errorf("summary with an axe item: %q", got)
	}
	report = exportReport()
	report["claim"] = "WCAG 2.1 AA"
	if got := ExportSummary(exportPaper(t, report)); !strings.Contains(got, "holds the WCAG claim") {
		t.Errorf("summary of a claimed export: %q", got)
	}
	if got := ExportSummary(Paper{}); got != "" {
		t.Errorf("summary without an export: %q", got)
	}
}

// An item the product placed itself keeps its file and line; one the source map cannot place is
// listed in the first file and the summary says the claim is not reachable from the lint's items.
func TestExportItemsWithoutAPlace(t *testing.T) {
	paper := Paper{Files: oneFile(t, "main.tex", exportSource)}.WithExport(exportReport(), exportDocument())
	items := Lint(paper, Options{})
	found := false
	for _, item := range items {
		found = found || item.Kind == "table" && strings.Contains(item.Message, "not found in the source")
	}
	if !found {
		t.Errorf("no unplaced table item in %+v", items)
	}
	if got := ExportSummary(paper); !strings.Contains(got, "could not place") {
		t.Errorf("summary: %q", got)
	}
}

// WithExportFile reads accessibility.json from disk, and does nothing without one.
func TestWithExportFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "accessibility.json")
	if paper := (Paper{}).WithExportFile(path, exportDocument()); paper.Export != nil {
		t.Errorf("a missing report gave %+v", paper.Export)
	}
	if err := os.WriteFile(path, []byte(`{"claim": null, "open_items": [], "structural": [{"check": "table_headers", "status": "gap", "count": 1, "items": [{"id": "g-tab", "cells": 4, "file": "main.tex", "line": 17}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	paper := Paper{Files: oneFile(t, "main.tex", exportSource)}.WithExportFile(path, exportDocument())
	if paper.Export == nil || len(paper.Export.Gaps) != 1 || paper.Export.Gaps[0].Line != 17 {
		t.Fatalf("export read from file: %+v", paper.Export)
	}
	if _, err := ojson.ReadFile(path); err != nil {
		t.Fatal(err)
	}
}

// A source item the drop rules of decision 167's amendment B3 remove stays when the export's own check
// still lists it (decision 184): the lint merges the export's gaps after the rules.
func TestExportGapsSurviveTheDropRules(t *testing.T) {
	source := "\\documentclass[english]{article}\n\\begin{document}\n\\begin{figure}\n\\includegraphics[alt={A hall}]{hall}\n" +
		"\\caption{Follow the red carpet to the door.}\n\\end{figure}\n" +
		"\\begin{figure}\n\\begin{tikzpicture}[alt={Legend}]\\node{\\begin{tabular}{ll}\na & b \\\\\nc & d\n\\end{tabular}};\\end{tikzpicture}\n" +
		"\\caption{Legend.}\n\\end{figure}\n\\end{document}\n"
	paper := Paper{Files: oneFile(t, "main.tex", source)}
	if items := Lint(paper, Options{}); len(items) != 0 {
		t.Fatalf("source items without an export: %+v", items)
	}
	paper.Export = &Export{Gaps: []ExportGap{
		{Check: "use_of_color", File: "main.tex", Line: 5, Colours: []string{"red"}},
		{Check: "table_headers", File: "main.tex", Line: 12, Float: "figure_float", Table: 1, Of: 1, Cells: 4},
	}}
	kinds := []string{}
	for _, item := range Lint(paper, Options{}) {
		kinds = append(kinds, item.Kind+":"+itoa(item.Line))
	}
	if strings.Join(kinds, ",") != "colour:5,table:8" {
		t.Errorf("items with the export's gaps = %v, want colour:5 and table:8", kinds)
	}
}
