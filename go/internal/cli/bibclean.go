package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/bibclean"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

const bibcleanLong = "Cleans one or more .bib files for Typst's Pergamon/citegeist bibliography engine: expands @string and plain argument-free @preamble macros, converts text-mode LaTeX to Typst markup and Unicode NFC, keeps case-protecting braces and plain $...$ math, and writes only the selected entries (with crossref/xdata parents) to stdout."

func bibcleanSpec() commandSpec {
	var keys []string
	all := false
	reportPath := ""
	options := []clihelp.Option{
		listFlag("--keys <keys>", "Comma-separated citation keys to keep (may be repeated)", func(list []string) { keys = append(keys, list...) }),
		valueFlag("--keys-file <file>", "Citation keys to keep, one per line (for more keys than a command line holds)",
			func(value string) error { keys = append(keys, readKeysFile(expandPath(value))...); return nil }),
		switchFlag("--all", "Clean and output every entry in the input .bib files", func() { all = true }),
		valueFlag("--report <file>", "Write the per-entry JSON report to FILE instead of stderr",
			func(value string) error { reportPath = expandPath(value); return nil }),
	}
	return commandSpec{name: "bibclean", description: "Clean .bib files for Typst's bibliography engine",
		long: bibcleanLong, usage: Name + " bibclean [--keys k1,k2|--keys-file FILE|--all] [--report FILE] FILE.bib...", options: options,
		args: clihelp.MinimumNArgs(1), run: func(rest []string) ojson.Object { return bibcleanCommand(rest, &keys, &all, &reportPath) }}
}

func bibcleanCommand(rest []string, selected *[]string, everything *bool, reportFile *string) ojson.Object {
	keys, all, reportPath := *selected, *everything, *reportFile
	if !all && len(keys) == 0 {
		fail("relatex bibclean needs --keys LIST, --keys-file FILE or --all")
	}
	if all && len(keys) > 0 {
		fail("relatex bibclean accepts either --keys or --all, not both")
	}
	paths := make([]string, len(rest))
	for i, arg := range rest {
		paths[i] = expandPath(arg)
	}
	cleaned, report, err := bibclean.CleanFiles(paths, bibclean.Options{Keys: keys, All: all})
	if err != nil {
		rec.Fail("%v", err)
	}
	stderrText := ""
	if reportPath != "" {
		data := append(ojson.Pretty(reportToOrdered(report)), '\n')
		if writeErr := os.WriteFile(reportPath, data, 0o644); writeErr != nil {
			panic(systemCall(writeErr))
		}
	} else {
		stderrText = formatBibcleanReport(report)
	}
	panic(&exitRequest{stdout: cleaned, stderr: stderrText, status: 0})
}

func formatBibcleanReport(report bibclean.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "bibclean: %d converted, %d unconverted\n", report.Converted, report.Unconverted)
	if len(report.MissingKeys) > 0 {
		fmt.Fprintf(&b, "missing keys: %s\n", strings.Join(report.MissingKeys, ", "))
	}
	if len(report.MissingParents) > 0 {
		fmt.Fprintf(&b, "missing parents: %s\n", strings.Join(report.MissingParents, ", "))
	}
	for _, entry := range report.Entries {
		if entry.Status == bibclean.StatusConverted {
			continue
		}
		parts := make([]string, 0, len(entry.Reasons))
		for _, r := range entry.Reasons {
			parts = append(parts, formatReason(r))
		}
		fmt.Fprintf(&b, "unconverted %s (%s): %s\n", entry.Key, entry.Type, strings.Join(parts, "; "))
	}
	return b.String()
}

func formatReason(r bibclean.Reason) string {
	switch {
	case r.Detail != "" && r.Field != "":
		return fmt.Sprintf("%s in %s", r.Detail, r.Field)
	case r.Name != "" && r.Field != "":
		return fmt.Sprintf("%s (%s) in %s", r.Kind, r.Name, r.Field)
	case r.Detail != "":
		return r.Detail
	case r.Field != "":
		return fmt.Sprintf("%s in %s", r.Kind, r.Field)
	default:
		return r.Kind
	}
}

func reportToOrdered(report bibclean.Report) ojson.Object {
	entries := make([]any, 0, len(report.Entries))
	for _, e := range report.Entries {
		obj := ojson.Object{
			{"key", e.Key},
			{"type", e.Type},
			{"status", string(e.Status)},
		}
		if len(e.Reasons) > 0 {
			reasons := make([]any, 0, len(e.Reasons))
			for _, r := range e.Reasons {
				ro := ojson.Object{{"kind", r.Kind}}
				if r.Name != "" {
					ro = append(ro, ojson.Pair{"name", r.Name})
				}
				if r.Field != "" {
					ro = append(ro, ojson.Pair{"field", r.Field})
				}
				if r.Detail != "" {
					ro = append(ro, ojson.Pair{"detail", r.Detail})
				}
				reasons = append(reasons, ro)
			}
			obj = append(obj, ojson.Pair{"reasons", reasons})
		}
		entries = append(entries, obj)
	}
	root := ojson.Object{
		{"converted", report.Converted},
		{"unconverted", report.Unconverted},
		{"entries", entries},
	}
	if len(report.MissingKeys) > 0 {
		root = append(root, ojson.Pair{"missing_keys", toAnyStrings(report.MissingKeys)})
	}
	if len(report.MissingParents) > 0 {
		root = append(root, ojson.Pair{"missing_parents", toAnyStrings(report.MissingParents)})
	}
	return root
}

func toAnyStrings(items []string) []any {
	out := make([]any, len(items))
	for i, s := range items {
		out[i] = s
	}
	return out
}

// readKeysFile reads one citation key per line; blank lines are ignored.
func readKeysFile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(systemCall(err))
	}
	var keys []string
	for _, text := range strings.Split(string(data), "\n") {
		if key := strings.TrimSpace(text); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}
