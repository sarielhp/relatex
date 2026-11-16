package authormath

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sarielhp/relatex/internal/authortex"
	"github.com/sarielhp/relatex/internal/mathsource"
	"github.com/sarielhp/relatex/internal/rec"
)

func TestScrubReplacesMaximalSubparts(t *testing.T) {
	// Counts of characters Ruby's String#scrub leaves, as Ruby 3.3 prints them.
	cases := map[string]int{"\xE2\x82": 1, "\xff\xfe": 2, "a\xE2\x82b": 3, "\xF0\x9F\x98": 1, "\xED\xA0\x80": 3, "\xC0\xAF": 2, "é": 1}
	for input, want := range cases {
		if got := len([]rune(Scrub([]byte(input)))); got != want {
			t.Errorf("Scrub(%q) has %d characters, want %d", input, got, want)
		}
	}
}

func TestShippedDropsLabelsCommentsAndBlankLines(t *testing.T) {
	got := authortex.Shipped("a \\label{eq:1} % note\r\n\n  \t\nb \\% c\\\\% d\n")
	if want := "a  \nb \\% c\\\\"; got != want {
		t.Fatalf("Shipped = %q, want %q", got, want)
	}
}

func TestRowsSplitOutsideBracesAndEnvironments(t *testing.T) {
	got := mathsource.Rows("a &= b \\\\[2pt] c \\begin{cases} x \\\\ y \\end{cases} \\\\ {d \\\\ e} % f \\\\\n")
	want := []string{"a &= b", "c \\begin{cases} x \\\\ y \\end{cases}", "{d \\\\ e}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Rows = %q, want %q", got, want)
	}
}

func TestForbiddenUsesUnicodeWordBoundaries(t *testing.T) {
	cases := map[string]bool{`\else{`: true, `\elseé`: false, `\else1`: false, `\ifx a`: true, `\@foo`: true, `\cite{a}`: true,
		`\ordinary`: false, `\alpha`: false}
	for body, want := range cases {
		if got := forbidden(body); got != want {
			t.Errorf("forbidden(%q) = %v, want %v", body, got, want)
		}
	}
}

func TestDifferenceFoldsAnchorsAndIdentifiers(t *testing.T) {
	original := `<math><mrow><mi>R</mi><mi>M</mi><mi>S</mi><mtext></mtext></mrow><mtext>(3)</mtext></math>`
	typeset := `<math><mi>RMS</mi><mtext>(12)</mtext></math>`
	if got := difference(original, typeset); got != "" {
		t.Fatalf("difference = %q, want none", got)
	}
	if got := difference(`<math><mi>x</mi></math>`, `<math><mi>y</mi></math>`); got != `/math[1]/mi: "x" vs "y"` {
		t.Fatalf("difference = %q", got)
	}
}

// TestSourceTextMatchesRuby replays scratchpad/sourcetext_dump.rb's spans; it runs only
// when RELATEX_SOURCETEXT_DUMP names that dump.
func TestSourceTextMatchesRuby(t *testing.T) {
	directory := os.Getenv("RELATEX_SOURCETEXT_DUMP")
	if directory == "" {
		t.Skip("RELATEX_SOURCETEXT_DUMP not set")
	}
	paths, _ := filepath.Glob(filepath.Join(directory, "*.json"))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var dump struct {
			Path  string           `json:"path"`
			Spans []map[string]any `json:"spans"`
		}
		if decodeErr := json.Unmarshal(data, &dump); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		source, err := os.ReadFile(dump.Path)
		if err != nil {
			t.Fatal(err)
		}
		spans := mathsource.NewSourceText(Scrub(source), []string{"eps"}).Spans
		if len(spans) != len(dump.Spans) {
			t.Errorf("%s: %d spans, Ruby %d", dump.Path, len(spans), len(dump.Spans))
			continue
		}
		for index, span := range spans {
			compareSpan(t, dump.Path, index, span, dump.Spans[index])
		}
	}
}

func compareSpan(t *testing.T, path string, index int, span *mathsource.Span, ruby map[string]any) {
	t.Helper()
	got := map[string]any{"form": span.Form, "environment": environmentOf(span), "line": float64(span.Line),
		"end_line": float64(span.EndLine), "body": span.Body, "start": float64(span.Start), "stop": float64(span.Stop)}
	var rows any
	if span.Form == "environment" {
		list := []any{}
		for _, row := range mathsource.Rows(span.Body) {
			list = append(list, row)
		}
		rows = list
	}
	got["rows"] = rows
	if !reflect.DeepEqual(got, ruby) {
		t.Errorf("%s span %d: go %v, ruby %v", path, index, rec.Map(got), rec.Map(ruby))
	}
}
