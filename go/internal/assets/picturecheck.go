package assets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/inkdiff"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/rec"
)

// checkDPI is the resolution both renderings of a picture are compared at (decision 137).
const checkDPI = "144"

// Picture fallback classes (decision 137): why a picture drawn by TeX keeps its labels as outlines.
const (
	fallbackTrace  = "trace"
	fallbackGlyph  = "glyph"
	fallbackFont   = "font"
	fallbackRender = "render"
	fallbackDiffer = "differs"
)

// pictureResult is one picture's outcome: converted, without text, or a fallback class and why.
type pictureResult struct {
	converted, empty bool
	class, reason    string
}

// TextPictures is decision 137: each picture cropped from the PDF (crops, by reading filename) gets
// display SVGs whose labels are SVG text in the document's fonts, where the text form renders like
// the crop within decision 119's tolerance; a picture that does not keeps its outlines, and a
// diagnostic names it. It runs after the document's fonts are known.
func TextPictures(document rec.Map, crops map[string][]byte, diags *diagnostics.Diagnostics) {
	items, _ := rec.FetchMap(document, "assets")["items"].(rec.Map)
	shas := []string{}
	for name, data := range crops {
		sha := hexDigest(data)
		if _, ok := items[sha]; ok && strings.HasPrefix(name, "rendered/") && !slices.Contains(shas, sha) {
			shas = append(shas, sha)
		}
	}
	if len(shas) == 0 {
		return
	}
	slices.Sort(shas)
	fonts := newPictureFonts(document)
	results := make([]pictureResult, len(shas))
	limit := make(chan struct{}, runtime.NumCPU())
	var group sync.WaitGroup
	for index, sha := range shas {
		group.Go(func() {
			limit <- struct{}{}
			defer func() { <-limit }()
			results[index] = textAsset(rec.AsMap(items[sha]), fonts)
		})
	}
	group.Wait()
	reportPictures(shas, results, diags)
}

func reportPictures(shas []string, results []pictureResult, diags *diagnostics.Diagnostics) {
	converted, classes := 0, map[string][]string{}
	for index, result := range results {
		if result.converted {
			converted++
		}
		if result.class != "" {
			classes[result.class] = append(classes[result.class], shas[index][:12]+": "+result.reason)
		}
	}
	if converted > 0 {
		diags.Add("figure.picture_text", fmt.Sprintf("%d figure(s) drawn by TeX show their labels as text in the document's fonts", converted), int64(converted))
	}
	names := make([]string, 0, len(classes))
	for class := range classes {
		names = append(names, class)
	}
	slices.Sort(names)
	for _, class := range names {
		reasons := classes[class]
		diags.Add("figure.picture_text_fallback", fmt.Sprintf("%d figure(s) drawn by TeX keep their labels as outlines (%s): %s",
			len(reasons), class, strings.Join(reasons, "; ")), int64(len(reasons)))
	}
}

