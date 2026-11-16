// Package stream is the Go port of Relatex::Stream and Relatex::StreamExtensions: the TeX
// collector's record stream (one JSON record per line), checked line by line against
// schemas/event.schema.json and as a whole (header, terminal count, adapters, extensions).
package stream

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/schema"
)

const (
	limit       = 256 * 1024 * 1024
	recordLimit = 1024 * 1024
)

// Stream is a validated record stream.
type Stream struct {
	Records []rec.Map
}

// Load reads and validates a stream; root is the repository root (for schemas/), stage the
// engine pass that wrote it (its exit and timeout).
func Load(path, root string, stage rec.Map) (result *Stream, err error) {
	defer rec.Recover(&err)
	exit, _ := stage["exit"].(int64)
	_, hasExit := stage["exit"].(int64)
	if !hasExit || exit != 0 || stage["timeout"] == true {
		rec.Fail("Engine did not exit successfully")
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		rec.Fail("%v", statErr)
	}
	if info.Size() > limit {
		rec.Fail("Stream size limit")
	}
	contract := loadContract(root)
	records := parse(path, contract)
	s := &Stream{Records: records}
	s.validate()
	return s, nil
}

func loadContract(root string) rec.Map {
	file, err := os.Open(filepath.Join(root, "schemas", "event.schema.json"))
	if err != nil {
		rec.Fail("%v", err)
	}
	defer file.Close()
	value, err := canonjson.Decode(file)
	if err != nil {
		rec.Fail("%v", err)
	}
	return rec.AsMap(value)
}

func parse(path string, contract rec.Map) []rec.Map {
	data, err := os.ReadFile(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	lines := [][]byte{}
	for rest := data; len(rest) > 0; {
		end := bytes.IndexByte(rest, '\n') + 1
		if end == 0 {
			end = len(rest)
		}
		lines = append(lines, rest[:end])
		rest = rest[end:]
	}
	records := make([]rec.Map, len(lines))
	failures := make([]any, len(lines))
	// The records are independent lines: they are read side by side, and the first failing one in the
	// stream's order is the one raised.
	var group sync.WaitGroup
	workers := min(runtime.NumCPU(), len(lines)/256+1)
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := worker; index < len(lines); index += workers {
				records[index], failures[index] = readRecord(lines[index], int64(index+1), contract)
			}
		}()
	}
	group.Wait()
	for _, failure := range failures {
		if failure != nil {
			panic(failure)
		}
	}
	return records
}

// readRecord is parseRecord with its failure returned rather than raised.
func readRecord(line []byte, seq int64, contract rec.Map) (record rec.Map, failure any) {
	defer func() { failure = recover() }()
	return parseRecord(line, seq, contract), nil
}

func parseRecord(line []byte, seq int64, contract rec.Map) rec.Map {
	if !utf8.Valid(line) || len(line) > recordLimit || !bytes.HasSuffix(line, []byte("\n")) {
		rec.Fail("Invalid UTF-8, oversized or unterminated record")
	}
	value, err := canonjson.Decode(bytes.NewReader(line))
	if err != nil {
		rec.Fail("Invalid JSON: %v", err)
	}
	schema.Validate(value, contract, "$")
	record, ok := value.(rec.Map)
	if !ok || !rec.Equal(record["seq"], seq) {
		rec.Fail("Expected object with consecutive sequence")
	}
	return record
}

// Header is the stream's first record.
func (s *Stream) Header() rec.Map { return s.Records[0] }

func (s *Stream) validate() {
	records := s.Records
	if len(records) == 0 || rec.Fetch(records[0], "kind") != "header" || rec.Fetch(records[len(records)-1], "kind") != "end" {
		rec.Fail("Missing header or terminal record")
	}
	header := records[0]
	if !slices.Contains([]string{"0.1", "0.2", "0.3", "0.4", "0.5", "0.6"}, fmt.Sprint(rec.Fetch(header, "protocol"))) {
		rec.Fail("Unsupported protocol")
	}
	if len(records) > 1_600_000 {
		rec.Fail("Too many records")
	}
	if !rec.Equal(rec.Fetch(records[len(records)-1], "records"), int64(len(records))) {
		rec.Fail("Incorrect terminal count")
	}
	if countKinds(records, "header", "end") != 2 {
		rec.Fail("Mixed or prematurely terminated streams")
	}
	if !ValidExtensions(header) {
		rec.Fail("Unsupported stream extension")
	}
	for _, record := range records {
		if record["code"] == "collector_error" {
			rec.Fail("Collector error on page %s: %s", rubyString(record["page"]), rubyString(record["message"]))
		}
	}
	s.validateRecords()
}

func (s *Stream) validateRecords() {
	candidates := map[int64]any{}
	for _, record := range s.Records {
		s.validateAdapter(record)
		switch rec.Fetch(record, "kind") {
		case "candidate":
			id, ok := rec.Fetch(record, "candidate").(int64)
			if _, seen := candidates[id]; !ok || id <= 0 || seen {
				rec.Fail("Invalid or duplicate candidate")
			}
			candidates[id] = rec.Fetch(record, "glyphs")
		case "occurrence":
			validateOccurrence(record, candidates)
		}
	}
}

func validateOccurrence(record rec.Map, candidates map[int64]any) {
	id, _ := rec.Fetch(record, "candidate").(int64)
	size := candidates[id]
	first, last, page := record["first"], record["last"], record["page"]
	valid := rec.Positive(size) && rec.Positive(first) && rec.Positive(last) && rec.Positive(page)
	if !valid || rec.AsInt(first) > rec.AsInt(last) || rec.AsInt(last) > rec.AsInt(size) {
		rec.Fail("Dangling or invalid occurrence")
	}
}

func countKinds(records []rec.Map, kinds ...string) int {
	count := 0
	for _, record := range records {
		if slices.Contains(kinds, fmt.Sprint(rec.Fetch(record, "kind"))) {
			count++
		}
	}
	return count
}

// rubyString formats a value as Ruby's string interpolation does for the values a record
// holds (nil prints as the empty string).
func rubyString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
