//go:build windows

package command

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ExeSuffix is what a program's file name adds to its name on Windows.
const ExeSuffix = ".exe"

func isPath(name string) bool {
	return strings.ContainsAny(name, `/\`) || filepath.VolumeName(name) != ""
}

// executableNames is path as given when it has an extension, then path with each PATHEXT
// extension (.COM, .EXE, .BAT, .CMD by default).
func executableNames(path string) []string {
	names := []string{}
	if filepath.Ext(path) != "" {
		names = append(names, path)
	}
	for _, extension := range pathExtensions() {
		names = append(names, path+extension)
	}
	return names
}

func pathExtensions() []string {
	list := os.Getenv("PATHEXT")
	if list == "" {
		list = ".com;.exe;.bat;.cmd"
	}
	extensions := []string{}
	for _, extension := range strings.Split(strings.ToLower(list), ";") {
		if strings.HasPrefix(extension, ".") {
			extensions = append(extensions, extension)
		}
	}
	return extensions
}

func runnable(fs.FileMode) bool { return true }

// shimTarget is the "path = ..." line of a scoop shim's .shim file.
var shimTarget = regexp.MustCompile(`(?m)^\s*path\s*=\s*"?([^"\r\n]+?)"?\s*$`)

// behindShim is the program a scoop shim starts, when path is one (NAME.exe beside NAME.shim),
// else path: every shim is the same small binary, so its hash and cached version would name the
// shim, not the tool, and a shim pointing at a removed program starts nothing (decision 129).
func behindShim(path string) string {
	data, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".shim")
	if err != nil {
		return path
	}
	match := shimTarget.FindSubmatch(data)
	if match == nil || !IsExecutable(string(match[1])) {
		return path
	}
	return string(match[1])
}

func systemDirectories() []string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return []string{filepath.Join(root, "System32"), root}
}