// textAsset converts every display SVG of one picture asset, all or none.
func textAsset(asset rec.Map, fonts pictureFonts) (result pictureResult) {
	defer func() {
		if failure := recover(); failure != nil {
			result = pictureResult{class: fallbackRender, reason: fmt.Sprint(failure)}
		}
	}()
	representations := rec.FetchMap(asset, "representations")
	crop := mediamodel.DataURIBytes(rec.FetchString(rec.FetchMap(representations, "original"), "data_uri"))
	page, err := tracePicture(crop)
	if err != nil {
		return pictureResult{class: fallbackTrace, reason: err.Error()}
	}
	traced := tracedGlyphs(page)
	replaced := rec.Map{}
	keys := make([]string, 0, len(representations))
	for key := range representations {
		if strings.HasPrefix(key, "display:") {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	for _, key := range keys {
		representation := rec.AsMap(representations[key])
		svg, failure := verifiedText(mediamodel.DataURIBytes(rec.FetchString(representation, "data_uri")), traced, fonts)
		if failure.class != "" || failure.empty {
			return failure
		}
		encoded := mediamodel.Encoded(svg, "image/svg+xml")
		encoded["placement"] = representation["placement"]
		replaced[key] = encoded
	}
	for key, value := range replaced {
		representations[key] = value
	}
	return pictureResult{converted: len(replaced) > 0}
}

// verifiedText is one display SVG in text form, when it renders like the crop.
func verifiedText(display []byte, traced []tracedGlyph, fonts pictureFonts) ([]byte, pictureResult) {
	svg, err := textPicture(display, traced, fonts)
	switch {
	case errors.Is(err, errNoText):
		return nil, pictureResult{empty: true}
	case err != nil && strings.HasPrefix(err.Error(), "font "):
		return nil, pictureResult{class: fallbackFont, reason: err.Error()}
	case err != nil && strings.HasPrefix(err.Error(), "glyph ") && strings.Contains(err.Error(), "no character"):
		return nil, pictureResult{class: fallbackGlyph, reason: err.Error()}
	case err != nil:
		return nil, pictureResult{class: fallbackTrace, reason: err.Error()}
	}
	svg = mediamodel.ValidateSVG(svg)
	if reason := compareRenderings(display, svg); reason != "" {
		class := fallbackDiffer
		if strings.HasPrefix(reason, "render") {
			class = fallbackRender
		}
		return nil, pictureResult{class: class, reason: reason}
	}
	return svg, pictureResult{}
}

// tracePicture is mutool's trace of the crop's one page.
func tracePicture(crop []byte) (*nativetrace.Node, error) {
	directory := command.Scratch("picture-text-")
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, "crop.pdf")
	if err := os.WriteFile(path, crop, 0o644); err != nil {
		return nil, fmt.Errorf("write crop: %w", err)
	}
	output, err := runTool("mutool", directory, "trace", path, "1")
	if err != nil {
		return nil, err
	}
	if nativetrace.Declared(output) {
		return nil, errors.New("mutool trace: declarations in the trace")
	}
	document, err := nativetrace.ParseTree(output)
	if err != nil {
		return nil, fmt.Errorf("mutool trace: %w", err)
	}
	pages := document.Descendants("page")
	if len(pages) != 1 {
		return nil, fmt.Errorf("mutool trace: %d pages", len(pages))
	}
	return pages[0], nil
}

// compareRenderings renders the crop's outline SVG (MuPDF's drawing of the crop, the figure shown
// before decision 137) and its text SVG with one rasterizer (rsvg-convert) at checkDPI and compares
// their ink (inkdiff: decision 119's rule without its glyph exemption); empty when alike. The two
// SVGs differ only in their glyphs, so the comparison sees only the text.
func compareRenderings(outline, text []byte) string {
	directory := command.Scratch("picture-check-")
	defer os.RemoveAll(directory)
	inks := make([]inkdiff.Ink, 2)
	for index, svg := range [][]byte{outline, text} {
		ink, err := renderInk(directory, []string{"outline", "text"}[index], svg)
		if err != nil {
			return "render: " + err.Error()
		}
		inks[index] = ink
	}
	return compareInk(inks[0], inks[1])
}

// renderInk is an SVG's ink at checkDPI.
func renderInk(directory, name string, svg []byte) (inkdiff.Ink, error) {
	source, target := filepath.Join(directory, name+".svg"), filepath.Join(directory, name+".png")
	if err := os.WriteFile(source, svg, 0o644); err != nil {
		return inkdiff.Ink{}, fmt.Errorf("write %s: %w", name, err)
	}
	if _, err := runTool("rsvg-convert", directory, "-d", checkDPI, "-p", checkDPI, "-b", "white", "-f", "png", "-o",
		target, source); err != nil {
		return inkdiff.Ink{}, err
	}
	return readInk(target)
}

// compareInk compares two inks of one picture; a size differing by a pixel (each renderer rounds the
// picture's size its own way) compares their common part.
func compareInk(before, after inkdiff.Ink) string {
	if abs(before.Width-after.Width) > 1 || abs(before.Height-after.Height) > 1 {
		return fmt.Sprintf("render: sizes %dx%d and %dx%d", before.Width, before.Height, after.Width, after.Height)
	}
	width, height := min(before.Width, after.Width), min(before.Height, after.Height)
	changed := inkdiff.Changed(before.Crop(width, height), after.Crop(width, height))
	if len(changed) == 0 {
		return ""
	}
	box := changed[0]
	return fmt.Sprintf("%d region(s) differ from the crop beyond a shift, the first at %d,%d-%d,%d px (%s dpi)",
		len(changed), box[0], box[1], box[2], box[3], checkDPI)
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func readInk(path string) (inkdiff.Ink, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return inkdiff.Ink{}, fmt.Errorf("read rendering: %w", err)
	}
	picture, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return inkdiff.Ink{}, fmt.Errorf("decode rendering: %w", err)
	}
	return inkdiff.FromImage(picture), nil
}

// runTool runs one helper program in directory, bounded in time, and returns its standard output.
func runTool(name, directory string, arguments ...string) ([]byte, error) {
	tool := command.Resolve(name)
	if tool == "" {
		return nil, fmt.Errorf("%s not found", name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, arguments...)
	cmd.Dir = directory
	cmd.Env = command.Environment(nil)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		line, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		return nil, fmt.Errorf("%s: %w: %s", name, err, line)
	}
	return out.Bytes(), nil
}
