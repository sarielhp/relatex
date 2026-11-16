// Package texsandbox is the Go port of the TeX toolchain pieces the reconcile stage runs:
// Relatex::Doctor (the engine's identity and package inventory), TeXSandbox (rootless local
// compilation in bubblewrap), ToolVersions and FontCache (the luaotfload font-name database
// copied into each run's TEXMFCACHE).
package texsandbox

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/lockfile"
	"github.com/sarielhp/relatex/internal/rec"
)

// packages is Doctor::PACKAGES.
var packages = []string{"lualatex.fmt", "latex.ltx", "article.cls", "amsmath.sty", "amsthm.sty", "unicode-math.sty",
	"latex-lab-testphase-math.sty", "latex-lab-math.ltx", "luamml.sty", "luamml-tex.lua",
	"ltluatex.lua", "tagpdf.sty", "lualibs-util-jsn.lua", "latex-lab-amsmath.ltx", "luamml-amsmath.lua",
	"latex-lab-testphase-block.sty", "tagpdf.lua", "tagpdf-mc-code-lua.sty", "luamml-structelemwriter.lua",
	"latex-lab-testphase-sec.sty"}

var provides = regexp.MustCompile(`Provides|fileversion`)

// Doctor is Relatex::Doctor: the engine and the identity of the TeX files a run depends on.
type Doctor struct {
	Engine string
	Report rec.Map
}

var (
	doctors     = map[string]*Doctor{}
	doctorsLock sync.Mutex
)

// Current is Doctor.current(engine): one identity per engine per process.
func Current(engine string) *Doctor {
	doctorsLock.Lock()
	defer doctorsLock.Unlock()
	key := engine + "\x00" + command.Executable(engine)
	if doctor, ok := doctors[key]; ok {
		return doctor
	}
	doctor := newDoctor(engine)
	doctors[key] = doctor
	return doctor
}

func newDoctor(engine string) *Doctor {
	path := command.Executable(engine)
	if path == "" {
		rec.Fail("Missing engine: %s", engine)
	}
	kpse := command.Sibling(path, "kpsewhich")
	if !command.IsExecutable(kpse) {
		rec.Fail("Missing matching kpsewhich: %s", kpse)
	}
	tools := rec.Map{}
	for _, name := range []string{"pdftotext", "pdftoppm"} {
		tools[name] = nilIfEmpty(command.Resolve(name))
	}
	// The implementation stands where Ruby records RUBY_DESCRIPTION: the fingerprint names
	// the shared font cache, which each implementation builds once for itself.
	report := rec.Map{"implementation": "go " + runtime.Version(), "engine": identity(path, "--version"),
		"kpsewhich": identity(kpse, "--version"), "files": inventory(kpse), "pdf_tools": tools}
	bytes, err := canonjson.Marshal(report)
	if err != nil {
		rec.Fail("%v", err)
	}
	sum := sha256.Sum256(bytes)
	report["fingerprint"] = hex.EncodeToString(sum[:])
	ready := true
	for _, value := range rec.AsMap(report["files"]) {
		ready = ready && value != nil
	}
	for _, value := range tools {
		ready = ready && value != nil
	}
	report["ready"] = ready
	return &Doctor{Engine: path, Report: report}
}

func nilIfEmpty(text string) any {
	if text == "" {
		return nil
	}
	return text
}

