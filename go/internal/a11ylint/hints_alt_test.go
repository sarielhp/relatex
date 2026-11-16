package a11ylint

import "testing"

// TestAltHintKnownLabel: an alt hint naming a label the paper writes raises nothing.
func TestAltHintKnownLabel(t *testing.T) {
	paper := hintPaper(t, "\\begin{figure}\n\\caption{A disk}\\label{fig:disk}\n\\end{figure}\n"+
		"%% relatex alt: fig:disk = \"A red disk\"\n")
	if item := hintOf(t, Lint(paper, Options{}), "a label the paper does not define"); item != nil {
		t.Errorf("a hint for an existing label was reported: %+v", item)
	}
}

// TestAltHintMissingLabel: an alt hint naming no label of the paper is a hint item.
func TestAltHintMissingLabel(t *testing.T) {
	paper := hintPaper(t, "\\begin{figure}\n\\caption{A disk}\\label{fig:disk}\n\\end{figure}\n"+
		"%% relatex alt: fig:missing = \"A red disk\"\n")
	item := hintOf(t, Lint(paper, Options{}), `alt hint for "fig:missing", a label the paper does not define`)
	if item == nil {
		t.Fatal("no item for an alt hint naming no label")
	}
	if item.Line != 4 {
		t.Errorf("line = %d, want 4", item.Line)
	}
}

// TestAltHintCommentedOutLabel: a \label the source comments out does not define it.
func TestAltHintCommentedOutLabel(t *testing.T) {
	paper := hintPaper(t, "% \\label{fig:gone}\n%% relatex alt: fig:gone = \"A red disk\"\n")
	if hintOf(t, Lint(paper, Options{}), "a label the paper does not define") == nil {
		t.Error("a commented-out label counted as defined")
	}
}
