package authorhints

import "testing"

// A table hint is read from a comment and from hints.json (decision 131); hints.json wins, and the
// two are compared in one canonical form.
func TestTableHints(t *testing.T) {
	hints := parse(t, []Source{{Path: "main.tex", Text: "%% relatex tables: tab:a = \"header_rows=2, header_columns=1\"\n" +
		"%% relatex tables:\n%%   3 = \"header_columns=0\"\n"}},
		`{"tables": {"tab:a": {"header_columns": 1, "header_rows": 1}, "A.1": {"header_rows": 0}}}`)
	hint, ok := hints.Table("tab:a")
	if !ok || hint.Rows == nil || *hint.Rows != 1 || hint.Columns == nil || *hint.Columns != 1 {
		t.Fatalf("tab:a = %+v %v", hint, ok)
	}
	if len(hints.Conflicts) != 1 || hints.Conflicts[0].Loser.Value != "header_rows=2, header_columns=1" ||
		hints.Conflicts[0].Winner.Value != "header_columns=1 header_rows=1" {
		t.Fatalf("conflicts = %+v", hints.Conflicts)
	}
	if hint, ok := hints.Table("3"); !ok || hint.Rows != nil || *hint.Columns != 0 {
		t.Fatalf("3 = %+v", hint)
	}
	if hint, ok := hints.Table("A.1"); !ok || *hint.Rows != 0 || hint.Columns != nil {
		t.Fatalf("A.1 = %+v", hint)
	}
	if len(hints.Unknown) != 0 {
		t.Fatalf("tables is a known section: %+v", hints.Unknown)
	}
}

func TestBadTableHints(t *testing.T) {
	for _, value := range []string{"", "header_rows", "header_rows=-1", "rows=1", "header_rows=x"} {
		if _, err := ParseTableHint(value); err == nil {
			t.Errorf("%q was accepted", value)
		}
	}
	hints := parse(t, []Source{{Path: "main.tex", Text: "%% relatex tables: tab:a = \"rows=1\"\n"}}, "")
	if _, ok := hints.Table("tab:a"); ok {
		t.Fatal("an invalid comment hint was applied")
	}
	for _, json := range []string{`{"tables": {"tab:a": 1}}`, `{"tables": {"tab:a": {"rows": 1}}}`,
		`{"tables": {"tab:a": {"header_rows": -1}}}`, `{"tables": {"tab:a": {}}}`} {
		if _, err := Parse(nil, []byte(json), contract(t)); err == nil {
			t.Errorf("%s was accepted", json)
		}
	}
}
