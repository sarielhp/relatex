package authormath

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/authortex"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

// preamble is MathVerification::PREAMBLE: standard packages, and math packages whose commands
// KaTeX and MathJax know too.
const preamble = "\\documentclass{article}\n\\usepackage{amsmath,amssymb,amsfonts,mathtools,bm,mathrsfs}\n"

// Root is the repository root: the collector (tex/) the verification run loads.
var Root string

type candidate struct {
	id     string
	nodes  []rec.Map
	source string
}

// verification is MathVerification: is the author's TeX self-contained? Every located
// formula is typeset again with a standard preamble and the kept macro block only, in one
// LuaLaTeX run through the same collector, and its MathML is compared with the paper's. A
// formula whose structure matches is verified; LuaTeX decides what the TeX means.
type verification struct {
	document  rec.Map
	nodes     []rec.Map
	work      string
	doctor    *texsandbox.Doctor
	cache     bool
	packages  bool
	directory string
	prefix    string
}

// Verify is MathVerification.new(document, work:, engine:).verify!; nodes are the document's
// math nodes in the order the document made them.
func Verify(document rec.Map, nodes []rec.Map, work, engine string) float64 {
	v := &verification{document: document, nodes: nodes, work: work, doctor: texsandbox.Current(engine), cache: true}
	candidates := v.candidates()
	if len(candidates) == 0 {
		return 0
	}
	started := time.Now()
	v.pass(candidates, false)
	// Whether a math font package changes how letters map depends on the class too: formulas
	// that differ are tried again with the paper's font packages loaded.
	if packages, _ := document["math_packages"].([]any); len(packages) > 0 {
		differing := []candidate{}
		for _, item := range candidates {
			for _, node := range item.nodes {
				if rec.AsMap(node["tex"])["status"] != "verified" {
					differing = append(differing, item)
					break
				}
			}
		}
		if len(differing) > 0 {
			v.pass(differing, true)
		}
	}
	return time.Since(started).Seconds()
}

func texOf(node rec.Map) rec.Map { return rec.AsMap(node["tex"]) }

func (v *verification) candidates() []candidate {
	singles := [][]rec.Map{}
	grouped := omap.New[any, []rec.Map]()
	for _, node := range v.nodes {
		tex, _ := node["tex"].(rec.Map)
		if node["type"] != "math" || tex["status"] != "unverified" {
			continue
		}
		if group := tex["group"]; truthy(group) {
			rows, _ := grouped.Get(group)
			grouped.Set(group, append(rows, node))
			continue
		}
		singles = append(singles, []rec.Map{node})
	}
	grouped.Each(func(_ any, rows []rec.Map) {
		sort.SliceStable(rows, func(i, j int) bool { return less(texOf(rows[i])["row"], texOf(rows[j])["row"]) })
		singles = append(singles, rows)
	})
	result := make([]candidate, len(singles))
	for index, nodes := range singles {
		result[index] = candidate{id: CandidateID(index), nodes: nodes, source: candidateSource(nodes)}
	}
	return result
}

func less(left, right any) bool {
	a, _ := rec.Number(left)
	b, _ := rec.Number(right)
	return a < b
}

// collectorDigest is MathVerification.collector_digest: the collector's files (tex/*), by
// name and content, as Ruby joins them.
func collectorDigest() string {
	paths, err := filepath.Glob(filepath.Join(Root, "tex", "*"))
	if err != nil {
		rec.Fail("%v", err)
	}
	sort.Strings(paths)
	lines := []string{}
	for _, path := range paths {
		if strings.HasPrefix(filepath.Base(path), ".") {
			continue
		}
		lines = append(lines, filepath.Base(path)+"    # A candidate typeset before"+command.FileDigest(path))
	}
	return digest(strings.Join(lines, "\n"))
}

func digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func (v *verification) cacheKey(item candidate) string {
	if v.prefix == "" {
		macros, _ := v.document["math_macros"].(rec.Map)
		v.prefix = digest(strings.Join([]string{v.preamble(), authortex.Block(macros) + PairedBlock(macros), collectorDigest()}, "\x00"))
	}
	return digest(v.prefix + "\x00" + item.source)
}

func cachePath(key string) string {
	return filepath.Join(command.CacheRoot(), "math-verification", key[:2], key+".json")
}

func readCache(key string) (any, bool) {
	file, err := os.Open(cachePath(key))
	if err != nil {
		return nil, false
	}
	defer file.Close()
	value, err := canonjson.Decode(file)
	if err != nil || value == nil {
		return nil, false
	}
	return value, true
}

