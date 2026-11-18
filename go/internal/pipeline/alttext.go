package pipeline

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/rec"
)

// altTextPath is the paper's figure descriptions (decision 086): relatex/alt.toml in the paper
// directory. The file is left out of a directory's snapshot, so editing it reruns only the export,
// not LuaLaTeX. A ZIP carries it as relatex/ beside its main file (decision 124; a store item's
// author files, decision 136); before the input is captured there is none to exclude.
func (c *conversion) altTextPath() string {
	if directory := c.inputDirectory(); directory != "" {
		return filepath.Join(directory, figdesc.FileName)
	}
	if c.archive == nil {
		return ""
	}
	main := filepath.FromSlash(rec.AsString(c.archive.Manifest["main"]))
	return filepath.Join(c.archive.Directory, "files", filepath.Dir(main), figdesc.FileName)
}

// applyAltText carries the sidecar's figure descriptions into the document before the formats
// are written, with diagnostics for drafts nobody has reviewed, entries written for an earlier
// figure, entries that name no figure, and a file that cannot be read (its descriptions are then
// left out; the conversion goes on).
func (c *conversion) applyAltText() {
	path := c.altTextPath()
	if path == "" {
		return
	}
	sidecar, err := figdesc.Read(path)
	if err != nil {
		c.diagnostics.Add("alt.invalid", err.Error()+"; no figure description from it is used", nil)
		return
	}
	if !sidecar.Exists() {
		return
	}
	before := digest(c.document)
	summary := figdesc.Apply(c.document, sidecar)
	// The source map is bound to the document it was made with; the descriptions change no word or
	// node of it, so a binding to the document before them moves to the described one.
	if c.sourceMap != nil && summary.Applied > 0 && c.sourceMap["document_sha256"] == before {
		c.sourceMap["document_sha256"] = digest(c.document)
	}
	if count := len(summary.Drafts); count > 0 {
		c.diagnostics.Add("alt.unreviewed", fmt.Sprintf("%d figure descriptions are model drafts nobody has reviewed (%s); "+
			"review them with relatex alt review and accept them with relatex alt accept", count, listed(summary.Drafts)), int64(count))
	}
	if count := len(summary.Stale); count > 0 {
		c.diagnostics.Add("alt.stale", fmt.Sprintf("%d figure descriptions were written for an earlier version of their figure (%s)",
			count, listed(summary.Stale)), int64(count))
	}
	if count := len(summary.Unmatched); count > 0 {
		c.diagnostics.Add("alt.unmatched", fmt.Sprintf("%s has entries for figures the paper does not have (%s)",
			figdesc.FileName, listed(summary.Unmatched)), int64(count))
	}
}

// listed is up to five names, then how many more.
func listed(names []string) string {
	if len(names) <= 5 {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:5], ", ") + fmt.Sprintf(" and %d more", len(names)-5)
}

// figureNameDiagnostics warns about each pair of figures whose source names differ only in case:
// the export keeps both (identical bytes share one name, a different file is numbered) and the
// paper keeps its names (decision 129, amendment 5).
func (c *conversion) figureNameDiagnostics() {
	for _, twin := range figures.CaseTwins(c.document) {
		outcome := "they hold different bytes, so the second is numbered"
		if twin.Identical {
			outcome = "they hold the same bytes, so they share one exported name"
		}
		c.diagnostics.Add("figures.case_twins", fmt.Sprintf("the figures %s and %s differ only in case, which Windows and macOS "+
			"cannot tell apart; %s", rec.InspectString(twin.First), rec.InspectString(twin.Second), outcome), nil)
	}
}
