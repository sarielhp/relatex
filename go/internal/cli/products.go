package cli

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/artifacts"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/evidence"
	"github.com/sarielhp/relatex/internal/exportwriter"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/pipeline"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
)

// streamLimit is Stream::LIMIT.
const streamLimit = 256 * 1024 * 1024

// verificationProducts is VerificationProducts: a read-only view of a completed attempt's
// stage products, every file bound to the hash its receipt records.
type verificationProducts struct {
	attempt, output, probe string
	configuration          rec.Map
	stages                 map[string]rec.Map
	resolvers              map[string]*artifacts.Resolver
	published              *artifacts.Resolver
	bindings               map[string]string
	bindingOrder           []string
	payload                rec.Map
}

func newVerificationProducts(attempt, products string) *verificationProducts {
	v := &verificationProducts{attempt: realpath(attempt), stages: map[string]rec.Map{}, resolvers: map[string]*artifacts.Resolver{},
		bindings: map[string]string{}}
	pipeline.ExpandAtRest(v.attempt)
	run := readJSON(filepath.Join(v.attempt, "run.json"))
	v.configuration = rec.FetchMap(run, "configuration")
	formats, ok := v.configuration["formats"].([]any)
	valid := ok && len(formats) > 0
	for _, format := range formats {
		valid = valid && slices.Contains(exportwriter.Formats, rec.ToS(format))
	}
	if !valid {
		rec.Fail("Completed conversion has invalid output formats")
	}
	if products == "" {
		products = rec.FetchString(run, "product")
	}
	v.output = realpath(products)
	v.loadStages()
	if !rec.Equal(v.configuration, rec.Fetch(v.stages["export"], "configuration")) {
		rec.Fail("Run configuration differs from bound export stage")
	}
	v.bindPublication()
	v.payload = readBundle(v.published.Resolve("document.json.gz"))
	if !rec.Equal(rec.Fetch(v.payload, "document"), readJSON(v.stage("reconcile", "document.json"))) {
		rec.Fail("Published model differs from reconciliation stage")
	}
	engine := readJSON(v.stage("engine", "engine-products.json"))
	probe := artifacts.Relative(rec.FetchString(engine, "probe"))
	v.probe = realpath(filepath.Join(v.stageRoot("engine"), probe))
	if !fspath.Inside(v.stageRoot("engine"), v.probe) {
		rec.Fail("Engine probe escapes its stage")
	}
	v.verify()
	return v
}

func readJSON(path string) rec.Map {
	value, err := canonjson.Decode(strings.NewReader(string(readFile(path))))
	if err != nil {
		rec.Fail("%s", jsonMessage(err))
	}
	return rec.AsMap(value)
}

func (v *verificationProducts) stageRoot(name string) string {
	return filepath.Join(v.attempt, "layers", fmt.Sprintf("%02d-%s", slices.Index(layerOrder, name)+1, name))
}

func (v *verificationProducts) stage(name, relative string) string {
	resolver, ok := v.resolvers[name]
	if !ok {
		rec.Fail("Missing required %s stage", name)
	}
	return resolver.Resolve(relative)
}

func (v *verificationProducts) stageFile(name, relative string) bool {
	artifacts.Relative(relative)
	outputs := rec.FetchMap(v.stages[name], "outputs")
	_, ok := outputs[relative]
	return ok
}

func (v *verificationProducts) bind(path, sha string) {
	if _, seen := v.bindings[path]; !seen {
		v.bindingOrder = append(v.bindingOrder, path)
	}
	v.bindings[path] = sha
}

// hashes is a receipt's outputs as names and their SHA-256.
func hashes(outputs rec.Map) map[string]string {
	result := map[string]string{}
	for name, value := range outputs {
		if entry, ok := value.(rec.Map); ok {
			value = entry["sha256"]
		}
		result[name] = rec.ToS(value)
	}
	return result
}

