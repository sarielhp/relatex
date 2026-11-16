package accessibility

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
)

// SchemaVersion names schemas/accessibility-report.schema.json.
const SchemaVersion = "accessibility-0.1"

// proseWords is how many words of three or more letters make a rendered picture an image of
// prose (WCAG 1.4.5): a drawing's labels and axis titles stay below it, a box of text does not.
const proseWords = 25

// textLimit bounds one pdftotext run on a figure original.
const textLimit = 30 * time.Second

var word = regexp.MustCompile(`\p{L}{3,}`)

// Report is accessibility.json: the targets, the claim, axe-core's result (absent when it was not
// measured), the structural rows and the open items both raise. status is "measured",
// "unmeasured" (axe failed) or "unverified" (no browser). The claim is made only when it is
// mechanically true (decision 080 item 5): axe-core measured both themes and found nothing, and
// every structural check passed (an unreviewed draft is a gap of its own); else it is null.
func Report(status, reason string, axe rec.Map, structural []any) rec.Map {
	items := openItems(axe, structural)
	claim := any(nil)
	if status == "measured" && len(items) == 0 && measuredThemes(axe) && len(structural) > 0 {
		claim = htmlkit.WCAGClaim
	}
	report := rec.Map{"schema_version": SchemaVersion, "status": status, "target": "WCAG 2.2 AA", "legal_bar": "WCAG 2.1 AA",
		"claim": claim, "structural": structural, "open_items": items}
	if axe != nil {
		report["axe"] = axe
	}
	if reason != "" {
		report["reason"] = reason
	}
	return report
}

// measuredThemes: axe-core's result holds both themes.
func measuredThemes(axe rec.Map) bool {
	themes, _ := axe["themes"].(rec.Map)
	_, light := themes["light"].([]any)
	_, dark := themes["dark"].([]any)
	return light && dark
}

// openItems: one per violated axe rule (its largest count over the themes, the themes it fails in)
// and one per structural gap.
func openItems(axe rec.Map, structural []any) []any {
	items := []any{}
	rules := map[string]rec.Map{}
	order := []string{}
	themes, _ := axe["themes"].(rec.Map)
	for _, theme := range []string{"light", "dark"} {
		violations, _ := themes[theme].([]any)
		for _, value := range violations {
			violation := rec.AsMap(value)
			id := str(violation["id"])
			count, _ := rec.Number(violation["count"])
			item, seen := rules[id]
			if !seen {
				item = rec.Map{"source": "axe", "id": id, "count": int64(0), "themes": []any{}}
				rules[id] = item
				order = append(order, id)
			}
			item["count"] = max(item["count"].(int64), int64(count))
			item["themes"] = append(item["themes"].([]any), theme)
		}
	}
	for _, id := range order {
		items = append(items, rules[id])
	}
	for _, value := range structural {
		row := rec.AsMap(value)
		if row["status"] == "gap" {
			items = append(items, rec.Map{"source": "structural", "id": row["check"], "criterion": row["criterion"], "count": row["count"]})
		}
	}
	return items
}

// textImageItems: content exported as a picture (decision 080 item 3) — an image inside a display
// formula, an image as a table float's content, and a picture rendered from the PDF whose own text
// is prose.
func textImageItems(document rec.Map, images []placedSpan, htmlRoot string) []any {
	items := []any{}
	var displayed map[string]string
	for _, image := range images {
		kind := enclosing(image.ancestors)
		if kind == "" && strings.HasPrefix(str(image.span["source"]), "rendered/") {
			if displayed == nil {
				displayed, _ = figures.Plan(document)
			}
			digest, _ := figures.Identity(document, image.span)
			if words := originalWords(htmlRoot, displayed[digest]); words >= proseWords {
				items = append(items, rec.Map{"id": image.span["id"], "kind": "prose", "words": words})
			}
			continue
		}
		if kind != "" {
			items = append(items, rec.Map{"id": image.span["id"], "kind": kind})
		}
	}
	return items
}

// enclosing is "math" for an image inside a display formula, "table" for one in a table float
// (not in a figure inside it), else "".
func enclosing(ancestors []string) string {
	for _, kind := range ancestors {
		switch kind {
		case "display_math":
			return "math"
		case "figure_float":
			return ""
		case "table_float":
			return "table"
		}
	}
	return ""
}

// originalWords counts the words of the PDF original kept beside a displayed figure (figs/x.pdf
// beside figs/x.svg); -1 when there is none or it cannot be read.
func originalWords(htmlRoot, relative string) int64 {
	if relative == "" {
		return -1
	}
	original := filepath.Join(htmlRoot, strings.TrimSuffix(relative, filepath.Ext(relative))+".pdf")
	tool := command.Resolve("pdftotext")
	if _, err := os.Stat(original); err != nil || tool == "" {
		return -1
	}
	limited, cancel := context.WithTimeout(context.Background(), textLimit)
	defer cancel()
	run := exec.CommandContext(limited, tool, "-q", "-enc", "UTF-8", original, "-")
	run.Env = command.Environment(nil)
	var output bytes.Buffer
	run.Stdout = &output
	if run.Run() != nil {
		return -1
	}
	return int64(len(word.FindAll(output.Bytes(), -1)))
}

// Nodes is the number of violating elements over both themes.
func Nodes(axe rec.Map) int64 {
	total := int64(0)
	themes, _ := axe["themes"].(rec.Map)
	for _, value := range themes {
		violations, _ := value.([]any)
		for _, violation := range violations {
			count, _ := rec.Number(rec.AsMap(violation)["count"])
			total += int64(count)
		}
	}
	return total
}

// str is a string value, or "" for an absent one.
func str(value any) string {
	text, _ := value.(string)
	return text
}
