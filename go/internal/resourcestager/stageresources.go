// Package resourcestager copies the files the relatex binary embeds (VERSION, tex/, schemas/,
// profiles/ and the non-Ruby files of lib/relatex/) into go/internal/resources/data and writes
// the per-stage source digests (stage-sources.json). It is the Go build step: a build needs
// only Go.
package resourcestager

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// dataDir is the staging target below the repository root; its README is the only file kept in git.
const dataDir = "go/internal/resources/data"

const keep = "README.md"

var trees = []string{"tex", "schemas", "profiles"}

// Stage rebuilds the staged resources of the repository at root and returns the number of
// copied resource files (stage-sources.json not counted).
func Stage(root string) (int, error) {
	target := filepath.Join(root, filepath.FromSlash(dataDir))
	if err := clearTarget(target); err != nil {
		return 0, err
	}
	files, err := resourceFiles(root)
	if err != nil {
		return 0, err
	}
	for _, relative := range files {
		if copyErr := copyFile(root, target, relative); copyErr != nil {
			return 0, copyErr
		}
	}
	digests, err := stageDigests(root)
	if err != nil {
		return 0, err
	}
	err = os.WriteFile(filepath.Join(target, "stage-sources.json"), []byte(digestJSON(digests)), 0o644)
	return len(files), err
}

// clearTarget removes everything in target except its README, and creates target when absent.
func clearTarget(target string) error {
	entries, err := os.ReadDir(target)
	if os.IsNotExist(err) {
		return os.MkdirAll(target, 0o755)
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == keep {
			continue
		}
		if err := os.RemoveAll(filepath.Join(target, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// resourceFiles are the regular files to embed, as slash paths relative to root, sorted: VERSION,
// the three trees and lib/relatex without its Ruby files. Hidden files and symbolic links are left out.
func resourceFiles(root string) ([]string, error) {
	files := []string{"VERSION"}
	for _, tree := range trees {
		found, err := walkTree(root, tree, func(string) bool { return true })
		if err != nil {
			return nil, err
		}
		files = append(files, found...)
	}
	found, err := walkTree(root, "lib/relatex", func(name string) bool { return !strings.HasSuffix(name, ".rb") })
	if err != nil {
		return nil, err
	}
	files = append(files, found...)
	sort.Strings(files)
	return files, nil
}

// walkTree lists the regular, non-hidden files below root/tree that accept returns true for.
func walkTree(root, tree string, accept func(string) bool) ([]string, error) {
	base := filepath.Join(root, filepath.FromSlash(tree))
	found := []string{}
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == base {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			return skipHidden(entry)
		}
		if !entry.Type().IsRegular() || !accept(entry.Name()) {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		found = append(found, filepath.ToSlash(relative))
		return err
	})
	return found, err
}

func skipHidden(entry fs.DirEntry) error {
	if entry.IsDir() {
		return fs.SkipDir
	}
	return nil
}

func copyFile(root, target, relative string) error {
	source := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	destination := filepath.Join(target, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destination, data, info.Mode().Perm())
}
