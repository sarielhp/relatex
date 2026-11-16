package structure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
)

// TestReplay replays every StructureEvidence report the Ruby reference made on the dumped
// corpus streams (scratch notes_dump.rb), with the same records and blocks, and compares
// the reports byte for byte. It runs only when RELATEX_NOTES_DUMP names the dump directory.
func TestReplay(t *testing.T) {
	dump := os.Getenv("RELATEX_NOTES_DUMP")
	if dump == "" {
		t.Skip("RELATEX_NOTES_DUMP is not set")
	}
	papers, err := os.ReadDir(dump)
	if err != nil {
		t.Fatal(err)
	}
	for _, paper := range papers {
		t.Run(paper.Name(), func(t *testing.T) { replayPaper(t, dump, paper.Name()) })
	}
}

func replayPaper(t *testing.T, dump, paper string) {
	streams := filepath.Join(dump, "..", "streams", paper)
	stage := decode(t, readFile(t, filepath.Join(streams, "stage.json"))).(rec.Map)
	loaded, err := stream.Load(filepath.Join(streams, "stream.jsonl"), filepath.Join("..", "..", ".."), stage)
	if err != nil {
		t.Fatal(err)
	}
	calls := decode(t, readFile(t, filepath.Join(dump, paper, "calls.json"))).([]any)
	for number, value := range calls {
		call := value.(rec.Map)
		if call["call"] != "structure" {
			continue
		}
		records := []rec.Map{}
		for _, index := range call["records"].([]any) {
			records = append(records, loaded.Records[index.(int64)])
		}
		blocks := []rec.Map{}
		for _, block := range decode(t, call["blocks"].(string)).([]any) {
			blocks = append(blocks, block.(rec.Map))
		}
		got, failure := report(records, blocks)
		if want, ok := call["failure"]; ok {
			if failure == nil || failure.Error() != want {
				t.Errorf("call %d: failure %v, want %q", number, failure, want)
			}
			continue
		}
		if failure != nil {
			t.Fatalf("call %d failed: %v", number, failure)
		}
		bytes, err := canonjson.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if string(bytes) != call["report"].(string) {
			t.Errorf("call %d (nested %v) report differs\n go:   %.300s\n ruby: %.300s", number, call["nested"], bytes, call["report"])
		}
	}
}

func report(records, blocks []rec.Map) (result rec.Map, err error) {
	defer rec.Recover(&err)
	return Report(records, blocks), nil
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes)
}

func decode(t *testing.T, text string) any {
	t.Helper()
	value, err := canonjson.Decode(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	return value
}
