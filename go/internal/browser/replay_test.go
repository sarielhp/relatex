package browser

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/mathexport"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// TestReplay runs the Go check on the HTML browser_ref.rb prepared for Ruby's (html.orig in
// RELATEX_BROWSER_REF/<paper>) and compares the report or failure, and dom.json with its
// directory's path set aside, with Ruby's.
func TestReplay(t *testing.T) {
	reference := os.Getenv("RELATEX_BROWSER_REF")
	if reference == "" {
		t.Skip("RELATEX_BROWSER_REF names browser_ref.rb's output")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	Root, paperdoc.Root, tablemodel.Root, fontmodel.Root, mathexport.Root = root, root, root, root, root
	papers, err := os.ReadDir(reference)
	if err != nil {
		t.Fatal(err)
	}
	for _, paper := range papers {
		t.Run(paper.Name(), func(t *testing.T) { replayPaper(t, filepath.Join(reference, paper.Name())) })
	}
}

func replayPaper(t *testing.T, directory string) {
	html := filepath.Join(directory, "html.go")
	if err := os.RemoveAll(html); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("cp", "-a", filepath.Join(directory, "html.orig"), html).CombinedOutput(); err != nil {
		t.Fatal(string(output))
	}
	document := readJSON(t, filepath.Join(directory, "document.json"))
	started := time.Now()
	report := checkOrFailure(html, document)
	t.Logf("%.2fs", time.Since(started).Seconds())
	expected, err := os.ReadFile(filepath.Join(directory, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	actual, err := canonjson.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Errorf("report differs\n ruby: %s\n go:   %s", expected, actual)
	}
	compareDOM(t, filepath.Join(directory, "html", "dom.json"), filepath.Join(html, "dom.json"))
}

func checkOrFailure(html string, document rec.Map) (report rec.Map) {
	defer failureReport(&report)
	report, err := CheckPaper(html, document)
	if err != nil {
		return rec.Map{"failure": err.Error()}
	}
	// The archived Ruby reports predate the accessibility measurement (decision 080).
	delete(report, "accessibility")
	return report
}

func failureReport(report *rec.Map) {
	recovered := recover()
	if recovered == nil {
		return
	}
	var failure rec.Failure
	if err, ok := recovered.(error); ok && errors.As(err, &failure) && !failure.Lookup {
		*report = rec.Map{"failure": failure.Message}
		return
	}
	panic(recovered)
}

func compareDOM(t *testing.T, rubyPath, goPath string) {
	ruby, rubyErr := os.ReadFile(rubyPath)
	golang, goErr := os.ReadFile(goPath)
	if rubyErr != nil || goErr != nil {
		if (rubyErr == nil) != (goErr == nil) {
			t.Errorf("dom.json written by one side only: %v %v", rubyErr, goErr)
		}
		return
	}
	// Both sides serve the page at Origin: their URLs agree without a directory to set aside.
	normalized := string(golang)
	if normalized != string(ruby) {
		at := 0
		for at < len(ruby) && at < len(normalized) && ruby[at] == normalized[at] {
			at++
		}
		t.Errorf("dom.json differs at %d\n ruby: %q\n go:   %q", at, excerpt(string(ruby), at), excerpt(normalized, at))
	}
}

func excerpt(text string, at int) string {
	return text[max(at-80, 0):min(at+120, len(text))]
}

func readJSON(t *testing.T, path string) rec.Map {
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
