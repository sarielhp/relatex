package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/authordir"
	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/ojson"
)

func TestAltTextSectionIsValidated(t *testing.T) {
	cases := map[string]string{
		`{"alt_text": {"command": ["describe", "--fast"], "uploads": false, "model": "m", "timeout_seconds": 30}}`: "",
		`{"alt_text": {"command": null}}`:                "",
		`{"alt_text": {"command": "describe"}}`:          `"alt_text.command" must be null or a list of strings (the program, then its arguments)`,
		`{"alt_text": {"command": []}}`:                  `"alt_text.command" must be null or a list of strings`,
		`{"alt_text": {"uploads": "no"}}`:                `"alt_text.uploads" must be true or false`,
		`{"alt_text": {"timeout_seconds": 0}}`:           `"alt_text.timeout_seconds" must be a number of seconds from 1 to 3600`,
		`{"alt_text": {"endpoints": "http://x"}}`:        `unknown setting "alt_text.endpoints"`,
		`{"alt_text": ["describe"]}`:                     `"alt_text" must be an object`,
		`{"alt_text": {"model": ""}}`:                    `"alt_text.model" must be null or a name`,
		`{"alt_text": {"timeout_seconds": 3601}}`:        `"alt_text.timeout_seconds" must be`,
		`{"alt_text": {"command": ["a", 3]}}`:            `"alt_text.command" must be`,
		`{"alt_text": {"uploads": true}, "x": 1}`:        `unknown setting "x"`,
		`{"alt_text": {"model": null, "uploads": true}}`: "",
	}
	for text, want := range cases {
		path := writeConfig(t, text)
		got := failure(func() { readConfig(path) })
		if want == "" && got != "" || want != "" && !strings.Contains(got, want) {
			t.Errorf("%s: got %q, want %q", text, got, want)
		}
	}
}

// TestPaperMayNotChooseTheDescriber: a paper from someone else must not run a program or declare
// its figures local; alt_text is read from the global file only.
func TestPaperMayNotChooseTheDescriber(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	paper := t.TempDir()
	project := `{"alt_text": {"command": ["rm", "-rf", "/"], "uploads": false}}`
	if err := authordir.Ensure(filepath.Join(paper, projectConfig)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paper, projectConfig), []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := failure(func() { loadConfig(paper) }); !strings.Contains(got, `"alt_text" may be set only in the global configuration`) {
		t.Errorf("a paper's alt_text was accepted: %q", got)
	}
	if strings.Contains(projectTemplate(), "alt_text") {
		t.Error("the project template offers alt_text")
	}
}

func TestAltTextSettingsFromConfig(t *testing.T) {
	settings := mergeSettings(configDefaults(), readConfig(writeConfig(t, `{"alt_text": {"command": ["~/bin/describe", "-q"]}}`)))
	got := altTextSettings(settings)
	home, _ := os.UserHomeDir()
	if !got.uploads || got.model != "describe" || got.command[0] != filepath.Join(home, "bin/describe") || got.command[1] != "-q" {
		t.Errorf("settings %+v", got)
	}
	if defaults := altTextSettings(configDefaults()); len(defaults.command) != 0 || !defaults.uploads {
		t.Errorf("defaults %+v: no command, uploads assumed", defaults)
	}
}

func TestApprovalIsAskedOnceAndRecorded(t *testing.T) {
	path := filepath.Join(t.TempDir(), figdesc.FileName)
	sidecar, err := figdesc.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	remote := altSettings{command: []string{"/usr/local/bin/describe"}, uploads: true}
	if got := failure(func() { approve(sidecar, remote, false) }); !strings.Contains(got, "--allow-upload") {
		t.Fatalf("an uploading command ran without approval: %q", got)
	}
	approve(sidecar, altSettings{command: []string{"local"}, uploads: false}, false)
	if sidecar.Exists() {
		t.Error("a local command recorded an approval")
	}
	approve(sidecar, remote, true)
	again, err := figdesc.Read(path)
	if err != nil || !again.Approval.Upload || again.Approval.Command != "describe" || again.Approval.Approved == "" {
		t.Fatalf("approval not recorded: %+v %v", again.Approval, err)
	}
	approve(again, remote, false)
}

