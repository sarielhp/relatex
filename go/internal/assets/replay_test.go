package assets

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// The replay compares the Go media code with Ruby's calls dumped by the scratch script
// assets_dump.rb (RELATEX_ASSETS_DUMP names its output; probes sit beside it). Excluded as
// run-dependent: the page count's sandbox command (scratch paths) and its seconds, and the
// bytes of crops pdftocairo writes (cairo stamps a creation date).

type replay struct {
	t      *testing.T
	dump   string
	probe  string
	paper  string
	failed bool
	times  map[string]time.Duration
}

func (r *replay) blob(key string) any {
	file, err := os.Open(filepath.Join(r.dump, "blobs", key+".json"))
	if err != nil {
		r.t.Fatal(err)
	}
	defer file.Close()
	value, err := canonjson.Decode(file)
	if err != nil {
		r.t.Fatal(err)
	}
	return value
}

func (r *replay) bytes(key string) []byte {
	data, err := os.ReadFile(filepath.Join(r.dump, "bytes", key))
	if err != nil {
		r.t.Fatal(err)
	}
	return data
}

func canonical(t *testing.T, value any) string {
	data, err := canonjson.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// run calls fn, returning its Failure message ("" when it returns).
func run(fn func()) (message string) {
	started := time.Now()
	defer func() { elapsed += time.Since(started) }()
	defer func() {
		if recovered := recover(); recovered != nil {
			failure, ok := recovered.(rec.Failure)
			if !ok {
				panic(recovered)
			}
			message = failure.Message
		}
	}()
	fn()
	return ""
}

// elapsed is the time spent in the ported calls, without the replay's JSON work.
var elapsed time.Duration

func (r *replay) same(kind string, expected, actual string) {
	if expected == actual {
		return
	}
	r.failed = true
	at := 0
	for at < len(expected) && at < len(actual) && expected[at] == actual[at] {
		at++
	}
	from := max(at-120, 0)
	r.t.Errorf("%s %s differs at %d\n ruby: %.300s\n go:   %.300s", r.paper, kind, at, expected[from:], actual[from:])
}

func TestReplay(t *testing.T) {
	root := os.Getenv("RELATEX_ASSETS_DUMP")
	if root == "" {
		t.Skip("RELATEX_ASSETS_DUMP is not set")
	}
	papers, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range papers {
		r := &replay{t: t, dump: filepath.Join(root, entry.Name()), probe: filepath.Join(root, "..", "probes", entry.Name()),
			paper: entry.Name(), times: map[string]time.Duration{}}
		started := time.Now()
		calls := rec.AsMap(r.blobFile("calls.json"))
		counts := map[string]int{}
		for _, value := range rec.FetchList(calls, "calls") {
			call := rec.AsMap(value)
			kind := rec.FetchString(call, "kind")
			counts[kind]++
			before := elapsed
			r.call(kind, call)
			r.times[kind] += elapsed - before
		}
		t.Logf("%s: %v in %.1fs (ported calls %v), identical: %v", r.paper, counts, time.Since(started).Seconds(), r.times, !r.failed)
	}
}

func (r *replay) blobFile(name string) any {
	file, err := os.Open(filepath.Join(r.dump, name))
	if err != nil {
		r.t.Fatal(err)
	}
	defer file.Close()
	value, err := canonjson.Decode(file)
	if err != nil {
		r.t.Fatal(err)
	}
	return value
}

func expectation(call rec.Map) (any, string) {
	failure, _ := call["failure"].(string)
	return call["result"], failure
}

func (r *replay) call(kind string, call rec.Map) {
	switch kind {
	case "rendered":
		r.rendered(call)
	case "from_reading":
		r.fromReading(call)
	case "page_count":
		r.pageCount(call)
	case "representation":
		r.representation(call)
	case "validate_png_assets":
		_, failure := expectation(call)
		images := []rec.Map{}
		for _, image := range rec.AsList(r.blob(rec.FetchString(call, "images"))) {
			images = append(images, rec.AsMap(image))
		}
		assets := rec.AsMap(r.blob(rec.FetchString(call, "assets")))
		r.same(kind, failure, run(func() { mediamodel.ValidatePNGAssets(assets, images) }))
	}
}

func (r *replay) rendered(call rec.Map) {
	result, failure := expectation(call)
	reading := rec.AsMap(r.blob(rec.FetchString(call, "reading")))
	diags := diagnostics.New(mustProfile(r.t), nil, false)
	var files map[string][]byte
	message := run(func() { files = ApplyRendered(reading, filepath.Join(r.probe, rec.FetchString(call, "pdf")), diags) })
	r.same("rendered failure", failure, message)
	if message != "" || failure != "" {
		return
	}
	expected := rec.AsMap(result)
	names := rec.Map{}
	for name := range files {
		names[name] = true
	}
	wantNames := rec.Map{}
	for name := range rec.FetchMap(expected, "files") {
		wantNames[name] = true
	}
	want := rec.AsMap(r.blob(rec.FetchString(expected, "reading")))
	// A crop's bytes are run-dependent (cairo's font subsets): compare its spans without them.
	if canonical(r.t, names) != canonical(r.t, wantNames) {
		r.t.Logf("%s: %d crops; their bytes differ from Ruby's, as from run to run", r.paper, len(files))
		for _, value := range []rec.Map{want, reading} {
			for _, span := range renderedImages(value) {
				delete(span, "filename")
				delete(span, "sha256")
				delete(span, "bytes")
			}
		}
	}
	r.same("rendered reading", canonical(r.t, want), canonical(r.t, reading))
	r.same("rendered diagnostics", canonical(r.t, expected["diagnostics"]), canonical(r.t, diags.Entries()))
}

func (r *replay) fromReading(call rec.Map) {
	result, failure := expectation(call)
	reading := rec.AsMap(r.blob(rec.FetchString(call, "reading")))
	reads := rec.FetchMap(call, "reads")
	var assets rec.Map
	message := run(func() {
		assets = FromReading(reading, func(name string) []byte { return r.bytes(rec.FetchString(reads, name)) })
	})
	r.same("from_reading failure", failure, message)
	if message == "" && failure == "" {
		r.same("from_reading", canonical(r.t, r.blob(rec.AsString(result))), canonical(r.t, assets))
	}
}

func (r *replay) pageCount(call rec.Map) {
	result, failure := expectation(call)
	output := filepath.Join(r.t.TempDir(), "count")
	var proof rec.Map
	message := run(func() { proof = CapturePageCount(filepath.Join(r.probe, rec.FetchString(call, "pdf")), output) })
	r.same("page_count failure", failure, message)
	if message != "" || failure != "" {
		return
	}
	expected := rec.AsMap(result)
	for _, value := range []rec.Map{expected, proof} {
		stage := rec.FetchMap(value, "stage")
		delete(stage, "command")
		delete(stage, "seconds")
	}
	r.same("page_count", canonical(r.t, expected), canonical(r.t, proof))
}

func (r *replay) representation(call rec.Map) {
	result, failure := expectation(call)
	descriptor := rec.AsMap(r.blob(rec.FetchString(call, "descriptor")))
	placement, _ := call["placement"].(rec.Map)
	var item rec.Map
	message := run(func() {
		item = mediamodel.NewMediaAsset(descriptor).Representation(rec.FetchString(call, "format"), placement)
	})
	r.same("representation failure", failure, message)
	if message == "" && failure == "" {
		r.same("representation", canonical(r.t, result), canonical(r.t, item))
	}
}

func mustProfile(t *testing.T) rec.Map {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := diagnostics.LoadProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func renderedImages(reading rec.Map) []rec.Map {
	result := []rec.Map{}
	for _, block := range rec.FetchList(reading, "blocks") {
		for _, span := range rec.FetchList(rec.AsMap(block), "content") {
			if _, ok := rec.AsMap(span)["rendered_from"]; ok {
				result = append(result, rec.AsMap(span))
			}
		}
	}
	return result
}
