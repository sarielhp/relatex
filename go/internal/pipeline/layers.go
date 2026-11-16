package pipeline

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// layers is LayerRun: each layer keeps its inputs, products and failure separately for replay.
type layers struct {
	directory string
	root      string // the repository root (the data files the implementations name)
	stages    []ojson.Object
	cache     *StageCache
	resume    string
	forced    []string
}

func newLayers(directory, root string, cache *StageCache, resume string, forced []string) *layers {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		rec.Fail("%v", err)
	}
	if _, statErr := os.Lstat(absolute); statErr == nil {
		rec.Fail("Layer run already exists")
	}
	mkdirAll(absolute)
	return &layers{directory: absolute, root: root, stages: []ojson.Object{}, cache: cache, resume: resume, forced: forced}
}

// path is a layer's directory: 01-input, 02-engine, ...
func (l *layers) path(name string) string {
	return filepath.Join(l.directory, fmt.Sprintf("%02d-%s", slices.Index(order, name)+1, name))
}

// receipt is the stage receipt by layer name; nil when the stage has none.
func (l *layers) receipt(name string) ojson.Object {
	for _, receipt := range l.stages {
		if receipt.Get("layer") == name {
			return receipt
		}
	}
	return nil
}

// stageRun is one LayerRun#stage call: its receipt, kept as it changes.
type stageRun struct {
	layers   *layers
	name     string
	root     string
	started  time.Time
	receipt  ojson.Object
	hashes   ojson.Object
	identity ojson.Object
}

// stage is LayerRun#stage: the layer's receipt, its block run in the layer's workspace (or
// its products restored from the stage cache), the outputs inventoried.
func (l *layers) stage(name string, inputs ojson.Object, configuration any, useCache bool, block func(string) any) any {
	if !slices.Contains(order, name) || l.receipt(name) != nil {
		rec.Fail("Unknown or repeated pipeline layer")
	}
	if len(l.stages) > 0 && slices.Index(order, l.stages[len(l.stages)-1].Get("layer").(string)) >= slices.Index(order, name) {
		rec.Fail("Pipeline layer order differs")
	}
	run := &stageRun{layers: l, name: name, root: l.path(name), started: time.Now()}
	command.Trace("stage " + name + " begins")
	if Progress != nil {
		Progress(name, slices.Index(order, name)+1, len(order))
	}
	defer command.Trace("stage " + name + " ends")
	defer run.finish()
	mkdirAll(run.root)
	run.hashes = ojson.Object{}
	digests := ojson.Object{}
	for _, pair := range inputs {
		path := absolutePath(pair.Value().(string))
		sha := command.FileDigest(path)
		run.hashes = append(run.hashes, ojson.Pair{pair.Key(), ojson.Object{{"path", path}, {"sha256", sha}}})
		digests = append(digests, ojson.Pair{pair.Key(), sha})
	}
	run.identity = implementation(l.root, name)
	run.receipt = ojson.Object{{"layer", name}, {"status", "running"}, {"inputs", run.hashes}, {"configuration", configuration},
		{"configuration_sha256", digest(configuration)}, {"implementation", run.identity},
		{"identity", stageKey(name, digests, configuration, run.identity)}}
	writeOrdered(filepath.Join(run.root, "stage.json"), run.receipt)
	command.Trace(fmt.Sprintf("stage %s sources %.8s identity %.12v implementation %.12v inputs %.12v configuration %.12v", name, stageSources()[name], run.receipt.Get("identity"), run.identity.Get("sha256"), digest(digests), run.receipt.Get("configuration_sha256")))
	var result any
	if l.cache != nil && useCache {
		previous := ""
		if l.resume != "" {
			previous = filepath.Join(l.resume, filepath.Base(run.root))
		}
		restored := l.cache.stage(run.receipt.Get("identity").(string), run.root, name, slices.Contains(l.forced, name), previous,
			func(destination string) any { return run.operate(destination, block) })
		result = restored.result
		run.receipt = run.receipt.Set("cache", restored.cache).Set("cache_metadata", restored.metadata)
	} else {
		result = run.operate(run.root, block)
		run.receipt = run.receipt.Set("cache", ojson.Object{{"status", "disabled"}, {"acceptance_reused", false}})
	}
	run.verifyInputs()
	run.receipt = run.receipt.Set("status", "complete").Set("outputs", sortedObject(outputInventory(run.root))).Set("result", result)
	return result
}

// operate runs the block in the layer's own workspace; its inputs and the implementation
// must not change while it runs.
func (r *stageRun) operate(destination string, block func(string) any) any {
	previous := command.Workspace
	command.Workspace = newWorkspace(r.root)
	defer func() { command.Workspace = previous }()
	result := block(destination)
	r.verifyInputs()
	if !rec.Equal(ojson.Plain(implementation(r.layers.root, r.name)), ojson.Plain(r.identity)) {
		rec.Fail("Stage implementation changed during execution")
	}
	return result
}

func (r *stageRun) verifyInputs() {
	for _, pair := range r.hashes {
		item := pair.Value().(ojson.Object)
		if command.FileDigest(item.Get("path").(string)) != item.Get("sha256") {
			rec.Fail("Layer input changed during execution")
		}
	}
}

// finish records the receipt, failed when the stage failed, with its time, in the layer's
// stage.json and the run's pipeline.json.
func (r *stageRun) finish() {
	recovered := recover()
	if r.receipt != nil {
		if recovered != nil {
			r.receipt = r.receipt.Set("status", "failed").Set("error", failureMessage(recovered)).
				Set("error_class", failureClass(recovered))
		}
		r.receipt = r.receipt.Set("seconds", time.Since(r.started).Seconds())
		r.layers.stages = append(r.layers.stages, r.receipt)
		writeOrdered(filepath.Join(r.root, "stage.json"), r.receipt)
		stages := make([]any, len(r.layers.stages))
		for index, receipt := range r.layers.stages {
			stages[index] = receipt
		}
		writeOrdered(filepath.Join(r.layers.directory, "pipeline.json"), ojson.Object{{"stages", stages}})
	}
	if recovered != nil {
		panic(recovered)
	}
}

// outputInventory is LayerRun#inventory: the layer's files but its receipt, with digests.
func outputInventory(root string) rec.Map {
	result := rec.Map{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.HasPrefix(entry.Name(), ".") && path != root {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			rec.Fail("Layer output symlink is unsupported")
		}
		if entry.Type().IsRegular() && path != filepath.Join(root, "stage.json") {
			result[fspath.Name(root, path)] = command.FileDigest(path)
		}
		return nil
	})
	if err != nil {
		rec.Fail("%v", err)
	}
	return result
}

// newWorkspace is RunWorkspace.new(root): the directory made, its real path.
func newWorkspace(root string) string {
	absolute := absolutePath(root)
	mkdirAll(absolute)
	if symlink(absolute) {
		rec.Fail("Workspace root is a symlink")
	}
	resolved, err := fspath.Real(absolute)
	if err != nil {
		rec.Fail("%v", err)
	}
	return resolved
}

func absolutePath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	return absolute
}

// failureMessage is a failure's message as Ruby's e.message prints it.
func failureMessage(recovered any) string {
	if failure, ok := recovered.(rec.Failure); ok {
		return failure.Message
	}
	if err, ok := recovered.(error); ok {
		return err.Error()
	}
	return fmt.Sprint(recovered)
}
