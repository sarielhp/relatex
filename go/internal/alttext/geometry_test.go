package alttext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// ipeFigureDocument is a document whose one figure is an Ipe PDF (its source embedded).
func ipeFigureDocument(t *testing.T) rec.Map {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "figscan", "testdata", "balanced.pdf"))
	if os.IsNotExist(err) {
		t.Skip("the figure fixtures are not in this tree (they are not published)")
	}
	if err != nil {
		t.Fatal(err)
	}
	original := mediamodel.Encoded(data, "application/pdf")
	sha := original["sha256"].(string)
	asset := rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": sha, "representations": rec.Map{"original": original,
		"html": mediamodel.Encoded([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "image/svg+xml")}}
	return rec.Map{"language": rec.Map{"tag": "en"}, "assets": rec.Map{"items": rec.Map{sha: asset}},
		"block_order": []any{"p1"},
		"nodes": rec.Map{"p1": rec.Map{"id": "p1", "type": "paragraph", "content": []any{
			rec.Map{"type": "image", "id": "i1", "asset": sha, "source": "figs/balanced.pdf"}}}}}
}

func TestDrafterSendsTheGeometryOfAnIpeFigure(t *testing.T) {
	script, _ := stub(t, `prompt=$(cat)
test -s "$RELATEX_FIGURE_GEOMETRY" || exit 7
case "$prompt" in *"Geometry of the figure"*) ;; *) exit 8;; esac
head -c 40 "$RELATEX_FIGURE_GEOMETRY" | grep -q . || exit 6
echo "Geometry seen."`)
	path := filepath.Join(t.TempDir(), figdesc.FileName)
	sidecar, err := figdesc.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	drafter := &Drafter{Command: shellCommand(t, script), Model: "stub", Timeout: 10 * time.Second, Today: "2026-11-11"}
	outcome, err := drafter.Run(ipeFigureDocument(t), sidecar)
	if err != nil || len(outcome.Drafted) != 1 {
		t.Fatalf("outcome %+v %v", outcome, err)
	}
	reread, err := figdesc.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	entry := reread.Entries["balanced"]
	if entry == nil || entry.RestedOn != RestedIpe || entry.Prompt != PromptVersion || entry.Alt != "Geometry seen." {
		t.Fatalf("entry %+v", entry)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `rested_on = "ipe"`) {
		t.Errorf("sidecar has no rested_on:\n%s", data)
	}
	page := ReviewPage(figdesc.Figures(ipeFigureDocument(t)), reread)
	if !strings.Contains(page, "rested_on = ipe") {
		t.Errorf("the review page does not say what the draft rested on")
	}
}

func TestDrafterSendsNoGeometryForARasterFigure(t *testing.T) {
	script, _ := stub(t, `prompt=$(cat)
test -z "$RELATEX_FIGURE_GEOMETRY" || exit 7
case "$prompt" in *"Geometry of the figure"*) exit 8;; esac
echo "No geometry."`)
	sidecar, err := figdesc.Read(filepath.Join(t.TempDir(), figdesc.FileName))
	if err != nil {
		t.Fatal(err)
	}
	drafter := &Drafter{Command: shellCommand(t, script), Model: "stub", Timeout: 10 * time.Second, Today: "2026-11-11", Limit: 1}
	outcome, err := drafter.Run(figureDocument(), sidecar)
	if err != nil || len(outcome.Drafted) != 1 {
		t.Fatalf("outcome %+v %v", outcome, err)
	}
	if entry := sidecar.Entries["disk"]; entry.RestedOn != RestedImage {
		t.Errorf("rested_on %q, want image", entry.RestedOn)
	}
}

