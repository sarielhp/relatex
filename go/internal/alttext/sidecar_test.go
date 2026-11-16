package alttext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/figdesc"
)

const authored = `# My descriptions.

[figs.disk]
# written by hand
alt = "The disk of radius r around p."

[figs."plot.v2"]
alt = """
A plot of f over [0, 1]:
it rises \"steeply\"."""
provenance = 'model'
model = "stub"
sha256 = "abc"

# trailing note
`

func TestParseReadsEntriesCommentsAndStrings(t *testing.T) {
	sidecar, err := figdesc.Parse(authored)
	if err != nil {
		t.Fatal(err)
	}
	disk := sidecar.Entries["disk"]
	if disk == nil || disk.Alt != "The disk of radius r around p." || disk.Provenance != figdesc.Author {
		t.Fatalf("disk entry %+v", disk)
	}
	plot := sidecar.Entries["plot.v2"]
	if plot == nil || plot.Provenance != figdesc.Model || plot.Model != "stub" || plot.SHA256 != "abc" {
		t.Fatalf("plot entry %+v", plot)
	}
	if want := "A plot of f over [0, 1]:\nit rises \"steeply\"."; plot.Alt != want {
		t.Errorf("multi-line alt %q, want %q", plot.Alt, want)
	}
	if got := string(sidecar.Bytes()); got != authored {
		t.Errorf("an unedited sidecar is not written back as it was:\n%s", got)
	}
}

func TestParseNamesTheLineOfAMistake(t *testing.T) {
	cases := map[string]string{
		"[figs.a]\nalt = \"x\"\nprovenence = \"model\"\n": "line 3: unknown key",
		"[figs.a]\nprovenance = \"ai\"\n":                 "line 2: provenance is",
		"[figs.a]\n[figs.a]\n":                            "line 2: table [figs.a] is defined twice",
		"alt = \"x\"\n":                                   "line 1: a setting outside a table",
		"[figs.a]\nalt = \"x\n":                           "line 2: a string is not closed",
		"[figs.a]\nalt = 3\n":                             "line 2: a value is a quoted string",
		"[pictures.a]\n":                                  "line 1: unknown table [pictures.a]",
		"[approval]\nupload = \"yes\"\n":                  "line 2: upload must be true or false",
		"[figs.a]\nalt = \"\\q\"\n":                       "line 2: unknown escape",
	}
	for text, want := range cases {
		if _, err := figdesc.Parse(text); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%q) = %v, want an error with %q", text, err, want)
		}
	}
}

func TestPutDraftNeverReplacesTheAuthorsText(t *testing.T) {
	sidecar, err := figdesc.Parse(authored)
	if err != nil {
		t.Fatal(err)
	}
	if err := sidecar.PutDraft(figdesc.Entry{Key: "disk", Alt: "drafted"}); err == nil {
		t.Fatal("a draft replaced the author's entry")
	}
	if err := sidecar.PutDraft(figdesc.Entry{Key: "plot.v2", Alt: "new draft", Model: "m2", SHA256: "def"}); err != nil {
		t.Fatal(err)
	}
	if err := sidecar.PutDraft(figdesc.Entry{Key: "fig-3", Alt: "third", Model: "m2", SHA256: "123"}); err != nil {
		t.Fatal(err)
	}
	text := string(sidecar.Bytes())
	for _, kept := range []string{"# My descriptions.", "# written by hand", "# trailing note", `alt = "The disk of radius r around p."`} {
		if !strings.Contains(text, kept) {
			t.Errorf("lost %q:\n%s", kept, text)
		}
	}
	if strings.Contains(text, "steeply") || !strings.Contains(text, `[figs."plot.v2"]`+"\n"+`alt = "new draft"`) {
		t.Errorf("the old draft was not replaced in place:\n%s", text)
	}
	if entry := sidecar.Entries["fig-3"]; entry == nil || entry.Provenance != figdesc.Model || entry.SHA256 != "123" {
		t.Errorf("new draft %+v", entry)
	}
	if entry := sidecar.Entries["disk"]; entry.Alt != "The disk of radius r around p." || entry.Provenance != figdesc.Author {
		t.Errorf("author entry changed: %+v", entry)
	}
}

