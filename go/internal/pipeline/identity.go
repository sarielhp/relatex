// Package pipeline is the Go port of Relatex::GeneralConversion: one conversion as layers
// (LayerRun), each with its receipt and, where it is worth it, its entry in the stage cache
// (ArtifactCache#stage), the stages run in-process through the Go stage packages.
package pipeline

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/resources"
)

// identityVersion is StageIdentity::VERSION.
const identityVersion = "stage-identity-1"

// Progress hears each stage as it begins, when set: its name, its place (from 1) and the number of stages.
var Progress func(stage string, index, total int)

// order is LayerRun::ORDER.
var order = []string{"input", "engine", "reconcile", "layout", "synchronize", "export", "publish"}

// dependents is StageIdentity::DEPENDENTS: forcing a stage forces the stages that read it.
var dependents = map[string][]string{
	"input":       {"engine", "reconcile", "layout", "synchronize", "export", "publish"},
	"engine":      {"reconcile", "layout", "synchronize", "export", "publish"},
	"reconcile":   {"synchronize", "export", "publish"},
	"layout":      {"synchronize", "export", "publish"},
	"synchronize": {"export", "publish"},
	"export":      {"publish"},
	"publish":     {},
}

// projectionFormats are StageIdentity::PROJECTIONS' keys: the export formats with a cache.
var projectionFormats = []string{"html", "markdown", "typst", "typst-source", "json", "json.gz", "pdf", "epub", "pdf-ua"}

// binaryKey names the program itself in an implementation's files; sourcesKey names, in its place, the
// digest of the Go source a stage can reach (see stageSources).
const (
	binaryKey   = "relatex (Go binary)"
	sourcesKey  = "relatex (Go stage sources)"
	sourcesFile = "stage-sources.json"
)

var (
	sourcesOnce    sync.Once
	sourcesDigests map[string]string
)

// stageSources are the digests tools/stage_go_resources computed and embedded in this binary: for each stage,
// the Go source of the packages the stage reaches (its package closure and the pipeline's glue), so a change
// to the code of a later stage does not invalidate an earlier one. The table describes the build, so it is
// read from the binary, not from the unpacked resources. A binary without it (a build that did not stage
// the resources) has none, and every stage then depends on the whole binary.
func stageSources() map[string]string {
	sourcesOnce.Do(func() {
		sourcesDigests = map[string]string{}
		if data := resources.File(sourcesFile); data != nil {
			_ = json.Unmarshal(data, &sourcesDigests)
		}
	})
	return sourcesDigests
}

var (
	binaryOnce   sync.Once
	binaryDigest string
)

// programDigest is the running binary's SHA-256: the Go stages' code.
func programDigest() string {
	binaryOnce.Do(func() {
		path, err := os.Executable()
		if err != nil {
			rec.Fail("%v", err)
		}
		binaryDigest = command.FileDigest(path)
	})
	return binaryDigest
}

// dataFiles are the files under root/relative (recursively) a stage reads at run time.
func dataFiles(root, relative string, keep func(string) bool) []string {
	result := []string{}
	base := filepath.Join(root, relative)
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name, _ := filepath.Rel(root, path)
		name = fspath.Slash(name)
		if strings.HasPrefix(entry.Name(), ".") && path != base {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() && keep(name) {
			result = append(result, name)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		rec.Fail("%v", err)
	}
	return result
}

func everything(string) bool { return true }

// resource is a lib/relatex file the Go writers read (scripts, Typst layouts, fonts), not Ruby.
func resource(name string) bool { return !strings.HasSuffix(name, ".rb") }

// implementationFiles are a stage's fingerprinted files besides the binary: the data the Go
// stages read, chosen as StageIdentity chooses Ruby's (the collector for the engine, the
// schemas where documents are validated, the diagnostics profile, the writers' resources).
func implementationFiles(root, name string) []string {
	files := []string{"VERSION"}
	if name == "engine" {
		files = append(files, dataFiles(root, "tex", everything)...)
	}
	if slices.Contains([]string{"input", "reconcile", "export"}, name) {
		files = append(files, dataFiles(root, "schemas", everything)...)
	}
	if name == "reconcile" || name == "export" {
		files = append(files, dataFiles(root, "profiles", everything)...)
	}
	if name == "export" {
		files = append(files, dataFiles(root, "lib/relatex", resource)...)
	}
	return files
}

// fingerprint is the implementation record of a set of files and the binary.
func fingerprint(root, stage string, files []string) ojson.Object {
	hashes := rec.Map{}
	if digest := stageSources()[stage]; digest != "" {
		hashes[sourcesKey] = digest
	} else {
		hashes[binaryKey] = programDigest()
	}
	for _, file := range files {
		path := filepath.Join(root, file)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			hashes[file] = command.FileDigest(path)
		}
	}
	return ojson.Object{{"schema_version", identityVersion}, {"files", sortedObject(hashes)}, {"sha256", digest(hashes)}}
}

// implementation is StageIdentity.implementation for the Go stages: the binary's SHA-256 and
// the data files the stage reads (docs/stage-contracts.md).
func implementation(root, name string) ojson.Object {
	return fingerprint(root, name, implementationFiles(root, name))
}

// formatResources are the lib/relatex files each export format's writer reads.
var formatResources = map[string][]string{
	"html":         {"fonts/math-accents", "preview.js"},
	"epub":         {"fonts/math-accents"},
	"markdown":     {"fonts/math-accents"},
	"typst":        {"math_layout.typ", "table_layout.typ"},
	"typst-source": {"typst_source_preamble.typ"},
	"pdf-ua":       {"pdfua"},
}

// projection is StageIdentity.projection: one export format's writer, the binary and the
// resources that writer reads.
func projection(root, format string) ojson.Object {
	files := []string{"VERSION"}
	for _, name := range formatResources[format] {
		files = append(files, dataFiles(root, filepath.Join("lib/relatex", name), everything)...)
	}
	return fingerprint(root, "export", files)
}

// forced is StageIdentity.forced: the named stages with every stage that depends on them.
func forced(stages []string) []string {
	result := []string{}
	add := func(name string) {
		if !slices.Contains(result, name) {
			result = append(result, name)
		}
	}
	for _, name := range stages {
		if format, ok := strings.CutPrefix(name, "export."); ok && slices.Contains(projectionFormats, format) {
			add(name)
			continue
		}
		after, ok := dependents[name]
		if !ok {
			rec.Fail("Unknown forced stage: %s", name)
		}
		add(name)
		for _, dependent := range after {
			add(dependent)
		}
	}
	return result
}

// stageKey is StageIdentity.key.
func stageKey(name string, inputs, configuration any, implementation ojson.Object) string {
	return digest(ojson.Object{{"schema_version", identityVersion}, {"stage", name}, {"inputs", inputs},
		{"configuration", configuration}, {"implementation", implementation.Get("sha256")}})
}

// digest is Acceptance.digest of a value that may hold ordered objects.
func digest(value any) string {
	text, err := canonjson.Digest(ojson.Plain(value))
	if err != nil {
		rec.Fail("%v", err)
	}
	return text
}

// sortedObject is a map as an object in key order (Ruby's sorted hashes).
func sortedObject(m rec.Map) ojson.Object {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make(ojson.Object, len(keys))
	for index, key := range keys {
		result[index] = ojson.Pair{key, m[key]}
	}
	return result
}
