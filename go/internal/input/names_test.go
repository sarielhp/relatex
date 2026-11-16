package input

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// D9 (decision 129): the names NTFS cannot store are refused on Windows with input.unsafe,
// naming the file; nothing is renamed. Unix stores them, so it keeps them.
func TestUnstorableNamesAreTheOnesNTFSRefuses(t *testing.T) {
	refused := []string{"aux.tex", "AUX", "con.sty", "Nul.bib", "com1.txt", "lpt9", "a<b.tex", "fig?.pdf", `say"hi".tex`,
		"pipe|.tex", "star*.tex", "def_median:estimator.tex", "trailing.", "space "}
	for _, name := range refused {
		if unstorable(name) == "" {
			t.Errorf("%q is storable, want refused", name)
		}
	}
	for _, name := range []string{"auxiliary.tex", "aux-notes.tex", "console.sty", "com10.tex", "main.tex", ".hidden", "a b.tex"} {
		if reason := unstorable(name); reason != "" {
			t.Errorf("%q refused: %s", name, reason)
		}
	}
}

func TestRefuseUnstorableNamesTheFile(t *testing.T) {
	previous := storableOnly
	defer func() { storableOnly = previous }()
	storableOnly = false
	refuseUnstorable("sub/aux.tex", []string{"sub", "aux.tex"})
	storableOnly = true
	defer func() {
		failure, ok := recover().(rec.Failure)
		if !ok || !strings.HasPrefix(failure.Message, "input.unsafe: ") || !strings.Contains(failure.Message, "sub/aux.tex") {
			t.Fatalf("failure %+v", failure)
		}
	}()
	refuseUnstorable("sub/aux.tex", []string{"sub", "aux.tex"})
	t.Fatal("aux.tex was accepted")
}
