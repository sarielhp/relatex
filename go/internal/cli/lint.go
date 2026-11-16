package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/a11ylint"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
)

// lintLong is the lint command's help (decision 096).
const lintLong = "Lists what only the author can supply for an accessible export, read from the source without converting it: figures without a description, the document's language, tables without a marked header row, link texts that name nothing, notes on complex figures (plots, rich diagrams of the last conversion) with a short description only, and with --math a document-level note on bars no macro gives a meaning. Each item's note (FILE:LINE: note:) is the exact text to add there: the language line for the language the text reads as, the figure's alt key or sidecar entry (or its long description), the rule under the row that looks like a table's header. Exit status 0, or 1 with --strict when anything but advice on complex figures is listed. --annotate DIR also writes a copy of the source tree with each place marked (% relatex-a11y: comments, alt={TODO: …} placeholders, model drafts as alt={DRAFT: …}) and DIR/REVIEW.md listing them; the original is never changed. --review-page DIR writes an HTML page of every open item, with the source around it, the figures the product can show and each place's excerpt of the PDF page, copied to DIR/figs and DIR/crops. --patch FILE writes the suggestions as a unified diff; --accept REVIEW.toml keeps only the items a review page's saved decisions tick (one the current lint no longer has, or whose source line changed since the page was built, is refused and the exit status 1)"

// lintRequest is what a lint command line asked for.
type lintRequest struct {
	accessibility, strict, asJSON, math               bool
	main, output, annotate, reviewPage, patch, accept string
}

// lintSpec is relatex lint --accessibility: a fast source-level accessibility report.
func lintSpec() commandSpec {
	var request lintRequest
	options := []clihelp.Option{
		clihelp.Required(switchFlag("--accessibility", "The accessibility lint (the only lint so far)", func() { request.accessibility = true })),
		textFlag("--main <file>", "The main .tex file, a path inside the paper", &request.main),
		textFlag("-o, --output <dir>", "The last conversion's product directory, to name drawn figures (default: the project cache's)", &request.output),
		switchFlag("--math", "Also report bars in formulas without a macro saying what they mean (\\abs, \\card, \\norm), once"+
			" for the document (opt-in)", func() { request.math = true }),
		switchFlag("--strict", "Exit with status 1 when anything is listed", func() { request.strict = true }),
		switchFlag("--json", "Print the items as JSON", func() { request.asJSON = true }),
		textFlag("--annotate <dir>", "Write the annotated review copy of the source to DIR (decision 098)", &request.annotate),
		textFlag("--patch <file>", "Write the source suggestions as a unified diff to FILE (apply with patch -p1)", &request.patch),
		textFlag("--accept <file>", "With --patch: keep only the items the review page's saved decisions tick (relatex/lint.toml)",
			&request.accept),
		textFlag("--review-page <dir>", "Write an HTML page of every open item to DIR/index.html, with its figures in DIR/figs",
			&request.reviewPage),
	}
	return commandSpec{name: "lint", description: "List what only the author can supply for an accessible export, read from the source",
		long: lintLong, usage: Name + " lint --accessibility [DIRECTORY] [options]", options: options, args: clihelp.MaximumNArgs(1),
		validator: clihelp.RequiredWith("--accept", "--patch"), run: func(rest []string) ojson.Object { return lintCommand(rest, request) }}
}

// lintCommand runs the lint on the paper in rest (a directory, default here).
func lintCommand(rest []string, request lintRequest) ojson.Object {
	strict, asJSON, math := request.strict, request.asJSON, request.math
	main, output, annotate, reviewPage, patch, accept := request.main, request.output, request.annotate, request.reviewPage,
		request.patch, request.accept
	place := newAltPlace(rest, output)
	paper := lintPaper(place, main)
	items := a11ylint.Lint(paper, a11ylint.Options{Math: math})
	status := 0
	if strict && slices.ContainsFunc(items, func(item a11ylint.Item) bool { return !item.Advisory }) {
		status = 1
	}
	report := lintReport(items, a11ylint.ExportSummary(paper), asJSON)
	if annotate != "" {
		if written := lintAnnotate(place, paper, items, expandPath(annotate)); !asJSON {
			report += written
		}
	}
	if patch != "" {
		acceptPath := ""
		if accept != "" {
			acceptPath = expandPath(accept)
		}
		written, refused := lintPatch(place, paper, items, expandPath(patch), acceptPath)
		if !asJSON {
			report += written
		}
		if len(refused) > 0 {
			status = 1
		}
	}
	if reviewPage != "" {
		if written := lintReviewPage(place, paper, items, expandPath(reviewPage)); !asJSON {
			report += written
		}
	}
	panic(&exitRequest{stdout: report, status: status})
}

