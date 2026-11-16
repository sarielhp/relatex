package cli

import (
	"fmt"
	"strings"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// The commands that report on a thing (export, validate, inspect, sync, capabilities, layers, config,
// alt, tag-pdf) print a short text for a person: what they did, where the result is and what went
// wrong. --report prints the JSON report on stdout, as the tools read it, the way convert does.

const (
	// outlineDepth is how many levels of a report the generic outline opens.
	outlineDepth = 2
	// listedItems is how many items of a list the text names before counting the rest.
	listedItems = 8
	// longestValue is the longest scalar the text prints in full.
	longestValue = 100
)

// withReport gives a command (and its subcommands) the --[no-]report option and the text it prints
// without it. A command that already prints text has no human view here and is left as it is.
func (s *session) withReport(spec commandSpec) commandSpec {
	for index := range spec.subcommands {
		spec.subcommands[index] = s.withReport(spec.subcommands[index])
	}
	if spec.human == nil {
		return spec
	}
	spec.options = append(spec.options, toggleFlag("--[no-]report", "Print the JSON report on stdout, as tools read it, instead of the short text",
		func(value bool) { s.asReport = value }))
	return spec
}

// outline prints the fields of a report one per line: a scalar in full (shortened when long), a short
// list of scalars inline, a longer list or a closed object as its size, an open object indented.
func outline(object ojson.Object, indent string, depth int) string {
	var out strings.Builder
	for _, pair := range object {
		key := pair.Key()
		switch value := pair.Value().(type) {
		case ojson.Object:
			if depth > 0 {
				out.WriteString(indent + key + ":\n" + outline(value, indent+"  ", depth-1))
			} else {
				fmt.Fprintf(&out, "%s%s: %s\n", indent, key, countOf(len(value), "field"))
			}
		case rec.Map:
			fmt.Fprintf(&out, "%s%s: %s\n", indent, key, countOf(len(value), "field"))
		case []any:
			out.WriteString(indent + key + ": " + listText(value) + "\n")
		default:
			out.WriteString(indent + key + ": " + scalarText(value) + "\n")
		}
	}
	return out.String()
}

// countOf is "1 field", "3 fields".
func countOf(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

// scalarText is a scalar as a person reads it, long text shortened.
func scalarText(value any) string {
	text := strings.Join(strings.Fields(rec.ToS(value)), " ")
	if value == nil {
		text = "none"
	}
	if len(text) > longestValue {
		text = text[:longestValue] + "..."
	}
	return text
}

// listText is a list: its few scalars inline, else how many items it has.
func listText(list []any) string {
	if len(list) == 0 {
		return "none"
	}
	if len(list) > listedItems {
		return countOf(len(list), "item")
	}
	parts := make([]string, len(list))
	for index, item := range list {
		if _, scalar := item.(ojson.Object); scalar {
			return countOf(len(list), "entry")
		}
		parts[index] = scalarText(item)
	}
	return strings.Join(parts, ", ")
}

// inline is the scalar fields of an object on one line: key=value pairs.
func inline(object ojson.Object) string {
	parts := []string{}
	for _, pair := range object {
		switch pair.Value().(type) {
		case ojson.Object, rec.Map, []any:
			continue
		}
		parts = append(parts, pair.Key()+"="+scalarText(pair.Value()))
	}
	return strings.Join(parts, " ")
}

// entries lists the objects of a list one line each, at most listedItems, then how many more.
func entries(list []any, indent string) string {
	var out strings.Builder
	for index, item := range list {
		if index == listedItems {
			fmt.Fprintf(&out, "%sand %d more in the --report output\n", indent, len(list)-listedItems)
			break
		}
		if object, ok := item.(ojson.Object); ok {
			out.WriteString(indent + inline(object) + "\n")
		} else {
			out.WriteString(indent + scalarText(item) + "\n")
		}
	}
	return out.String()
}

// problemLines are a list of losses or diagnostics as the "problems" of a text: none, or each one line.
func problemLines(label string, list []any) string {
	if len(list) == 0 {
		return ""
	}
	return fmt.Sprintf("  %s: %s\n%s", label, countOf(len(list), "entry"), entries(list, "    "))
}

// exportHuman is what an export or reconstruct did: the format, where the directory is, the losses.
func exportHuman(format string, result ojson.Object) string {
	text := fmt.Sprintf("Wrote %s to %s\n", format, rec.ToS(result.Get("directory")))
	if file := rec.ToS(result.Get("file")); file != "" {
		text += "  main file: " + file + "\n"
	}
	if status := rec.ToS(result.Get("status")); status != "success" {
		text += "  status: " + status + "\n"
	}
	losses, _ := result.Get("losses").([]any)
	if len(losses) == 0 {
		return text + "  no losses\n"
	}
	return text + problemLines("losses", losses)
}

// validateHuman is the verdict on a bundle and what it holds.
func validateHuman(result ojson.Object) string {
	text := fmt.Sprintf("Bundle %s: %s nodes, %s images, %s fonts\n", rec.ToS(result.Get("status")), rec.ToS(result.Get("nodes")),
		rec.ToS(result.Get("images")), rec.ToS(result.Get("fonts")))
	return text + "  scope: " + rec.ToS(result.Get("scope")) + "\n"
}

// inspectHuman is what the inspected part of a bundle holds, with the bundle named.
func inspectHuman(result ojson.Object) string {
	inner, _ := result.Get("result").(ojson.Object)
	subject := "MathML"
	if bundle := rec.ToS(result.Get("bundle")); bundle != "" {
		subject = bundle
	}
	return "Inspected " + subject + "\n" + outline(inner, "  ", outlineDepth)
}

// syncHuman is the words a query found, one line each.
func syncHuman(result ojson.Object) string {
	matches, _ := result.Get("matches").([]any)
	text := fmt.Sprintf("%s for the %s query\n", countOf(len(matches), "match"), rec.ToS(result.Get("direction")))
	text += entries(matches, "  ")
	if editor, ok := result.Get("editor").(ojson.Object); ok {
		text += "  editor: " + inline(editor) + "\n"
	}
	return text
}

// capabilitiesHuman is what the build implements, one line per area.
func capabilitiesHuman(result ojson.Object) string {
	return "relatex capabilities (--report for the whole JSON)\n" + outline(result, "  ", 0)
}

// layersHuman is an attempt's stages one line each, or one stage's receipt.
func layersHuman(result ojson.Object) string {
	stages, ok := result.Get("stages").([]any)
	if !ok {
		return outline(result, "", outlineDepth)
	}
	return outline(withoutList(result, "stages"), "", 0) + countOf(len(stages), "stage") + ":\n" + entries(stages, "  ")
}

// withoutList is the object without one field.
func withoutList(object ojson.Object, key string) ojson.Object {
	kept := ojson.Object{}
	for _, pair := range object {
		if pair.Key() != key {
			kept = append(kept, pair)
		}
	}
	return kept
}

// configHuman is the settings files read and the settings in force.
func configHuman(result ojson.Object) string {
	text := ""
	if written := rec.ToS(result.Get("written")); written != "" {
		text += "Wrote " + written + "\n"
	}
	files, _ := result.Get("files").([]any)
	text += "Settings files: " + listText(files) + "\n"
	if settings, ok := result.Get("settings").(ojson.Object); ok {
		text += "Settings in force:\n" + outline(settings, "  ", 1)
	}
	return text
}

// altHuman is what an alt command did to the sidecar, field by field, the sidecar first.
func altHuman(name string) func(ojson.Object) string {
	return func(result ojson.Object) string {
		text := fmt.Sprintf("alt %s: %s\n", name, rec.ToS(result.Get("status")))
		if sidecar := rec.ToS(result.Get("sidecar")); sidecar != "" {
			text += "  sidecar: " + sidecar + "\n"
		}
		return text + outline(withoutList(withoutList(result, "status"), "sidecar"), "  ", 1)
	}
}

// tagPDFHuman is the tagged PDF, its verdict and its problems.
func tagPDFHuman(result ojson.Object) string {
	tagging, _ := result.Get("tagging").(ojson.Object)
	text := fmt.Sprintf("Tagged %s (%s, %s)\n", rec.ToS(tagging.Get("pdf")), rec.ToS(result.Get("standard")), rec.ToS(result.Get("status")))
	if language := rec.ToS(result.Get("language")); language != "" {
		text += "  language: " + language + "\n"
	}
	diagnostics, _ := result.Get("diagnostics").([]any)
	if len(diagnostics) == 0 {
		return text + "  no diagnostics\n"
	}
	return text + problemLines("diagnostics", diagnostics)
}
