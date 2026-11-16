// Package resources carries the files the Go relatex reads at run time (VERSION, tex/,
// schemas/, profiles/ and the non-Ruby files of lib/relatex/), embedded in the binary by
// go/cmd/stageresources and go:embed, so the installed relatex is one file. They are
// unpacked once per content into the runtime cache, as the Ruby package's launcher did.
package resources

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

//go:embed all:data
var embedded embed.FS

// readme is the one file a binary built without the staged copy embeds.
const readme = "README.md"

// File is the embedded file with this name under data/ (nil when the binary does not embed it): the
// files that describe the build itself, as opposed to the ones a run reads from the unpacked tree.
func File(name string) []byte {
	data, err := embedded.ReadFile("data/" + name)
	if err != nil {
		return nil
	}
	return data
}

// Root is the directory of the unpacked resources under cacheRoot, unpacking them first when
// needed; "" when this binary embeds none (a development build, which finds them beside it).
func Root(cacheRoot string) (string, error) {
	names, err := files()
	if err != nil || len(names) == 0 {
		return "", err
	}
	digest, err := contentDigest(names)
	if err != nil {
		return "", err
	}
	directory := filepath.Join(cacheRoot, "runtime", "go-"+digest[:16])
	if complete(directory, digest) {
		return directory, nil
	}
	return directory, unpack(directory, digest, names)
}

// files are the embedded resources' paths under data/, sorted, without the README.
func files() ([]string, error) {
	names := []string{}
	err := fs.WalkDir(embedded, "data", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && path != "data/"+readme {
			names = append(names, path)
		}
		return nil
	})
	sort.Strings(names)
	return names, err
}

// contentDigest names the resources by their paths and bytes.
func contentDigest(names []string) (string, error) {
	hash := sha256.New()
	for _, name := range names {
		data, err := embedded.ReadFile(name)
		if err != nil {
			return "", err
		}
		hash.Write([]byte(name + "\x00"))
		sum := sha256.Sum256(data)
		hash.Write(sum[:])
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// complete reports whether directory holds these resources: its marker names their digest.
func complete(directory, digest string) bool {
	marker, err := os.ReadFile(filepath.Join(directory, ".complete"))
	return err == nil && string(marker) == digest
}

// unpack writes the resources into a temporary directory beside directory and renames it into
// place, so a concurrent run never sees a partial copy.
func unpack(directory, digest string, names []string) error {
	parent := filepath.Dir(directory)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(parent, ".go-unpack-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	for _, name := range names {
		data, readErr := embedded.ReadFile(name)
		if readErr != nil {
			return readErr
		}
		target := filepath.Join(temporary, filepath.FromSlash(name[len("data/"):]))
		if mkdirErr := os.MkdirAll(filepath.Dir(target), 0o755); mkdirErr != nil {
			return mkdirErr
		}
		if writeErr := os.WriteFile(target, data, 0o644); writeErr != nil {
			return writeErr
		}
	}
	if writeErr := os.WriteFile(filepath.Join(temporary, ".complete"), []byte(digest), 0o644); writeErr != nil {
		return writeErr
	}
	if renameErr := os.Rename(temporary, directory); renameErr != nil {
		if complete(directory, digest) {
			return nil // Another run unpacked the same resources first.
		}
		if errors.Is(renameErr, fs.ErrExist) {
			if removeErr := os.RemoveAll(directory); removeErr != nil {
				return removeErr
			}
			return os.Rename(temporary, directory)
		}
		return renameErr
	}
	return nil
}
