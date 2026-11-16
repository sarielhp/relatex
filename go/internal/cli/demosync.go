package cli

import (
	"fmt"
	"path/filepath"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/syncdemo"
)

// demoSyncLong is the demo-sync command's help (decision 192).
const demoSyncLong = "Writes a click-to-source demo of a finished conversion: one directory whose index.html shows the paper's HTML and its LaTeX source side by side. A click (or Enter) on a word or a formula of the paper scrolls the source to it and highlights it; a click on a word of the source does the same in the paper. It reads the full product (convert with --product DIR; the default is the last conversion's product): the word-level source map, the glyph owners that tie each word to its paragraph, and the export's HTML, whose figures and fonts it copies as they are. The demo page carries one small inline script; the HTML and EPUB exports never do, and this command changes nothing they write. Words the map gives a line only highlight their source line; words and formulas without a source range are not clickable"

// demoSyncSpec is relatex demo-sync.
func demoSyncSpec() commandSpec {
	var output string
	options := []clihelp.Option{
		clihelp.Required(textFlag("-o, --output <dir>", "The directory the demo is written to", &output)),
	}
	return commandSpec{name: "demo-sync", description: "Write a click-to-source demo of a finished conversion: its HTML beside its LaTeX source",
		long: demoSyncLong, usage: Name + " demo-sync [PRODUCT_DIRECTORY] -o DIRECTORY", options: options,
		args: clihelp.MaximumNArgs(1), human: demoSyncHuman, run: func(rest []string) ojson.Object { return demoSyncCommand(rest, output) }}
}

// demoSyncCommand builds the demo from the product in rest (default: the last conversion's).
func demoSyncCommand(rest []string, output string) ojson.Object {
	product := lastProduct(".", nil)
	if len(rest) == 1 {
		product = rest[0]
	}
	product, output = expandPath(product), expandPath(output)
	stats, err := syncdemo.Write(product, output)
	if err != nil {
		rec.Fail("demo-sync: %v", err)
	}
	return ojson.Object{{"status", "success"}, {"product", product}, {"output", output}, {"page", filepath.Join(output, "index.html")},
		{"words", int64(stats.Words)}, {"owned", int64(stats.Owned)}, {"matched", int64(stats.Matched)}, {"exact", int64(stats.Exact)},
		{"line_only", int64(stats.LineOnly)}, {"unmapped_words", stats.Missed}, {"formulas", int64(stats.Formulas)}, {"formulas_mapped", int64(stats.FormulasMapped)}}
}

func demoSyncHuman(result ojson.Object) string {
	number := func(key string) int64 { value, _ := result.Get(key).(int64); return value }
	words, matched := number("words"), number("matched")
	percent := 0.0
	if words > 0 {
		percent = 100 * float64(matched) / float64(words)
	}
	return fmt.Sprintf("Wrote %s\n  %d of %d prose words map to the source (%.1f%%; %d with a column, %d with the line only)\n"+
		"  %d of %d formulas map to a source range\n", rec.ToS(result.Get("page")), matched, words, percent, number("exact"),
		number("line_only"), number("formulas_mapped"), number("formulas"))
}
