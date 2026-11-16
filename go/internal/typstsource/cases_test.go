package typstsource

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// TestCases compares the helpers, the test fixtures' projects and random MathML with
// Ruby's (RELATEX_TYPST_CASES: the directory typst_cases.rb and typst_math_cases.rb wrote).
func TestCases(t *testing.T) {
	directory := os.Getenv("RELATEX_TYPST_CASES")
	if directory == "" {
		t.Skip("RELATEX_TYPST_CASES is not set")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	Root, tablemodel.Root, fontmodel.Root, paperdoc.Root = root, root, root, root
	t.Run("helpers", func(t *testing.T) { compareHelpers(t, filepath.Join(directory, "helpers.json")) })
	t.Run("math", func(t *testing.T) { compareMath(t, filepath.Join(directory, "math.json")) })
	fixtures, _ := filepath.Glob(filepath.Join(directory, "fixtures", "*"))
	for _, fixture := range fixtures {
		t.Run(filepath.Base(fixture), func(t *testing.T) { compareFixture(t, root, fixture) })
	}
}

func helperResult(name, input string) (result any) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = rec.Map{"error": fmt.Sprint(recovered)}
		}
	}()
	switch name {
	case "string":
		return typstString(input)
	case "escape":
		return escape(input)
	case "escape_nobrackets":
		return escapeBrackets(input, false)
	case "protect_start":
		return protectStart(input)
	case "label_name":
		return labelName(input)
	case "comparable_number":
		return comparableNumber(input)
	case "parse":
		components, ok := parseNumber(input)
		if !ok {
			return nil
		}
		parts := []any{}
		for _, part := range components {
			parts = append(parts, []any{componentValue(part.value), string(part.kind)})
		}
		return parts
	case "component":
		part, ok := parseComponent(input)
		if !ok {
			return nil
		}
		return []any{componentValue(part.value), string(part.kind)}
	}
	return nil
}

func componentValue(number value) any {
	if !number.valid {
		return nil
	}
	return number.number.Int64()
}

func compareHelpers(t *testing.T, path string) {
	cases := decodeFile(t, path).(rec.Map)
	failures := 0
	for name, value := range cases {
		for _, item := range rec.AsList(value) {
			pair := rec.AsList(item)
			var got any
			switch name {
			case "round":
				got = roundText(rec.AsInt(pair[0]))
			case "length":
				got = length(rec.AsString(pair[0]))
			default:
				got = helperResult(name, rec.AsString(pair[0]))
			}
			if _, isError := rec.AsList(item)[1].(rec.Map); isError {
				if _, failed := got.(rec.Map); failed {
					continue
				}
			}
			if !sameJSON(pair[1], got) {
				failures++
				if failures < 30 {
					t.Errorf("%s(%q): ruby %v, go %v", name, pair[0], pair[1], got)
				}
			}
		}
	}
	t.Logf("%d helper differences", failures)
}

func roundText(sp int64) string {
	bytes, _ := canonjson.Marshal(rubyRound(geometry.SPToBP(float64(sp)), 2))
	return string(bytes)
}

func sameJSON(left, right any) bool {
	a, errA := canonjson.Marshal(left)
	b, errB := canonjson.Marshal(right)
	return errA == nil && errB == nil && bytes.Equal(a, b) || rec.Equal(left, right)
}

func mathResult(run func() any) (result any) {
	defer func() {
		if recovered := recover(); recovered != nil {
			failure, ok := recovered.(rec.Failure)
			if !ok {
				panic(recovered)
			}
			result = rec.Map{"failure": failure.Message}
		}
	}()
	return run()
}

func compareMath(t *testing.T, path string) {
	cases := rec.AsList(decodeFile(t, path))
	failures := 0
	for _, item := range cases {
		entry := rec.AsList(item)
		converted := mathResult(func() any { return (&mathWriter{}).convert(rec.AsString(entry[0])) })
		rows := mathResult(func() any {
			found, ok := (&mathWriter{}).rows(rec.AsString(entry[2]))
			if !ok {
				return nil
			}
			result := []any{}
			for _, row := range found {
				cells := []any{}
				for _, cell := range row {
					cells = append(cells, cell)
				}
				result = append(result, cells)
			}
			return result
		})
		if !sameJSON(entry[1], converted) || !sameJSON(entry[3], rows) {
			failures++
			if failures < 20 {
				t.Errorf("%s:\n ruby %v | %v\n go   %v | %v", entry[0], entry[1], entry[3], converted, rows)
			}
		}
	}
	t.Logf("%d of %d math cases differ", failures, len(cases))
}

func compareFixture(t *testing.T, root, reference string) {
	name := filepath.Base(reference)
	document := decodeFile(t, filepath.Join(root, "test", "fixtures", "typst", name+".json")).(rec.Map)
	out := t.TempDir()
	result := mathResult(func() any {
		report, losses := Write(document, out)
		return rec.Map{"writer": report, "losses": losses}
	})
	expected := decodeFile(t, filepath.Join(reference, "result.json"))
	if !sameJSON(expected, result) {
		t.Errorf("result differs: ruby %v, go %v", expected, result)
	}
	compareFiles(t, reference, out)
}

// TestMutations compares mutated documents (RELATEX_TYPST_MUTATIONS: typst_mutations.rb's
// output) with Ruby's projects and results.
func TestMutations(t *testing.T) {
	directory := os.Getenv("RELATEX_TYPST_MUTATIONS")
	if directory == "" {
		t.Skip("RELATEX_TYPST_MUTATIONS is not set")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	Root, tablemodel.Root, fontmodel.Root, paperdoc.Root = root, root, root, root
	cases, _ := filepath.Glob(filepath.Join(directory, "*"))
	for _, reference := range cases {
		t.Run(filepath.Base(reference), func(t *testing.T) {
			document := decodeFile(t, filepath.Join(reference, "document.json")).(rec.Map)
			out := t.TempDir()
			result := mathResult(func() any {
				report, losses := Write(document, out)
				return rec.Map{"writer": report, "losses": losses}
			})
			expected := decodeFile(t, filepath.Join(reference, "result.json"))
			if !sameJSON(expected, result) {
				t.Errorf("result differs: ruby %v, go %v", expected, result)
			}
			if _, failed := expected.(rec.Map)["failure"]; !failed {
				compareMutated(t, reference, out)
			}
		})
	}
}

func compareMutated(t *testing.T, reference, out string) {
	entries, _ := os.ReadDir(reference)
	for _, entry := range entries {
		if entry.Name() == "document.json" || entry.Name() == "result.json" {
			continue
		}
		want, _ := os.ReadFile(filepath.Join(reference, entry.Name()))
		got, err := os.ReadFile(filepath.Join(out, entry.Name()))
		if err != nil || !bytes.Equal(want, got) {
			at := 0
			for at < len(want) && at < len(got) && want[at] == got[at] {
				at++
			}
			t.Errorf("%s differs at %d:\n ruby %q\n go   %q", entry.Name(), at, excerpt(want, at), excerpt(got, at))
		}
	}
}
