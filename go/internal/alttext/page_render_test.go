package alttext

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/authordir"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/media"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// pagePDF is a small valid PDF whose three pages carry a filled square in a different place, so a
// render of one page differs from a render of another.
func pagePDF(t *testing.T) []byte {
	t.Helper()
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 5 0 R 7 0 R] /Count 3 >>",
	}
	for page := 0; page < 3; page++ {
		content := fmt.Sprintf("0 0 1 rg %d 20 40 40 re f", 10+page*50)
		pageObject := len(objects) + 1
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents %d 0 R >>", pageObject+1),
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	}
	data := "%PDF-1.7\n"
	offsets := []int{}
	for index, object := range objects {
		offsets = append(offsets, len(data))
		data += fmt.Sprintf("%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	start := len(data)
	data += fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		data += fmt.Sprintf("%010d 00000 n \n", offset)
	}
	data += fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, start)
	return []byte(data)
}

// pageFigureDocument is a document whose one asset is a three-page PDF, its three pages included in
// reading order as views of the same file (as Ipe views are).
func pageFigureDocument(t *testing.T) rec.Map {
	t.Helper()
	original := mediamodel.Encoded(pagePDF(t), "application/pdf")
	sha := original["sha256"].(string)
	asset := rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": sha, "metadata": rec.Map{},
		"placements": rec.Map{}, "representations": rec.Map{"original": original}}
	nodes, order := rec.Map{}, []any{}
	for page := int64(1); page <= 3; page++ {
		size := geometry.BPToSP(200)
		placement := media.Placement(rec.Map{"page": page, "pagebox": "cropbox", "natural_width": size, "natural_height": size},
			rec.Map{"width": size, "height": size, "viewport": []any{0.0, 0.0, 200.0, 200.0}, "clip": false}, []any{})
		id := rec.FetchString(placement, "id")
		rec.FetchMap(asset, "placements")[id] = placement
		display := mediamodel.Encoded([]byte(fmt.Sprintf(
			`<svg xmlns="http://www.w3.org/2000/svg"><rect x="%d" y="0" width="10" height="10"/></svg>`, page)), "image/svg+xml")
		display["placement"] = id
		rec.FetchMap(asset, "representations")["display:"+id] = display
		block := fmt.Sprintf("p%d", page)
		nodes[block] = rec.Map{"id": block, "type": "paragraph", "content": []any{rec.Map{"type": "image",
			"id": fmt.Sprintf("i%d", page), "asset": sha, "source": "figures/views.pdf", "placement": placement}}}
		order = append(order, block)
	}
	return rec.Map{"language": rec.Map{"tag": "en"}, "assets": rec.Map{"items": rec.Map{sha: asset}},
		"nodes": nodes, "block_order": order}
}

func TestWriteImageRendersTheFiguresOwnPage(t *testing.T) {
	tool := command.Resolve("pdftocairo")
	if tool == "" {
		t.Skip("pdftocairo not installed")
	}
	document := pageFigureDocument(t)
	figures := figdesc.Figures(document)
	first, _, err := writeImage(document, figures[0], t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, mime, err := writeImage(document, figures[1], t.TempDir())
	if err != nil || mime != "image/png" {
		t.Fatalf("page 2: %q %v", mime, err)
	}
	data, _ := figures[1].Image(document)
	directory := t.TempDir()
	pdf, target := filepath.Join(directory, "views.pdf"), filepath.Join(directory, "figure")
	if err = os.WriteFile(pdf, data, 0o600); err != nil {
		t.Fatal(err)
	}
	direct := exec.Command(tool, "-png", "-singlefile", "-r", rasterDPI, "-f", "2", "-l", "2", pdf, target)
	output, runErr := direct.CombinedOutput()
	if runErr != nil {
		t.Fatalf("pdftocairo page 2: %v: %s", runErr, output)
	}
	want, err := os.ReadFile(target + ".png")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("page 2 was not rendered from page 2 (%d bytes, want %d)", len(got), len(want))
	}
	firstPNG, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(firstPNG, got) {
		t.Errorf("pages 1 and 2 rendered the same picture")
	}
}

