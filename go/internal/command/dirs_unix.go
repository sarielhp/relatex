//go:build !windows

package command

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// TempRoot is /tmp/<user>/relatex, where every temporary file of the program lives.
func TempRoot() (string, bool) {
	account, err := user.Current()
	if err != nil {
		return "", false
	}
	return filepath.Join("/tmp", account.Username, "relatex"), true
}

// UseTempRoot is Relatex::TempRoot.use!: TMPDIR points at TempRoot unless the caller chose one
// (a run workspace's own _tmp), so os.MkdirTemp and the programs started from here follow it.
func UseTempRoot() {
	current := strings.TrimRight(os.Getenv("TMPDIR"), "/")
	if current != "" && current != "/tmp" && current != "/var/tmp" {
		return
	}
	root, ok := TempRoot()
	if !ok || os.MkdirAll(root, 0o700) != nil {
		return
	}
	for _, directory := range []string{filepath.Dir(root), root} {
		_ = os.Chmod(directory, 0o700)
	}
	_ = os.Setenv("TMPDIR", root)
}

// temporaryVariables are the variables that name a program's temporary directory.
func temporaryVariables() []string { return []string{"TMPDIR"} }

func userCacheBase() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache")
}

func defaultTeXLiveRoot() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".texlive")
}

// texLiveBin is the binary in the tree's single platform directory, else in <cpu>-linux.
func texLiveBin(root, name string) string {
	matches, _ := filepath.Glob(filepath.Join(root, "bin", "*"))
	platforms := []string{}
	for _, path := range matches {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			platforms = append(platforms, path)
		}
	}
	if len(platforms) == 1 {
		return filepath.Join(platforms[0], name)
	}
	return filepath.Join(root, "bin", hostCPU()+"-linux", name)
}

func hostCPU() string {
	if runtime.GOARCH == "amd64" {
		return "x86_64"
	}
	if runtime.GOARCH == "arm64" {
		return "aarch64"
	}
	return runtime.GOARCH
}
