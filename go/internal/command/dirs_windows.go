//go:build windows

package command

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// systemTemporary is the user's temporary directory (%TMP%, else %TEMP%) as the process found
// it, before UseTempRoot points TMP and TEMP at TempRoot.
var systemTemporary = os.TempDir()

// TempRoot is %TEMP%\relatex: Windows' temporary directory is already the user's own
// (decision 129).
func TempRoot() (string, bool) {
	if systemTemporary == "" {
		return "", false
	}
	return filepath.Join(systemTemporary, "relatex"), true
}

// UseTempRoot points TMP, TEMP and TMPDIR at TempRoot, unless the caller chose a TMPDIR (a run
// workspace's own _tmp), which TMP and TEMP then follow, so os.MkdirTemp and the programs started
// from here use it.
func UseTempRoot() {
	root := os.Getenv("TMPDIR")
	if root == "" {
		chosen, ok := TempRoot()
		if !ok || os.MkdirAll(chosen, 0o700) != nil {
			return
		}
		root = chosen
	}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		_ = os.Setenv(name, root)
	}
}

// temporaryVariables are the variables that name a program's temporary directory: Windows
// programs read TMP and TEMP, ported Unix programs TMPDIR.
func temporaryVariables() []string { return []string{"TMPDIR", "TMP", "TEMP"} }

func userCacheBase() string {
	if base, err := os.UserCacheDir(); err == nil {
		return base
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Local")
}

// defaultTeXLiveRoot is the tree of the lualatex on PATH (TeX Live's installer puts its
// bin\windows there), else the newest texlive\YYYY with a lualatex on the system drive or in
// the user's profile, else C:\texlive.
func defaultTeXLiveRoot() string {
	if engine, err := exec.LookPath("lualatex"); err == nil {
		bin := filepath.Dir(engine)
		if strings.EqualFold(filepath.Base(bin), "windows") && strings.EqualFold(filepath.Base(filepath.Dir(bin)), "bin") {
			return filepath.Dir(filepath.Dir(bin))
		}
	}
	trees := []string{}
	for _, parent := range texLiveParents() {
		matches, _ := filepath.Glob(filepath.Join(parent, "texlive", "[0-9][0-9][0-9][0-9]"))
		for _, tree := range matches {
			if IsExecutable(texLiveBin(tree, "lualatex")) {
				trees = append(trees, tree)
			}
		}
	}
	if len(trees) == 0 {
		return filepath.Join(systemDrive(), "texlive")
	}
	slices.SortFunc(trees, func(left, right string) int { return strings.Compare(filepath.Base(right), filepath.Base(left)) })
	return trees[0]
}

func texLiveParents() []string {
	parents := []string{systemDrive()}
	if home, err := os.UserHomeDir(); err == nil {
		parents = append(parents, home)
	}
	return parents
}

func systemDrive() string {
	if drive := os.Getenv("SystemDrive"); drive != "" {
		return drive + `\`
	}
	return `C:\`
}

// texLiveBin is the binary in bin\windows: TeX Live has one Windows platform, x86-64 (it runs
// emulated on arm64), whatever the architecture relatex was built for.
func texLiveBin(root, name string) string {
	return filepath.Join(root, "bin", "windows", name+ExeSuffix)
}