func writeCache(key string, mathml []any) {
	path := cachePath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		rec.Fail("%v", err)
	}
	bytes, err := canonjson.Marshal(mathml)
	if err != nil {
		rec.Fail("%v", err)
	}
	if err := canonjson.WriteAtomic(path, bytes); err != nil {
		rec.Fail("%v", err)
	}
}

// cached: a candidate typeset before, with the same macros, preamble and collector, is not
// typeset again: its MathML is kept by content.
func (v *verification) cached(candidates []candidate) map[string]any {
	if !v.cache {
		return v.typesetAll(candidates)
	}
	keys := map[string]string{}
	found := map[string]any{}
	remaining := []candidate{}
	for _, item := range candidates {
		keys[item.id] = v.cacheKey(item)
		if value, ok := readCache(keys[item.id]); ok {
			found[item.id] = value
		} else {
			remaining = append(remaining, item)
		}
	}
	typeset := v.typesetAll(remaining)
	for id, mathml := range typeset {
		key, ok := keys[id]
		if !ok {
			panic(rec.Failure{Message: fmt.Sprintf("key not found: %q", id), Lookup: true})
		}
		writeCache(key, rec.AsList(mathml))
		found[id] = mathml
	}
	return found
}

func candidateSource(nodes []rec.Map) string {
	tex := texOf(nodes[0])
	environment, _ := tex["environment"].(string)
	if truthy(tex["group"]) {
		rows := make([]string, len(nodes))
		for index, node := range nodes {
			rows[index] = authortex.Shipped(rec.FetchString(texOf(node), "text"))
		}
		return "\\begin{" + environment + "}\n" + strings.Join(rows, "\\\\\n") + "\n\\end{" + environment + "}\\par\n"
	}
	text := authortex.Shipped(rec.FetchString(tex, "text"))
	// Each candidate ends a paragraph: TeX stops after 100 errors within one.
	if rec.Fetch(tex, "form") == "inline" {
		return "\\leavevmode\\setbox0\\hbox{$" + text + "$}\\par\n"
	}
	if environment != "" {
		return "\\begin{" + environment + "}\n" + text + "\n\\end{" + environment + "}\\par\n"
	}
	return "\\[\n" + text + "\n\\]\\par\n"
}

// preamble is the standard preamble, with the paper's math font packages on the second pass.
func (v *verification) preamble() string {
	text := preamble
	if v.packages {
		packages, _ := v.document["math_packages"].([]any)
		for _, name := range packages {
			text += "\\usepackage" + rec.AsString(name) + "\n"
		}
	}
	return text
}

func (v *verification) pass(candidates []candidate, packages bool) {
	v.packages, v.prefix = packages, ""
	v.directory = v.work
	if packages {
		v.directory = filepath.Join(v.work, "fonts")
	}
	mkdir(v.directory)
	for _, item := range candidates {
		writeFile(filepath.Join(v.directory, item.id+".tex"), item.source)
	}
	results := v.cached(candidates)
	for _, item := range candidates {
		typeset, _ := results[item.id].([]any)
		judge(item, typeset)
	}
}

func mkdir(path string) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		rec.Fail("%v", err)
	}
}

func writeFile(path, text string) {
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		rec.Fail("%v", err)
	}
}

func judge(item candidate, typeset []any) {
	// Nested formulas (\text{$x$}) come before their outer one: the last ones are the candidate's.
	if len(typeset) > len(item.nodes) {
		typeset = typeset[len(typeset)-len(item.nodes):]
	}
	for index, node := range item.nodes {
		tex := texOf(node)
		difference := "not typeset"
		if index < len(typeset) && truthy(typeset[index]) {
			difference = Difference(rec.FetchString(node, "mathml"), rec.AsString(typeset[index]))
		}
		updated := rec.Map{}
		for key, value := range tex {
			updated[key] = value
		}
		if difference != "" {
			updated["status"], updated["reason"], updated["difference"] = "unverified", "math.source_differs", difference
		} else {
			delete(updated, "reason")
			delete(updated, "difference")
			updated["status"] = "verified"
		}
		node["tex"] = updated
	}
}

// typesetAll typesets the candidates after the standard preamble and the kept macro block.
func (v *verification) typesetAll(candidates []candidate) map[string]any {
	if len(candidates) == 0 {
		return map[string]any{}
	}
	macros, _ := v.document["math_macros"].(rec.Map)
	ids := make([]string, len(candidates))
	for index, item := range candidates {
		ids[index] = item.id
	}
	typesetter := &Typesetter{Directory: v.directory, Header: v.preamble() + authortex.Block(macros) + PairedBlock(macros),
		Doctor: v.doctor, Attempt: "math-verification"}
	return typesetter.Typeset(ids)
}
