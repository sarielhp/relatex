package reading

import "testing"

// TestHyperrefLinkTextNamesTheNumberBareOrParenthesized: the book 0003 links equations with
// \hyperref[label]{Eq.~(\ref*{label})} ("Eq. (30.2)") and {(\ref*{label})} ("(30.2)"); a number
// glued to other text, a different number or a prefix of a longer one is still no use.
func TestHyperrefLinkTextNamesTheNumberBareOrParenthesized(t *testing.T) {
	for printed, want := range map[string]bool{
		"Lemma 2.1": true, "2.1": true, "Eq. (2.1)": true, "(2.1)": true,
		"Eq.(2.1)": false, "Eq. (2.10)": false, "(2.1) more": false, "Lemma 2.10": false, "Eq. 2.1)": false,
	} {
		if got := printedUse(`\hyperref`, printed, "2.1"); got != want {
			t.Errorf("%q: %v, want %v", printed, got, want)
		}
	}
	if printedUse(`\@firstoffive`, "(2.1)", "2.1") {
		t.Error(`\ref prints the number alone`)
	}
}

// TestHyperrefLinkTextWithoutANumberIsTheAuthorsName: \hyperref[label]{$c$-packed} prints a
// name, not the number: it links as printed, while an empty text or one printing another number
// does not.
func TestHyperrefLinkTextWithoutANumberIsTheAuthorsName(t *testing.T) {
	for printed, want := range map[string]bool{
		"-packed": true, "dense set": true, "(ii)": true, "c": true, "": false, "the 2 cases": false, "Step 3": false,
	} {
		if got := printedUse(`\hyperref`, printed, "2.1"); got != want {
			t.Errorf("%q: %v, want %v", printed, got, want)
		}
	}
	if printedUse(`\@firstoffive`, "dense", "2.1") {
		t.Error(`\ref prints the number, never a name`)
	}
}
