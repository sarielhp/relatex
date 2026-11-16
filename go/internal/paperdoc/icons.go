package paperdoc

import (
	"path"
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/rec"
)

// Known icons (decision 111, amendment 2026-11-14): a small included file whose name is a standard icon
// says what the icon says, so its text alternative is that word, with provenance inferred like a chess
// diagram's position (decision 163). A class's ORCID mark beside an author's name is the common case
// (aastex, lipics, beamer templates), and Elsevier's cas templates print an envelope and a globe before an
// author's e-mail and web address. An image the source or the author describes keeps its description.
const iconSP = 16 * 65_536

var knownIcons = []struct {
	name *regexp.Regexp
	alt  string
}{
	{regexp.MustCompile(`(?i)\Aorcid([-_]?id)?(_icon|[-_]logo)?\z`), "ORCID iD"},
	{regexp.MustCompile(`\Acas-email\z`), "Email"},
	{regexp.MustCompile(`\Acas-url\z`), "URL"},
}

// InferIconAlt names the document's undescribed small icons by their file names; it returns how many it named.
func InferIconAlt(nodes rec.Map) int {
	named := 0
	for _, value := range nodes {
		content, _ := rec.AsMap(value)["content"].([]any)
		for _, image := range inline.OfType(content, "image") {
			if !undescribed(image) || !within(image, iconSP) {
				continue
			}
			if alt := iconAlt(image); alt != "" {
				image["alt"], image["alt_provenance"] = alt, Inferred
				named++
			}
		}
	}
	return named
}

// iconAlt is the word a known icon's file name stands for, or "" for any other image (a drawn one has
// no file name of its own).
func iconAlt(image rec.Map) string {
	source, _ := image["source"].(string)
	if source == "" || drawn(image) {
		return ""
	}
	base := strings.TrimSuffix(path.Base(source), path.Ext(source))
	for _, icon := range knownIcons {
		if icon.name.MatchString(base) {
			return icon.alt
		}
	}
	return ""
}
