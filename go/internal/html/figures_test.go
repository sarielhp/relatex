package html

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// The expectations are PaperImages.plan's (test/paper_images_test.rb): a figure is named after
// the LaTeX file it was included from, else numbered in reading order (decision 068).
const figureSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>`

func figureDigest(bytes []byte) string {
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:])
}

func svgOf(width string) []byte {
	return []byte(strings.Replace(figureSVG, `width="10"`, `width="`+width+`"`, 1))
}

// figurePair is one figure: its media-1 asset and the occurrence that shows it.
type figurePair struct{ asset, span rec.Map }

// figure is one media-1 asset and its occurrence: an original PDF source and the SVG shown.
func figure(source, width string) figurePair {
	original := mediamodel.Encoded([]byte(source+" source"), "application/pdf")
	shown := mediamodel.Encoded(svgOf(width), "image/svg+xml")
	asset := rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": original["sha256"],
		"representations": rec.Map{"original": original, "html": shown, "markdown": shown, "typst": shown}}
	span := rec.Map{"type": "image", "asset": asset["sha256"], "width_sp": int64(655_360), "height_sp": int64(655_360)}
	if source != "" {
		span["source"] = source
	}
	return figurePair{asset: asset, span: span}
}

func figurePaper(figures ...figurePair) rec.Map {
	nodes, items, order := rec.Map{}, rec.Map{}, []any{}
	for index, figure := range figures {
		id := "n-" + string(rune('0'+index))
		span := rec.Map{"id": id + "-image"}
		for key, value := range figure.span {
			span[key] = value
		}
		nodes[id] = rec.Map{"id": id, "content": []any{span}}
		items[rec.FetchString(figure.asset, "sha256")] = figure.asset
		order = append(order, id)
	}
	return rec.Map{"block_order": order, "nodes": nodes, "assets": rec.Map{"items": items}}
}

func TestFigurePlanNamesFiguresAfterTheirLaTeXSource(t *testing.T) {
	displayed, originals := figures.Plan(figurePaper(figure("figs/plot.pdf", "10")))

	if got := displayed[figureDigest(svgOf("10"))]; got != "figs/plot.svg" {
		t.Errorf("displayed name = %q, want figs/plot.svg", got)
	}
	if got := originals[figureDigest([]byte("figs/plot.pdf source"))]; got != "figs/plot.pdf" {
		t.Errorf("original name = %q, want figs/plot.pdf", got)
	}
}

func TestFigurePlanNumbersFiguresThePaperDrawsItself(t *testing.T) {
	rendered := "rendered/" + strings.Repeat("0", 64) + ".pdf"
	displayed, _ := figures.Plan(figurePaper(figure("", "10"), figure(rendered, "11"), figure("figs/plot.pdf", "12")))

	want := map[string]string{
		figureDigest(svgOf("10")): "figs/fig-1.svg", figureDigest(svgOf("11")): "figs/fig-2.svg",
		figureDigest(svgOf("12")): "figs/plot.svg",
	}
	if len(displayed) != len(want) {
		t.Fatalf("displayed = %v, want %v", displayed, want)
	}
	for digest, name := range want {
		if displayed[digest] != name {
			t.Errorf("displayed[%s] = %q, want %q", digest[:8], displayed[digest], name)
		}
	}
}

func TestFigurePlanSuffixesARepeatedFileNameAndSlugsAName(t *testing.T) {
	displayed, _ := figures.Plan(figurePaper(figure("figs/plot.pdf", "10"), figure("figs/other/plot.pdf", "11"),
		figure("figs/My Plot (final).pdf", "12")))

	want := map[string]string{
		figureDigest(svgOf("10")): "figs/plot.svg", figureDigest(svgOf("11")): "figs/plot-2.svg",
		figureDigest(svgOf("12")): "figs/My-Plot-final.svg",
	}
	for digest, name := range want {
		if displayed[digest] != name {
			t.Errorf("displayed[%s] = %q, want %q", digest[:8], displayed[digest], name)
		}
	}
}

func TestFigurePlanNamesOneFileForOneFigureShownTwice(t *testing.T) {
	shown := figure("figs/plot.pdf", "10")
	displayed, _ := figures.Plan(figurePaper(shown, shown))

	if len(displayed) != 1 || displayed[figureDigest(svgOf("10"))] != "figs/plot.svg" {
		t.Errorf("displayed = %v, want one figs/plot.svg", displayed)
	}
}

func TestFigurePlanWithoutAssets(t *testing.T) {
	displayed, originals := figures.Plan(rec.Map{"block_order": []any{}, "nodes": rec.Map{}, "assets": rec.Map{"status": "unverified"}})

	if len(displayed) != 0 || len(originals) != 0 {
		t.Errorf("displayed = %v, originals = %v, want none", displayed, originals)
	}
}

// A paper without images has unverified assets and no items; writing its figures writes nothing.
func TestWriteImageFilesToleratesAssetsWithoutItems(t *testing.T) {
	document := rec.Map{"block_order": []any{}, "nodes": rec.Map{}, "assets": rec.Map{"status": "unverified"}}
	WriteImageFiles(document, t.TempDir())
}

// Two figures whose names differ only in case (decision 129, amendment 5) are not refused: a file
// with other bytes takes a number, identical bytes share the first name, and both are reported.
func TestFigurePlanKeepsCaseTwinsApart(t *testing.T) {
	document := figurePaper(figure("figs/Plot.pdf", "10"), figure("img/plot.pdf", "11"))
	displayed, _ := figures.Plan(document)
	if displayed[figureDigest(svgOf("10"))] != "figs/Plot.svg" || displayed[figureDigest(svgOf("11"))] != "figs/plot-2.svg" {
		t.Errorf("different bytes: %v", displayed)
	}
	twins := figures.CaseTwins(document)
	if len(twins) != 1 || twins[0].First != "figs/Plot.pdf" || twins[0].Second != "img/plot.pdf" || twins[0].Identical {
		t.Errorf("twins = %+v", twins)
	}
}

func TestFigurePlanGivesIdenticalCaseTwinsOneName(t *testing.T) {
	first := figure("figs/Plot.pdf", "10")
	second := figure("img/plot.pdf", "10")
	second.span["asset"] = first.span["asset"]
	document := figurePaper(first, second)
	displayed, _ := figures.Plan(document)
	if len(displayed) != 1 || displayed[figureDigest(svgOf("10"))] != "figs/Plot.svg" {
		t.Errorf("identical bytes: %v", displayed)
	}
	if twins := figures.CaseTwins(document); len(twins) != 1 || !twins[0].Identical {
		t.Errorf("twins = %+v", twins)
	}
}
