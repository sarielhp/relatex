package typstsource

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// TestReplay writes each corpus bundle's Typst source and compares every file with Ruby's
// export (RELATEX_TYPST_CORPUS: the scratch directory holding export_corpus/ and typst_ref/).
func TestReplay(t *testing.T) {
	corpus := os.Getenv("RELATEX_TYPST_CORPUS")
	if corpus == "" {
		t.Skip("RELATEX_TYPST_CORPUS is not set")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	Root, tablemodel.Root, fontmodel.Root, paperdoc.Root = root, root, root, root
	papers, err := os.ReadDir(filepath.Join(corpus, "export_corpus"))
	if err != nil {
		t.Fatal(err)
	}
	for _, paper := range papers {
		t.Run(paper.Name(), func(t *testing.T) { replayPaper(t, corpus, paper.Name()) })
	}
}

func replayPaper(t *testing.T, corpus, paper string) {
	bundles, _ := filepath.Glob(filepath.Join(corpus, "export_corpus", paper, "output", "run-*", "document.json.gz"))
	references, _ := filepath.Glob(filepath.Join(corpus, "typst_ref", paper, "export-*"))
	if len(bundles) != 1 || len(references) != 1 {
		t.Fatalf("missing bundle or reference: %v %v", bundles, references)
	}
	document := readBundle(t, bundles[0])
	out := t.TempDir()
	if os.Getenv("RELATEX_TYPST_WARM") != "" {
		paperdoc.Validate(document)
	}
	started := time.Now()
	var report rec.Map
	var losses []any
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("Go writer failed: %v", recovered)
			}
		}()
		report, losses = Write(document, out)
	}()
	t.Logf("%s: Go %.2fs", paper, time.Since(started).Seconds())
	compareFiles(t, references[0], out)
	expected := decodeFile(t, filepath.Join(references[0], "export.json")).(rec.Map)
	want, _ := canonjson.Marshal(rec.Map{"writer": expected["writer"], "losses": expected["losses"]})
	got, _ := canonjson.Marshal(rec.Map{"writer": report, "losses": losses})
	if !bytes.Equal(want, got) {
		t.Errorf("report differs:\n ruby %s\n go   %s", want, got)
	}
}

func compareFiles(t *testing.T, reference, out string) {
	names := map[string]bool{}
	for _, directory := range []string{reference, out} {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.Name() != "export.json" && entry.Name() != "manifest.json" && entry.Name() != "result.json" {
				names[entry.Name()] = true
			}
		}
	}
	sorted := []string{}
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	t.Logf("comparing %d files", len(sorted))
	for _, name := range sorted {
		want, wantErr := os.ReadFile(filepath.Join(reference, name))
		got, gotErr := os.ReadFile(filepath.Join(out, name))
		if wantErr != nil || gotErr != nil {
			t.Errorf("%s: present only on one side (ruby %v, go %v)", name, wantErr, gotErr)
			continue
		}
		if !bytes.Equal(want, got) {
			at := 0
			for at < len(want) && at < len(got) && want[at] == got[at] {
				at++
			}
			t.Errorf("%s differs at byte %d:\n ruby %q\n go   %q", name, at, excerpt(want, at), excerpt(got, at))
		}
	}
}

func excerpt(text []byte, at int) string {
	start, end := max(at-120, 0), min(at+120, len(text))
	return string(text[start:end])
}

func readBundle(t *testing.T, path string) rec.Map {
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	value, err := canonjson.Decode(reader)
	if err != nil {
		t.Fatal(err)
	}
	return rec.FetchMap(rec.FetchMap(rec.AsMap(value), "payload"), "document")
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
