package alttext

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/authordir"
	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// pngBytes is a tiny PNG; the drafter passes PNG figures on as they are.
var pngBytes = []byte("\x89PNG\r\n\x1a\nnot really a picture, but bytes with an identity")

func mediaAsset(data []byte) (string, rec.Map) {
	original := mediamodel.Encoded(data, "image/png")
	sha := original["sha256"].(string)
	return sha, rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": sha, "representations": rec.Map{"original": original}}
}

// figureDocument has a cited figure with a caption (disk.png) and a figure outside any float
// (plot.png), in a paper whose language is English.
func figureDocument() rec.Map {
	disk, diskAsset := mediaAsset(pngBytes)
	plot, plotAsset := mediaAsset(append([]byte{}, append(pngBytes, '2')...))
	text := func(value string) rec.Map { return rec.Map{"type": "text", "text": value} }
	return rec.Map{
		"language":    rec.Map{"tag": "en"},
		"assets":      rec.Map{"items": rec.Map{disk: diskAsset, plot: plotAsset}},
		"block_order": []any{"p1", "p2", "p3", "p4", "p5"},
		"nodes": rec.Map{
			"p1": rec.Map{"id": "p1", "type": "paragraph", "children": []any{"m1"}, "content": []any{
				text("The disk is drawn in Figure"), rec.Map{"type": "reference", "text": " 1", "target": "p3"},
				text("; its radius is "), rec.Map{"type": "math", "node": "m1"}, text(".")}},
			"m1": rec.Map{"id": "m1", "type": "math", "tex": rec.Map{"status": "verified", "text": "r"}},
			"f1": rec.Map{"id": "f1", "type": "figure_float", "children": []any{"p2", "c1"}},
			"p2": rec.Map{"id": "p2", "type": "paragraph", "children": []any{}, "content": []any{
				rec.Map{"type": "image", "id": "i1", "asset": disk, "source": "figures/disk.png", "width_sp": int64(10), "height_sp": int64(10)}}},
			"c1": rec.Map{"id": "c1", "type": "float_caption", "children": []any{"p3"}},
			"p3": rec.Map{"id": "p3", "type": "paragraph", "children": []any{}, "content": []any{text("Figure 1: The disk.")}},
			"p4": rec.Map{"id": "p4", "type": "paragraph", "children": []any{}, "content": []any{text("Before the plot.")}},
			"p5": rec.Map{"id": "p5", "type": "paragraph", "children": []any{}, "content": []any{
				rec.Map{"type": "image", "id": "i2", "asset": plot, "source": "plot.png", "width_sp": int64(10), "height_sp": int64(10)}}},
		},
	}
}

func TestFiguresCarryTheirCaptionAndCitingText(t *testing.T) {
	figures := figdesc.Figures(figureDocument())
	if len(figures) != 2 {
		t.Fatalf("figures %+v", figures)
	}
	disk, plot := figures[0], figures[1]
	if disk.Key != "disk" || disk.File != "figs/disk.png" || disk.Caption != "Figure 1: The disk." {
		t.Errorf("disk %+v", disk)
	}
	if disk.Context != "The disk is drawn in Figure →1←; its radius is $r$.\n\nBefore the plot." {
		t.Errorf("disk context %q", disk.Context)
	}
	if len(disk.Citations) != 1 || disk.Citations[0] != "The disk is drawn in Figure →1←; its radius is $r$.\n\nBefore the plot." {
		t.Errorf("disk citations %q", disk.Citations)
	}
	if plot.Key != "plot" || plot.Caption != "" || plot.Context != "Before the plot." {
		t.Errorf("plot %+v", plot)
	}
	if disk.SHA256 == plot.SHA256 || len(disk.SHA256) != 64 {
		t.Errorf("identities %s %s", disk.SHA256, plot.SHA256)
	}
}

// TestPromptNumbersTheCitationWindows: the describer gets each window with its number, not the
// joined context (decision 123).
func TestPromptNumbersTheCitationWindows(t *testing.T) {
	prompt := Prompt(&figdesc.Figure{Context: "joined", Citations: []string{"The disk in →Figure 1←.", "A second →reference←."}}, "", Geometry{})
	want := "Caption:\n(none)\n\nText around each reference to the figure:\n1. The disk in →Figure 1←.\n\n2. A second →reference←.\n"
	if !strings.Contains(prompt, want) {
		t.Errorf("prompt lacks the numbered windows:\n%s", prompt)
	}
	if strings.Contains(prompt, "joined") {
		t.Error("the prompt still carries the joined context")
	}
}

