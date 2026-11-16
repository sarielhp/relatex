package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// rlx builds no product: its run's roots, which the source snapshot excludes, must not hold the paper's
// directory (an empty product expanded to the current directory, and rlx paper.tex found no .tex file).
func TestProductsOnlyRootsLeaveThePaperOut(t *testing.T) {
	paper := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	saved, _ := os.Getwd()
	if err := os.Chdir(paper); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(saved) }()
	places := conversionPlaces(map[string]any{"work_mode": "cache", "output": "output"}, paper, true)
	resolved, _ := filepath.EvalSymlinks(paper)
	for _, root := range places.roots {
		if root == "" || root == paper || root == resolved || slices.Contains([]string{".", saved}, root) {
			t.Fatalf("roots %v hold the paper's directory %s", places.roots, paper)
		}
	}
}

// The default output root is the paper's directory: it is never a workspace, "work": "output" cannot put
// _product there, and the product stays in the project cache. An output directory elsewhere keeps its
// _product.
func TestPaperDirectoryOutputIsNotAWorkspace(t *testing.T) {
	paper := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	resolved, _ := filepath.EvalSymlinks(paper)
	places := conversionPlaces(map[string]any{"work_mode": "output", "output": paper}, paper, false)
	if !places.cached || places.scratch == "" || slices.Contains(places.roots, resolved) || slices.Contains(places.roots, paper) {
		t.Errorf("the paper directory is a workspace: %+v", places)
	}
	if got := defaultProduct(paper, places.scratch); filepath.Dir(got) != places.scratch {
		t.Errorf("product %s is not in the project cache %s", got, places.scratch)
	}
	elsewhere := filepath.Join(t.TempDir(), "reading")
	other := conversionPlaces(map[string]any{"work_mode": "output", "output": elsewhere}, paper, false)
	if other.cached || !slices.Contains(other.roots, other.workspace) {
		t.Errorf("an explicit output directory lost its workspace: %+v", other)
	}
}

func TestOutputHoldsInput(t *testing.T) {
	paper := t.TempDir()
	if !outputHoldsInput(paper, paper) || !outputHoldsInput(filepath.Dir(paper), paper) {
		t.Error("the paper's directory and its parents hold the paper")
	}
	if outputHoldsInput(filepath.Join(paper, "html"), paper) || outputHoldsInput(t.TempDir(), paper) {
		t.Error("a directory below or beside the paper holds it")
	}
}

// -o . names the paper's directory as the default does: a relative root is resolved before it is compared.
func TestRelativeOutputRootHoldsThePaper(t *testing.T) {
	paper := t.TempDir()
	saved, _ := os.Getwd()
	if err := os.Chdir(paper); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(saved) }()
	if !outputHoldsInput(".", paper) {
		t.Error(`-o . is not the paper's directory`)
	}
	if outputHoldsInput("elsewhere", paper) {
		t.Error("a new subdirectory holds the paper")
	}
}
