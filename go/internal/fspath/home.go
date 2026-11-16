package fspath

import (
	"os"
	"path/filepath"
)

// ExpandHome replaces a leading "~" (alone, or before a separator: "~/", and "~\" on Windows)
// with the user's home directory; any other path comes back unchanged.
func ExpandHome(path string) string {
	rest, ok := homeRest(path)
	if !ok {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if rest == "" {
		return home
	}
	return filepath.Join(home, rest)
}

// homeRest is what follows "~" and its separator, and whether path starts with the home.
func homeRest(path string) (string, bool) {
	if path == "~" {
		return "", true
	}
	if len(path) < 2 || path[0] != '~' || !isSeparator(path[1]) {
		return "", false
	}
	return path[2:], true
}

func isSeparator(c byte) bool { return c == '/' || c == filepath.Separator }

// Abbreviate writes a path below the home directory as "~/" and the rest of the path, in "/"
// form; any other path comes back unchanged.
func Abbreviate(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rest, inside := Cut(home, path); inside {
		return "~/" + rest
	}
	return path
}
