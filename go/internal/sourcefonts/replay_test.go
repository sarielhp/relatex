package sourcefonts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// The replay compares SourceFonts with Ruby's calls dumped by the scratch script
// assets_dump.rb (RELATEX_ASSETS_DUMP). Run-dependent and excluded: the evidence directory's
// path (each evidence file is compared with it replaced), the vftovp logs' bytes.

func load(t *testing.T, path string) any {
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
	data, err := canonjson.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func failure(fn func()) (message string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			caught, ok := recovered.(rec.Failure)
			if !ok {
				panic(recovered)
			}
			message = caught.Message
		}
	}()
	fn()
	return ""
}

func same(t *testing.T, label, expected, actual string) bool {
	if expected == actual {
		return true
	}
	at := 0
	for at < len(expected) && at < len(actual) && expected[at] == actual[at] {
		at++
	}
	from := max(at-120, 0)
	t.Errorf("%s differs at %d\n ruby: %.300s\n go:   %.300s", label, at, expected[from:], actual[from:])
	return false
}

func TestReplay(t *testing.T) {
	root := os.Getenv("RELATEX_ASSETS_DUMP")
	if root == "" {
		t.Skip("RELATEX_ASSETS_DUMP is not set")
	}
	repository, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	Root, fontmodel.Root = repository, repository
	papers, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range papers {
		dump := filepath.Join(root, entry.Name())
		blob := func(key string) any { return load(t, filepath.Join(dump, "blobs", key+".json")) }
		calls := rec.AsMap(load(t, filepath.Join(dump, "calls.json")))
		for _, value := range rec.FetchList(calls, "calls") {
			call := rec.AsMap(value)
			expected, _ := call["failure"].(string)
			label := entry.Name() + " " + rec.FetchString(call, "kind")
			switch call["kind"] {
			case "validate_fonts":
				document := rec.AsMap(blob(rec.FetchString(call, "document")))
				same(t, label, expected, failure(func() { fontmodel.Validate(document) }))
			case "source_fonts":
				replayCapture(t, label, filepath.Join(root, "..", "probes", entry.Name()), call, expected, blob)
			}
		}
	}
}

func replayCapture(t *testing.T, label, probe string, call rec.Map, expected string, blob func(string) any) {
	document := rec.AsMap(blob(rec.FetchString(call, "document")))
	sources := []rec.Map{}
	for _, row := range rec.FetchList(call, "font_sources") {
		sources = append(sources, rec.AsMap(row))
	}
	started := time.Now()
	var fonts rec.Map
	var directory string
	message := failure(func() {
		fonts, directory = Capture(filepath.Join(probe, rec.FetchString(call, "pdf")), document, sources, "")
	})
	if !same(t, label+" failure", expected, message) || message != "" {
		return
	}
	result := rec.FetchMap(call, "result")
	same(t, label, canonical(t, result["fonts"]), canonical(t, fonts))
	origins := map[string]int{}
	for _, item := range rec.FetchList(fonts, "items") {
		origins[rec.FetchString(rec.AsMap(item), "origin")]++
	}
	t.Logf("%s: %d fonts %v in %.2fs", label, len(rec.FetchList(fonts, "items")), origins, time.Since(started).Seconds())
	evidence := rec.FetchMap(result, "evidence")
	actual := map[string]bool{}
	_ = filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			relative, _ := filepath.Rel(directory, path)
			actual[relative] = true
		}
		return nil
	})
	for name, value := range evidence {
		if !actual[name] {
			t.Errorf("%s: evidence %s missing", label, name)
			continue
		}
		reference, isJSON := value.(rec.Map)
		if !isJSON {
			continue // vftovp's logs and property lists: bytes of the TeX tools, not of this port.
		}
		want := blob(rec.FetchString(reference, "json"))
		got := load(t, filepath.Join(directory, name))
		// The evidence directory is the run's: Ruby's and Go's differ by path only.
		normal := func(value any) string {
			if stage, ok := value.(rec.Map); ok {
				delete(stage, "seconds") // vftovp's run time
			}
			text := canonical(t, value)
			for _, prefix := range []string{directory} {
				text = strings.ReplaceAll(text, prefix, "<evidence>")
			}
			return text
		}
		wantText := normal(want)
		if index := strings.Index(wantText, "/evidence/"); index >= 0 {
			start := strings.LastIndex(wantText[:index], "\"") + 1
			wantText = strings.ReplaceAll(wantText, wantText[start:index+len("/evidence")], "<evidence>")
		}
		same(t, label+" evidence "+name, wantText, normal(got))
	}
	for name := range actual {
		_, ok := evidence[name]
		switch {
		case ok:
		case strings.HasSuffix(name, "-vftovp.json") || strings.HasSuffix(name, "-vftovp.log"):
			// Ruby resolved this font earlier in its process (table paint) and reused the binding
			// without rerunning vftovp; a fresh process runs it.
			t.Logf("%s: %s written fresh (Ruby reused its process cache)", label, name)
		default:
			t.Errorf("%s: evidence %s is not Ruby's", label, name)
		}
	}
}
