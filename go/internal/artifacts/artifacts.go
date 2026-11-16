// Package artifacts is the Go port of Relatex::ArtifactResolver: logical artifact names
// resolved inside an immutable root against their inventoried digests, and producer path
// prefixes mapped to the directories that hold them now.
package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/rec"
)

// Resolver is ArtifactResolver: a root, its inventory (name to SHA-256) and the prefixes of
// a producer's paths.
type Resolver struct {
	Root     string
	Files    map[string]string
	prefixes map[string]string
	order    []string
}

// New is ArtifactResolver.new(root:, files:, prefixes:); root is resolved to its real path.
func New(root string, files, prefixes map[string]string) *Resolver {
	resolved, err := fspath.Real(root)
	if err != nil {
		rec.Fail("%v", err)
	}
	r := &Resolver{Root: resolved, Files: files, prefixes: map[string]string{}}
	for old, current := range prefixes {
		r.prefixes[ExpandPath(old, "")] = ExpandPath(current, "")
	}
	for prefix := range r.prefixes {
		r.order = append(r.order, prefix)
	}
	// The longest prefix first (Ruby's sort_by is not stable; equal lengths are distinct paths).
	sort.SliceStable(r.order, func(i, j int) bool { return len(r.order[i]) > len(r.order[j]) })
	return r
}

// Canonical collapses doubled slashes: an author's "figures//a.pdf" names the inventoried
// "figures/a.pdf". Escapes and absolute names stay for Relative to refuse.
func Canonical(name string) string {
	for strings.Contains(name, "//") {
		name = strings.ReplaceAll(name, "//", "/")
	}
	return name
}

// Relative is ArtifactResolver.relative!: a logical name without escapes.
func Relative(name string) string {
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\") || control(name) {
		rec.Fail("Unsafe logical artifact name")
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			rec.Fail("Unsafe logical artifact name")
		}
	}
	return name
}

func control(name string) bool {
	for index := 0; index < len(name); index++ {
		if name[index] < 0x20 {
			return true
		}
	}
	return false
}

// Resolve is ArtifactResolver#resolve: the inventoried file, unchanged, inside the root.
func (r *Resolver) Resolve(name string) string {
	name = Canonical(name)
	Relative(name)
	expected, ok := r.Files[name]
	if !ok {
		rec.Fail("Uninventoried artifact: %s", name)
	}
	path, err := fspath.Real(filepath.Join(r.Root, name))
	if err != nil {
		rec.Fail("%v", err)
	}
	if !fspath.Inside(r.Root, path) {
		rec.Fail("Artifact escapes immutable root")
	}
	if command.FileDigest(path) != expected {
		rec.Fail("Changed artifact: %s", name)
	}
	return path
}

// Source is ArtifactResolver#source: a source file's identity record.
func (r *Resolver) Source(name string) rec.Map {
	path := r.Resolve(name)
	hash := command.FileDigest(path)
	info, err := os.Stat(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	identity, err := canonjson.Marshal([]any{name, hash})
	if err != nil {
		rec.Fail("%v", err)
	}
	sum := sha256.Sum256(identity)
	return rec.Map{"id": "source:" + hex.EncodeToString(sum[:]), "file": name, "sha256": hash, "bytes": info.Size()}
}

// Translate is ArtifactResolver#translate: a path under a producer prefix, mapped to the
// directory that holds it now.
func (r *Resolver) Translate(path, relativeTo string) string {
	absolute := ExpandPath(path, relativeTo)
	for _, prefix := range r.order {
		if fspath.Within(prefix, absolute) {
			rest := fspath.Name(prefix, absolute)
			if fspath.Equal(prefix, absolute) {
				return r.prefixes[prefix] + string(filepath.Separator) // File.join(dir, "")
			}
			return filepath.Join(r.prefixes[prefix], filepath.FromSlash(rest))
		}
	}
	return absolute
}

// ExpandPath is File.expand_path(path, base); an empty base is the working directory.
func ExpandPath(path, base string) string {
	if !filepath.IsAbs(path) {
		if base == "" {
			base, _ = os.Getwd()
		}
		path = filepath.Join(base, path)
	}
	return filepath.Clean(path)
}

// FileManifest is file_manifest(root, '**/*') (or '{top,...}/**/*'): every file under root,
// or under each top directory of it, named relative to root, with its SHA-256. Dir.glob skips
// dot files and dot directories and does not descend into linked directories.
func FileManifest(root string, tops ...string) rec.Map {
	files := rec.Map{}
	if len(tops) == 0 {
		tops = []string{"."}
	}
	for _, top := range tops {
		start := filepath.Join(root, top)
		err := filepath.WalkDir(start, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				if path == start && os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if path != start && strings.HasPrefix(entry.Name(), ".") {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if info, statErr := os.Stat(path); statErr != nil || !info.Mode().IsRegular() {
				return nil
			}
			name, _ := filepath.Rel(root, path)
			files[fspath.Slash(name)] = command.FileDigest(path)
			return nil
		})
		if err != nil {
			rec.Fail("%v", err)
		}
	}
	return files
}
