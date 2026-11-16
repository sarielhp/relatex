package accessibility

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

func fixtureDocument() rec.Map {
	heading := func(id string, level int64) rec.Map {
		return rec.Map{"id": id, "type": "heading", "children": []any{}, "semantics": rec.Map{"level": level},
			"content": []any{rec.Map{"type": "text", "text": id}}}
	}
	return rec.Map{
		"language":    rec.Map{"tag": "de", "source": "babel", "name": "ngerman"},
		"block_order": []any{"h1", "p1", "h3", "p2", "t1", "d1"},
		"nodes": rec.Map{
			"root": rec.Map{"id": "root", "type": "document", "children": []any{"h1", "p1", "h3", "p2", "f1", "t1"}},
			"h1":   heading("h1", 1),
			"h3":   heading("h3", 3),
			"p1": rec.Map{"id": "p1", "type": "paragraph", "children": []any{}, "content": []any{
				rec.Map{"type": "text", "text": "Hallo"},
				rec.Map{"type": "text", "text": "hello", "lang": "en"},
				rec.Map{"type": "text", "text": "???", "lang": "und", "id": "s1"},
				rec.Map{"type": "math", "node": "m1"}, rec.Map{"type": "math", "node": "m2"}}},
			"m1": rec.Map{"id": "m1", "type": "math", "mathml": `<math xmlns="http://www.w3.org/1998/Math/MathML"><mi>x</mi></math>`},
			"m2": rec.Map{"id": "m2", "type": "math", "mathml": `<math alttext="y" xmlns="http://www.w3.org/1998/Math/MathML"><mi>y</mi></math>`},
			"f1": rec.Map{"id": "f1", "type": "figure_float", "children": []any{"p2"}},
			"p2": rec.Map{"id": "p2", "type": "paragraph", "children": []any{}, "content": []any{
				rec.Map{"type": "image", "id": "i1", "asset": "a"},
				rec.Map{"type": "image", "id": "i2", "asset": "b", "alt": "A plot", "alt_provenance": "model"}}},
			"t1": rec.Map{"id": "t1", "type": "table_float", "children": []any{}, "content": []any{
				rec.Map{"type": "table_grid", "id": "g1", "grid": rec.Map{"cells": []any{
					rec.Map{"row": int64(0), "column": int64(0), "content": []any{rec.Map{"type": "image", "id": "i3", "asset": "c", "alt": "x"}}}}}},
				ruledTable()}},
			"d1": rec.Map{"id": "d1", "type": "display_math", "children": []any{}, "content": []any{
				rec.Map{"type": "image", "id": "i4", "asset": "d", "alt": "formula"}}},
		},
	}
}

// ruledTable is a cohort table whose first row a rule sets apart: it has column headers.
func ruledTable() rec.Map {
	tablemodel.Root = filepath.Join("..", "..", "..")
	file, err := os.Open(filepath.Join("..", "html", "testdata", "ruled-table.json"))
	if err != nil {
		panic(err)
	}
	defer file.Close()
	span, err := canonjson.Decode(file)
	if err != nil {
		panic(err)
	}
	return rec.AsMap(span)
}

func rowsByCheck(t *testing.T, html string) map[string]rec.Map {
	t.Helper()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := Structural(fixtureDocument(), nil, directory)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]rec.Map{}
	for _, value := range rows {
		row := rec.AsMap(value)
		result[row["check"].(string)] = row
	}
	return result
}