func identity(path, flag string) rec.Map {
	version := FirstLine(path, flag)
	if version == nil {
		rec.Fail("Cannot identify tool: %s", path)
	}
	resolved, err := fspath.Real(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	return rec.Map{"path": path, "realpath": resolved, "sha256": command.FileDigest(path), "version": *version}
}

// inventory is Doctor#package_inventory: one kpsewhich start for every package.
func inventory(kpse string) rec.Map {
	cmd := exec.Command(kpse, append([]string{"-engine=luahbtex"}, packages...)...)
	cmd.Env = command.Environment(nil)
	out, _ := cmd.Output()
	found := map[string][]string{}
	for _, line := range strings.Split(string(out), "\n") {
		line = rec.Strip(line)
		if line != "" {
			found[filepath.Base(line)] = append(found[filepath.Base(line)], line)
		}
	}
	files := rec.Map{}
	for _, name := range packages {
		files[name] = nil
		paths := found[name]
		if len(paths) != 1 {
			continue
		}
		if info, err := os.Stat(paths[0]); err != nil || !info.Mode().IsRegular() {
			continue
		}
		var version any
		if !strings.HasSuffix(name, ".fmt") {
			version = versionLine(paths[0])
		}
		files[name] = rec.Map{"path": paths[0], "sha256": command.FileDigest(paths[0]), "version": version}
	}
	return files
}

// versionLine is the first line naming Provides or fileversion, stripped (nil without one).
func versionLine(path string) any {
	file, err := os.Open(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<26)
	for scanner.Scan() {
		if provides.MatchString(scanner.Text()) {
			return rec.Strip(scanner.Text())
		}
	}
	return nil
}

// FormatPath is the lualatex.fmt the doctor found.
func (d *Doctor) FormatPath() string {
	format, ok := rec.FetchMap(d.Report, "files")["lualatex.fmt"].(rec.Map)
	if !ok {
		rec.Fail("undefined method 'fetch' for nil")
	}
	return rec.FetchString(format, "path")
}

// Fingerprint is the doctor report's fingerprint.
func (d *Doctor) Fingerprint() string { return rec.FetchString(d.Report, "fingerprint") }

// FirstLine is ToolVersions.first_line: the first line the tool prints (stdout, then
// stderr) for flag, nil if it fails, kept per binary hash and flag in the shared cache.
func FirstLine(path, flag string) *string {
	sum := sha256.Sum256([]byte(command.FileDigest(path) + "\x00" + flag))
	cached := filepath.Join(command.CacheRoot(), "toolchain", "versions", hex.EncodeToString(sum[:])+".json")
	if data, err := os.ReadFile(cached); err == nil {
		var entry struct {
			Line *string `json:"line"`
		}
		if json.Unmarshal(data, &entry) == nil {
			return entry.Line
		}
		_ = os.Remove(cached)
	}
	cmd := exec.Command(path, flag)
	cmd.Env = command.Environment(nil)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if cmd.Run() != nil {
		return nil
	}
	var line *string
	if text := out.String() + errOut.String(); text != "" {
		first, _, _ := strings.Cut(text, "\n")
		stripped := rec.Strip(first)
		line = &stripped
	}
	record, _ := json.Marshal(map[string]any{"tool": filepath.Base(path), "flag": flag, "line": line})
	writeAtomic(cached, record)
	return line
}

func writeAtomic(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		rec.Fail("%v", err)
	}
	temporary := fmt.Sprintf("%s.%d.%s", path, os.Getpid(), randomHex())
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		rec.Fail("%v", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		rec.Fail("%v", err)
	}
}

func randomHex() string {
	buffer := make([]byte, 3)
	_, _ = rand.Read(buffer)
	return hex.EncodeToString(buffer)
}

// HelperIdentity is TexEnvironment's identity of a recorded system helper: its path on PATH,
// real path, SHA-256 and first version line.
func HelperIdentity(name, flag string) rec.Map {
	path := command.Executable(name)
	if path == "" {
		rec.Fail("Missing recorded system helper: %s", name)
	}
	return identity(path, flag)
}

// Wrap is TeXSandbox#wrap: the arguments inside bubblewrap, with only the public trees, the
// TeX tree and the run's own directories visible, run in the source directory.
func (d *Doctor) Wrap(arguments []string, source, output string, environment map[string]string) []string {
	return d.WrapIn(arguments, source, output, source, environment)
}