// stub writes a describing command that prints what it was given and counts its runs.
func stub(t *testing.T, body string) (string, string) {
	t.Helper()
	directory := t.TempDir()
	count := filepath.Join(directory, "runs")
	script := filepath.Join(directory, "describe")
	text := "#!/bin/sh\necho run >> " + filepath.ToSlash(count) + "\n" + body + "\n"
	if err := os.WriteFile(script, []byte(text), 0o755); err != nil {
		t.Fatal(err)
	}
	return script, count
}

func runs(t *testing.T, count string) int {
	t.Helper()
	data, err := os.ReadFile(count)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "run")
}

func TestDrafterDraftsOnceAndKeepsTheAuthorsText(t *testing.T) {
	script, count := stub(t, `prompt=$(cat); test -f "$1" || exit 9
case "$prompt" in *"Figure 1: The disk."*) ;; *) exit 8;; esac
echo "  Drafted $RELATEX_FIGURE_NAME ($RELATEX_FIGURE_TYPE, $RELATEX_LANGUAGE)
  in two lines. "`)
	document := figureDocument()
	path := filepath.Join(t.TempDir(), figdesc.FileName)
	if err := authordir.Ensure(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[figs.plot]\nalt = \"My own plot text.\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sidecar, err := figdesc.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	drafter := &Drafter{Command: shellCommand(t, script), Model: "stub", Timeout: 10 * time.Second, Today: "2026-10-30"}
	outcome, err := drafter.Run(document, sidecar)
	if err != nil || len(outcome.Drafted) != 1 || outcome.Drafted[0] != "disk" || len(outcome.Kept) != 1 || len(outcome.Failed) != 0 {
		t.Fatalf("first run %+v %v", outcome, err)
	}
	entry := sidecar.Entries["disk"]
	if entry.Alt != "Drafted disk (image/png, en) in two lines." || entry.Provenance != figdesc.Model || entry.Model != "stub" ||
		entry.Prompt != PromptVersion || entry.SHA256 != figdesc.Figures(document)[0].SHA256 {
		t.Errorf("draft %+v", entry)
	}
	reread, err := figdesc.Read(path)
	if err != nil || reread.Entries["disk"] == nil || reread.Entries["plot"].Alt != "My own plot text." {
		t.Fatalf("the draft was not saved beside the author's entry: %v", err)
	}
	outcome, err = drafter.Run(document, reread)
	if err != nil || len(outcome.Cached) != 1 || len(outcome.Drafted) != 0 || runs(t, count) != 1 {
		t.Errorf("second run sent an unchanged figure again: %+v, %d runs", outcome, runs(t, count))
	}
}

func TestDrafterReportsFailuresAndHonoursTheLimit(t *testing.T) {
	script, _ := stub(t, `echo "the service said no" >&2; exit 3`)
	sidecar, err := figdesc.Read(filepath.Join(t.TempDir(), figdesc.FileName))
	if err != nil {
		t.Fatal(err)
	}
	drafter := &Drafter{Command: shellCommand(t, script), Model: "stub", Timeout: 10 * time.Second, Today: "2026-10-30", Limit: 1}
	outcome, err := drafter.Run(figureDocument(), sidecar)
	if err != nil || len(outcome.Failed) != 1 || len(outcome.Waiting) != 1 || len(sidecar.Entries) != 0 {
		t.Fatalf("outcome %+v %v", outcome, err)
	}
	if reason := outcome.Failed[0]["reason"].(string); !strings.Contains(reason, "the service said no") {
		t.Errorf("reason %q", reason)
	}
}

func TestDrafterReusesADraftForTheSameBytes(t *testing.T) {
	script, count := stub(t, `echo never`)
	document := figureDocument()
	sha := figdesc.Figures(document)[0].SHA256
	sidecar, err := figdesc.Parse("[figs.old-name]\nalt = \"A disk.\"\nprovenance = \"reviewed\"\nsha256 = \"" + sha + "\"\n" +
		"[figs.plot]\nalt = \"Mine.\"\n")
	if err != nil {
		t.Fatal(err)
	}
	sidecar.Path = filepath.Join(t.TempDir(), figdesc.FileName)
	drafter := &Drafter{Command: shellCommand(t, script), Model: "stub", Timeout: 10 * time.Second, Today: "2026-10-30"}
	outcome, err := drafter.Run(document, sidecar)
	if err != nil || len(outcome.Reused) != 1 || runs(t, count) != 0 {
		t.Fatalf("outcome %+v %v, %d runs", outcome, err, runs(t, count))
	}
	if entry := sidecar.Entries["disk"]; entry.Alt != "A disk." || entry.Provenance != figdesc.Model {
		t.Errorf("reused entry %+v: a copy for another name is a draft to review again", entry)
	}
}

// A describer's answer after an empty line is the long description (decision 110): drafted into
// the sidecar beside the alternative, shown on the review page.
func TestDrafterKeepsALongDescriptionAfterAnEmptyLine(t *testing.T) {
	if got := SplitDescription("  A plot.\n \n The error\n falls.\n\nThen levels off. "); got != (Description{Alt: "A plot.",
		Long: "The error falls. Then levels off."}) {
		t.Fatalf("split %+v", got)
	}
	script, _ := stub(t, `printf 'A plot of %s.\n\nThe horizontal axis is the step.\n' "$RELATEX_FIGURE_NAME"`)
	document := figureDocument()
	path := filepath.Join(t.TempDir(), figdesc.FileName)
	sidecar, err := figdesc.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	drafter := &Drafter{Command: shellCommand(t, script), Model: "stub", Timeout: 10 * time.Second, Today: "2026-11-02"}
	if _, err = drafter.Run(document, sidecar); err != nil {
		t.Fatal(err)
	}
	reread, err := figdesc.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if entry := reread.Entries["disk"]; entry == nil || entry.Alt != "A plot of disk." || entry.Long != "The horizontal axis is the step." {
		t.Fatalf("drafted entry %+v", entry)
	}
	if page := ReviewPage(figdesc.Figures(document), reread); !strings.Contains(page, "<dt>Short description</dt><dd>The horizontal axis is the step.</dd>") {
		t.Errorf("the review page lacks the long description:\n%s", page)
	}
}

func TestApplyDescribesSpansWithProvenance(t *testing.T) {
	document := figureDocument()
	sha := figdesc.Figures(document)[0].SHA256
	sidecar, err := figdesc.Parse("[figs.disk]\nalt = \"A disk\\n  of radius r.\"\nprovenance = \"model\"\nsha256 = \"" + sha + "\"\n" +
		"[figs.plot]\nalt = \"Mine.\"\nsha256 = \"changed\"\n[figs.gone]\nalt = \"x\"\n")
	if err != nil {
		t.Fatal(err)
	}
	summary := figdesc.Apply(document, sidecar)
	if len(summary.Drafts) != 1 || summary.Author != 1 || len(summary.Stale) != 1 || summary.Stale[0] != "plot" ||
		len(summary.Unmatched) != 1 || summary.Applied != 2 {
		t.Errorf("summary %+v", summary)
	}
	nodes := rec.FetchMap(document, "nodes")
	disk := rec.AsMap(rec.FetchList(rec.AsMap(nodes["p2"]), "content")[0])
	plot := rec.AsMap(rec.FetchList(rec.AsMap(nodes["p5"]), "content")[0])
	if disk["alt"] != "A disk of radius r." || disk["alt_provenance"] != figdesc.Model {
		t.Errorf("disk span %v", disk)
	}
	if plot["alt"] != "Mine." || plot["alt_provenance"] != figdesc.Author {
		t.Errorf("a stale author's text is still used: %v", plot)
	}
}

func TestApplyLeavesOutAStaleDraft(t *testing.T) {
	document := figureDocument()
	sidecar, err := figdesc.Parse("[figs.disk]\nalt = \"Old.\"\nprovenance = \"model\"\nsha256 = \"other\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if summary := figdesc.Apply(document, sidecar); summary.Applied != 0 || len(summary.Stale) != 1 {
		t.Errorf("summary %+v", summary)
	}
}

func TestReviewPageListsEachFigureWithWhatToDo(t *testing.T) {
	document := figureDocument()
	sidecar, err := figdesc.Parse("[figs.disk]\nalt = \"A <disk>.\"\nprovenance = \"model\"\nmodel = \"stub\"\nsha256 = \"" +
		figdesc.Figures(document)[0].SHA256 + "\"\n")
	if err != nil {
		t.Fatal(err)
	}
	page := ReviewPage(figdesc.Figures(document), sidecar)
	for _, want := range []string{`<img src="figs/disk.png" alt="A &lt;disk&gt;." loading="lazy" decoding="async">`, "draft, not reviewed", "relatex alt accept disk",
		"Figure 1: The disk.", "no description", "1 drafts to review, 1 without a description"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	// The bulk review (decision 112): every figure the sidecar can describe has its text in an editable
	// field that remembers what was shown, an accept checkbox, and the save button's script.
	for _, want := range []string{`<fieldset class="decision" data-key="disk" data-sha="` + figdesc.Figures(document)[0].SHA256 + `">`,
		`class="alt" rows="2" data-shown="A &lt;disk&gt;.">A &lt;disk&gt;.</textarea>`, `class="accept"`, "Save the review file",
		"<script>", "[" + figdesc.ReviewTable + "]", ReviewFile, "relatex alt import"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if fields := strings.Count(page, `<fieldset class="decision"`); fields != len(figdesc.Figures(document)) {
		t.Errorf("%d decision fields for %d figures", fields, len(figdesc.Figures(document)))
	}
}

// An answer over the length limits is asked for again once, with the shorten note on the prompt
// (decision 123, relatex-alt-5 and later); the shorter answer is the draft.
func TestDrafterAsksOnceMoreForAShorterAnswer(t *testing.T) {
	script, count := stub(t, `prompt=$(cat); case "$prompt" in *"Shorten to the limits"*) echo "A disk around p.";;
*) printf '%0250d\n' 0;; esac`)
	sidecar, err := figdesc.Read(filepath.Join(t.TempDir(), figdesc.FileName))
	if err != nil {
		t.Fatal(err)
	}
	drafter := &Drafter{Command: shellCommand(t, script), Model: "stub", Timeout: 10 * time.Second, Today: "2026-11-11", Limit: 1}
	outcome, err := drafter.Run(figureDocument(), sidecar)
	if err != nil || len(outcome.Drafted) != 1 || len(outcome.TooLong) != 0 || runs(t, count) != 2 {
		t.Fatalf("outcome %+v %v, %d runs", outcome, err, runs(t, count))
	}
	if entry := sidecar.Entries["disk"]; entry.Alt != "A disk around p." {
		t.Errorf("draft %q, want the shorter answer", entry.Alt)
	}
}

// A second answer still over the limits is kept whole, never cut, and reported as too long.
func TestDrafterKeepsAnAnswerStillTooLongWhole(t *testing.T) {
	words := strings.Repeat("word ", 120)
	script, count := stub(t, `cat >/dev/null; printf 'A disk.\n\n`+words+`\n'`)
	sidecar, err := figdesc.Read(filepath.Join(t.TempDir(), figdesc.FileName))
	if err != nil {
		t.Fatal(err)
	}
	drafter := &Drafter{Command: shellCommand(t, script), Model: "stub", Timeout: 10 * time.Second, Today: "2026-11-11", Limit: 1}
	outcome, err := drafter.Run(figureDocument(), sidecar)
	if err != nil || len(outcome.TooLong) != 1 || outcome.TooLong[0] != "disk" || runs(t, count) != 2 {
		t.Fatalf("outcome %+v %v, %d runs", outcome, err, runs(t, count))
	}
	if entry := sidecar.Entries["disk"]; len(strings.Fields(entry.Long)) != 120 { // over 60 words, kept whole
		t.Errorf("the long description was cut: %d words", len(strings.Fields(entry.Long)))
	}
	within := Description{Alt: strings.Repeat("a", 200), Long: strings.Repeat("w ", 60), Details: strings.Repeat("w ", 200)}
	if within.TooLong() || !(Description{Alt: strings.Repeat("a", 201)}).TooLong() ||
		!(Description{Long: strings.Repeat("w ", 61)}).TooLong() || !(Description{Details: strings.Repeat("w ", 201)}).TooLong() {
		t.Error("limits are 200 characters, 60 and 200 words")
	}
}

// shellCommand runs a stub describer: by itself on Unix; on Windows through the sh of Git for
// Windows, since Windows cannot start a #! script itself.
func shellCommand(t *testing.T, script string) []string {
	t.Helper()
	if runtime.GOOS != "windows" {
		return []string{script}
	}
	candidates := []string{filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "sh.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "usr", "bin", "sh.exe")}
	if sh, err := exec.LookPath("sh"); err == nil {
		candidates = append([]string{sh}, candidates...)
	}
	// A package manager's shim can point at a shell that is gone: use the first one that runs.
	for _, sh := range candidates {
		if exec.Command(sh, "-c", "exit 0").Run() == nil {
			return []string{sh, script}
		}
	}
	t.Skip("no working sh to run the stub describer")
	return nil
}

// A draft records the model the command names in RELATEX_DESCRIBER_MODEL_FILE, not the configured
// one; after the shorten retry, the model of the kept answer (decision 133). A command that names no
// model, or writes something that is not a name, leaves the configured name.
func TestDrafterRecordsTheModelThatAnswered(t *testing.T) {
	script, _ := stub(t, `prompt=$(cat); case "$prompt" in
*"Shorten to the limits"*) echo "lumen/second" > "$RELATEX_DESCRIBER_MODEL_FILE"; echo "A disk around p.";;
*) case "$RELATEX_FIGURE_NAME" in
   disk) echo "lumen/first" > "$RELATEX_DESCRIBER_MODEL_FILE"; printf '%0250d\n' 0;;
   *) printf 'two words\n' > "$RELATEX_DESCRIBER_MODEL_FILE"; echo "A plot.";;
   esac;;
esac`)
	sidecar, err := figdesc.Read(filepath.Join(t.TempDir(), figdesc.FileName))
	if err != nil {
		t.Fatal(err)
	}
	drafter := &Drafter{Command: shellCommand(t, script), Model: "configured", Timeout: 10 * time.Second, Today: "2026-11-13"}
	if outcome, err := drafter.Run(figureDocument(), sidecar); err != nil || len(outcome.Drafted) != 2 {
		t.Fatalf("outcome %+v %v", outcome, err)
	}
	if model := sidecar.Entries["disk"].Model; model != "lumen/second" {
		t.Errorf("disk model %q, want the model of the kept (shortened) answer", model)
	}
	if model := sidecar.Entries["plot"].Model; model != "configured" {
		t.Errorf("plot model %q, want the configured name for a report that is no name", model)
	}
}

func TestAnsweringModelReadsOnePlainName(t *testing.T) {
	directory := t.TempDir()
	cases := map[string]string{"lumen/qwen3.8-flash\n": "lumen/qwen3.8-flash", "  m1  \nignored\n": "m1", "": "",
		"a b\n": "", "say \"x\"\n": "", strings.Repeat("m", modelNameLimit+1): ""}
	for content, want := range cases {
		path := filepath.Join(directory, "model")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := answeringModel(path); got != want {
			t.Errorf("answeringModel(%q) = %q, want %q", content, got, want)
		}
	}
	if got := answeringModel(filepath.Join(directory, "missing")); got != "" {
		t.Errorf("a missing file names %q", got)
	}
}

// The command base in a directory of the run's own temporary directory (command.Scratch: the
// run's workspace, else RELATEX_RUNS_DIR), never the system's default, and Run removes it.
func TestDrafterWorksInTheRunsOwnTemporaryDirectory(t *testing.T) {
	base := t.TempDir()
	t.Setenv("RELATEX_RUNS_DIR", base)
	seen := filepath.Join(t.TempDir(), "seen")
	script, _ := stub(t, `cat >/dev/null; pwd >> "`+seen+`"; echo "A disk."`)
	sidecar, err := figdesc.Read(filepath.Join(t.TempDir(), figdesc.FileName))
	if err != nil {
		t.Fatal(err)
	}
	drafter := &Drafter{Command: shellCommand(t, script), Model: "stub", Timeout: 10 * time.Second, Today: "2026-11-14"}
	if outcome, runErr := drafter.Run(figureDocument(), sidecar); runErr != nil || len(outcome.Drafted) != 2 {
		t.Fatalf("outcome %+v %v", outcome, runErr)
	}
	data, err := os.ReadFile(seen)
	if err != nil {
		t.Fatal(err)
	}
	resolved, _ := filepath.EvalSymlinks(base)
	for _, directory := range strings.Fields(string(data)) {
		if !strings.HasPrefix(directory, base) && !strings.HasPrefix(directory, resolved) {
			t.Errorf("the command ran in %s, outside the run's temporary directory %s", directory, base)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(base, "alt-draft-*")); len(left) != 0 {
		t.Errorf("Run left its temporary directory: %v", left)
	}
}