// lintAnnotate writes the annotated review copy and says where it is.
func lintAnnotate(place altPlace, paper a11ylint.Paper, items []a11ylint.Item, destination string) string {
	marks, err := a11ylint.Annotate(paper, items, place.directory, destination, []string{place.output})
	if err != nil {
		rec.Fail("%v", err)
	}
	placed := 0
	for _, mark := range marks {
		if mark.CopyLine > 0 {
			placed++
		}
	}
	return fmt.Sprintf("Annotated copy: %s (%d marks in the source, %d listed only in %s)\n",
		destination, placed, len(marks)-placed, a11ylint.ReviewFile)
}

// lintPatch writes the source suggestions as a unified diff and says where it is. With a decisions
// file, only the items it accepts go in; each refusal is reported and the caller exits 1.
func lintPatch(place altPlace, paper a11ylint.Paper, items []a11ylint.Item, destination, accept string) (string, []a11ylint.Refusal) {
	if info, err := os.Stat(destination); err == nil && info.IsDir() {
		rec.Fail("the patch %s is a directory; name a file", destination)
	}
	carried, refused := items, []a11ylint.Refusal{}
	if accept != "" {
		decisions, err := a11ylint.ReadDecisions(accept)
		if err != nil {
			rec.Fail("%v", err)
		}
		accepted := a11ylint.Accept(paper, items, decisions)
		carried, refused = accepted.Items, accepted.Refused
	}
	diff, err := a11ylint.Patch(paper, carried)
	if err != nil {
		rec.Fail("%v", err)
	}
	if err := os.WriteFile(destination, []byte(diff), 0o644); err != nil {
		rec.Fail("%v", err)
	}
	edits, files := patchStats(diff)
	written := fmt.Sprintf("Patch: %s (%d edits in %d files; apply: patch -p1 -d %s < %s)\n",
		destination, edits, files, place.directory, destination)
	for _, refusal := range refused {
		written += fmt.Sprintf("Refused %s: %s\n", refusal.ID, refusal.Reason)
	}
	return written, refused
}

// reviewFigureLimit bounds the figures the review page copies.
const reviewFigureLimit = 300

// lintReviewPage writes the review page and copies its figures; DIR must be new, empty or an
// earlier review page (decision 098).
func lintReviewPage(place altPlace, paper a11ylint.Paper, items []a11ylint.Item, destination string) string {
	if sameDirectory(destination, place.directory) {
		rec.Fail("the review page %s cannot be the paper directory; name a new directory", destination)
	}
	if sameDirectory(destination, place.output) {
		rec.Fail("the review page %s cannot be the product directory; name a new directory", destination)
	}
	if err := prepareReviewPage(destination); err != nil {
		rec.Fail("%v", err)
	}
	crops, cropNote := lintReviewCrops(paper, items, place.output, destination)
	page, figures := a11ylint.ReviewPage(paper, items, place.output, crops...)
	if err := os.WriteFile(filepath.Join(destination, "index.html"), []byte(page), 0o644); err != nil {
		rec.Fail("%v", err)
	}
	copied, skipped := copyReviewFigures(figures, destination)
	written := fmt.Sprintf("Review page: %s (%d items, %d figure groups)\n",
		filepath.Join(destination, "index.html"), len(items), a11ylint.ReviewGroups(items))
	if len(figures) > 0 {
		written += fmt.Sprintf("%d figures copied, %d skipped (at most %d)\n", copied, skipped, reviewFigureLimit)
	}
	written += cropNote
	return written
}

// lintReviewCrops renders the items' PDF crops into DIR/crops before the page names them; a missing
// PDF or a failed pdftocairo leaves the page without crops and says why instead of failing the lint.
func lintReviewCrops(paper a11ylint.Paper, items []a11ylint.Item, product, destination string) ([]a11ylint.Crop, string) {
	crops, err := a11ylint.Crops(paper, items, product)
	if err == nil && len(crops) > 0 {
		lowerPriority()
		_, err = a11ylint.RenderCrops(crops, filepath.Join(destination, "crops"), a11ylint.Poppler{})
	}
	if err != nil {
		_ = os.RemoveAll(filepath.Join(destination, "crops"))
		return nil, fmt.Sprintf("warning: no crops of the PDF pages: %v\n", err)
	}
	if len(crops) == 0 {
		return nil, ""
	}
	note := fmt.Sprintf("%d crops of the PDF pages", len(crops))
	if len(crops) == a11ylint.MaxCrops {
		note += fmt.Sprintf(" (at most %d; the rest listed without)", a11ylint.MaxCrops)
	}
	return crops, note + "\n"
}