// WrapIn is Wrap with the program run in working, a directory inside source or output: a main
// file in a subdirectory is built there, as `cd sub && lualatex main.tex` would, while the
// whole source stays the readable tree. The program's TMPDIR is the run's own: the workspace's
// when the run has one, else a tmp directory beside TEXMFVAR (Ghostscript writes its temporary
// files there; the sandbox's /tmp holds nothing else).
func (d *Doctor) WrapIn(arguments []string, source, output, working string, environment map[string]string) []string {
	return d.WrapWith(arguments, source, output, working, environment, nil)
}

// sandboxTemporary is the TMPDIR a sandboxed program gets: the workspace's, else tmp beside
// the run's TEXMFVAR.
func sandboxTemporary(environment map[string]string) string {
	if command.Workspace != "" {
		return filepath.Join(command.Workspace, "_tmp")
	}
	variable, _ := filepath.Abs(environment["TEXMFVAR"])
	return filepath.Join(filepath.Dir(variable), "tmp")
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func mkdir(path string) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		rec.Fail("%v", err)
	}
}

// names is FontCache::NAMES, the database inside a cache.
var names = filepath.Join("luatex-cache", "generic", "names")

// SeedFontCache is FontCache.seed!: the cache directory a run will use as TEXMFCACHE, holding
// a copy of the shared font-name database; an already seeded one is kept.
func SeedFontCache(cache string, doctor *Doctor) string {
	if isDir(filepath.Join(cache, names)) {
		return cache
	}
	shared := sharedFontCache(doctor)
	// Copied beside the cache and renamed in, since parallel builds may share one cache.
	temporary := fmt.Sprintf("%s.seed-%d-%s", cache, os.Getpid(), randomHex())
	mkdir(filepath.Dir(cache))
	copyTree(filepath.Join(shared, names), temporary)
	mkdir(filepath.Join(cache, filepath.Dir(names)))
	if err := os.Rename(temporary, filepath.Join(cache, names)); err != nil {
		_ = os.RemoveAll(temporary)
		if !fspath.LostRace(err, filepath.Join(cache, names)) {
			rec.Fail("%v", err)
		}
	}
	return cache
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func copyTree(from, to string) {
	err := filepath.WalkDir(from, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(from, path)
		target := filepath.Join(to, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		rec.Fail("%v", err)
	}
}

func sharedFontCache(doctor *Doctor) string {
	root := filepath.Join(command.CacheRoot(), "luatex-cache")
	directory := filepath.Join(root, doctor.Fingerprint())
	if isDir(filepath.Join(directory, names)) {
		return directory
	}
	mkdir(root)
	lock, err := os.OpenFile(filepath.Join(root, "build.lock"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		rec.Fail("%v", err)
	}
	defer lock.Close()
	if err := lockfile.Lock(lock); err != nil {
		rec.Fail("%v", err)
	}
	if !isDir(filepath.Join(directory, names)) {
		buildFontCache(directory, doctor)
	}
	return directory
}

func buildFontCache(directory string, doctor *Doctor) {
	staging := fmt.Sprintf("%s.%d.%s", directory, os.Getpid(), randomHex())
	tool := command.Sibling(doctor.Engine, "luaotfload-tool")
	if !command.IsExecutable(tool) {
		rec.Fail("Missing matching luaotfload-tool: %s", tool)
	}
	mkdir(staging)
	environment := map[string]string{"PATH": command.ToolPath(doctor.Engine), "TEXMFVAR": staging, "TEXMFCACHE": staging}
	// Indexed inside the build sandbox, so the database lists exactly the fonts a build can
	// open; personal fonts in the home directory stay out of it.
	wrapped := doctor.Wrap([]string{tool, "--update"}, staging, staging, environment)
	stage := command.Run(wrapped, staging, filepath.Join(staging, "update.log"), 600*time.Second, environment)
	if !rec.Equal(stage["exit"], int64(0)) || !isDir(filepath.Join(staging, names)) {
		rec.Fail("Font name database build failed; see %s/update.log", staging)
	}
	if err := os.Rename(staging, directory); err != nil {
		rec.Fail("%v", err)
	}
}
