// Package typstvendor writes the vendored Typst Universe packages (Pergamon, citegeist and the
// packages Pergamon imports) into a Typst export, so the export compiles without the network or a
// package path (plans/archive/typst-bibliography.md, decision D). The files are resources under
// profiles/typst-vendor/NAME/VERSION/, pinned by profiles/typst-vendor/pin.json with each file's
// SHA-256; they are written to typst-source/support/vendor/NAME/ with every @preview import rewritten to a
// relative path.
package typstvendor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Directory is the vendored packages' resource directory, relative to the resource root.
const Directory = "profiles/typst-vendor"

// PinFile is the pin file, relative to the resource root.
const PinFile = Directory + "/pin.json"

// ExportDirectory is where the packages go, relative to the typst-source directory.
const ExportDirectory = "support/vendor"

// besideDirectory is the vendor directory as a file in support/ names it.
const besideDirectory = "vendor"

// Package is one pinned package: its version, the series a patch update stays in, its entry point
// and each vendored file's SHA-256 (paths relative to the package).
type Package struct {
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Series        string            `json:"series"`
	License       string            `json:"license"`
	Entrypoint    string            `json:"entrypoint"`
	TarballURL    string            `json:"tarball_url"`
	TarballSHA256 string            `json:"tarball_sha256"`
	Files         map[string]string `json:"files"`
}

// Pin is the pin file: the packages and the cap on their total size.
type Pin struct {
	MaxBytes int64     `json:"max_bytes"`
	Packages []Package `json:"packages"`
}

// File is one written file: its path relative to the typst-source directory, its SHA-256 for
// manifest.json and its size.
type File struct {
	Path   string
	SHA256 string
	Bytes  int
}

// LoadPin reads ROOT/profiles/typst-vendor/pin.json.
func LoadPin(root string) (Pin, error) {
	var pin Pin
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(PinFile)))
	if err != nil {
		return pin, err
	}
	if err = json.Unmarshal(data, &pin); err != nil {
		return pin, fmt.Errorf("typstvendor: %s: %w", PinFile, err)
	}
	return pin, nil
}

// SortedFiles is the package's vendored file paths, sorted.
func (p Package) SortedFiles() []string {
	names := make([]string, 0, len(p.Files))
	for name := range p.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Source is the resource path of one of the package's files, relative to the resource root.
func (p Package) Source(name string) string {
	return path.Join(Directory, p.Name, p.Version, name)
}

// ReadVerified reads one vendored file from the resource root and checks it against the pin.
func ReadVerified(root string, p Package, name string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p.Source(name))))
	if err != nil {
		return nil, err
	}
	if digest(data) != p.Files[name] {
		return nil, fmt.Errorf("typstvendor: %s does not match its pinned SHA-256", p.Source(name))
	}
	return data, nil
}

// Write writes every pinned file into DIRECTORY/support/vendor/NAME/ (DIRECTORY is the typst-source
// directory), rewriting the .typ files' @preview imports, and returns the written files in order.
func Write(root, directory string) ([]File, error) {
	pin, err := LoadPin(root)
	if err != nil {
		return nil, err
	}
	written := []File{}
	var total int64
	for _, p := range pin.Packages {
		for _, name := range p.SortedFiles() {
			data, readErr := ReadVerified(root, p, name)
			if readErr != nil {
				return nil, readErr
			}
			total += int64(len(data))
			target := path.Join(ExportDirectory, p.Name, name)
			if strings.HasSuffix(name, ".typ") {
				if data, readErr = Rewrite(data, path.Join(p.Name, name), pin.Packages); readErr != nil {
					return nil, readErr
				}
			}
			if writeErr := writeFile(filepath.Join(directory, filepath.FromSlash(target)), data); writeErr != nil {
				return nil, writeErr
			}
			written = append(written, File{Path: target, SHA256: digest(data), Bytes: len(data)})
		}
	}
	if total > pin.MaxBytes {
		return nil, fmt.Errorf("typstvendor: the vendored packages are %d bytes, over the %d-byte cap", total, pin.MaxBytes)
	}
	return written, nil
}

// previewImport is a Universe package reference in a string literal.
var previewImport = regexp.MustCompile(`"@preview/([a-z0-9][a-z0-9-]*):([0-9]+\.[0-9]+\.[0-9]+)"`)

// Rewrite replaces every "@preview/NAME:VERSION" reference outside comments in the file at FROM
// (relative to the vendor directory, for example pergamon/src/bibtypst.typ) with the relative path
// of the vendored package's entry point. A reference to a package or version not vendored is an
// error; comment lines (documentation examples) are left as they are.
func Rewrite(source []byte, from string, packages []Package) ([]byte, error) {
	return rewrite(source, from, packages, func(entry string) string { return relative(path.Dir(from), entry) })
}

// RewriteBeside is Rewrite for a file NAME in the typst-source directory itself, beside vendor/ (the
// style file support/bibstyle.typ): its imports become vendor/NAME/ENTRY.
func RewriteBeside(source []byte, name string, packages []Package) ([]byte, error) {
	return rewrite(source, name, packages, func(entry string) string { return path.Join(besideDirectory, entry) })
}

func rewrite(source []byte, from string, packages []Package, target func(entry string) string) ([]byte, error) {
	entries := map[string]string{}
	for _, p := range packages {
		entries[p.Name+":"+p.Version] = path.Join(p.Name, p.Entrypoint)
	}
	lines := strings.SplitAfter(string(source), "\n")
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		var missing string
		lines[index] = previewImport.ReplaceAllStringFunc(line, func(match string) string {
			parts := previewImport.FindStringSubmatch(match)
			entry, ok := entries[parts[1]+":"+parts[2]]
			if !ok {
				missing = parts[1] + ":" + parts[2]
				return match
			}
			return `"` + target(entry) + `"`
		})
		if missing != "" {
			return nil, fmt.Errorf("typstvendor: %s imports @preview/%s, which is not vendored", from, missing)
		}
		if strings.Contains(lines[index], "@preview/") {
			return nil, fmt.Errorf("typstvendor: %s line %d keeps a @preview reference", from, index+1)
		}
	}
	return []byte(strings.Join(lines, "")), nil
}

// relative is the slash path from the directory FROM to TARGET, both relative to the vendor directory.
func relative(from, target string) string {
	up := 0
	if from != "." {
		up = strings.Count(from, "/") + 1
	}
	return strings.Repeat("../", up) + target
}

func writeFile(target string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