// relatex alt import applies the review page's file: the accepted draft becomes reviewed, the
// draft the author did not tick stays a draft.
func TestAltImportTakesBackTheReviewFile(t *testing.T) {
	paper := t.TempDir()
	text := "[figs.a]\nalt = \"Drafted.\"\nprovenance = \"model\"\n\n[figs.b]\nalt = \"Not ticked.\"\nprovenance = \"model\"\n"
	review := "[review]\n[figs.\"a\"]\nalt = \"Drafted.\"\nlong = \"\"\nshown = \"Drafted.\"\nshown_long = \"\"\nsha256 = \"\"\n"
	saved := filepath.Join(paper, "saved.toml")
	for path, content := range map[string]string{filepath.Join(paper, figdesc.FileName): text, saved: review} {
		if err := authordir.Ensure(path); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	report := commandResult("alt", "import", saved, "--paper", paper)
	if accepted := ojson.Plain(report.Get("accepted")).([]any); len(accepted) != 1 || accepted[0] != "a" {
		t.Errorf("accepted %v", report.Get("accepted"))
	}
	sidecar, err := figdesc.Read(filepath.Join(paper, figdesc.FileName))
	if err != nil || sidecar.Entries["a"].Provenance != figdesc.Reviewed || sidecar.Entries["b"].Provenance != figdesc.Model {
		t.Errorf("after import: %+v %+v %v", sidecar.Entries["a"], sidecar.Entries["b"], err)
	}
	if got := failure(func() { commandResult("alt", "import", filepath.Join(paper, figdesc.FileName), "--paper", paper) }); !strings.Contains(got, "not a review file") {
		t.Errorf("importing the sidecar itself: %q", got)
	}
}

// relatex alt set writes details beside the short description in any flag order (decision 125)
// and refuses details for a figure without one.
func TestAltSetWritesDetailsBesideTheShortDescription(t *testing.T) {
	paper := t.TempDir()
	if got := failure(func() { commandResult("alt", "set", "a", "--details", "Step 1.", "--paper", paper) }); !strings.Contains(got, "write long first") {
		t.Fatalf("details without a short description: %q", got)
	}
	report := commandResult("alt", "set", "a", "--details", "Step 1.", "--long", "The root splits.", "--alt", "A tree.", "--paper", paper)
	if report.Get("details") != "Step 1." || report.Get("long") != "The root splits." {
		t.Errorf("report %v", report)
	}
	sidecar, err := figdesc.Read(filepath.Join(paper, figdesc.FileName))
	if err != nil || sidecar.Entries["a"].Details != "Step 1." {
		t.Fatalf("sidecar %+v %v", sidecar.Entries["a"], err)
	}
}

func TestAltAcceptMarksDraftsReviewed(t *testing.T) {
	paper := t.TempDir()
	text := "[figs.a]\nalt = \"Drafted.\"\nprovenance = \"model\"\n\n[figs.b]\nalt = \"Mine.\"\n"
	if err := authordir.Ensure(filepath.Join(paper, figdesc.FileName)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paper, figdesc.FileName), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	report := commandResult("alt", "accept", "--all", "--paper", paper)
	if accepted := ojson.Plain(report.Get("accepted")).([]any); len(accepted) != 1 || accepted[0] != "a" {
		t.Errorf("accepted %v", report.Get("accepted"))
	}
	sidecar, err := figdesc.Read(filepath.Join(paper, figdesc.FileName))
	if err != nil || sidecar.Entries["a"].Provenance != figdesc.Reviewed || sidecar.Entries["b"].Provenance != figdesc.Author {
		t.Errorf("after accept: %+v %+v %v", sidecar.Entries["a"], sidecar.Entries["b"], err)
	}
	if got := failure(func() { commandResult("alt", "accept", "--paper", paper, "missing") }); !strings.Contains(got, "no entry for figure missing") {
		t.Errorf("accepting an unknown figure: %q", got)
	}
}

// alt accept, set and import take the paper directory positionally, as alt draft and alt review do.
func TestAltDirectoryPositional(t *testing.T) {
	paper := t.TempDir()
	text := "[figs.a]\nalt = \"Drafted.\"\nprovenance = \"model\"\n"
	if err := authordir.Ensure(filepath.Join(paper, figdesc.FileName)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paper, figdesc.FileName), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	commandResult("alt", "set", "a", paper, "--long", "A longer text.")
	report := commandResult("alt", "accept", "--all", paper)
	if accepted := ojson.Plain(report.Get("accepted")).([]any); len(accepted) != 1 || accepted[0] != "a" {
		t.Errorf("accept --all DIRECTORY: %v", report.Get("accepted"))
	}
	commandResult("alt", "set", "b", paper, "--alt", "Second.")
	report = commandResult("alt", "accept", "b", paper)
	if unchanged := ojson.Plain(report.Get("unchanged")).([]any); len(unchanged) != 1 {
		t.Errorf("accept NAME DIRECTORY: %v", report.Get("unchanged"))
	}
	refused := func() (refused bool) {
		defer func() { refused = recover() != nil }()
		commandResult("alt", "accept", "--all", "x", "y")
		return false
	}
	if !refused() {
		t.Error("two positionals with --all were accepted")
	}
}