func TestStructuralRowsCountTheirGaps(t *testing.T) {
	rows := rowsByCheck(t, `<!DOCTYPE html><html lang="de"><body><h1 id="t">T</h1><h2 id="h1">A</h2><h4 id="h3">B</h4>`+
		`<math id='m1' xmlns='http://www.w3.org/1998/Math/MathML'><mi>x</mi></math><math id="m2" alttext="y"><mi>y</mi></math></body></html>`)
	want := map[string]int64{"language_declared": 0, "language_parts": 1, "image_alternatives": 1, "image_drafts_reviewed": 1, "formula_alternatives": 1,
		"table_headers": 1, "heading_levels": 1, "images_of_text": 2}
	for check, count := range want {
		row := rows[check]
		if row == nil || row["count"] != count {
			t.Errorf("%s: %v, want count %d", check, row, count)
			continue
		}
		if status := map[bool]string{true: "pass", false: "gap"}[count == 0]; row["status"] != status {
			t.Errorf("%s status %v", check, row["status"])
		}
	}
	heading := rec.AsMap(rows["heading_levels"]["items"].([]any)[0])
	if heading["id"] != "h3" || heading["level"] != int64(4) || heading["previous"] != int64(2) {
		t.Errorf("heading skip %v", heading)
	}
	kinds := []any{}
	for _, item := range rows["images_of_text"]["items"].([]any) {
		kinds = append(kinds, rec.AsMap(item)["kind"])
	}
	if len(kinds) != 2 || kinds[0] != "table" || kinds[1] != "math" {
		t.Errorf("images of text %v", kinds)
	}
}

// The landmark, group and visual rows read the page as written: a page from before decision 095
// has every gap; a page with the banner, named groups and the stylesheet's rules has none.
func TestLandmarkGroupAndVisualRows(t *testing.T) {
	old := rowsByCheck(t, `<html lang="de"><head><style>:root { --bg: #000000; }</style></head><body><a class="skip-link" href="#main">Skip</a>`+
		`<p role="status">Notice</p><main id="main"><div id="th" data-structure="theorem" class="theorem"></div></main></body></html>`)
	for check, count := range map[string]int64{"landmarks": 1, "named_groups": 1, "focus_visible": 4, "reduced_motion": 1, "forced_colors": 4} {
		if row := old[check]; row["count"] != count || row["status"] != "gap" {
			t.Errorf("%s: %v, want %d gaps", check, row, count)
		}
	}
	if old["non_text_contrast"]["status"] != "gap" {
		t.Errorf("an undeclared palette passed: %v", old["non_text_contrast"])
	}
	current := rowsByCheck(t, `<html lang="de"><body><a class="skip-link" href="#main">Skip</a><header><p role="status">Notice</p>`+
		`<p class="page-links"><a href="#contents">Contents</a></p></header><main id="main">`+
		`<div id="th" data-structure="theorem" class="theorem" role="group" aria-label="Theorem 1"></div></main><nav id="contents"></nav></body></html>`)
	for _, check := range []string{"landmarks", "named_groups"} {
		if current[check]["status"] != "pass" {
			t.Errorf("%s: %v", check, current[check])
		}
	}
	unlinked := rowsByCheck(t, `<body><a class="skip-link" href="#main">Skip</a><header><p role="status">N</p></header><main id="main"></main><nav id="contents"></nav>`)
	if item := rec.AsMap(unlinked["landmarks"]["items"].([]any)[0]); item["missing"] != "contents" {
		t.Errorf("banner without the contents link: %v", item)
	}
}

// A page without a title starts at h2: its first heading is no skip, a later jump is.
func TestHeadingLevelsStartAnywhere(t *testing.T) {
	items := headingItems([]byte(`<h2 id="a">A</h2><h3 id="b">B</h3><h2>C</h2><h4 id="d">D</h4>`))
	if len(items) != 1 || rec.AsMap(items[0])["id"] != "d" {
		t.Fatalf("heading skips %v", items)
	}
}

// A run-in title's role heading counts as a heading of its aria-level.
func TestRuninHeadingsCountAsHeadings(t *testing.T) {
	page := `<h2 id="a">A</h2><p><span class="runin-heading" role="heading" aria-level="3">T.</span> b</p>` +
		`<p><span class="runin-heading" role="heading" aria-level="5">U.</span> c</p>`
	items := headingItems([]byte(page))
	if len(items) != 1 || rec.AsMap(items[0])["level"] != int64(5) || rec.AsMap(items[0])["previous"] != int64(3) {
		t.Fatalf("heading skips %v", items)
	}
}