// Set writes an entry's alt or long text in place (decision 110): the other lines, the comments and
// the provenance stay; an empty long text removes it; a new figure gets an author's entry.
func TestSetWritesOneTextOfAnEntry(t *testing.T) {
	sidecar, err := figdesc.Parse(authored)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range [][3]string{{"disk", "long", "Its  centre is\n p."}, {"plot.v2", "alt", "A new plot text."},
		{"new", "long", "Only long."}, {"disk", "alt", "The disk."}} {
		if err := sidecar.Set(step[0], step[1], step[2]); err != nil {
			t.Fatal(err)
		}
	}
	text := string(sidecar.Bytes())
	disk, plot, added := sidecar.Entries["disk"], sidecar.Entries["plot.v2"], sidecar.Entries["new"]
	if disk.Alt != "The disk." || disk.Long != "Its centre is p." || !strings.Contains(text, "# written by hand") {
		t.Errorf("disk %+v\n%s", disk, text)
	}
	if plot.Alt != "A new plot text." || plot.Provenance != figdesc.Model || plot.SHA256 != "abc" {
		t.Errorf("a draft keeps its provenance: %+v", plot)
	}
	if added == nil || added.Long != "Only long." || added.Provenance != figdesc.Author {
		t.Errorf("new entry %+v", added)
	}
	if err := sidecar.Set("disk", "long", ""); err != nil || sidecar.Entries["disk"].Long != "" || strings.Contains(string(sidecar.Bytes()), "centre") {
		t.Errorf("removing the long text: %v\n%s", err, sidecar.Bytes())
	}
	if err := sidecar.Set("disk", "provenance", "x"); err == nil {
		t.Error("Set wrote another field")
	}
}

func TestAcceptMakesADraftReviewed(t *testing.T) {
	sidecar, err := figdesc.Parse(authored)
	if err != nil {
		t.Fatal(err)
	}
	if done, err := sidecar.Accept("plot.v2", "2026-10-30"); err != nil || !done {
		t.Fatalf("accept: %v %v", done, err)
	}
	entry := sidecar.Entries["plot.v2"]
	if entry.Provenance != figdesc.Reviewed || entry.Reviewed != "2026-10-30" || !strings.Contains(entry.Alt, "steeply") {
		t.Errorf("accepted entry %+v", entry)
	}
	if done, err := sidecar.Accept("disk", "2026-10-30"); err != nil || done {
		t.Errorf("accepting the author's own entry: %v %v", done, err)
	}
	if _, err := sidecar.Accept("missing", "2026-10-30"); err == nil {
		t.Error("accepted an entry that does not exist")
	}
	if err := sidecar.PutDraft(figdesc.Entry{Key: "plot.v2", Alt: "x"}); err == nil {
		t.Error("a draft replaced a reviewed entry")
	}
}

func TestAcceptReplacesAnEarlierReviewDate(t *testing.T) {
	sidecar, err := figdesc.Parse("[figs.a]\nreviewed = \"2020-01-01\"\nalt = \"x\"\nprovenance = \"model\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sidecar.Accept("a", "2026-10-30"); err != nil {
		t.Fatal(err)
	}
	if got := string(sidecar.Bytes()); got != "[figs.a]\nalt = \"x\"\nprovenance = \"reviewed\"\nreviewed = \"2026-10-30\"\n" {
		t.Errorf("accepted twice:\n%s", got)
	}
}

func TestRecordApprovalAndWriteRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), figdesc.FileName)
	sidecar, err := figdesc.Read(path)
	if err != nil || sidecar.Exists() || len(sidecar.Entries) != 0 {
		t.Fatalf("missing file: %v %v", sidecar, err)
	}
	if err = sidecar.PutDraft(figdesc.Entry{Key: "a", Alt: "first \"quoted\"\tline", Model: "m", SHA256: "s"}); err != nil {
		t.Fatal(err)
	}
	if err = sidecar.RecordApproval(figdesc.Approval{Upload: true, Approved: "2026-10-30", Command: "describe"}); err != nil {
		t.Fatal(err)
	}
	if err = sidecar.Write(); err != nil {
		t.Fatal(err)
	}
	again, err := figdesc.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Approval.Upload || again.Approval.Command != "describe" || again.Entries["a"].Alt != "first \"quoted\"\tline" {
		t.Errorf("read back %+v %+v", again.Approval, again.Entries["a"])
	}
	text, _ := os.ReadFile(path)
	if strings.Index(string(text), "[approval]") > strings.Index(string(text), "[figs.a]") {
		t.Errorf("the approval does not open the file:\n%s", text)
	}
}
