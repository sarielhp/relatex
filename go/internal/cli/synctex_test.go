package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSynctexWaitsOnlyForAPDFWrittenAfterItStarted(t *testing.T) {
	directory := t.TempDir()
	pdf := filepath.Join(directory, "paper.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-old"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(pdf, old, old); err != nil {
		t.Fatal(err)
	}
	run := &synctexRun{started: time.Now(), pdf: pdf, wait: 300 * time.Millisecond}
	request := exitOf(t, run.awaitPDF)
	if request == nil || request.status != 3 || !strings.HasPrefix(request.stderr, "relatex synctex: timed out") {
		t.Fatalf("the previous build's PDF must not be taken: %+v", request)
	}
	run = &synctexRun{started: time.Now().Add(-time.Second), pdf: pdf, wait: 5 * time.Second}
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = os.WriteFile(pdf, []byte("%PDF-new"), 0o644)
	}()
	if request := exitOf(t, run.awaitPDF); request != nil {
		t.Fatalf("a PDF written after the start is taken once it is stable: %+v", request)
	}
}

func exitOf(t *testing.T, body func()) (request *exitRequest) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			typed, ok := recovered.(*exitRequest)
			if !ok {
				panic(recovered)
			}
			request = typed
		}
	}()
	body()
	return nil
}

func TestSynctexInstallsAtomicallyBesideThePDF(t *testing.T) {
	directory := t.TempDir()
	run := &synctexRun{pdf: filepath.Join(directory, "paper.pdf"), synctex: []byte("word")}
	if err := os.WriteFile(filepath.Join(directory, "paper.synctex.gz"), []byte("line"), 0o644); err != nil {
		t.Fatal(err)
	}
	if name := run.install(); name != "paper.synctex.gz" {
		t.Fatalf("installed %q", name)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 1 || string(readFile(filepath.Join(directory, "paper.synctex.gz"))) != "word" {
		t.Fatalf("the directory holds %v", entries)
	}
}

// --replace-pdf keeps the first PDF written after the start, installs relatex's pair over it, and
// installs nothing (exit status 4) over a PDF a newer build wrote meanwhile.
func TestSynctexReplacesOnlyThePDFItWaitedFor(t *testing.T) {
	for _, newer := range []bool{false, true} {
		directory := t.TempDir()
		pdf := filepath.Join(directory, "paper.pdf")
		run := &synctexRun{started: time.Now().Add(-time.Second), input: directory, pdf: pdf, wait: 5 * time.Second,
			replace: true, ours: []byte("%PDF-lualatex"), synctex: []byte("word")}
		run.watchPDF()
		writeTestFile(t, pdf, "%PDF-xelatex")
		writeTestFile(t, filepath.Join(directory, "paper.synctex.gz"), "line")
		seen := run.seenPDF()
		if newer {
			writeTestFile(t, pdf, "%PDF-newer xelatex")
		}
		var name string
		request := exitOf(t, func() { name = run.replacePair(seen) })
		gotPDF, gotSynctex := string(readFile(pdf)), string(readFile(filepath.Join(directory, "paper.synctex.gz")))
		switch {
		case !newer && (request != nil || name != "paper.pdf and paper.synctex.gz" || gotPDF != "%PDF-lualatex" ||
			gotSynctex != "word"):
			t.Fatalf("replace: %+v %q %q %q", request, name, gotPDF, gotSynctex)
		case newer && (request == nil || request.status != 4 || gotPDF != "%PDF-newer xelatex" || gotSynctex != "line" ||
			!strings.HasPrefix(request.stderr, "relatex synctex: superseded: paper.pdf changed")):
			t.Fatalf("a newer PDF must stay: %+v %q %q", request, gotPDF, gotSynctex)
		}
		if entries, _ := os.ReadDir(directory); len(entries) != 2 {
			t.Fatalf("the directory holds %v", entries)
		}
	}
}

func writeTestFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
