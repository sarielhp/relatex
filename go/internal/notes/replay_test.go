package notes

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
)

// TestReplay replays the NoteEvidence calls the Ruby reference made on each dumped corpus
// stream (scratch notes_dump.rb) and compares every answer byte for byte. It runs only
// when RELATEX_NOTES_DUMP names the dump directory; the streams are in its sibling streams/.
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
		t.Run(paper.Name(), func(t *testing.T) {
			records, calls := load(t, dump, paper.Name())
			replay(t, records, calls)
		})
	}
}

func load(t *testing.T, dump, paper string) ([]rec.Map, []any) {
	t.Helper()
	streams := filepath.Join(dump, "..", "streams", paper)
	stage := decode(t, filepath.Join(streams, "stage.json")).(rec.Map)
	loaded, err := stream.Load(filepath.Join(streams, "stream.jsonl"), filepath.Join("..", "..", ".."), stage)
	if err != nil {
		t.Fatal(err)
	}
	return loaded.Records, decode(t, filepath.Join(dump, paper, "calls.json")).([]any)
}

func decode(t *testing.T, path string) any {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	value, err := canonjson.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func canonical(t *testing.T, value any) string {
	t.Helper()
	bytes, err := canonjson.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes)
}

func blocksOf(t *testing.T, text string) []rec.Map {
	t.Helper()
	value, err := canonjson.Decode(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	blocks := []rec.Map{}
	for _, block := range value.([]any) {
		blocks = append(blocks, block.(rec.Map))
	}
	return blocks
}

func replay(t *testing.T, records []rec.Map, calls []any) {
	index := map[uintptr]int64{}
	for position, record := range records {
		index[rec.Identity(record)] = int64(position)
	}
	var notes *Notes
	var failure error
	func() {
		defer rec.Recover(&failure)
		notes = New(records)
	}()
	if failure != nil {
		t.Fatalf("New failed: %v", failure)
	}
	for number, value := range calls {
		call := value.(rec.Map)
		switch call["call"] {
		case "note_for_entity":
			got, message := guarded(func() any { return notes.NoteForEntity(call["entity"]) })
			expectSame(t, number, call, got, message)
		case "main_records":
			var got []any
			for _, record := range notes.MainRecords() {
				got = append(got, index[rec.Identity(record)])
			}
			if canonical(t, got) != canonical(t, call["records"]) {
				t.Errorf("call %d main_records differ", number)
			}
		case "project":
			project(t, notes, number, call)
		}
	}
}

func project(t *testing.T, notes *Notes, number int, call rec.Map) {
	blocks := blocksOf(t, call["blocks"].(string))
	report, message := guarded(func() any { return notes.Project(blocks) })
	if failure, ok := call["failure"]; ok {
		if message != failure {
			t.Errorf("call %d project: failure %q, want %q", number, message, failure)
		}
		return
	}
	if message != "" {
		t.Fatalf("call %d project failed: %s", number, message)
	}
	compare(t, number, "report", canonical(t, report), call["report"].(string))
	compare(t, number, "blocks after", canonical(t, asList(blocks)), call["after"].(string))
	ids := []any{}
	for _, block := range notes.MainBlocks() {
		ids = append(ids, block["id"])
	}
	compare(t, number, "main blocks", canonical(t, ids), canonical(t, call["main_blocks"]))
}

func compare(t *testing.T, number int, what, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	at := 0
	for at < min(len(got), len(want)) && got[at] == want[at] {
		at++
	}
	from := max(at-120, 0)
	t.Errorf("call %d %s differ at %d\n go:   %s\n ruby: %s", number, what, at, got[from:min(len(got), at+200)],
		want[from:min(len(want), at+200)])
}

func expectSame(t *testing.T, number int, call rec.Map, got any, message string) {
	t.Helper()
	if failure, ok := call["failure"]; ok {
		if message != failure {
			t.Errorf("call %d: failure %q, want %q", number, message, failure)
		}
		return
	}
	if message != "" || !slices.Equal([]string{canonical(t, got)}, []string{canonical(t, call["result"])}) {
		t.Errorf("call %d note_for_entity(%v) = %v (%s), want %v", number, call["entity"], got, message, call["result"])
	}
}

// guarded runs fn and returns its value or the message of its Failure.
func guarded(fn func() any) (value any, message string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			failure, ok := recovered.(rec.Failure)
			if !ok {
				panic(recovered)
			}
			message = failure.Message
		}
	}()
	return fn(), ""
}
