// Package command is the Go port of Relatex::Command, RuntimeTools and RunWorkspace.scratch:
// external programs (mutool, qpdf, pdfinfo, kpsewhich) run with the TeX search variables
// cleared, under a timeout, in scratch directories of the run.
package command

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/rec"
)

var searchVariable = regexp.MustCompile(`\A(?:TEX|BIB|BST|LUA|OSFONT|VFFONT|TFMFONT|T1FONT|TTFONT|OPENTYPE|ENCFONT)`)

// Workspace is RunWorkspace.current's root, when the run has one: its scratch directories
// and the TMPDIR and XDG directories of the programs it runs live there.
var Workspace string

// Environment is Command.clean_environment applied to this process's environment.
func Environment(extra map[string]string) []string {
	result := []string{}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !searchVariable.MatchString(name) && !overridden(name, extra) {
			result = append(result, entry)
		}
	}
	for name, value := range workspaceEnvironment() {
		if _, ok := extra[name]; !ok {
			result = append(result, name+"="+value)
		}
	}
	for name, value := range extra {
		result = append(result, name+"="+value)
	}
	return result
}

func overridden(name string, extra map[string]string) bool {
	if _, ok := extra[name]; ok {
		return true
	}
	_, ok := workspaceEnvironment()[name]
	return ok
}

func workspaceEnvironment() map[string]string {
	if Workspace == "" {
		return nil
	}
	values := map[string]string{"XDG_CACHE_HOME": filepath.Join(Workspace, "_cache"), "XDG_CONFIG_HOME": filepath.Join(Workspace, "_config")}
	for _, name := range temporaryVariables() {
		values[name] = filepath.Join(Workspace, "_tmp")
	}
	for _, directory := range values {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			rec.Fail("%v", err)
		}
	}
	return values
}

// Capture is Command.capture: the program's standard output, stripped; a failure is a
// rec.Failure with its standard error.
func Capture(arguments ...string) string {
	cmd := exec.Command(arguments[0], arguments[1:]...)
	cmd.Env = Environment(nil)
	var out, err bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &err
	if runErr := cmd.Run(); runErr != nil {
		rec.Fail("%s: %s", arguments[0], strings.TrimSpace(err.String()))
	}
	return rec.Strip(out.String())
}

// CaptureBytes runs a program and returns its raw standard output.
func CaptureBytes(arguments ...string) []byte {
	cmd := exec.Command(arguments[0], arguments[1:]...)
	cmd.Env = Environment(nil)
	var out, err bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &err
	if runErr := cmd.Run(); runErr != nil {
		rec.Fail("%s: %s", arguments[0], strings.TrimSpace(err.String()))
	}
	return out.Bytes()
}

// Run is Command.run: the program in directory, its output (both streams) in log, killed
// with its process group after timeout. The result is Ruby's: command, exit, signal,
// timeout and seconds.
func Run(arguments []string, directory, log string, timeout time.Duration, environment map[string]string) rec.Map {
	started := time.Now()
	output, err := os.Create(log)
	if err != nil {
		rec.Fail("%v", err)
	}
	defer output.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, arguments[0], arguments[1:]...)
	cmd.Dir = directory
	cmd.Env = Environment(environment)
	cmd.Stdout, cmd.Stderr = output, output
	var leader atomic.Pointer[Group]
	cmd.Cancel = func() error { return killGroup(&leader, cmd) }
	group, runErr := StartGroup(cmd)
	if runErr == nil {
		leader.Store(group)
		release := hold(cmd.Process.Pid, group)
		runErr = cmd.Wait()
		release()
		group.Kill()
		group.Close()
	}
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	words := make([]any, len(arguments))
	for index, word := range arguments {
		words[index] = word
	}
	result := rec.Map{"command": words, "exit": nil, "signal": nil, "timeout": timedOut,
		"seconds": time.Since(started).Seconds()}
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) && cmd.ProcessState == nil {
		rec.Fail("%s: %v", arguments[0], runErr)
	}
	if state := cmd.ProcessState; state != nil && !timedOut {
		status, _ := state.Sys().(syscall.WaitStatus)
		if status.Signaled() {
			result["signal"] = int64(status.Signal())
		} else {
			result["exit"] = int64(status.ExitStatus())
		}
	}
	return result
}