func TestPageLanguageMustMatchTheDocument(t *testing.T) {
	rows := rowsByCheck(t, `<html lang="en">`)
	items := rows["language_declared"]["items"].([]any)
	if len(items) != 1 || rec.AsMap(items[0])["problem"] != "page_language" {
		t.Fatalf("language items %v", items)
	}
}

func TestUndeclaredLanguageIsAGap(t *testing.T) {
	document := rec.Map{"language": rec.Map{"tag": "en", "source": "default"}, "block_order": []any{}, "nodes": rec.Map{}}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte(`<html lang="en">`), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := Structural(document, nil, directory)
	if err != nil {
		t.Fatal(err)
	}
	if row := rec.AsMap(rows[0]); row["status"] != "gap" || rec.AsMap(row["items"].([]any)[0])["problem"] != "undeclared" {
		t.Fatalf("language row %v", row)
	}
}

func TestReportKeepsTheClaimNullAndListsOpenItems(t *testing.T) {
	violation := func(id string, count int64) any { return rec.Map{"id": id, "count": count} }
	axe := rec.Map{"axe": "4.14.0", "tags": []any{}, "themes": rec.Map{
		"light": []any{violation("image-alt", 2)}, "dark": []any{violation("image-alt", 2), violation("color-contrast", 5)}}}
	structural := []any{rec.Map{"check": "table_headers", "criterion": "1.3.1", "status": "gap", "count": int64(3)},
		rec.Map{"check": "heading_levels", "criterion": "2.4.6", "status": "pass", "count": int64(0)}}
	report := Report("measured", "", axe, structural)
	if _, ok := report["claim"]; !ok || report["claim"] != nil || report["schema_version"] != SchemaVersion {
		t.Fatalf("report %v", report)
	}
	items := report["open_items"].([]any)
	if len(items) != 3 {
		t.Fatalf("open items %v", items)
	}
	first := rec.AsMap(items[0])
	if first["id"] != "image-alt" || first["count"] != int64(2) || len(first["themes"].([]any)) != 2 {
		t.Errorf("first item %v", first)
	}
	if third := rec.AsMap(items[2]); third["source"] != "structural" || third["id"] != "table_headers" {
		t.Errorf("structural item %v", third)
	}
	if Nodes(axe) != 9 {
		t.Errorf("nodes %d", Nodes(axe))
	}
	if unverified := Report("unverified", "no browser", nil, structural); unverified["reason"] != "no browser" || unverified["axe"] != nil {
		t.Errorf("unverified %v", unverified)
	}
}

func TestReportClaimsOnlyWhenEveryCheckPasses(t *testing.T) {
	clean := rec.Map{"axe": "4.14.0", "tags": []any{}, "themes": rec.Map{"light": []any{}, "dark": []any{}}}
	passing := []any{rec.Map{"check": "image_drafts_reviewed", "criterion": "1.1.1", "status": "pass", "count": int64(0)}}
	if claim := Report("measured", "", clean, passing)["claim"]; claim != htmlkit.WCAGClaim {
		t.Errorf("clean export claims %v", claim)
	}
	drafted := []any{rec.Map{"check": "image_drafts_reviewed", "criterion": "1.1.1", "status": "gap", "count": int64(1)}}
	lightOnly := rec.Map{"axe": "4.14.0", "tags": []any{}, "themes": rec.Map{"light": []any{}}}
	for name, report := range map[string]rec.Map{"draft": Report("measured", "", clean, drafted),
		"one theme": Report("measured", "", lightOnly, passing), "unmeasured": Report("unmeasured", "axe failed", nil, passing),
		"no structural checks": Report("measured", "", clean, []any{})} {
		if report["claim"] != nil {
			t.Errorf("%s: claims %v", name, report["claim"])
		}
	}
}
