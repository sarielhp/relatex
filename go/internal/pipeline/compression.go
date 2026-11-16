package pipeline

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"

	"github.com/sarielhp/relatex/internal/artifacts"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// RestCompression: files at rest in the project cache (kept attempts and stage-cache
// entries) are compressed with zstd as the last step of a run, and expanded again before
// anything reads them. A manifest in each root lists the files it compressed, so expansion
// restores exactly those. Without a zstd program the files stay as they are.
const (
	restManifest = ".relatex-compressed.json"
	restSuffix   = ".zst"
	restMinimum  = 16 * 1024
	restBatch    = 256
)

// restSkip: already compressed, or not worth it.
var restSkip = regexp.MustCompile(`(?i)\.(?:pdf|gz|zst|xz|zip|png|jpe?g|gif|webp|woff2?|otf|ttf|ttc|pfb)\z`)

// CompressAtRest is RestCompression.compress!: each root's large files compressed, the
// originals removed once its manifest names them.
func CompressAtRest(roots []string) {
	if command.Executable("zstd") == "" {
		os.Stderr.WriteString("zstd not found; the project cache stays uncompressed\n")
		return
	}
	plan := make([][]string, len(roots))
	all := []string{}
	for index, root := range roots {
		plan[index] = restCandidates(root)
		all = append(all, plan[index]...)
	}
	for start := 0; start < len(all); start += restBatch {
		zstd(append([]string{"-3", "-T0", "--keep"}, all[start:min(start+restBatch, len(all))]...)...)
	}
	for index, root := range roots {
		if len(plan[index]) > 0 {
			recordCompressed(root, plan[index])
		}
	}
}

// ExpandAtRest is RestCompression.expand!: the files a root's manifest lists expanded again.
func ExpandAtRest(root string) {
	manifest := filepath.Join(root, restManifest)
	if info, err := os.Lstat(manifest); err != nil || !info.Mode().IsRegular() {
		return
	}
	stored := []string{}
	for _, item := range rec.AsList(readPlain(manifest)) {
		path := filepath.Join(root, artifacts.Relative(rec.AsString(item)))
		// An interrupted compression leaves both copies; the original wins.
		if isFile(path) && isFile(path+restSuffix) {
			removeFile(path + restSuffix)
			continue
		}
		if isFile(path + restSuffix) {
			stored = append(stored, path+restSuffix)
		}
	}
	for start := 0; start < len(stored); start += restBatch {
		zstd(append([]string{"-d", "--rm"}, stored[start:min(start+restBatch, len(stored))]...)...)
	}
	removeFile(manifest)
}

func restCandidates(root string) []string {
	result := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() || entry.Name() == restManifest || restSkip.MatchString(path) || isFile(path+restSuffix) {
			return nil
		}
		if info, err := entry.Info(); err == nil && info.Size() >= restMinimum {
			result = append(result, path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		rec.Fail("%v", err)
	}
	sort.Strings(result)
	return result
}

// recordCompressed: the originals go only once the manifest names their compressed copies.
func recordCompressed(root string, files []string) {
	manifest := filepath.Join(root, restManifest)
	listed := []string{}
	if isFile(manifest) {
		for _, item := range rec.AsList(readPlain(manifest)) {
			listed = append(listed, rec.AsString(item))
		}
	}
	for _, path := range files {
		listed = append(listed, fspath.Name(root, path))
	}
	sort.Strings(listed)
	listed = slices.Compact(listed)
	names := make([]any, len(listed))
	for index, name := range listed {
		names[index] = name
	}
	writeOrdered(manifest, names)
	for _, path := range files {
		removeFile(path)
	}
}

func zstd(arguments ...string) {
	output, err := exec.Command("zstd", append([]string{"-q", "-f"}, arguments...)...).CombinedOutput()
	if err != nil {
		rec.Fail("zstd failed: %s", rec.Strip(string(output)))
	}
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func removeFile(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		rec.Fail("%v", err)
	}
}

// readPlain reads a JSON file in its order (nil-safe callers check the type).
func readPlain(path string) any {
	value, err := ojson.ReadFile(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	return value
}

// writeOrdered is Relatex.write_json: objects in their order.
func writeOrdered(path string, value any) {
	if err := ojson.WriteFile(path, value); err != nil {
		rec.Fail("%v", err)
	}
}