// prepareReviewPage creates DIR, or replaces an earlier review page; any other non-empty directory
// is refused.
func prepareReviewPage(directory string) error {
	entries, err := os.ReadDir(directory)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return os.MkdirAll(directory, 0o755)
	case err != nil:
		return err
	case len(entries) == 0:
		return nil
	}
	index, err := os.ReadFile(filepath.Join(directory, "index.html"))
	if err != nil || !strings.Contains(string(index), `<meta name="generator" content="`+a11ylint.ReviewPageGenerator+`">`) {
		return fmt.Errorf("%s exists and is not an earlier review page; name a new directory", directory)
	}
	if err := os.RemoveAll(filepath.Join(directory, "figs")); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(directory, "crops")); err != nil {
		return err
	}
	return os.Remove(filepath.Join(directory, "index.html"))
}

// copyReviewFigures copies the page's figures under DIR/figs, at most reviewFigureLimit of them.
func copyReviewFigures(figures map[string]string, destination string) (copied, skipped int) {
	directory := filepath.Join(destination, "figs")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		rec.Fail("%v", err)
	}
	names := slices.Sorted(maps.Keys(figures))
	for _, name := range names {
		if copied == reviewFigureLimit {
			return copied, len(names) - copied
		}
		if err := copyReviewFigure(figures[name], filepath.Join(directory, filepath.Base(name))); err != nil {
			rec.Fail("%v", err)
		}
		copied++
	}
	return copied, 0
}

func copyReviewFigure(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// sameDirectory reports whether two paths name the same directory.
func sameDirectory(left, right string) bool {
	first, err := filepath.Abs(left)
	if err != nil {
		return false
	}
	second, err := filepath.Abs(right)
	if err != nil {
		return false
	}
	return filepath.Clean(first) == filepath.Clean(second)
}

// patchStats counts Patch's suggestions and files from its diff: an added line is an edit unless it
// re-adds the line just deleted (a last line that only changes its missing newline).
func patchStats(diff string) (edits, files int) {
	inHunk, removed := false, ""
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case !inHunk && strings.HasPrefix(line, "+++ b/"), inHunk && strings.HasPrefix(line, "+++ b/") && removed != "":
			files++
			inHunk, removed = false, ""
		case strings.HasPrefix(line, "@@ "):
			inHunk = true
		case !inHunk || strings.HasPrefix(line, `\`):
		case strings.HasPrefix(line, "-"):
			removed = line[1:]
		case strings.HasPrefix(line, "+"):
			if line[1:] != removed {
				edits++
			}
			removed = ""
		default:
			removed = ""
		}
	}
	return edits, files
}

// lintPaper reads the paper's sources, its sidecar and the last conversion's drawn figures.
func lintPaper(place altPlace, requested string) a11ylint.Paper {
	main, err := a11ylint.FindMain(place.directory, requested)
	if err != nil {
		rec.Fail("%v", err)
	}
	files, err := a11ylint.Load(place.directory, main)
	if err != nil {
		panic(systemCall(err))
	}
	paper := a11ylint.Paper{Files: files, Sidecar: place.sidecar(), Directory: place.directory}
	paperdoc.Root = Root
	bundle := filepath.Join(place.output, "document.json.gz")
	if _, err := os.Stat(bundle); err != nil {
		return paper
	}
	payload := readBundle(bundle)
	document := rec.AsMap(payload["document"])
	paper = paper.WithConversion(document)
	if sourceMap, ok := payload["source_map"].(rec.Map); ok {
		paper.SourceMap = sourceMap
	}
	paper = paper.WithExportFile(filepath.Join(place.output, "accessibility.json"), document)
	a11ylint.AddPlaces(place.directory, &paper)
	return paper
}

// lintReport is the items as compiler-style lines with a count by kind, or as JSON.
func lintReport(items []a11ylint.Item, summary string, asJSON bool) string {
	if asJSON {
		result := map[string]any{"items": items, "count": len(items)}
		if summary != "" {
			result["export"] = summary
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			rec.Fail("%v", err)
		}
		return string(data) + "\n"
	}
	var out strings.Builder
	counts := map[string]int{}
	kinds := []string{}
	for _, item := range items {
		location := item.File
		if item.Line > 0 {
			location += fmt.Sprintf(":%d", item.Line)
		}
		fmt.Fprintf(&out, "%s: %s: %s\n  fix: %s\n", location, item.Kind, item.Message, item.Fix)
		if item.Note != "" {
			fmt.Fprintf(&out, "%s:%d: note: %s\n", item.File, max(item.NoteLine, 1), item.Note)
		}
		if counts[item.Kind] == 0 {
			kinds = append(kinds, item.Kind)
		}
		counts[item.Kind]++
	}
	parts := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		parts = append(parts, fmt.Sprintf("%d %s", counts[kind], kind))
	}
	if len(items) == 0 {
		out.WriteString("Nothing for the author to supply was found in the source.\n")
	} else {
		fmt.Fprintf(&out, "%d items only the author can supply (%s)\n", len(items), strings.Join(parts, ", "))
	}
	if summary != "" {
		out.WriteString(summary + "\n")
	}
	return out.String()
}
