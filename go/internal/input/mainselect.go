package input

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// When the plain rules leave several documents, three narrowing rules follow, each sound for
// real sources and each recorded in the manifest's selection. They apply in this order; a rule
// that leaves one file ends the search, and if none does the choice stays the user's.
//
//  1. arXiv's own choice: the files its 00README.json (or 00README.XXX) marks as top level.
//  2. A file another candidate inputs or includes is a part of that one, not a main file.
//  3. A candidate in the paper's root directory over those in subdirectories (copies, archives).

var (
	inputCommand = regexp.MustCompile(`\\(?:input|include)\s*(?:\{([^}\n]+)\}|([^\s{}\\%]+))`)
	readmeLine   = regexp.MustCompile(`^\s*([^\s:]+)\s*:?\s+toplevelfile\b`)
)

// mainRule is one narrowing rule's verdict: the pool that remains (empty: the rule says nothing)
// and the words recording it.
type mainRule struct {
	pool   []string
	reason string
}

// narrowMain applies the rules to the candidates. It returns the single main file with the
// recording words, or "" and the narrowed pool when the choice stays ambiguous.
func (a *Archive) narrowMain(files, candidates, ignored []string) (string, string, []string) {
	pool := slices.Clone(candidates)
	rules := []func([]string) mainRule{
		func(pool []string) mainRule { return a.readmeRule(files, pool, ignored) },
		a.includedRule,
		rootRule,
	}
	for _, rule := range rules {
		verdict := rule(pool)
		if len(verdict.pool) == 0 {
			continue
		}
		pool = verdict.pool
		if len(pool) == 1 {
			return pool[0], verdict.reason, pool
		}
	}
	return "", "", pool
}

// readmeRule keeps the candidates arXiv's README marks as top level.
func (a *Archive) readmeRule(files, pool, ignored []string) mainRule {
	marked := []string{}
	for _, name := range a.readmeTopLevel(files) {
		if slices.Contains(pool, name) && !slices.Contains(ignored, lower.String(filepath.Base(name))) {
			marked = append(marked, name)
		}
	}
	return mainRule{pool: marked, reason: "arXiv's 00README marks it as the top-level file"}
}

// readmeTopLevel is the file names the README in the archive's root marks as top level.
func (a *Archive) readmeTopLevel(files []string) []string {
	if slices.Contains(files, "00README.json") {
		return jsonTopLevel(a.read("00README.json"))
	}
	if slices.Contains(files, "00README.XXX") {
		return xxxTopLevel(a.read("00README.XXX"))
	}
	return nil
}

func jsonTopLevel(text string) []string {
	var readme struct {
		Sources []struct {
			Usage    string `json:"usage"`
			Filename string `json:"filename"`
		} `json:"sources"`
	}
	if json.Unmarshal([]byte(text), &readme) != nil {
		return nil
	}
	names := []string{}
	for _, source := range readme.Sources {
		if source.Usage == "toplevel" {
			names = append(names, path.Clean(source.Filename))
		}
	}
	return names
}

func xxxTopLevel(text string) []string {
	names := []string{}
	for _, line := range strings.Split(text, "\n") {
		if match := readmeLine.FindStringSubmatch(line); match != nil {
			names = append(names, path.Clean(match[1]))
		}
	}
	return names
}

// includedRule drops the candidates another candidate inputs or includes. When every candidate
// is dropped (a cycle) the rule says nothing.
func (a *Archive) includedRule(pool []string) mainRule {
	included := map[string]bool{}
	for _, name := range pool {
		for _, target := range inputTargets(a.read(name)) {
			if other := resolveInput(target, name, pool); other != "" && other != name {
				included[other] = true
			}
		}
	}
	rest := []string{}
	for _, name := range pool {
		if !included[name] {
			rest = append(rest, name)
		}
	}
	return mainRule{pool: rest, reason: "the other candidates are inputs of it"}
}

// rootRule keeps the candidates in the paper's root directory when some are deeper.
func rootRule(pool []string) mainRule {
	root := []string{}
	for _, name := range pool {
		if !strings.Contains(name, "/") {
			root = append(root, name)
		}
	}
	if len(root) == len(pool) {
		return mainRule{}
	}
	return mainRule{pool: root, reason: "only candidate in the root directory"}
}

// inputTargets is the file names the \input and \include commands of text name, comments removed.
func inputTargets(text string) []string {
	targets := []string{}
	for _, line := range strings.Split(text, "\n") {
		for _, match := range inputCommand.FindAllStringSubmatch(stripComment(line), -1) {
			targets = append(targets, strings.TrimSpace(match[1]+match[2]))
		}
	}
	return targets
}

// stripComment cuts a line at its first unescaped percent sign.
func stripComment(line string) string {
	for index := 0; index < len(line); index++ {
		switch line[index] {
		case '\\':
			index++
		case '%':
			return line[:index]
		}
	}
	return line
}

// resolveInput is the pool member an input target names: relative to the root (where TeX runs)
// or to the including file's directory, with or without the .tex suffix.
func resolveInput(target, from string, pool []string) string {
	for _, base := range []string{"", path.Dir(from)} {
		for _, suffix := range []string{"", ".tex"} {
			name := path.Clean(path.Join(base, target+suffix))
			if slices.Contains(pool, name) {
				return name
			}
		}
	}
	return ""
}

func (a *Archive) read(name string) string {
	text, err := os.ReadFile(filepath.Join(a.Directory, "files", name))
	check(err)
	return string(text)
}
