package a11ylint

import "testing"

// A table hint keyed by a defined label or a caption number is quiet; an unreadable one, or one
// keyed by a label the paper does not define, is an item (decision 131).
func TestTableHintItems(t *testing.T) {
	paper := hintPaper(t, "%% relatex tables:\n"+
		"%%   tab:a = \"header_rows=1\"\n"+
		"%%   A.1 = \"header_columns=1\"\n"+
		"%%   tab:missing = \"header_rows=1\"\n"+
		"%%   tab:b = \"rows=1\"\n"+
		"\\begin{table}\\caption{x}\\label{tab:a}\\label{tab:b}\\end{table}\n")
	items := Lint(paper, Options{})
	if hintOf(t, items, `"tab:a"`) != nil || hintOf(t, items, `"A.1"`) != nil {
		t.Error("a valid table hint was flagged")
	}
	if hintOf(t, items, `table hint for "tab:missing", neither a label`) == nil {
		t.Error("no item for a key naming no label")
	}
	if hintOf(t, items, `table hint for "tab:b" cannot be read`) == nil {
		t.Error("no item for an unreadable table hint")
	}
	if hintOf(t, items, "unknown hint section") != nil {
		t.Error("tables is a known section")
	}
}
