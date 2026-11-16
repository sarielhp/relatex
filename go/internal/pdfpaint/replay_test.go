package pdfpaint

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/paintmodel"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// TestReplay replays the paint captures and validations Ruby made on the probe corpus
// (scratchpad paint_dump.rb) and compares the results byte for byte. It runs only when
// RELATEX_PAINT_DUMP names the dump directory.
func TestReplay(t *testing.T) {
	dump := os.Getenv("RELATEX_PAINT_DUMP")
	if dump == "" {
		t.Skip("RELATEX_PAINT_DUMP is not set")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	tablemodel.Root = root
	t.Setenv("RELATEX_RUNS_DIR", t.TempDir())
	files, err := filepath.Glob(filepath.Join(dump, os.Getenv("RELATEX_PAINT_ONLY")+"*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, path := range files {
		call := load(t, path)
		started := time.Now()
		actual := replay(call)
		seconds := time.Since(started).Seconds()
		expected := outcome(call)
		name := strings.TrimPrefix(path, dump+"/")
		if expected != actual {
			t.Errorf("%s: differs\n  ruby: %.300s\n  go:   %.300s", name, expected, actual)
			continue
		}
		if kind := call["kind"]; kind == "tables" || kind == "text" {
			t.Logf("%s: identical (ruby %.2fs, go %.2fs)", name, paintmodel.Number(call, "seconds"), seconds)
		}
	}
}

func load(t *testing.T, path string) rec.Map {
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	value, err := canonjson.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return rec.AsMap(value)
}

func outcome(call rec.Map) string {
	if failure, ok := call["failure"]; ok {
		return "FAILURE " + failure.(string)
	}
	return marshal(call["result"])
}

func marshal(value any) string {
	data, err := canonjson.Marshal(value)
	if err != nil {
		return "MARSHAL " + err.Error()
	}
	return string(bytes.TrimSpace(data))
}

// replay runs one call in Go: its result as canonical JSON, or FAILURE and the message.
func replay(call rec.Map) (result string) {
	defer describeFailure(&result)
	input := rec.FetchMap(call, "input")
	switch rec.FetchString(call, "kind") {
	case "tables":
		paint, _ := CaptureTables(rec.FetchString(input, "pdf"), rec.FetchMap(input, "reading"), rec.FetchMap(input, "document"))
		if paint == nil {
			return marshal(nil)
		}
		return marshal(paint)
	case "text":
		paint, _ := CaptureText(rec.FetchString(input, "pdf"), rec.Fetch(input, "media_groups") == true)
		return marshal(paint)
	case "pdf_media":
		return marshal(paintmodel.PDFMedia(rec.FetchMap(input, "document")))
	case "validate:Relatex::PDFTablePaint":
		paintmodel.ValidateTables(rec.FetchMap(input, "document"))
		return marshal(nil)
	case "validate:Relatex::PDFTextPaint":
		paintmodel.ValidateText(rec.FetchMap(input, "document"))
		return marshal(nil)
	}
	return "UNKNOWN " + rec.FetchString(call, "kind")
}

// describeFailure turns a panic of the replayed call into its outcome.
func describeFailure(result *string) {
	recovered := recover()
	if recovered == nil {
		return
	}
	if failure, ok := recovered.(rec.Failure); ok {
		*result = "FAILURE " + failure.Message
	} else {
		*result = fmt.Sprintf("PANIC %v", recovered)
	}
}
