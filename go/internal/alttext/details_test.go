package alttext

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/rec"
)

// A describer's answer is the alternative, the short description and, in a paragraph that starts
// with "Details:", the details (decision 125); details given without a short description are it.
func TestSplitDescriptionReadsTheDetailsParagraph(t *testing.T) {
	cases := []struct {
		answer string
		want   Description
	}{
		{"A tree T*.\n\nThe root splits.\n\nDetails: Step 1 cuts.\n\nStep 2 joins.",
			Description{Alt: "A tree T*.", Long: "The root splits.", Details: "Step 1 cuts. Step 2 joins."}},
		{"A tree.\n\nThe root\n splits.", Description{Alt: "A tree.", Long: "The root splits."}},
		{"A tree.\n\nDetails: Only details.", Description{Alt: "A tree.", Long: "Only details."}},
		{"A tree.", Description{Alt: "A tree."}},
	}
	for _, test := range cases {
		if got := SplitDescription(test.answer); got != test.want {
			t.Errorf("split %q: %+v, want %+v", test.answer, got, test.want)
		}
	}
}

// A draft keeps its details in the sidecar; the review page shows them and lets the author edit them.
func TestDrafterKeepsDetailsAndTheReviewPageEditsThem(t *testing.T) {
	script, _ := stub(t, `cat >/dev/null; printf 'A tree T*.\n\nThe root splits.\n\nDetails: Step 1 cuts the tree.\n'`)
	document := figureDocument()
	path := filepath.Join(t.TempDir(), figdesc.FileName)
	sidecar, err := figdesc.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	drafter := &Drafter{Command: shellCommand(t, script), Model: "stub", Timeout: 10 * time.Second, Today: "2026-11-11", Limit: 1}
	if _, err = drafter.Run(document, sidecar); err != nil {
		t.Fatal(err)
	}
	reread, err := figdesc.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	entry := reread.Entries["disk"]
	if entry == nil || entry.Long != "The root splits." || entry.Details != "Step 1 cuts the tree." || entry.Prompt != PromptVersion {
		t.Fatalf("drafted entry %+v", entry)
	}
	page := ReviewPage(figdesc.Figures(document), reread)
	for _, want := range []string{"<dt>Details</dt><dd>Step 1 cuts the tree.</dd>",
		`class="details" rows="4" data-shown="Step 1 cuts the tree.">Step 1 cuts the tree.</textarea>`,
		"'details = '+q(details.value)", "'shown_details = '+q(details.dataset.shown)"} {
		if !strings.Contains(page, want) {
			t.Errorf("the review page lacks %q", want)
		}
	}
}

// Details over the limit are asked for again once, like an over-long alternative.
func TestDrafterAsksAgainForOverlongDetails(t *testing.T) {
	words := strings.Repeat("word ", 210)
	script, count := stub(t, `prompt=$(cat); case "$prompt" in *"Shorten to the limits"*) printf 'A disk.\n\nAround p.\n\nDetails: Short.\n';;
*) printf 'A disk.\n\nAround p.\n\nDetails: `+words+`\n';; esac`)
	sidecar, err := figdesc.Read(filepath.Join(t.TempDir(), figdesc.FileName))
	if err != nil {
		t.Fatal(err)
	}
	drafter := &Drafter{Command: shellCommand(t, script), Model: "stub", Timeout: 10 * time.Second, Today: "2026-11-11", Limit: 1}
	outcome, err := drafter.Run(figureDocument(), sidecar)
	if err != nil || len(outcome.TooLong) != 0 || runs(t, count) != 2 || sidecar.Entries["disk"].Details != "Short." {
		t.Fatalf("outcome %+v %v, %d runs, entry %+v", outcome, err, runs(t, count), sidecar.Entries["disk"])
	}
}

// Apply carries details into the image span beside the long description, and never without one.
func TestApplyCarriesDetailsBesideTheShortDescription(t *testing.T) {
	document := figureDocument()
	sidecar, err := figdesc.Parse("[figs.disk]\nalt = \"A disk.\"\nlong = \"Around p.\"\ndetails = \"Built in two steps.\"\n")
	if err != nil {
		t.Fatal(err)
	}
	figdesc.Apply(document, sidecar)
	disk := rec.AsMap(rec.FetchList(rec.AsMap(rec.FetchMap(document, "nodes")["p2"]), "content")[0])
	if disk["long_description"] != "Around p." || disk[figdesc.DetailsField] != "Built in two steps." {
		t.Errorf("disk span %v", disk)
	}
	if figure := figdesc.Figures(document)[0]; figure.Details() != "Built in two steps." {
		t.Errorf("figure details %q", figure.Details())
	}
}
