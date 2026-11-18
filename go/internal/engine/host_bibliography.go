package engine

import (
	"os"

	"github.com/sarielhp/relatex/internal/command"
)

// WithBibliography adds the caller's BIBINPUTS and BSTINPUTS to the host (decision 103 amended: BibTeX
// finds the author's shortcuts.bib or style where the author's own build does), read like TEXINPUTS
// from directory. A host is made when only these are set; nil comes back when neither is.
func WithBibliography(host *Host, bibinputs, bstinputs, directory string) *Host {
	if bibinputs == "" && bstinputs == "" {
		return host
	}
	if host == nil {
		host = &Host{}
	}
	home, _ := os.UserHomeDir()
	host.BibPath, host.BstPath = absoluteEntries(bibinputs, directory, home), absoluteEntries(bstinputs, directory, home)
	return host
}

func absoluteEntries(list, directory, home string) []string {
	entries := []string{}
	for _, entry := range command.SplitSearchList(list) {
		entries = append(entries, absoluteEntry(entry, directory, home))
	}
	return entries
}

// bibliographyPath is a BibTeX search path: the build's own directories, then the author's entries
// from the host's list, then kpathsea's default path.
func bibliographyPath(author []string, own ...string) string {
	seen := map[string]bool{}
	entries := []string{}
	for _, entry := range append(append([]string{}, own...), author...) {
		if entry != "" && entry != "." && !seen[entry] {
			seen[entry] = true
			entries = append(entries, entry)
		}
	}
	return command.SearchList(append(entries, "")...)
}

// bibliographyPaths are the host's BIBINPUTS and BSTINPUTS entries (none for a nil host).
func (h *Host) bibliographyPaths() (bib, bst []string) {
	if h == nil {
		return nil, nil
	}
	return h.BibPath, h.BstPath
}
