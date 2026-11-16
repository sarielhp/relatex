package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func referencePaper(language string) *Paper {
	nodes := rec.Map{
		"math":   rec.Map{"type": "display_math"},
		"row":    rec.Map{"type": "math_row"},
		"figure": rec.Map{"type": "figure_float"},
		"table":  rec.Map{"type": "table_float"},
		"th":     rec.Map{"type": "theorem", "semantics": rec.Map{"title": "Lemma"}, "children": []any{"thp"}},
		"thp":    rec.Map{"type": "paragraph", "children": []any{}},
		"thm":    rec.Map{"type": "theorem", "semantics": rec.Map{"title": "Theorem"}, "children": []any{"thmp"}},
		"thmp":   rec.Map{"type": "paragraph", "children": []any{}},
		"blank":  rec.Map{"type": "theorem", "semantics": rec.Map{"title": ""}, "children": []any{"blankp"}},
		"blankp": rec.Map{"type": "paragraph", "children": []any{}},
		"head":   rec.Map{"type": "heading"},
		"item":   rec.Map{"type": "list_item", "children": []any{"itemp"}},
		"itemp":  rec.Map{"type": "paragraph", "children": []any{}},
		"orphan": rec.Map{"type": "paragraph", "children": []any{}},
	}
	document := rec.Map{}
	if language != "" {
		document["language"] = rec.Map{"tag": language}
	}
	return &Paper{document: document, nodes: nodes}
}

func referenceSpan(text, target string) rec.Map {
	return rec.Map{"type": "reference", "label": "l", "text": text, "target": target}
}

// A cross-reference is named by its target's kind and its text, unless the sentence says the
// word already or the link's text holds it (plan accessibility §7.2.2, decision 111).
func TestReferenceNames(t *testing.T) {
	rows := []struct {
		language, target, text, previous, want string
	}{
		{"en", "math", "16", "see ", "Equation 16"},
		{"en", "math", "16", "Eq. (", ""},
		{"en", "math", "16", "Equations ", ""},
		{"en", "math", "16", "", "Equation 16"},
		{"en", "row", "5", "", "Equation 5"},
		{"en", "figure", "2", "", "Figure 2"},
		{"en", "table", "3", "", "Table 3"},
		{"en", "thp", "3.2", "", "Lemma 3.2"},
		{"en", "thp", "3.2", "by ", "Lemma 3.2"},
		{"en", "thp", "3.2", "Lemma ", ""},
		{"en", "thp", "3.2", "porosity (Lemma", ""},
		{"en", "thmp", "Theorem 3.2", "", ""},
		{"en", "thp", "3.2", "§", ""},
		{"en", "head", "2", "", "Section 2"},
		{"en", "item", "1", "", "Item 1"},
		{"en", "orphan", "1", "", ""},
		{"en", "blankp", "1", "", ""},
		{"de", "math", "16", "see ", ""},
		{"de", "head", "2", "", ""},
		{"de", "thp", "3.2", "", "Lemma 3.2"},
	}
	for _, row := range rows {
		got := referencePaper(row.language).referenceName(referenceSpan(row.text, row.target), row.previous)
		if got != row.want {
			t.Errorf("%s/%s after %q: %q, want %q", row.language, row.target, row.previous, got, row.want)
		}
	}
}

// A named reference keeps its text and address and gains only the name; an unnamed one keeps its
// markup as it was.
func TestReferenceMarkup(t *testing.T) {
	paper := referencePaper("en")
	named := paper.baseSpan(referenceSpan("16", "math"))
	if !strings.Contains(named, `aria-label="Equation 16"`) || !strings.Contains(named, ">16</a>") {
		t.Errorf("named reference %s", named)
	}
	paper.previous = "Eq. ("
	plain := paper.baseSpan(referenceSpan("16", "math"))
	want := `<a data-kind="reference" data-label="l" href="#math">16</a>`
	if plain != want {
		t.Errorf("unnamed reference %q, want %q", plain, want)
	}
}