func (v *verificationProducts) loadStages() {
	for _, name := range layerOrder {
		path := filepath.Join(v.stageRoot(name), "stage.json")
		if !regular(path) {
			continue
		}
		receipt := readJSON(path)
		if receipt["layer"] != name || receipt["status"] != "complete" {
			rec.Fail("Incomplete or mislabeled %s stage", name)
		}
		v.stages[name] = receipt
		v.bind(path, command.FileDigest(path))
		outputs := hashes(rec.FetchMap(receipt, "outputs"))
		resolver := artifacts.New(v.stageRoot(name), outputs, nil)
		for relative, sha := range outputs {
			v.bind(resolver.Resolve(relative), sha)
		}
		v.resolvers[name] = resolver
	}
	missing := []string{}
	for _, name := range []string{"input", "engine", "reconcile", "export", "publish"} {
		if _, ok := v.stages[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		rec.Fail("Required conversion stages are missing: %s", strings.Join(missing, ", "))
	}
}

func (v *verificationProducts) bindPublication() {
	manifest := filepath.Join(v.output, "manifest.json")
	files := hashes(readJSON(manifest))
	v.published = artifacts.New(v.output, files, nil)
	for name, sha := range files {
		v.bind(v.published.Resolve(name), sha)
	}
	v.bind(manifest, command.FileDigest(manifest))
	for name, sha := range hashes(rec.FetchMap(v.stages["export"], "outputs")) {
		if files[name] != sha {
			rec.Fail("Published export artifact differs: %s", name)
		}
	}
	entry := "html/index.html"
	if _, split := files["split/index.html"]; split {
		// A split book opens on its contents page, and its chapter files are the split's (decision 081).
		entry = "split/index.html"
		for name, sha := range files {
			if page, ok := strings.CutPrefix(name, "split/"); ok && strings.HasSuffix(page, ".html") && files[page] != sha {
				rec.Fail("Published chapter file differs from the split: %s", page)
			}
		}
	}
	if index, ok := files["index.html"]; ok && index != files[entry] {
		rec.Fail("Published entry page differs from HTML output")
	}
	result, _ := v.stages["publish"]["result"].(rec.Map)
	if recorded, ok := result["manifest_sha256"].(string); ok && recorded != v.bindings[manifest] {
		rec.Fail("Published manifest differs from publish receipt")
	}
}

func (v *verificationProducts) verify() {
	for _, path := range v.bindingOrder {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || command.FileDigest(path) != v.bindings[path] {
			rec.Fail("Acceptance artifact changed: %s", path)
		}
	}
}

func (v *verificationProducts) engineArtifact(relative string) string {
	return v.stage("engine", path.Join(filepath.Base(v.probe), relative))
}

// mathEvidence is ProductInspection#math_evidence: the node's formula in the native record
// stream it was reconciled from.
func (v *verificationProducts) mathEvidence(node rec.Map) (result ojson.Object) {
	defer func() {
		if message, ok := failureMessage(recover()); ok {
			result = ojson.Object{{"status", "unavailable"}, {"reason", message}}
		}
	}()
	probe := readJSON(v.engineArtifact("probe.json"))
	pass := rec.Fetch(rec.FetchMap(rec.FetchMap(probe, "results"), "external"), "passes")
	streamPath := v.engineArtifact(fmt.Sprintf("external/output/pass-%s.jsonl", rec.ToS(pass)))
	stage := readJSON(v.engineArtifact(fmt.Sprintf("external/output/pass-%s-stage.json", rec.ToS(pass))))
	loaded, err := stream.Load(streamPath, Root, stage)
	if err != nil {
		rec.Fail("%v", err)
	}
	retention := evidence.MathReport(loaded.Records)
	key := rec.Fetch(rec.FetchMap(node, "provenance"), "inspection_key")
	var occurrence rec.Map
	for _, value := range rec.FetchList(retention, "occurrences") {
		if candidate := rec.AsMap(value); rec.Equal(candidate["id"], key) {
			occurrence = candidate
			break
		}
	}
	if occurrence == nil {
		rec.Fail("Node occurrence has no matching retained evidence")
	}
	formula := rec.Fetch(occurrence, "formula")
	definitions := orderedLines(streamPath, func(row ojson.Object) bool {
		return row.Get("kind") == "math" && rec.Equal(row.Get("formula"), formula)
	})
	matched := false
	for _, row := range definitions {
		matched = matched || row.(ojson.Object).Get("mathml") == node["mathml"]
	}
	if !matched {
		rec.Fail("Node MathML differs from native definition")
	}
	return ojson.Object{{"stream", streamPath}, {"stream_sha256", command.FileDigest(streamPath)},
		{"retention_status", rec.Fetch(retention, "status")},
		{"occurrence", exportwriter.Ordered(occurrence, "id", "formula", "fragments", "pages")},
		{"definitions", withoutMathML(definitions)}, {"authored_cases", v.caseBindings(rec.ToS(pass), formula)}}
}

// orderedLines are a JSON-lines file's rows the test selects, keys in the file's order.
func orderedLines(path string, test func(ojson.Object) bool) []any {
	file, err := os.Open(path)
	if err != nil {
		panic(systemCall(err))
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	rows := []any{}
	for scanner.Scan() {
		value, err := ojson.Decode(strings.NewReader(scanner.Text()))
		if err != nil {
			rec.Fail("%s", jsonMessage(err))
		}
		if row, ok := value.(ojson.Object); ok && test(row) {
			rows = append(rows, row)
		}
	}
	if err := scanner.Err(); err != nil {
		panic(systemCall(err))
	}
	return rows
}

func withoutMathML(rows []any) []any {
	result := make([]any, len(rows))
	for index, row := range rows {
		kept := ojson.Object{}
		for _, pair := range row.(ojson.Object) {
			if pair.Key() != "mathml" {
				kept = append(kept, pair)
			}
		}
		result[index] = kept
	}
	return result
}

func (v *verificationProducts) caseBindings(pass string, formula any) []any {
	relative := path.Join(filepath.Base(v.probe), fmt.Sprintf("external/output/pass-%s.jsonl.feature-cases.jsonl", pass))
	if !v.stageFile("engine", relative) {
		return []any{}
	}
	path := v.stage("engine", relative)
	if info, err := os.Stat(path); err == nil && info.Size() > streamLimit {
		rec.Fail("Authored case sidecar exceeds stream limit")
	}
	return withoutMathML(orderedLines(path, func(row ojson.Object) bool {
		return row.Get("kind") == "math" && rec.Equal(row.Get("formula"), formula)
	}))
}
