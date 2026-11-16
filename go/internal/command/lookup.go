package command

import (
	"os"
	"path/filepath"
	"strings"
)

// Executable is Command.executable: a name with a separator (or, on Windows, a volume) as
// given, else found on PATH. On Windows a name without an extension also tries each PATHEXT
// extension (decision 129), and "executable" means a regular file; on Unix it means a regular
// file with an execute bit.
func Executable(name string) string {
	candidates := []string{}
	if isPath(name) {
		absolute, _ := filepath.Abs(name)
		candidates = append(candidates, executableNames(absolute)...)
	} else {
		for _, directory := range filepath.SplitList(os.Getenv("PATH")) {
			candidates = append(candidates, executableNames(filepath.Join(directory, name))...)
		}
	}
	for _, path := range candidates {
		if IsExecutable(path) {
			return behindShim(path)
		}
	}
	return ""
}

// IsExecutable reports whether path is a program this platform can start: a regular file,
// with an execute bit on Unix.
func IsExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && runnable(info.Mode())
}

// Sibling is the path of the program name in the directory of program (a TeX Live binary
// beside the engine), with the platform's suffix: "kpsewhich" beside lualatex, "kpsewhich.exe"
// beside lualatex.exe. Whether it exists is the caller's question (IsExecutable).
func Sibling(program, name string) string {
	return filepath.Join(filepath.Dir(program), name+ExeSuffix)
}

// SearchList joins a search path (TEXINPUTS, LUAINPUTS, PATH) with the platform's separator:
// ":" on Unix, ";" on Windows, where ":" follows a drive letter.
func SearchList(entries ...string) string {
	return strings.Join(entries, string(os.PathListSeparator))
}

// SplitSearchList is SearchList's inverse.
func SplitSearchList(list string) []string {
	return strings.Split(list, string(os.PathListSeparator))
}

// ToolPath is the PATH of a TeX Live helper run: the engine's directory first, then the
// system's own programs (/usr/bin and /bin on Unix, System32 on Windows).
func ToolPath(engine string) string {
	return SearchList(append([]string{filepath.Dir(engine)}, systemDirectories()...)...)
}
