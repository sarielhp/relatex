package pdfua

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// FigureFont is a font an included figure brought into the tagged PDF: the figure's file, the
// font's BaseFont with its subset tag, the program that made the figure (its /PTEX.InfoDict
// Creator) and whether its embedded program is a TrueType collection.
type FigureFont struct {
	Source     string
	Font       string
	Creator    string
	Collection bool
}

// fontSegment is the font of a veraPDF check's context: font[0](DCWGQU+CambriaMath).
var fontSegment = regexp.MustCompile(`font\[\d+\]\(([^)]*)\)`)

// FigureFontDiagnostics names the figures whose font makes veraPDF reject the tagged PDF: a font
// from a figure is the author's to re-export, so it gets its own diagnostic instead of the rule's
// name alone in pdfua.invalid.
func FigureFontDiagnostics(validation Validation, figures []FigureFont) []Diagnostic {
	if validation.Status != "failed" {
		return nil
	}
	byName := map[string][]FigureFont{}
	for _, figure := range figures {
		byName[figure.Font] = append(byName[figure.Font], figure)
	}
	type key struct{ source, font string }
	type group struct {
		figure FigureFont
		rules  []string
		checks int64
	}
	groups := map[key]*group{}
	order := []key{}
	for _, check := range validation.Checks {
		if check.Status == "passed" {
			continue
		}
		matches := fontSegment.FindAllStringSubmatch(check.Context, -1)
		if len(matches) == 0 {
			continue
		}
		name := matches[len(matches)-1][1]
		for _, figure := range byName[name] {
			id := key{figure.Source, figure.Font}
			found, ok := groups[id]
			if !ok {
				found = &group{figure: figure}
				groups[id] = found
				order = append(order, id)
			}
			rule := fmt.Sprintf("%s-%d: %s", check.Clause, check.Test, check.Message)
			if !slices.Contains(found.rules, rule) {
				found.rules = append(found.rules, rule)
			}
			found.checks++
		}
	}
	diagnostics := make([]Diagnostic, 0, len(order))
	for _, id := range order {
		found := groups[id]
		detail := strings.Join(found.rules, "; ")
		if found.figure.Collection {
			detail += "; its embedded program is a TrueType collection, which PDF does not allow"
		}
		message := fmt.Sprintf("figure %s: font %s breaks PDF/UA-2 (%s)", found.figure.Source, found.figure.Font, detail)
		if found.figure.Creator != "" {
			message += "; made by " + found.figure.Creator
		}
		message += " — re-export the figure with the font embedded as a single font"
		diagnostics = append(diagnostics, Diagnostic{Code: "pdfua.figure_font", Message: message, Magnitude: found.checks})
	}
	return diagnostics
}
