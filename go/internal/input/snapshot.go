package input

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/rec"
)

// Capture is SourceSnapshot.capture: a ZIP checked into directory/input, or a paper directory
// zipped into directory/directory-source.zip first; then the main file selected.
func Capture(input, directory, main string, exclude, ignore []string) *Archive {
	resolved, err := fspath.Real(input)
	check(err)
	resolved, err = filepath.Abs(resolved)
	check(err)
	path := resolved
	var skipped []any
	if info, statErr := os.Stat(resolved); statErr == nil && info.IsDir() {
		path, skipped = directoryZIP(resolved, directory, exclude)
	}
	archive := NewArchive(path, filepath.Join(directory, "input"))
	if len(skipped) > 0 {
		archive.Manifest["skipped"] = skipped
	}
	archive.SelectMain(main, ignore)
	return archive
}

func within(path, root string) bool { return fspath.Within(root, path) }

// directoryZIP is SourceSnapshot.directory_zip: the directory's files, outside the excluded
// paths, zipped as rubyzip zips them, and checked against the files after zipping.
func directoryZIP(input, output string, exclude []string) (string, []any) {
	excluded := []string{}
	for _, path := range exclude {
		absolute, err := filepath.Abs(path)
		check(err)
		if !slices.Contains(excluded, absolute) {
			excluded = append(excluded, absolute)
		}
	}
	if within(output, input) && !slices.ContainsFunc(excluded, func(path string) bool { return within(output, path) }) {
		rec.Fail("Snapshot output inside source must be explicitly excluded")
	}
	files, skipped := sourceFiles(input, excluded)
	total := int64(0)
	expected := map[string]string{}
	names := []string{}
	for _, file := range files {
		info, err := os.Stat(file)
		check(err)
		total += info.Size()
		name := fspath.Name(input, file)
		names = append(names, name)
		expected[name] = fileDigest(file)
	}
	if total > Limits["expanded_bytes"] {
		rec.Fail("Source expanded byte limit exceeded")
	}
	slices.Sort(names)
	path := filepath.Join(output, "directory-source.zip")
	writeZip(path, names, func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(input, name))
		check(err)
		return data
	})
	for _, name := range names {
		if fileDigest(filepath.Join(input, name)) != expected[name] {
			rec.Fail("Input changed during snapshot: %s", name)
		}
	}
	verifyZIP(path, expected)
	return path, skipped
}

// sourceFiles is SourceSnapshot.source_files: every regular file, plus every symbolic link to a
// regular file inside the source (kept under the link's name with the target's bytes), failing
// on more kept entries than the limit. Dangling links, links leaving the source or naming a
// directory, and special files are skipped and returned as {path, code, reason} records.
func sourceFiles(input string, excluded []string) (files []string, skipped []any) {
	files = []string{}
	entries := int64(0)
	err := filepath.WalkDir(input, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == input {
			return nil
		}
		if slices.ContainsFunc(excluded, func(excludedPath string) bool { return within(path, excludedPath) }) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if keep, reason := classify(input, path, entry); !keep {
			skipped = append(skipped, skippedEntry(input, path, entry, reason))
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		entries++
		if entries > Limits["entries"] {
			rec.Fail("Source entry limit exceeded")
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	check(err)
	return files, skipped
}

// skippedEntry is the record of a skipped entry: its relative path, diagnostic code and reason.
func skippedEntry(input, path string, entry fs.DirEntry, reason string) rec.Map {
	code := "input.special_skipped"
	switch {
	case unportableName(entry.Name()):
		code = "input.name_skipped"
	case entry.Type()&fs.ModeSymlink != 0:
		code = "input.symlink_skipped"
	}
	return rec.Map{"path": fspath.Name(input, path), "code": code, "reason": reason}
}

// classify says whether a walked entry is kept, and why not when it is skipped. Links are
// never followed into directories, so the walk cannot cycle.
func classify(input, path string, entry fs.DirEntry) (bool, string) {
	mode := entry.Type()
	switch {
	case unportableName(entry.Name()):
		return false, "file name with a backslash or control character cannot be stored portably in the snapshot"
	case entry.IsDir() || mode.IsRegular():
		return true, ""
	case mode&fs.ModeSymlink == 0:
		return false, "special file (socket, FIFO or device)"
	}
	target, err := fspath.Real(path)
	if err != nil {
		return false, "dangling symbolic link"
	}
	if !within(target, input) {
		return false, "symbolic link leading outside the source directory"
	}
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return false, "symbolic link to a directory or special file"
	}
	return true, ""
}

// verifyZIP is SourceSnapshot.verify_zip: every entry read back with the source file's digest.
func verifyZIP(path string, expected map[string]string) {
	data, err := os.ReadFile(path)
	check(err)
	for _, entry := range centralDirectory(data) {
		var content strings.Builder
		archive := &Archive{}
		archive.copyEntry(data, entry, &content)
		if digest([]byte(content.String())) != expected[entry.name] {
			rec.Fail("Directory ZIP differs from source snapshot")
		}
	}
}

// unportableName is a file name the snapshot ZIP cannot hold portably: a backslash or a control
// character (readers rewrite the former as a slash).
func unportableName(name string) bool { return unsafeName.MatchString(name) }
