package pipeline

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The default output root is the paper's own directory (the amendment of 2026-11-16 to decision 191):
// nothing but the requested entries may appear there.

// rootRun is a finished conversion's product and the conversion that delivers it.
func rootRun(t *testing.T, paper, output string, formats ...string) *conversion {
	t.Helper()
	product := t.TempDir()
	putFile(t, product, "markdown/document.md", "first")
	putFile(t, product, "markdown/export.json", "receipt")
	putFile(t, product, "html/index.html", "page")
	putFile(t, product, "html/figs/a.svg", "figure")
	putFile(t, product, "pdf-ua/paper.pdf", "tagged")
	putFile(t, product, "paper.epub", "package")
	return &conversion{input: paper, output: output, product: product, pdf: "/x/paper.pdf",
		options: Options{Formats: formats, Records: filepath.Join(t.TempDir(), "outputs")}}
}

// listing is the names in a directory, hidden ones included.
func listing(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	return names
}

func failsWith(run func()) (message string) {
	defer func() {
		if value := recover(); value != nil {
			message = failureMessage(value)
		}
	}()
	run()
	return ""
}

func TestDefaultRootHoldsOnlyTheRequestedEntryAndASecondRunReplacesIt(t *testing.T) {
	paper := t.TempDir()
	putFile(t, paper, "main.tex", "\\documentclass{article}")
	c := rootRun(t, paper, paper, "markdown")
	c.deliver()
	if got, want := listing(t, paper), []string{"main.tex", "markdown"}; !slices.Equal(got, want) {
		t.Fatalf("the paper directory holds %v, want %v", got, want)
	}
	if present(paper, "markdown/export.json") {
		t.Error("a step receipt was delivered")
	}
	// A second conversion replaces it, and drops what the new one no longer writes.
	putFile(t, c.product, "markdown/document.md", "second")
	putFile(t, paper, "markdown/leftover.md", "of the first run")
	c.deliver()
	data, _ := os.ReadFile(filepath.Join(paper, "markdown/document.md"))
	if string(data) != "second" || present(paper, "markdown/leftover.md") {
		t.Errorf("the second run did not replace the entry: %q", data)
	}
	if got := listing(t, paper); !slices.Equal(got, []string{"main.tex", "markdown"}) {
		t.Errorf("after the second run: %v", got)
	}
}

func TestAUserEntryOfTheRequestedNameStopsTheRunAndSurvives(t *testing.T) {
	paper := t.TempDir()
	putFile(t, paper, "html/mine.html", "the author's own")
	c := rootRun(t, paper, paper, "html")
	message := failsWith(c.checkOutputEarly)
	if !strings.Contains(message, "html") || !strings.Contains(message, paper) || !strings.Contains(message, "-o") {
		t.Errorf("message %q must name the entry and -o", message)
	}
	if message := failsWith(func() { c.deliver() }); message == "" {
		t.Error("delivery replaced an entry relatex did not write")
	}
	if data, err := os.ReadFile(filepath.Join(paper, "html/mine.html")); err != nil || string(data) != "the author's own" {
		t.Errorf("the user's html/ did not survive: %q, %v", data, err)
	}
	if got := listing(t, paper); !slices.Equal(got, []string{"html"}) {
		t.Errorf("the paper directory holds %v", got)
	}
}

func TestAFailedRunLeavesNothingInThePaperDirectory(t *testing.T) {
	paper := t.TempDir()
	putFile(t, paper, "main.tex", "x")
	c := rootRun(t, paper, paper, "markdown", "html")
	c.deliver()
	if !setAsideNames(c.output, c.outputRecords(), c.owned(), "LaTeX Error: boom") {
		t.Fatal("no failure note was written")
	}
	if got := listing(t, paper); !slices.Equal(got, []string{"main.tex"}) {
		t.Errorf("a failed run leaves %v in the paper directory", got)
	}
	records := c.outputRecords()
	for _, kept := range []string{"_stale/markdown/document.md", "_stale/html/index.html", failureNote} {
		if !present(records, kept) {
			t.Errorf("%s is not in the project cache's records", kept)
		}
	}
	// The next success clears the leftovers.
	c.deliver()
	if present(records, "_stale") || present(records, failureNote) {
		t.Error("a successful run left the failure behind")
	}
}

func TestTheTaggedPDFNeverReplacesTheAuthorsPDF(t *testing.T) {
	paper := t.TempDir()
	putFile(t, paper, "paper.pdf", "built by rlx")
	c := rootRun(t, paper, paper, "pdf-ua", "epub")
	c.exports = nil
	c.deliver()
	if data, _ := os.ReadFile(filepath.Join(paper, "paper.pdf")); string(data) != "built by rlx" {
		t.Errorf("the author's PDF was replaced: %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(paper, "paper-tagged.pdf")); string(data) != "tagged" {
		t.Errorf("paper-tagged.pdf = %q", data)
	}
	if !present(paper, "paper.epub") {
		t.Error("the EPUB is missing")
	}
	if present(paper, ".relatex-output.json") {
		t.Error("a hidden marker is in the paper directory")
	}
}

func TestAnExplicitOutputDirectoryKeepsItsLayoutAndHoldsNoRecords(t *testing.T) {
	paper := t.TempDir()
	putFile(t, paper, "main.tex", "x")
	output := filepath.Join(t.TempDir(), "reading")
	c := rootRun(t, paper, output, "markdown", "html")
	c.deliver()
	if got, want := listing(t, output), []string{"html", "markdown"}; !slices.Equal(got, want) {
		t.Errorf("-o DIR holds %v, want %v (no hidden marker, no records)", got, want)
	}
	if c.holdsInput() {
		t.Error("an output directory elsewhere holds the input")
	}
	if got := c.outputExclusions(); !slices.Equal(got, []string{output}) {
		t.Errorf("exclusions %v: a separate output root is excluded whole", got)
	}
	if got := listing(t, paper); !slices.Equal(got, []string{"main.tex"}) {
		t.Errorf("the paper directory holds %v", got)
	}
}

func TestTheSnapshotLeavesOutOnlyWhatRelatexWroteInThePaperDirectory(t *testing.T) {
	paper := t.TempDir()
	putFile(t, paper, "main.tex", "x")
	c := rootRun(t, paper, paper, "markdown")
	c.deliver()
	excluded := c.outputExclusions()
	if slices.Contains(excluded, paper) || !slices.Contains(excluded, filepath.Join(paper, "markdown")) {
		t.Errorf("exclusions %v: the paper itself must stay, markdown/ must go", excluded)
	}
}

func TestAnEarlierLayoutOutputDirectoryIsLeftAlone(t *testing.T) {
	paper := t.TempDir()
	putFile(t, paper, "output/manifest.json", "{}")
	putFile(t, paper, "output/index.html", "old")
	c := rootRun(t, paper, paper, "markdown")
	c.deliver()
	for _, kept := range []string{"output/manifest.json", "output/index.html"} {
		if !present(paper, kept) {
			t.Errorf("%s of the earlier layout was touched", kept)
		}
	}
	if !slices.Contains(c.outputExclusions(), filepath.Join(paper, "output")) {
		t.Error("the earlier output/ would be read as source")
	}
	// A manifest.json of the author's own never makes the whole directory relatex's.
	putFile(t, paper, "manifest.json", "{}")
	putFile(t, paper, "notes.txt", "mine")
	if owned := c.owned(); slices.Contains(owned, "notes.txt") || slices.Contains(owned, "manifest.json") {
		t.Errorf("owned %v", owned)
	}
}
