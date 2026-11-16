//go:build !windows

package command

import (
	"io/fs"
	"strings"
)

// ExeSuffix is what a program's file name adds to its name: nothing on Unix.
const ExeSuffix = ""

func isPath(name string) bool { return strings.Contains(name, "/") }

func executableNames(path string) []string { return []string{path} }

func runnable(mode fs.FileMode) bool { return mode&0o111 != 0 }

func systemDirectories() []string { return []string{"/usr/bin", "/bin"} }

// behindShim is path: Unix package managers link the real program.
func behindShim(path string) string { return path }
