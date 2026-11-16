package frontmatter

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

// TestMetadataReplay compares PaperTexMetadata on every .tex file of the cohort's archives
// with Ruby's results (scratch meta_ref.rb). It runs only when RELATEX_META_REF names them.
func TestMetadataReplay(t *testing.T) {
	directory := os.Getenv("RELATEX_META_REF")
	if directory == "" {
		t.Skip("RELATEX_META_REF is not set")
	}
	paths, err := filepath.Glob(filepath.Join(directory, "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	checks := 0
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 1<<20), 1<<28)
		for scanner.Scan() {
			if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
				continue
			}
			value, err := canonjson.Decode(bytes.NewReader(scanner.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			checks += compareMetadata(t, filepath.Base(path), rec.AsMap(value))
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		file.Close()
	}
	t.Logf("%d results identical or reported", checks)
}

func compareMetadata(t *testing.T, paper string, entry rec.Map) int {
	name := paper + ":" + rec.FetchString(entry, "file")
	var parser *TexMetadata
	newResult := result(func() any { parser = NewTexMetadata(decode(entry["source"])); return nil })
	check(t, name+" new", entry["new"], newResult)
	if parser == nil {
		return 1
	}
	check(t, name+" project", entry["project"], result(func() any { return orNil(parser.Project()) }))
	check(t, name+" links", entry["links"], result(func() any { return maps(parser.Links()) }))
	if (entry["ams"] == true) != parser.AMS() {
		t.Errorf("%s ams differs", name)
	}
	abstracts := scanFirst(abstractEnv, parser.clean)
	expected := rec.FetchList(entry, "prose")
	if len(abstracts) != len(expected) {
		t.Errorf("%s: %d abstracts, ruby %d", name, len(abstracts), len(expected))
	}
	for index := range min(len(abstracts), len(expected)) {
		check(t, name+" prose", expected[index], result(func() any { return parser.AbstractProse(abstracts[index]) }))
	}
	arguments := []string{}
	check(t, name+" arguments", entry["arguments"], result(func() any {
		values := []any{}
		for _, row := range parser.commands(parser.clean, []string{"title", "author", "affiliation", "address", "thanks", "institute", "name"}) {
			values = append(values, row.argument)
			arguments = append(arguments, row.argument)
		}
		return values
	}))
	literal := rec.FetchList(entry, "literal")
	for index := range min(len(arguments), len(literal)) {
		check(t, name+" literal", literal[index], result(func() any { return parser.LiteralText(arguments[index]) }))
	}
	return 5 + len(literal)
}

func orNil(value rec.Map) any {
	if value == nil {
		return nil
	}
	return value
}

func result(fn func() any) (outcome rec.Map) {
	defer func() {
		if recovered := recover(); recovered != nil {
			failure, ok := recovered.(rec.Failure)
			if !ok {
				panic(recovered)
			}
			outcome = rec.Map{"failure": failure.Message}
		}
	}()
	return rec.Map{"output": fn()}
}

func check(t *testing.T, name string, expected any, actual rec.Map) {
	want, got := marshal(expected), marshal(actual)
	if want != got {
		at := mismatch(want, got)
		t.Errorf("%s differs at %d\nruby: %s\ngo:   %s", name, at, window(want, at), window(got, at))
	}
}

func maps(values []rec.Map) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}
