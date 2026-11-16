package pipeline

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/engine"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

// stampVersion names the format of an up-to-date stamp.
const stampVersion = "relatex-rlx-stamp-1"

// stampPath is where an rlx run records what its products were made from: one file per directory and
// main file, under the runtime cache.
func (c *conversion) stampPath(directory, main string) string {
	key, err := canonjson.Marshal([]any{directory, main})
	if err != nil {
		rec.Fail("%v", err)
	}
	return filepath.Join(command.CacheRoot(), "rlx", digest(string(key))[:16]+".json")
}

// productFiles are the files an rlx run writes beside the main file (the products, not sources).
func productFiles(directory, main string) []string {
	job := engine.JobName(main)
	base := filepath.Join(directory, filepath.Dir(filepath.FromSlash(main)))
	return []string{filepath.Join(base, job+".pdf"), filepath.Join(base, job+".synctex.gz"), filepath.Join(base, job+".bbl")}
}

// sourcesKey is what the products depend on: the version, the toolchain, the SyncTeX mode and every
// source file with its content, but the products themselves (the bibliography, written beside the
// paper, is checked against its recorded hash instead).
func (c *conversion) sourcesKey(directory, main string) string {
	skip := productFiles(directory, main)
	files := []any{}
	for _, value := range rec.FetchList(c.archive.Manifest, "files") {
		file := rec.AsMap(value)
		if file["directory"] == true {
			continue
		}
		path := rec.AsString(rec.Fetch(file, "path"))
		if slices.Contains(skip, filepath.Join(directory, filepath.FromSlash(path))) {
			continue
		}
		files = append(files, []any{path, rec.Fetch(file, "sha256")})
	}
	key := []any{stampVersion, executableDigest(), texsandbox.Current(c.options.Engine).Fingerprint(),
		c.options.Synctex, main, files}
	if date := sourceDate(); len(date) > 0 {
		key = append(key, date)
	}
	// The caller's search path and what its directories hold (decision 103): a newer shared style rebuilds.
	if host := c.host(); host != nil {
		key = append(key, []any{"texinputs", strings.Join(host.SearchPath, ":"), host.Fingerprint()})
	}
	return digest(key)
}

// productsUpToDate is latexmk's "nothing to do": the sources, the toolchain and the products on disk
// are those of the run that wrote them. It calls Unchanged and reports the products.
func (c *conversion) productsUpToDate() bool {
	directory, main := c.inputDirectory(), rec.AsString(c.archive.Manifest["main"])
	if directory == "" || main == "" || (c.options.Publish != nil && !*c.options.Publish) {
		return false
	}
	data, err := os.ReadFile(c.stampPath(directory, main))
	if err != nil {
		return false
	}
	value, err := canonjson.Decode(strings.NewReader(string(data)))
	stamp, ok := value.(rec.Map)
	if err != nil || !ok || stamp["key"] != c.sourcesKey(directory, main) {
		return false
	}
	recorded, _ := stamp["products"].(rec.Map)
	products := productFiles(directory, main)
	existing := []string{}
	for _, product := range products {
		sum, known := recorded[filepath.Base(product)]
		info, statErr := os.Stat(product)
		if !known {
			continue
		}
		if statErr != nil || !info.Mode().IsRegular() || command.FileDigest(product) != sum {
			return false
		}
		existing = append(existing, product)
	}
	if len(existing) == 0 || recorded[engine.JobName(main)+".pdf"] == nil {
		return false
	}
	if c.options.Unchanged != nil {
		c.options.Unchanged()
	}
	if c.options.Published != nil {
		c.options.Published(existing)
	}
	return true
}

// recordProducts writes the stamp after the products were published.
func (c *conversion) recordProducts(published []string) {
	directory, main := c.inputDirectory(), rec.AsString(c.archive.Manifest["main"])
	if directory == "" || main == "" || len(published) == 0 {
		return
	}
	sums := rec.Map{}
	for _, product := range published {
		sums[filepath.Base(product)] = command.FileDigest(product)
	}
	path := c.stampPath(directory, main)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = canonjson.WriteFile(path, rec.Map{"key": c.sourcesKey(directory, main), "products": sums})
}

// executableDigest is the hash of this program: a rebuilt relatex makes new products.
func executableDigest() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return command.FileDigest(path)
}
