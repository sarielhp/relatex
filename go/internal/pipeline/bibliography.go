package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/bibstyle"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/typstsource"
)

// The Typst bibliography's inputs (decision 172): what the build asked BibTeX or biber for, the style
// found along BSTINPUTS, and the .bib files, for the typst-source writer; its choice becomes a
// diagnostic.

// Diagnostic codes of the Typst bibliography (profiles/diagnostics.json).
const (
	bibliographyLiteral  = "typst.bibliography_literal"
	bibliographyUnstyled = "typst.bibliography_unstyled"
)

// typstBibliography is the writer's bibliography inputs, read once from the engine's build; nil when
// the export writes no typst-source.
func (c *conversion) typstBibliography() *typstsource.Bibliography {
	if !slices.Contains(c.options.Formats, "typst-source") || c.pdf == "" || c.probe == "" {
		return nil
	}
	if c.bibliography != nil {
		return c.bibliography
	}
	output := filepath.Dir(c.pdf)
	job := strings.TrimSuffix(filepath.Base(c.pdf), ".pdf")
	main := filepath.Dir(filepath.FromSlash(rec.AsString(c.archive.Manifest["main"])))
	sources := filepath.Join(c.probe, "input", "files")
	document := filepath.Join(sources, main)
	request := bibstyle.ReadRequest(output, job, filepath.Join(c.probe, "external", "working", main), document)
	kpsewhich := kpsewhichFor(c.options.Engine)
	search := bibstyle.Search{Kpsewhich: kpsewhich, BstInputs: c.options.BstInputs, Directory: document, Root: c.options.Root}
	bibliography := &typstsource.Bibliography{Processor: request.Processor, Names: request.Styles, Typst: command.Resolve("typst"),
		Root: c.options.Root, Sources: sources}
	if request.Processor != "" {
		style, path, found := search.Find(request.Styles)
		bibliography.Path = homeWritten(path)
		if found {
			bibliography.Style = &style
		}
		bibliography.Databases, bibliography.Missing = resolveDatabases(request.Databases, kpsewhich, document, sources)
	}
	c.bibliography = bibliography
	return bibliography
}

// resolveDatabases finds each named .bib as the build did: in the document's directory, the sources'
// root, then the TeX tree (kpsewhich without the caller's BIBINPUTS).
func resolveDatabases(names []string, kpsewhich, document, sources string) (found, missing []string) {
	for _, name := range names {
		file := name
		if filepath.Ext(file) != ".bib" {
			file += ".bib"
		}
		if path := findDatabase(file, kpsewhich, document, sources); path != "" {
			if !slices.Contains(found, path) {
				found = append(found, path)
			}
			continue
		}
		missing = append(missing, name)
	}
	return found, missing
}

func findDatabase(file, kpsewhich, document, sources string) string {
	if filepath.IsAbs(file) {
		return regularFile(file)
	}
	for _, directory := range []string{document, sources} {
		if path := regularFile(filepath.Join(directory, file)); path != "" {
			return path
		}
	}
	if kpsewhich == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, kpsewhich, file)
	cmd.Dir = document
	cmd.Env = command.Environment(nil)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return regularFile(strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]))
}

func regularFile(path string) string {
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return path
}

// homeWritten writes the home directory "~", as the configuration's TeX search path does.
func homeWritten(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	return strings.ReplaceAll(path, home+"/", "~/")
}

// bibliographyInputs identify the typst-source export's bibliography inputs for its stage cache: a
// changed style along BSTINPUTS or a changed .bib writes the export again.
func (c *conversion) bibliographyInputs() any {
	b := c.typstBibliography()
	if b == nil || b.Processor == "" {
		return nil
	}
	databases := []any{}
	for _, path := range b.Databases {
		databases = append(databases, command.FileDigest(path))
	}
	var style any
	if b.Style != nil {
		style = ojson.Object{{"name", b.Style.Name}, {"origin", b.Style.Origin}, {"sha256", b.Style.SHA256}}
	}
	return ojson.Object{{"processor", b.Processor}, {"names", stringList(b.Names)}, {"style", style},
		{"databases", databases}, {"missing", stringList(b.Missing)}, {"typst", c.helper("typst")}}
}

// bibliographyDiagnostic: a style that did not match is a warning naming the style file and the entries
// that differ (decision C); no style for a .bib bibliography is information.
func (c *conversion) bibliographyDiagnostic(results ojson.Object) {
	if c.diagnostics == nil || !results.Has("typst-source") {
		return
	}
	record, _ := ojson.Plain(asObject(results.Get("typst-source")).Get("bibliography")).(rec.Map)
	if record == nil || record["status"] != "literal" || rec.AsString(record["processor"]) == "" {
		return
	}
	style, _ := record["style"].(rec.Map)
	if style == nil {
		c.diagnostics.Add(bibliographyUnstyled, "Typst bibliography written as literal entries: "+rec.AsString(record["reason"]), nil)
		return
	}
	message := fmt.Sprintf("Typst style %s (%s) not used, literal entries written instead: %s",
		rec.AsString(style["name"])+".typ", rec.AsString(style["path"]), rec.AsString(record["reason"]))
	if differing := differingKeys(record["differing"]); differing != "" {
		message += "; differing entries: " + differing
	}
	c.diagnostics.Add(bibliographyLiteral, message, nil)
}

func differingKeys(value any) string {
	keys := []string{}
	items, _ := value.([]any)
	for _, item := range items {
		entry := rec.AsMap(item)
		keys = append(keys, rec.AsString(entry["key"])+" ("+rec.AsString(entry["field"])+")")
	}
	return strings.Join(keys, ", ")
}