func TestPromptVersionEightAsksForConcreteShortTexts(t *testing.T) {
	prompt := Prompt(&figdesc.Figure{}, "", Geometry{Text: "path 1: polygon (0, 0) (10, 0)\n"})
	for _, want := range []string{"one sentence of at most 150 characters", "at most 40 words in complete sentences",
		"(never repeat it)", "Its first word names something in the picture", "the axes with their quantities and ranges",
		"starts with \"Details:\"", "in at most 150 words: labels,\n   counts, values", "Never retell the caption",
		"a construction a proof walks through", "\"Illustrates\"", "a purpose clause (\"To compare ...\"", "an imperative",
		"story the picture cannot show", "Name its panel (letter or position)\n  only when the input says which one it is (\"Which picture of the figure this is\" below, when\n  given",
		"never give\n  this image the content or conclusion of other panels", "(T*, not \"T star\")",
		"mention colour only when it tells things apart", "trust it over the picture for counts and labels",
		"never by its LaTeX source or macro names", "Never put the geometry's coordinates, radii, pen widths or opacities",
		"x grows to the right and y grows upward", "only when you are certain", "x to the right, y up",
		"without listing it", "separated by one empty line", "End every part with a complete sentence",
		"Geometry of the figure", "path 1: polygon (0, 0) (10, 0)\n"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	for _, gone := range []string{"purpose first", "Supports Lemma"} {
		if strings.Contains(prompt, gone) {
			t.Errorf("prompt still asks %q", gone)
		}
	}
	if PromptVersion != "relatex-alt-10" || rasterDPI != "200" {
		t.Errorf("version %s at %s dpi", PromptVersion, rasterDPI)
	}
	if strings.Contains(Prompt(&figdesc.Figure{}, "", Geometry{}), "Geometry of the figure") {
		t.Errorf("a figure without geometry has a geometry section")
	}
}

func TestParsePageBoxReadsTheNamedBox(t *testing.T) {
	output := []byte("Page    1 MediaBox:      0.00     0.00   144.00   144.00\nPage    1 CropBox:       3.80    11.80   132.20   140.20\n")
	box, ok := parsePageBox(output, boxLabel("cropbox"))
	if !ok || box.X0 != 3.8 || box.Y1 != 140.2 {
		t.Errorf("box %+v %v", box, ok)
	}
	if box, ok = parsePageBox(output, boxLabel("mediabox")); !ok || box.X1 != 144 {
		t.Errorf("media box %+v %v", box, ok)
	}
}

// The prompt says which picture of its figure the describer sees, and for one view of an Ipe file
// with several, the view, its layers and its printed labels (decision 133).
func TestPromptSaysWhichPanelAndView(t *testing.T) {
	figure := &figdesc.Figure{Caption: "Figure 2: (a) Before. (b) After.",
		Panel: figdesc.Panel{InFigure: true, Index: 2, Count: 2, Column: 1, RowSize: 1, Subcaption: "(b) After."}}
	view := Geometry{Text: "geometry", View: "Ipe page 1, view 3 of 4", Layers: []string{"alpha", "cut"}, Labels: []string{"x", "y"}}
	prompt := Prompt(figure, "", view)
	for _, want := range []string{"Which picture of the figure this is:\nThis picture is picture 2 of 2 of its figure",
		`"(b) After."`, "It is Ipe page 1, view 3 of 4 of its Ipe file, showing the layers alpha, cut; its labels: x, y."} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(Prompt(figure, "", Geometry{View: "Ipe page 1", Layers: []string{"alpha"}}), "of its Ipe file") {
		t.Error("a page without views is named as a view")
	}
}

// geometryOf keeps an Ipe view's identity: its view, layers and printed labels, never LaTeX source.
func TestGeometryOfAnIpeViewKeepsItsIdentity(t *testing.T) {
	scanned, err := ScanFigure(ipeFigureDocument(t), figdesc.Figures(ipeFigureDocument(t))[0], t.TempDir())
	if err != nil || scanned == nil {
		t.Fatalf("scan %v", err)
	}
	found := geometryOf(scanned)
	if found.RestedOn != RestedIpe || found.View == "" || len(found.Layers) == 0 {
		t.Errorf("geometry %+v", found)
	}
	for _, label := range found.Labels {
		if strings.ContainsAny(label, `\$`) {
			t.Errorf("label %q is LaTeX source", label)
		}
	}
}

// Version 9 guards the readings round 2's blind grades found misread (decision 138): plot values,
// logarithmic axes, connections and OCR text.
func TestPromptVersionNineGuardsTheReadingsModelsMisread(t *testing.T) {
	prompt := Prompt(&figdesc.Figure{}, "", Geometry{})
	for _, want := range []string{"with a value only where the picture prints it or a curve plainly meets a\n   labelled tick",
		"leave out a value you would have to estimate", "Call an axis logarithmic only when its tick labels grow by a constant factor",
		"judge each axis on its own", "connected only where a drawn line or arrow joins them", "from its tail to its head",
		"Text marked \"read by OCR\" is different", "check its numbers against the picture"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if strings.Contains(prompt, "a few values read from the picture") {
		t.Error("the prompt still asks for values read from the picture")
	}
}

// promptInput is a panel and the geometry a prompt is made from.
type promptInput struct {
	panel    figdesc.Panel
	geometry Geometry
}

// The panel section is printed only when it tells something (decision 138): a subcaption, the text
// of the picture's group, or an Ipe view; "picture i of n" without one, the only picture and a
// picture outside a figure get none, as the blind A/B found it hurt there.
func TestPromptNamesThePanelOnlyWhenItIsKnown(t *testing.T) {
	section := "Which picture of the figure this is:"
	unknown := []figdesc.Panel{{}, {InFigure: true, Index: 1, Count: 1}, {InFigure: true, Index: 2, Count: 3, Column: 2, RowSize: 3}}
	for _, panel := range unknown {
		if prompt := Prompt(&figdesc.Figure{Panel: panel}, "", Geometry{}); strings.Contains(prompt, section) {
			t.Errorf("panel %+v has a section", panel)
		}
	}
	known := map[string]promptInput{
		"subcaption": {figdesc.Panel{InFigure: true, Index: 1, Count: 2, Subcaption: "(a) Before."}, Geometry{}},
		"group text": {figdesc.Panel{InFigure: true, Index: 1, Count: 2, RowText: "Left and right."}, Geometry{}},
		"Ipe view":   {figdesc.Panel{InFigure: true, Index: 1, Count: 1}, Geometry{View: "Ipe page 1, view 2 of 5"}},
	}
	for name, input := range known {
		if prompt := Prompt(&figdesc.Figure{Panel: input.panel}, "", input.geometry); !strings.Contains(prompt, section) {
			t.Errorf("%s: no section", name)
		}
	}
}

// Version 10 keeps a draft to what is visible or stated (decision 138, amended): no invented purpose,
// conclusion, name or structure.
func TestPromptVersionTenForbidsInventedPurposeAndNames(t *testing.T) {
	prompt := Prompt(&figdesc.Figure{}, "", Geometry{})
	for _, want := range []string{"Describe only what is visible in the picture and what the caption or the text states",
		"\"The text says\n  this shows ...\"", "Draw no conclusion of\n  your own", "Take every name from the picture's printed labels",
		"call it \"unlabeled\"", "Add no domain fact that is not printed or stated", "parallel, aligned, in sequence or\n  merging only when the drawn lines make them so"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}
