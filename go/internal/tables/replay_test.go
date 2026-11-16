package tables

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// TestReplay replays the table projections Ruby's ReadingEvidence made (dumped by the
// scratch tables_dump.rb into $RELATEX_TABLES_DUMP) and compares the Go projections with
// them byte for byte. Each case directory holds records.json, or names a stream of the
// corpus beside the dump (../streams/<case>).
func TestReplay(t *testing.T) {
	dump := os.Getenv("RELATEX_TABLES_DUMP")
	if dump == "" {
		t.Skip("RELATEX_TABLES_DUMP is not set")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	tablemodel.Root = root
	cases, err := os.ReadDir(dump)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range cases {
		t.Run(entry.Name(), func(t *testing.T) {
			replayCase(t, filepath.Join(dump, entry.Name()), root)
		})
	}
}

func replayCase(t *testing.T, directory, root string) {
	records := caseRecords(t, directory, root)
	calls, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range calls {
		if !call.IsDir() {
			continue
		}
		path := filepath.Join(directory, call.Name())
		input := rec.AsMap(decodeFile(t, filepath.Join(path, "input.json")))
		unlink(records, rec.AsList(input["unlinked"]))
		expected, err := os.ReadFile(filepath.Join(path, "output.json"))
		if err != nil {
			t.Fatal(err)
		}
		actual := replayCall(records, input, strings.HasSuffix(call.Name(), "-rich"))
		if !bytes.Equal(actual, expected) {
			at := 0
			for at < len(actual) && at < len(expected) && actual[at] == expected[at] {
				at++
			}
			t.Errorf("%s differs at %d\n  ruby: %s\n  go:   %s", call.Name(), at, excerpt(expected, at), excerpt(actual, at))
		}
	}
}

func excerpt(text []byte, at int) string {
	return string(text[max(at-120, 0):min(at+200, len(text))])
}

func caseRecords(t *testing.T, directory, root string) []rec.Map {
	if _, err := os.Stat(filepath.Join(directory, "records.json")); err == nil {
		return maps(rec.AsList(decodeFile(t, filepath.Join(directory, "records.json"))))
	}
	streamDirectory := filepath.Join(directory, "..", "..", "streams", filepath.Base(directory))
	stage := rec.AsMap(decodeFile(t, filepath.Join(streamDirectory, "stage.json")))
	loaded, err := stream.Load(filepath.Join(streamDirectory, "stream.jsonl"), root, stage)
	if err != nil {
		t.Fatal(err)
	}
	return loaded.Records
}

func decodeFile(t *testing.T, path string) any {
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

// unlink deletes the external links ReadingEvidence dropped before the call.
func unlink(records []rec.Map, slots []any) {
	owners := map[string]rec.Map{}
	for _, row := range records {
		if row["kind"] == "flow_candidate" {
			owners[key(row["owner"])] = row
		}
	}
	for _, value := range slots {
		slot := rec.AsList(value)
		items := rec.AsList(owners[key(slot[0])]["items"])
		delete(rec.AsMap(items[rec.AsInt(slot[1])-1]), "external_link")
	}
}

// replayCall runs one projection on the dumped input and writes what Ruby's dump wrote.
func replayCall(records []rec.Map, input rec.Map, isRich bool) (output []byte) {
	blocks := maps(rec.AsList(input["blocks"]))
	diagnostics := maps(rec.AsList(input["diagnostics"]))
	defer func() {
		if recovered := recover(); recovered != nil {
			output = marshal(rec.Map{"failure": failureText(recovered)})
		}
	}()
	if !isRich {
		blocks, diagnostics = Project(records, blocks, diagnostics)
		return marshal(rec.Map{"blocks": anyList(blocks), "diagnostics": anyList(diagnostics)})
	}
	targets := maps(rec.AsList(input["targets"]))
	origins := map[uintptr]rec.Map{}
	for _, value := range rec.AsList(input["origins"]) {
		position := rec.AsList(value)
		content := rec.AsList(blocks[rec.AsInt(position[0])]["content"])
		origins[rec.Identity(rec.AsMap(content[rec.AsInt(position[1])]))] = rec.AsMap(position[2])
	}
	blocks, diagnostics, targets, _, projected := ProjectRich(records, origins, blocks, diagnostics, targets)
	return marshal(rec.Map{"blocks": anyList(blocks), "diagnostics": anyList(diagnostics), "targets": anyList(targets), "rich": projected})
}

func failureText(recovered any) string {
	switch typed := recovered.(type) {
	case rec.Failure:
		return "Relatex::Failure: " + typed.Message
	case indexError:
		return "IndexError: " + typed.message
	}
	return "panic"
}

func marshal(value any) []byte {
	bytes, err := canonjson.Marshal(value)
	if err != nil {
		panic(err)
	}
	return bytes
}
