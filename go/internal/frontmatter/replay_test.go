package frontmatter

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/extlinks"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
	"github.com/sarielhp/relatex/internal/textstyle"
)

// TestReplay replays the calls Ruby made to PaperFrontmatter.project/validate! and
// PaperExternalLinks.project/validate! (scratch front_dump.rb) and compares the results
// byte for byte. It runs only when RELATEX_FRONT_DUMP names the dump directory.
func TestReplay(t *testing.T) {
	directory := os.Getenv("RELATEX_FRONT_DUMP")
	if directory == "" {
		t.Skip("RELATEX_FRONT_DUMP is not set")
	}
	_, file, _, _ := runtime.Caller(0)
	tablemodel.Root = filepath.Join(filepath.Dir(file), "..", "..", "..")
	papers, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, paper := range papers {
		t.Run(paper.Name(), func(t *testing.T) { replayPaper(t, filepath.Join(directory, paper.Name(), "calls.jsonl")) })
	}
}

func replayPaper(t *testing.T, path string) {
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<30)
	calls := 0
	for scanner.Scan() {
		value, err := canonjson.Decode(bytes.NewReader(scanner.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		entry := value.(rec.Map)
		expected := expectation(entry)
		actual := outcome(func() any { return replay(entry) })
		if expected != actual {
			at := mismatch(expected, actual)
			t.Errorf("%s call %d differs at %d\nruby: %s\ngo:   %s", entry["method"], calls, at, window(expected, at), window(actual, at))
		}
		calls++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d calls identical", calls)
}

func replay(entry rec.Map) any {
	input := rec.FetchMap(entry, "input")
	switch entry["method"] {
	case "frontmatter.project":
		records := []rec.Map{}
		for _, record := range rec.FetchList(input, "records") {
			records = append(records, rec.AsMap(record))
		}
		result := Project(func(rows []rec.Map) Styles { return textstyle.NewCatalog(rows) }, records, decode(input["source"]), decode(input["text"]), rec.FetchString(input, "path"),
			rec.FetchMap(input, "identity"), rec.FetchMap(input, "document"))
		if result == nil {
			return nil
		}
		return result
	case "frontmatter.validate":
		Validate(rec.FetchMap(input, "document"))
	case "extlinks.project":
		document := rec.FetchMap(input, "document")
		identity := rec.FetchMap(document, "identity")
		result := extlinks.Project(rec.FetchMap(input, "inspection"), document, func(scope string, key any) string {
			return identifier(identity, scope, key)
		})
		if result == nil {
			return nil
		}
		return result
	case "extlinks.validate":
		extlinks.Validate(rec.FetchMap(input, "document"))
	}
	return nil
}

func decode(value any) string {
	bytes, err := base64.StdEncoding.DecodeString(value.(string))
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

func expectation(entry rec.Map) string {
	if failure, ok := entry["failure"].(string); ok {
		return "FAILURE " + failure
	}
	return marshal(entry["output"])
}

func marshal(value any) string {
	bytes, err := canonjson.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

// outcome runs fn, returning its canonical result or "FAILURE Relatex::Failure: message".
func outcome(fn func() any) (result string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			failure, ok := recovered.(rec.Failure)
			if !ok {
				panic(recovered)
			}
			class := "Relatex::Failure"
			if failure.Lookup {
				class = "KeyError"
			}
			result = "FAILURE " + class + ": " + failure.Message
		}
	}()
	return marshal(fn())
}

func mismatch(left, right string) int {
	for index := 0; index < min(len(left), len(right)); index++ {
		if left[index] != right[index] {
			return index
		}
	}
	return min(len(left), len(right))
}

func window(text string, at int) string {
	start := max(at-100, 0)
	end := min(at+200, len(text))
	return strings.ToValidUTF8(text[start:end], "?")
}
