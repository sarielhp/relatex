// Package fspath compares and names file-system paths the same way on every platform
// (decision 129): a path is inside a root when it starts with the root and a separator,
// compared without case on Windows, where either slash separates; a name that goes into a
// record (manifest.json, document.json, stage digests) always uses "/", so records are
// byte-identical across platforms. On Unix every function is the plain byte comparison the
// callers made before (strings.HasPrefix(path, root+"/")).
package fspath

import (
	"path/filepath"
	"runtime"
	"strings"
)

// style is a platform's path rules: its separator, and whether names compare without case.
type style struct {
	separator byte
	fold      bool
}

var native = style{separator: filepath.Separator, fold: runtime.GOOS == "windows"}

// windowsStyle is Windows' rules, for tests on any platform.
var windowsStyle = style{separator: '\\', fold: true}

// Inside reports whether path lies strictly below root.
func Inside(root, path string) bool { return native.inside(root, path) }

// Within reports whether path is root or lies below it.
func Within(root, path string) bool { return native.within(root, path) }

// Name is path's name below root with "/" separators; a path not below root comes back whole,
// in "/" form (strings.TrimPrefix's rule).
func Name(root, path string) string { return native.name(root, path) }

// Cut is Name with whether path lay below root (strings.CutPrefix's rule).
func Cut(root, path string) (string, bool) { return native.cut(root, path) }

// Slash is a path or relative name in record form: "/" between its parts on every platform.
func Slash(name string) string { return native.slash(name) }

// Equal reports whether two paths name the same place by spelling: without case on Windows,
// where either slash separates.
func Equal(left, right string) bool { return native.equal(left, right) }

// Below reports whether a relative name (a filepath.Rel result) stays inside its base: not
// ".." and not starting with "../".
func Below(relative string) bool {
	relative = filepath.ToSlash(relative)
	return relative != ".." && !strings.HasPrefix(relative, "../")
}

func (s style) normal(path string) string {
	if s.separator == '\\' {
		return strings.ReplaceAll(path, "/", `\`)
	}
	return path
}

// prefix is root with one trailing separator: a volume root ("C:\", "/") keeps its own.
func (s style) prefix(root string) string {
	root = s.normal(root)
	if root != "" && root[len(root)-1] == s.separator {
		return root
	}
	return root + string(s.separator)
}

func (s style) hasPrefix(path, prefix string) bool {
	if !s.fold {
		return strings.HasPrefix(path, prefix)
	}
	return len(path) >= len(prefix) && strings.EqualFold(path[:len(prefix)], prefix)
}

func (s style) equal(left, right string) bool {
	left, right = s.normal(left), s.normal(right)
	if s.fold {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func (s style) inside(root, path string) bool {
	return s.hasPrefix(s.normal(path), s.prefix(root))
}

func (s style) within(root, path string) bool { return s.equal(root, path) || s.inside(root, path) }

func (s style) cut(root, path string) (string, bool) {
	prefix := s.prefix(root)
	normal := s.normal(path)
	if !s.hasPrefix(normal, prefix) {
		return s.slash(path), false
	}
	return s.slash(normal[len(prefix):]), true
}

func (s style) name(root, path string) string {
	name, _ := s.cut(root, path)
	return name
}

func (s style) slash(path string) string {
	if s.separator == '\\' {
		return strings.ReplaceAll(path, `\`, "/")
	}
	return path
}
