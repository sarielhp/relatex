//go:build !windows

package texsandbox

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
)

// Sandboxed is true on Unix: TeX and its helpers run in bubblewrap.
const Sandboxed = true

// publicPaths is TeXSandbox::PUBLIC_PATHS: what a sandboxed program may read besides the TeX tree.
var publicPaths = []string{"/usr", "/bin", "/lib", "/lib64", "/etc/alternatives", "/etc/fonts", "/etc/texmf", "/etc/ld.so.cache",
	"/var/lib/texmf", "/var/cache/fontconfig"}

// WrapWith is WrapIn with more directories the program may read: the author's own TeX search
// directories, when a build reproduces the author's (decision 101). They are mounted after /tmp,
// so one under /tmp stays visible.
func (d *Doctor) WrapWith(arguments []string, source, output, working string, environment map[string]string, extra []string) []string {
	binary := command.Executable("bwrap")
	if binary == "" {
		rec.Fail("Missing bubblewrap for untrusted paper compilation")
	}
	kpse := command.Sibling(d.Engine, "kpsewhich")
	texRoot := command.Capture(kpse, "-var-value=TEXMFROOT")
	if info, err := os.Stat(filepath.Join(texRoot, "texmf-dist")); err != nil || !info.IsDir() {
		rec.Fail("Invalid selected TeX root")
	}
	readOnly := []string{}
	for _, path := range append(append([]string{}, publicPaths...), texRoot, filepath.Dir(d.FormatPath())) {
		if _, err := os.Lstat(path); err == nil && !contains(readOnly, path) {
			readOnly = append(readOnly, path)
		}
	}
	writable := []string{}
	for _, path := range []string{source, output, environment["TEXMFVAR"], environment["TEXMFCACHE"]} {
		absolute, _ := filepath.Abs(path)
		writable = append(writable, absolute)
	}
	temporary := sandboxTemporary(environment)
	writable = append(writable, temporary)
	for _, path := range writable {
		mkdir(path)
	}
	wrapped := []string{binary, "--unshare-all", "--die-with-parent", "--new-session", "--uid", strconv.Itoa(os.Getuid()),
		"--gid", strconv.Itoa(os.Getgid())}
	for _, path := range readOnly {
		wrapped = append(wrapped, "--ro-bind", path, path)
	}
	wrapped = append(wrapped, "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp")
	for _, path := range extra {
		if _, err := os.Stat(path); err == nil && !contains(readOnly, path) {
			wrapped = append(wrapped, "--ro-bind", path, path)
		}
	}
	if generated := GeneratedFontsDir(); !contains(readOnly, generated) && !contains(writable, generated) {
		if info, err := os.Stat(generated); err == nil && info.IsDir() {
			wrapped = append(wrapped, "--ro-bind", generated, generated)
		}
	}
	for _, path := range writable {
		wrapped = append(wrapped, "--bind", path, path)
	}
	directory, _ := filepath.Abs(working)
	return append(append(wrapped, "--setenv", "TMPDIR", temporary, "--chdir", directory, "--"), arguments...)
}
