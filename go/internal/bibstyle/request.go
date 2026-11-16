package bibstyle

import (
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Processors of a bibliography.
const (
	Biber  = "biber"
	BibTeX = "bibtex"
)

var (
	bcfDatasource = regexp.MustCompile(`<bcf:datasource\b[^>]*\bdatatype="bibtex"[^>]*>([^<]*)</bcf:datasource>`)
	auxStyle      = regexp.MustCompile(`\\bibstyle\{([^}]*)\}`)
	auxData       = regexp.MustCompile(`\\bibdata\{([^}]*)\}`)
	loadsBiblatex = regexp.MustCompile(`\\(?:RequirePackage|usepackage)\s*(?:\[[^\]]*\])?\s*\{[^}]*\bbiblatex\b[^}]*\}`)
	texComment    = regexp.MustCompile(`(?m)(^|[^\\])%.*$`)
)

// Request is what a build asked of its bibliography processor: the processor, the style names it
// loaded (most specific first) and the databases it named, as the document wrote them.
type Request struct {
	Processor string
	Styles    []string
	Databases []string
}

// ReadRequest reads a finished build's request from its output directory (JOB.bcf, JOB.aux and the
// recorder's JOB.fls). Relative recorder paths resolve against the directories given, in order (the
// build's working directory, then the author's copy of it). No request is a zero Request.
func ReadRequest(output, job string, directories ...string) Request {
	if data, err := os.ReadFile(filepath.Join(output, job+".bcf")); err == nil {
		return Request{Processor: Biber, Styles: biblatexStyles(filepath.Join(output, job+".fls"), directories),
			Databases: bcfDatabases(string(data))}
	}
	aux, err := os.ReadFile(filepath.Join(output, job+".aux"))
	if err != nil {
		return Request{}
	}
	style := auxStyle.FindSubmatch(aux)
	databases := []string{}
	for _, match := range auxData.FindAllSubmatch(aux, -1) {
		databases = append(databases, splitNames(string(match[1]))...)
	}
	if style == nil || len(databases) == 0 {
		return Request{}
	}
	return Request{Processor: BibTeX, Styles: []string{strings.TrimSpace(string(style[1]))}, Databases: databases}
}

func splitNames(list string) []string {
	names := []string{}
	for _, name := range strings.Split(list, ",") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// bcfDatasource: the .bib data sources of a biblatex control file, in order.
func bcfDatabases(bcf string) []string {
	names := []string{}
	for _, match := range bcfDatasource.FindAllStringSubmatch(bcf, -1) {
		if name := strings.TrimSpace(html.UnescapeString(match[1])); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// biblatexStyles are the style names a biblatex build loaded, in the recorder's order: each package or
// class that loads biblatex (sariel_biblatex.sty, biblatex-chicago.sty), then the first bibliography
// style file (alphabetic.bbx; it loads standard.bbx after itself). relatex's own packages
// (relatex-external.sty, which the build loads around the paper) are not the paper's style.
func biblatexStyles(recorder string, directories []string) []string {
	data, err := os.ReadFile(recorder)
	if err != nil {
		return nil
	}
	names := []string{}
	seen := map[string]bool{}
	bbx := ""
	for _, line := range strings.Split(string(data), "\n") {
		file, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "INPUT ")
		if !ok {
			continue
		}
		extension := filepath.Ext(file)
		name := strings.TrimSuffix(filepath.Base(file), extension)
		switch {
		case extension == ".bbx" && bbx == "":
			bbx = name
		case (extension == ".sty" || extension == ".cls") && name != "biblatex" && !strings.HasPrefix(name, "relatex") && !seen[name]:
			seen[name] = true
			if loadsBiblatexFile(file, directories) {
				names = append(names, name)
			}
		}
	}
	if bbx != "" {
		names = append(names, bbx)
	}
	return names
}

// loadsBiblatexFile: the package or class requires biblatex (outside comments).
func loadsBiblatexFile(file string, directories []string) bool {
	candidates := []string{file}
	if !filepath.IsAbs(file) {
		candidates = candidates[:0]
		for _, directory := range directories {
			candidates = append(candidates, filepath.Join(directory, file))
		}
	}
	for _, candidate := range candidates {
		if data, err := os.ReadFile(candidate); err == nil {
			return loadsBiblatex.MatchString(texComment.ReplaceAllString(string(data), "$1"))
		}
	}
	return false
}