// croppedFigureDocument is a document whose one figure is a view of the second page of the same
// three-page PDF: the placement's viewport covers the part of the page the figure shows.
func croppedFigureDocument(t *testing.T) rec.Map {
	t.Helper()
	original := mediamodel.Encoded(pagePDF(t), "application/pdf")
	sha := original["sha256"].(string)
	asset := rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": sha, "metadata": rec.Map{},
		"placements": rec.Map{}, "representations": rec.Map{"original": original}}
	size := geometry.BPToSP(200)
	placement := media.Placement(rec.Map{"page": int64(2), "pagebox": "cropbox", "natural_width": size, "natural_height": size},
		rec.Map{"width": size, "height": size, "viewport": []any{24.0, 0.0, 120.0, 96.0}, "clip": false}, []any{})
	id := rec.FetchString(placement, "id")
	rec.FetchMap(asset, "placements")[id] = placement
	display := mediamodel.Encoded([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect x="2" y="0" width="10" height="10"/></svg>`), "image/svg+xml")
	display["placement"] = id
	rec.FetchMap(asset, "representations")["display:"+id] = display
	nodes := rec.Map{"p": rec.Map{"id": "p", "type": "paragraph", "content": []any{rec.Map{"type": "image",
		"id": "i", "asset": sha, "source": "figures/views.pdf", "placement": placement}}}}
	return rec.Map{"language": rec.Map{"tag": "en"}, "assets": rec.Map{"items": rec.Map{sha: asset}},
		"nodes": nodes, "block_order": []any{"p"}}
}

// A view is rendered from its own page and cropped to the placement's box, not the whole page.
func TestWriteImageCropsAViewOfItsPage(t *testing.T) {
	tool := command.Resolve("pdftocairo")
	if tool == "" {
		t.Skip("pdftocairo not installed")
	}
	document := croppedFigureDocument(t)
	figures := figdesc.Figures(document)
	if len(figures) != 1 || figures[0].Page != 2 || len(figures[0].Box) != 4 {
		t.Fatalf("figures %+v", figures)
	}
	got, mime, err := writeImage(document, figures[0], t.TempDir())
	if err != nil || mime != "image/png" {
		t.Fatalf("view: %q %v", mime, err)
	}
	file, err := os.Open(got)
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(file)
	file.Close()
	if err != nil || config.Width != 266 || config.Height != 267 {
		t.Errorf("crop %dx%d (%v), want 266x267 (96 bp at 200 dpi)", config.Width, config.Height, err)
	}
	data, _ := figures[0].Image(document)
	directory := t.TempDir()
	pdf, target := filepath.Join(directory, "views.pdf"), filepath.Join(directory, "figure")
	if err = os.WriteFile(pdf, data, 0o600); err != nil {
		t.Fatal(err)
	}
	direct := exec.Command(tool, "-png", "-singlefile", "-r", rasterDPI, "-f", "2", "-l", "2", "-cropbox",
		"-x", "67", "-y", "289", "-W", "266", "-H", "267", pdf, target)
	output, runErr := direct.CombinedOutput()
	if runErr != nil {
		t.Fatalf("pdftocairo crop of page 2: %v: %s", runErr, output)
	}
	want, err := os.ReadFile(target + ".png")
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rendered, want) {
		t.Errorf("the view was not cropped from page 2 (%d bytes, want %d)", len(rendered), len(want))
	}
}

func TestCropPixelsMapsPointsToRenderPixels(t *testing.T) {
	x, y, width, height := cropPixels([]float64{0, 0, 100, 200})
	if x != 0 || y != 0 || width != 278 || height != 556 {
		t.Errorf("crop %d,%d %dx%d, want 0,0 278x556", x, y, width, height)
	}
}

// A whole page renders in its crop box (decision 123 amendment 3: the media box may hold leftovers
// the paper never prints), uncut; only a view is cropped, and then from the box the geometry was
// measured in.
func TestRasterArgsCropOnlyAView(t *testing.T) {
	whole := rasterArgs(&figdesc.Figure{Page: 2, PageBox: "cropbox"}, "in.pdf", "out")
	if !slices.Contains(whole, "-cropbox") || slices.Contains(whole, "-x") {
		t.Errorf("a whole page is not its crop box: %v", whole)
	}
	if unnamed := rasterArgs(&figdesc.Figure{Page: 1}, "in.pdf", "out"); !slices.Contains(unnamed, "-cropbox") {
		t.Errorf("a whole page with no box named (graphicx's default, the crop box): %v", unnamed)
	}
	if named := rasterArgs(&figdesc.Figure{Page: 1, PageBox: "mediabox"}, "in.pdf", "out"); slices.Contains(named, "-cropbox") {
		t.Errorf("a media-box page rendered in its crop box: %v", named)
	}
	view := rasterArgs(&figdesc.Figure{Page: 2, PageBox: "cropbox", Box: []float64{0, 0, 100, 200}}, "in.pdf", "out")
	for _, flag := range []string{"-cropbox", "-x", "-y", "-W", "-H"} {
		if !slices.Contains(view, flag) {
			t.Errorf("a view lacks %s: %v", flag, view)
		}
	}
	media := rasterArgs(&figdesc.Figure{Page: 2, PageBox: "mediabox", Box: []float64{0, 0, 100, 200}}, "in.pdf", "out")
	if slices.Contains(media, "-cropbox") {
		t.Errorf("a media-box view selected the crop box: %v", media)
	}
	trim := rasterArgs(&figdesc.Figure{Page: 2, PageBox: "trimbox", Box: []float64{0, 0, 100, 200}}, "in.pdf", "out")
	if slices.Contains(trim, "-x") {
		t.Errorf("a trim-box view was cropped in another box: %v", trim)
	}
}

func TestDrafterDraftsEachViewAndRedraftsCopiedDrafts(t *testing.T) {
	script, count := stub(t, `echo "drafted $RELATEX_FIGURE_NAME"`)
	document := pageFigureDocument(t)
	figures := figdesc.Figures(document)
	if len(figures) != 3 {
		t.Fatalf("figures %+v", figures)
	}
	// A sidecar as drafting before decision 123 left it: page 1's draft, copied to the other views.
	var sidecarText string
	for _, figure := range figures {
		sidecarText += fmt.Sprintf("[figs.%s]\nalt = \"first view\"\nsha256 = \"%s\"\nprovenance = \"model\"\n\n",
			figure.Key, figures[0].SHA256)
	}
	path := filepath.Join(t.TempDir(), figdesc.FileName)
	if err := authordir.Ensure(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(sidecarText), 0o644); err != nil {
		t.Fatal(err)
	}
	sidecar, err := figdesc.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	drafter := &Drafter{Command: shellCommand(t, script), Model: "stub", Timeout: 10 * time.Second, Today: "2026-11-11"}
	outcome, err := drafter.Run(document, sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Cached) != 1 || len(outcome.Drafted) != 2 || len(outcome.Reused) != 0 || runs(t, count) != 2 {
		t.Fatalf("first run %+v, %d runs", outcome, runs(t, count))
	}
	if entry := sidecar.Entries[figures[1].Key]; entry == nil || entry.SHA256 != figures[1].Identity() {
		t.Errorf("page 2's draft: %+v", entry)
	}
	outcome, err = drafter.Run(document, sidecar)
	if err != nil || len(outcome.Cached) != 3 || len(outcome.Drafted) != 0 || runs(t, count) != 2 {
		t.Errorf("second run %+v, %d runs", outcome, runs(t, count))
	}
}
