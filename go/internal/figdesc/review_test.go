package figdesc

import (
	"github.com/sarielhp/relatex/internal/authordir"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const reviewedSidecar = `# my comment
[figs.accepted]
alt = "A drafted text."
provenance = "model"
model = "describer"
sha256 = "aaa"
drafted = "2026-11-01"

[figs.edited]
alt = "Another draft."
provenance = "model"
sha256 = "bbb"

[figs.mine]
alt = "My own words."

[figs.redrafted]
alt = "A newer draft."
provenance = "model"

[figs.untouched]
alt = "Left as a draft."
provenance = "model"
`

// The review file holds only the figures the author ticked; each records the text the page showed.
const reviewFile = `[review]
saved = "2026-11-02"

[figs."accepted"]
alt = "A drafted text."
long = ""
shown = "A drafted text."
shown_long = ""
sha256 = "aaa"

[figs."edited"]
alt = "A corrected text."
long = "With a long description."
shown = "Another draft."
shown_long = ""
sha256 = "bbb"

[figs."mine"]
alt = "My own words."
long = ""
shown = "My own words."
shown_long = ""
sha256 = ""

[figs."redrafted"]
alt = "The old draft."
long = ""
shown = "The old draft."
shown_long = ""
sha256 = ""

[figs."new"]
alt = "Written on the page."
long = ""
shown = ""
shown_long = ""
sha256 = "ccc"

[figs."empty"]
alt = ""
long = ""
shown = ""
shown_long = ""
sha256 = ""
`

func TestImportMakesOnlyTheAuthorsDecisionsReviewed(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "review.toml")
	if err := os.WriteFile(path, []byte(reviewFile), 0o644); err != nil {
		t.Fatal(err)
	}
	decisions, err := ReadReview(path)
	if err != nil || len(decisions) != 6 {
		t.Fatalf("ReadReview: %v %v", decisions, err)
	}
	sidecar, err := Parse(reviewedSidecar)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := sidecar.Import(decisions, "2026-11-02")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(outcome.Accepted, []string{"accepted"}) || !slices.Equal(outcome.Edited, []string{"edited", "new"}) ||
		!slices.Equal(outcome.Unchanged, []string{"mine"}) || len(outcome.Refused) != 2 || outcome.Refused["redrafted"] == "" ||
		outcome.Refused["empty"] == "" {
		t.Fatalf("outcome %+v", outcome)
	}
	want := map[string]string{"accepted": Reviewed, "edited": Author, "mine": Author, "new": Author, "redrafted": Model, "untouched": Model}
	for key, provenance := range want {
		if got := sidecar.Entries[key]; got == nil || got.Provenance != provenance {
			t.Errorf("%s: %+v, want provenance %s", key, got, provenance)
		}
	}
	edited := sidecar.Entries["edited"]
	if edited.Alt != "A corrected text." || edited.Long != "With a long description." || edited.Reviewed != "2026-11-02" || edited.SHA256 != "bbb" {
		t.Errorf("edited entry %+v", edited)
	}
	if fresh := sidecar.Entries["new"]; fresh.SHA256 != "ccc" || fresh.Alt != "Written on the page." {
		t.Errorf("new entry %+v", fresh)
	}
	if accepted := sidecar.Entries["accepted"]; accepted.Model != "describer" || accepted.Drafted != "2026-11-01" {
		t.Errorf("the draft's history was lost: %+v", accepted)
	}
	if text := string(sidecar.Bytes()); !strings.HasPrefix(text, "# my comment\n") {
		t.Errorf("the author's comment was lost:\n%s", text)
	}
}

func TestReviewFileIsNotASidecar(t *testing.T) {
	if _, err := Parse(reviewFile); err == nil {
		t.Fatal("a review file read as the sidecar")
	}
	path := filepath.Join(t.TempDir(), FileName)
	if err := authordir.Ensure(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(reviewedSidecar), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReview(path); err == nil || !strings.Contains(err.Error(), "not a review file") {
		t.Fatalf("a sidecar read as a review file: %v", err)
	}
}

func TestAdoptMarksAnEntryWithoutProvenance(t *testing.T) {
	sidecar, err := Parse("[figs.a]\nalt = \"Old.\"\nreviewed = \"2026-01-01\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := sidecar.Adopt(Entry{Key: "a", Alt: "New."}, "2026-11-02"); err != nil {
		t.Fatal(err)
	}
	if entry := sidecar.Entries["a"]; entry.Alt != "New." || entry.Provenance != Author || entry.Reviewed != "2026-11-02" {
		t.Fatalf("adopted %+v\n%s", entry, sidecar.Bytes())
	}
}
