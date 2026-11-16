package a11ylint

import (
	"github.com/sarielhp/relatex/internal/authordir"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// decisionsSource has a language and a table item, both suggestions the patch can carry.
const decisionsSource = "\\documentclass{article}\n\\usepackage{booktabs}\n\\begin{document}\n" +
	"\\begin{tabular}{cc}\n\\toprule\na & b \\\\\n1 & 2 \\\\\n\\bottomrule\n\\end{tabular}\n\\end{document}\n"

// pageCheckbox is a review page checkbox with the item id and sha the page shows.
var pageCheckbox = regexp.MustCompile(`data-item="([^"]+)" data-sha="([0-9a-f]{64})"`)

// decisionsPaper is the synthetic paper with the page's ticked choices: each box's id and sha,
// written as the page's script writes them.
func decisionsPaper(t *testing.T) (Paper, []Item, string, []Decision) {
	t.Helper()
	directory := writePaper(t, map[string]string{"main.tex": decisionsSource})
	paper := patchPaper(t, directory)
	items := Lint(paper, Options{})
	page, _ := ReviewPage(paper, items, "")
	decisions := []Decision{}
	for _, match := range pageCheckbox.FindAllStringSubmatch(page, -1) {
		decisions = append(decisions, Decision{ID: match[1], SHA: match[2]})
	}
	return paper, items, page, decisions
}

// decisionsFile writes decisions in the review page's own format and reads them back.
func decisionsFile(t *testing.T, decisions []Decision) []Decision {
	t.Helper()
	var out strings.Builder
	out.WriteString("[review]\nsaved = \"2026-11-10\"\n")
	for _, decision := range decisions {
		out.WriteString("\n[[accept]]\nid = \"" + decision.ID + "\"\nsha = \"" + decision.SHA + "\"\n")
	}
	path := filepath.Join(t.TempDir(), DecisionsFile)
	if err := authordir.Ensure(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(out.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	read, err := ReadDecisions(path)
	if err != nil {
		t.Fatal(err)
	}
	return read
}

// TestDecisionsFileCarriesOnlyTheTickedItem: the page shows a checkbox with an id and sha for each
// item the patch can carry; the ticked one alone goes in the patch.
func TestDecisionsFileCarriesOnlyTheTickedItem(t *testing.T) {
	paper, items, page, decisions := decisionsPaper(t)
	if len(decisions) != 2 {
		t.Fatalf("%d checkboxes on the page, want 2", len(decisions))
	}
	if !strings.Contains(page, "Apply this suggestion") || strings.Count(page, "<script") != 1 {
		t.Error("the page does not offer the suggestions with its one script")
	}
	var ticked Decision
	for _, decision := range decisions {
		if strings.HasSuffix(decision.ID, ":table") {
			ticked = decision
		}
	}
	if ticked.ID == "" {
		t.Fatalf("no table checkbox among %v", decisions)
	}
	accepted := Accept(paper, items, decisionsFile(t, []Decision{ticked}))
	if len(accepted.Refused) != 0 || len(accepted.Items) != 1 || accepted.Items[0].Kind != "table" {
		t.Fatalf("accepted %v, refused %v", summary(accepted.Items), accepted.Refused)
	}
	diff, err := Patch(paper, accepted.Items)
	if err != nil {
		t.Fatal(err)
	}
	if edits := strings.Count(diff, "\n+") - strings.Count(diff, "\n+++"); edits != 1 || strings.Contains(diff, "babel") {
		t.Errorf("%d edits, want the table's one:\n%s", edits, diff)
	}
}

// TestDecisionsRefuseAStaleSHA: a source line that changed after the page was built is refused.
func TestDecisionsRefuseAStaleSHA(t *testing.T) {
	paper, items, _, decisions := decisionsPaper(t)
	stale := Decision{ID: decisions[0].ID, SHA: strings.Repeat("0", 64)}
	accepted := Accept(paper, items, []Decision{stale})
	if len(accepted.Items) != 0 || len(accepted.Refused) != 1 ||
		!strings.Contains(accepted.Refused[0].Reason, "changed since the page was built") {
		t.Errorf("accepted %v, refused %v", summary(accepted.Items), accepted.Refused)
	}
}

// TestDecisionsRefuseAnUnknownID: an id the current lint no longer has is refused, not dropped.
func TestDecisionsRefuseAnUnknownID(t *testing.T) {
	paper, items, _, decisions := decisionsPaper(t)
	unknown := Decision{ID: "main.tex:99:table", SHA: decisions[0].SHA}
	accepted := Accept(paper, items, []Decision{unknown})
	if len(accepted.Items) != 0 || len(accepted.Refused) != 1 ||
		!strings.Contains(accepted.Refused[0].Reason, "no item with this id") {
		t.Errorf("accepted %v, refused %v", summary(accepted.Items), accepted.Refused)
	}
}

// TestDecisionsRefuseAFigureSidecarTable: a decisions file holding a [figs.*] table is a mix-up
// with the figure sidecar and is refused, as a review file read as the sidecar is.
func TestDecisionsRefuseAFigureSidecarTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), DecisionsFile)
	body := "[review]\nsaved = \"2026-11-10\"\n\n[figs.plot]\nalt = \"A plot.\"\n"
	if err := authordir.Ensure(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDecisions(path); err == nil || !strings.Contains(err.Error(), "figure sidecar table") {
		t.Fatalf("a [figs.*] table in a decisions file: %v", err)
	}
}
