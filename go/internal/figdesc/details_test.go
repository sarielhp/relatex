package figdesc

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// An entry's details (decision 125) round-trip, and the strict reader refuses details without the
// short description they extend, naming the line.
func TestDetailsRoundTripAndNeedAShortDescription(t *testing.T) {
	sidecar, err := Parse("[figs.a]\nalt = \"A tree.\"\nlong = \"The root splits.\"\ndetails = \"Step 1 cuts.\"\nprovenance = \"model\"\n")
	if err != nil || sidecar.Entries["a"].Details != "Step 1 cuts." {
		t.Fatalf("parse: %v %+v", err, sidecar)
	}
	if err = sidecar.PutDraft(Entry{Key: "a", Alt: "A tree.", Long: "Split.", Details: "Two steps."}); err != nil {
		t.Fatal(err)
	}
	if text := string(sidecar.Bytes()); !strings.Contains(text, "long = \"Split.\"\ndetails = \"Two steps.\"\nprovenance") {
		t.Errorf("rendered draft:\n%s", text)
	}
	_, err = Parse("[figs.a]\nalt = \"A tree.\"\ndetails = \"Alone.\"\n")
	if err == nil || !strings.Contains(err.Error(), "line 3") || !strings.Contains(err.Error(), "also has long") {
		t.Errorf("details without long: %v", err)
	}
}

// Set keeps details beside a short description: writing details without one, or removing the short
// description under details, is refused and changes nothing; SetTexts orders a joint change.
func TestSetKeepsDetailsBesideAShortDescription(t *testing.T) {
	sidecar, err := Parse("[figs.a]\nalt = \"A tree.\"\n")
	if err != nil {
		t.Fatal(err)
	}
	before := string(sidecar.Bytes())
	if err := sidecar.Set("a", "details", "Step 1."); err == nil || string(sidecar.Bytes()) != before {
		t.Fatalf("details without long: %v\n%s", err, sidecar.Bytes())
	}
	if err := sidecar.Set("new", "details", "Step 1."); err == nil {
		t.Fatal("details for a figure without an entry")
	}
	if err := sidecar.SetTexts("a", map[string]string{"details": "Step 1.", "long": "The root splits."}); err != nil {
		t.Fatal(err)
	}
	if entry := sidecar.Entries["a"]; entry.Long != "The root splits." || entry.Details != "Step 1." {
		t.Fatalf("entry %+v", entry)
	}
	if err := sidecar.Set("a", "long", ""); err == nil || sidecar.Entries["a"].Long == "" {
		t.Fatalf("removing long under details: %v", err)
	}
	if err := sidecar.SetTexts("a", map[string]string{"details": "", "long": ""}); err != nil {
		t.Fatal(err)
	}
	if entry := sidecar.Entries["a"]; entry.Long != "" || entry.Details != "" || strings.Contains(string(sidecar.Bytes()), "details") {
		t.Errorf("after removing both: %+v\n%s", entry, sidecar.Bytes())
	}
}

// The review file carries details and the details the page showed: an edit to them is the
// author's, a changed sidecar text refuses the decision, and details without a short one are refused.
func TestImportTakesDetails(t *testing.T) {
	sidecar, err := Parse("[figs.a]\nalt = \"A tree.\"\nlong = \"Split.\"\ndetails = \"Old.\"\nprovenance = \"model\"\n" +
		"[figs.b]\nalt = \"B.\"\nprovenance = \"model\"\n[figs.c]\nalt = \"C.\"\nprovenance = \"model\"\n")
	if err != nil {
		t.Fatal(err)
	}
	review := "[review]\n[figs.a]\nalt = \"A tree.\"\nlong = \"Split.\"\ndetails = \"New.\"\nshown = \"A tree.\"\nshown_long = \"Split.\"\n" +
		"shown_details = \"Old.\"\n[figs.b]\nalt = \"B.\"\nlong = \"\"\ndetails = \"Alone.\"\nshown = \"B.\"\nshown_long = \"\"\nshown_details = \"\"\n" +
		"[figs.c]\nalt = \"C.\"\nshown = \"C.\"\nshown_details = \"Was there.\"\n"
	path := filepath.Join(t.TempDir(), "review.toml")
	if err = os.WriteFile(path, []byte(review), 0o644); err != nil {
		t.Fatal(err)
	}
	decisions, err := ReadReview(path)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := sidecar.Import(decisions, "2026-11-11")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(outcome.Edited, []string{"a"}) || outcome.Refused["b"] == "" || outcome.Refused["c"] == "" {
		t.Fatalf("outcome %+v", outcome)
	}
	if entry := sidecar.Entries["a"]; entry.Details != "New." || entry.Provenance != Author {
		t.Errorf("entry a %+v", entry)
	}
}