// killGroup kills a timed-out program with its group, or the program alone when its group is not
// recorded yet (Run kills the group once the program has ended).
func killGroup(leader *atomic.Pointer[Group], cmd *exec.Cmd) error {
	if group := leader.Load(); group != nil {
		group.Kill()
		return nil
	}
	return cmd.Process.Kill()
}

// Resolve is RuntimeTools.resolve for the PDF helpers: RELATEX_NAME, else the configuration's
// "tools" path, else PATH (Locate).
func Resolve(name string) string {
	path, _ := Locate(name)
	return path
}

// Identity is RuntimeTools.identity.
func Identity(name string) rec.Map {
	path := Resolve(name)
	if path == "" {
		return rec.Map{"available": false}
	}
	resolved, err := fspath.Real(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	return rec.Map{"available": true, "path": resolved, "sha256": FileDigest(path)}
}

// FileDigest is Digest::SHA256.file(path).hexdigest.
func FileDigest(path string) string {
	file, err := os.Open(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		rec.Fail("%v", err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// CacheRoot is Relatex::CACHE_ROOT: XDG_CACHE_HOME/relatex when it is set, else the platform's
// user cache directory (~/.cache on Unix, %LOCALAPPDATA% on Windows).
func CacheRoot() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		base = userCacheBase()
	}
	absolute, _ := filepath.Abs(base)
	return filepath.Join(absolute, "relatex")
}

// Scratch is RunWorkspace.scratch: a new directory under the run's workspace, else under
// RELATEX_RUNS_DIR or the cache's tmp.
func Scratch(prefix string) string {
	parent := filepath.Join(Workspace, "_scratch")
	if Workspace == "" {
		parent = os.Getenv("RELATEX_RUNS_DIR")
		if parent == "" {
			parent = filepath.Join(CacheRoot(), "tmp")
		}
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		rec.Fail("%v", err)
	}
	directory, err := os.MkdirTemp(parent, prefix)
	if err != nil {
		rec.Fail("%v", err)
	}
	return directory
}

// TeXLiveRoot is Relatex.texlive_root: RELATEX_TEXLIVE, else the tree of the lualatex the
// configuration names, else the platform's default tree (~/.texlive on Unix; on Windows the tree
// of the lualatex on PATH, else the newest texlive\YYYY on the system drive or in the user
// profile).
func TeXLiveRoot() string {
	root := os.Getenv("RELATEX_TEXLIVE")
	if root == "" {
		if tree := configuredTree(); tree != "" {
			return tree
		}
		return defaultTeXLiveRoot()
	}
	absolute, _ := filepath.Abs(fspath.ExpandHome(root))
	return absolute
}

// TeXLiveBin is Relatex.texlive_bin: a binary in the TeX Live tree's platform directory.
func TeXLiveBin(name string) string { return texLiveBin(TeXLiveRoot(), name) }

// receiptKeys are Run's result keys in the order Command.run builds them.
var receiptKeys = []string{"command", "exit", "signal", "timeout", "seconds"}

// WriteReceipt is Relatex.write_json of a Run result: the stage receipt beside its log.
func WriteReceipt(path string, stage rec.Map) {
	bytes, err := canonjson.PrettyKeys(stage, receiptKeys)
	if err == nil {
		err = canonjson.WriteAtomic(path, append(bytes, '\n'))
	}
	if err != nil {
		rec.Fail("%v", err)
	}
}

// MakeTemp is Dir.mktmpdir(prefix, parent): a new directory named prefix, the date, the
// process id and a random base-36 number (Ruby's Dir::Tmpname), mode 0700.
func MakeTemp(parent, prefix string) string {
	if err := os.MkdirAll(parent, 0o755); err != nil {
		rec.Fail("%v", err)
	}
	for {
		buffer := make([]byte, 4)
		if _, err := cryptorand.Read(buffer); err != nil {
			rec.Fail("%v", err)
		}
		name := fmt.Sprintf("%s%s-%d-%s", prefix, time.Now().Format("20060102"), os.Getpid(),
			strconv.FormatUint(uint64(binary.LittleEndian.Uint32(buffer)), 36))
		path := filepath.Join(parent, name)
		err := os.Mkdir(path, 0o700)
		if err == nil {
			return path
		}
		if !errors.Is(err, os.ErrExist) {
			rec.Fail("%v", err)
		}
	}
}
