package pipeline

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/artifacts"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/lockfile"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

// stageCacheVersion is ArtifactCache::STAGE_CACHE_VERSION.
const stageCacheVersion = int64(2)

var (
	stageKeyPattern = regexp.MustCompile(`\A[0-9a-f]{64}\z`)
	fontFile        = regexp.MustCompile(`(?i)\.(?:otf|ttf|ttc)\z`)
)

// systemFontRoots are the directories of the fonts installed on this machine: fontconfig's on
// Unix, the system's and the user's Fonts folders on Windows.
func systemFontRoots() []string {
	if runtime.GOOS == "windows" {
		return []string{filepath.Join(os.Getenv("WINDIR"), "Fonts"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts")}
	}
	home, _ := os.UserHomeDir()
	return []string{"/usr/share/fonts", "/usr/local/share/fonts", filepath.Join(home, ".fonts"), filepath.Join(home, ".local/share/fonts")}
}

// StageCache is ArtifactCache's stage storage: one entry per stage identity, its products
// and the TeX inputs outside them that it depended on. Locks are per identity.
type StageCache struct {
	root      string
	kpsewhich string
	tex       string
}

// NewStageCache is ArtifactCache.new(root, kpsewhich:); tex is the collector directory
// kpsewhich searches first, as TeX does.
func NewStageCache(root, kpsewhich, tex string) *StageCache {
	absolute, err := filepath.Abs(root)
	if err != nil {
		rec.Fail("%v", err)
	}
	return &StageCache{root: absolute, kpsewhich: kpsewhich, tex: tex}
}

// cached is ArtifactCache::StageResult: the stage's result, its cache state and metadata.
type cached struct {
	result   any
	cache    ojson.Object
	metadata any
}

// stage is ArtifactCache#stage: the entry restored, the previous attempt's products resumed,
// or the products made by produce, kept for the next run.
func (c *StageCache) stage(key, destination, name string, force bool, previous string, produce func(string) any) cached {
	if !stageKeyPattern.MatchString(key) {
		rec.Fail("Unsafe stage cache identity")
	}
	mkdirAll(c.root)
	if symlink(c.root) {
		rec.Fail("Stage cache root must not be a symlink")
	}
	lockPath := filepath.Join(c.root, key+".lock")
	if symlink(lockPath) {
		rec.Fail("Stage cache lock must not be a symlink")
	}
	lock := lockFile(lockPath, true)
	defer lock.Close()
	entry := filepath.Join(c.root, key)
	if isDirectory(entry) && !symlink(entry) {
		ExpandAtRest(filepath.Join(entry, "files"))
	}
	var metadata ojson.Object
	reason := "explicitly forced"
	if !force {
		metadata, reason = c.validEntry(entry, key)
	}
	if metadata != nil {
		c.copyFiles(filepath.Join(entry, "files"), destination, rec.AsMap(ojson.Plain(metadata.Get("files"))))
		touch(entry) // eviction keeps the recently used entries
		return result(metadata, "hit", reason, entry)
	}
	var prior ojson.Object
	priorReason := ""
	if !force {
		prior, priorReason = c.validPrevious(previous, key)
	}
	if prior != nil {
		c.copyFiles(previous, destination, rec.AsMap(ojson.Plain(prior.Get("files"))))
		return result(prior, "resumed", reason, previous)
	}
	produced := produce(destination)
	metadata = c.metadata(key, name, destination, produced)
	c.publish(entry, destination, metadata)
	reasons := []string{}
	for _, text := range []string{reason, priorReason} {
		if text != "" {
			reasons = append(reasons, text)
		}
	}
	return result(metadata, "miss", strings.Join(reasons, "; "), entry)
}

func result(metadata ojson.Object, status, reason, entry string) cached {
	state := ojson.Object{{"status", status}, {"key", metadata.Get("key")}, {"entry", entry},
		{"producer_directory", metadata.Get("producer_directory")}, {"acceptance_reused", false}}
	if reason != "" {
		state = append(state, ojson.Pair{"reason", reason})
	}
	return cached{result: metadata.Get("result"), cache: state, metadata: metadata}
}

// Settle is ArtifactCache#settle_stages!: per stage, the keep most recently used entries are
// kept and compressed, older ones removed. An entry another run holds is left as it is.
func (c *StageCache) Settle(keep int) {
	matches, _ := filepath.Glob(filepath.Join(c.root, "*"))
	groups := map[string][]string{}
	names := []string{}
	for _, entry := range matches {
		if !stageKeyPattern.MatchString(filepath.Base(entry)) || !isDirectory(filepath.Join(entry, "files")) || symlink(entry) ||
			symlink(entry+".lock") {
			continue
		}
		name := stageName(entry)
		if _, seen := groups[name]; !seen {
			names = append(names, name)
		}
		groups[name] = append(groups[name], entry)
	}
	for _, name := range names {
		group := groups[name]
		sort.SliceStable(group, func(i, j int) bool { return modified(group[i]).After(modified(group[j])) })
		for index, entry := range group {
			if index < keep {
				c.holding(entry, func() { CompressAtRest([]string{filepath.Join(entry, "files")}) })
			} else {
				c.holding(entry, func() { removeAll(entry) })
			}
		}
	}
}

func stageName(entry string) string {
	value, err := ojson.ReadFile(filepath.Join(entry, "manifest.json"))
	if err != nil {
		return ""
	}
	manifest, _ := value.(ojson.Object)
	name, _ := manifest.Get("stage").(string)
	return name
}

// holding runs work only when the entry's lock is free.
func (c *StageCache) holding(entry string, work func()) {
	lock := lockFile(entry+".lock", false)
	if lock == nil {
		return
	}
	defer lock.Close()
	work()
}

// lockFile opens and locks a lock file: waiting when wait, else nil when another holds it.
func lockFile(path string, wait bool) *os.File {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		rec.Fail("%v", err)
	}
	held := true
	if wait {
		err = lockfile.Lock(file)
	} else {
		held, err = lockfile.TryLock(file)
	}
	if err != nil || !held {
		file.Close()
	}
	if err != nil {
		rec.Fail("%v", err)
	}
	if !held {
		return nil
	}
	return file
}

// rejected is the reason an entry or a previous stage is not reused; nil when it is.
func rejected(prefix string, recovered any) string {
	if failure, ok := recovered.(rec.Failure); ok {
		return prefix + failure.Message
	}
	if err, ok := recovered.(error); ok {
		return prefix + err.Error()
	}
	panic(recovered)
}

func (c *StageCache) validEntry(entry, key string) (metadata ojson.Object, reason string) {
	if !isDirectory(entry) {
		return nil, "no complete cache entry"
	}
	defer c.rejection("cache rejected: ", &metadata, &reason)
	if symlink(entry) {
		rec.Fail("Stage cache entry must not be a symlink")
	}
	path := filepath.Join(entry, "manifest.json")
	if symlink(path) {
		rec.Fail("Stage cache manifest must not be a symlink")
	}
	object, ok := readPlain(path).(ojson.Object)
	if !ok {
		rec.Fail("Invalid stage cache metadata")
	}
	c.validateMetadata(object, key, filepath.Join(entry, "files"))
	return object, ""
}

// rejection turns a failure while checking an entry into its reason.
func (c *StageCache) rejection(prefix string, metadata *ojson.Object, reason *string) {
	if recovered := recover(); recovered != nil {
		*metadata, *reason = nil, rejected(prefix, recovered)
	}
}

func (c *StageCache) validPrevious(previous, key string) (metadata ojson.Object, reason string) {
	if previous == "" || !isFile(filepath.Join(previous, "stage.json")) {
		return nil, ""
	}
	defer c.rejection("previous stage rejected: ", &metadata, &reason)
	receipt, ok := readPlain(filepath.Join(previous, "stage.json")).(ojson.Object)
	if !ok {
		rec.Fail("Invalid previous stage receipt")
	}
	if receipt.Get("status") != "complete" || receipt.Get("identity") != key {
		return nil, "previous stage is incomplete or its identity changed"
	}
	object, ok := receipt.Get("cache_metadata").(ojson.Object)
	if !ok {
		panic(rec.Failure{Message: `key not found: "cache_metadata"`, Lookup: true})
	}
	c.validateMetadata(object, key, previous)
	return object, ""
}

func (c *StageCache) validateMetadata(metadata ojson.Object, key, files string) {
	_, producer := metadata.Get("producer_directory").(string)
	valid := producer
	for _, field := range []string{"files", "dependencies", "resolution"} {
		_, isObject := metadata.Get(field).(ojson.Object)
		valid = valid && isObject
	}
	if !valid {
		rec.Fail("Invalid stage cache metadata")
	}
	expected := rec.AsMap(ojson.Plain(metadata.Get("files")))
	if !rec.Equal(metadata.Get("version"), stageCacheVersion) || metadata.Get("key") != key || len(expected) == 0 {
		rec.Fail("Stage cache contract or key differs")
	}
	if !rec.Equal(stageInventory(files), expected) {
		rec.Fail("Stage cache products differ")
	}
	recorded := rec.AsMap(ojson.Plain(metadata.Get("dependencies")))
	paths := sortedKeys(recorded)
	if !rec.Equal(dependencyHashes(paths), recorded) {
		rec.Fail("Stage external dependency changed")
	}
	producerDirectory := metadata.Get("producer_directory").(string)
	dependencies := externalDependencies(files, producerDirectory)
	if !rec.Equal(stringList(dependencies), stringList(paths)) {
		rec.Fail("Stage dependency inventory differs")
	}
	if len(dependencies) > 0 && !rec.Equal(ojson.Plain(c.resolution(dependencies, producerDirectory)), ojson.Plain(metadata.Get("resolution"))) {
		rec.Fail("Stage dependency search resolution changed")
	}
}

func (c *StageCache) metadata(key, name, directory string, produced any) ojson.Object {
	files := stageInventory(directory)
	if len(files) == 0 {
		rec.Fail("Stage produced no reusable artifacts")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		rec.Fail("%v", err)
	}
	dependencies := externalDependencies(directory, absolute)
	resolution := any(ojson.Object{})
	if len(dependencies) > 0 {
		resolution = c.resolution(dependencies, directory)
	}
	return ojson.Object{{"version", stageCacheVersion}, {"key", key}, {"stage", name}, {"producer_directory", absolute},
		{"files", sortedObject(files)}, {"result", produced}, {"dependencies", sortedObject(dependencyHashes(dependencies))},
		{"resolution", resolution}, {"acceptance_reused", false}}
}

// stageInventory is every file under root but its receipt, with its SHA-256.
func stageInventory(root string) rec.Map {
	inventory := cacheInventory(root)
	delete(inventory, "stage.json")
	return inventory
}

// cacheInventory is ArtifactCache#inventory: every file under root, dot files included.
func cacheInventory(root string) rec.Map {
	if symlink(root) {
		rec.Fail("Symlink cache root")
	}
	result := rec.Map{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			rec.Fail("Symlink in cached artifacts")
		}
		if entry.Type().IsRegular() {
			result[fspath.Name(root, path)] = command.FileDigest(path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		rec.Fail("%v", err)
	}
	return result
}

// externalDependencies are the inputs TeX recorded (.fls) outside the producer directory.
// Only a recorder a build of this stage wrote counts: its PWD lies inside the producer. A
// recorder the author shipped with the source (main.fls with "PWD /Users/...", third cohort
// 781) records another machine's build, not this stage's inputs.
func externalDependencies(directory, producer string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, recorder := range globRecursive(directory, ".fls") {
		lines := readLines(recorder)
		pwd := ""
		for _, line := range lines {
			if value, ok := strings.CutPrefix(line, "PWD "); ok {
				pwd = rec.Strip(value)
				break
			}
		}
		if !producedBy(pwd, producer) {
			continue
		}
		for _, line := range lines {
			value, ok := strings.CutPrefix(line, "INPUT ")
			if !ok {
				continue
			}
			path := artifacts.ExpandPath(rec.Strip(value), pwd)
			if fspath.Within(producer, path) || seen[path] {
				continue
			}
			seen[path] = true
			result = append(result, path)
		}
	}
	sort.Strings(result)
	return result
}

// producedBy is whether TeX's working directory pwd (getcwd, symlinks resolved) lies in producer.
func producedBy(pwd, producer string) bool {
	roots := []string{producer}
	if resolved, err := fspath.Real(producer); err == nil {
		roots = append(roots, resolved)
	}
	for _, root := range roots {
		if fspath.Within(root, pwd) {
			return true
		}
	}
	return false
}

// globRecursive is Dir.glob(root/**/*suffix): files under root, no dot entries.
func globRecursive(root, suffix string) []string {
	result := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.HasPrefix(entry.Name(), ".") && path != root {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			result = append(result, path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		rec.Fail("%v", err)
	}
	sort.Strings(result)
	return result
}

func readLines(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	defer file.Close()
	lines := []string{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<26)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		rec.Fail("%v", err)
	}
	return lines
}

func dependencyHashes(paths []string) rec.Map {
	result := rec.Map{}
	for _, path := range paths {
		result[path] = command.FileDigest(path)
	}
	return result
}

// resolution is ArtifactCache#dependency_resolution: new files or reordered overlays can
// change TeX lookup while all old input hashes stay the same, so the lookup order is kept as
// well as the input bytes.
func (c *StageCache) resolution(paths []string, producer string) ojson.Object {
	names := []string{}
	seen := map[string]bool{}
	for _, path := range paths {
		if fontFile.MatchString(path) || strings.HasPrefix(path, producer+"/worker-cache/") || seen[filepath.Base(path)] {
			continue
		}
		seen[filepath.Base(path)] = true
		names = append(names, filepath.Base(path))
	}
	sort.Strings(names)
	return ojson.Object{{"tex", c.texResolution(names)}, {"font_inventory", fontInventory(paths)}}
}

func (c *StageCache) texResolution(names []string) []any {
	if len(names) == 0 {
		return []any{}
	}
	arguments := append([]string{"-engine=luahbtex", "-progname=lualatex", "-must-exist", "-no-mktex=pk", "-no-mktex=mf",
		"-no-mktex=tex", "-no-mktex=tfm", "--"}, names...)
	cmd := exec.Command(c.kpsewhich, arguments...)
	cmd.Env = command.Environment(texsandbox.WithGeneratedFonts(map[string]string{
		"TEXINPUTS": command.SearchList(c.tex, ""), "LUAINPUTS": command.SearchList(c.tex, "")}))
	var output, errorOutput strings.Builder
	cmd.Stdout, cmd.Stderr = &output, &errorOutput
	runErr := cmd.Run()
	lines := []string{}
	for _, line := range strings.SplitAfter(output.String(), "\n") {
		if line != "" {
			lines = append(lines, rec.Strip(line))
		}
	}
	lines = padLastMiss(lines, len(names)) // kpsewhich prints no line for a missing last name
	missing := 0
	for _, line := range lines {
		if line == "" {
			missing++
		}
	}
	// Kpathsea returns the number of missing names (modulo the exit-byte range).
	status := cmd.ProcessState
	valid := status != nil && status.Exited() && errorOutput.Len() == 0 && len(lines) == len(names) &&
		status.ExitCode() == missing%256
	if !valid {
		detail := rec.Strip(errorOutput.String())
		if runErr != nil && status == nil {
			detail = runErr.Error()
		}
		if detail == "" {
			detail = lookupMismatch(status, len(lines), len(names), missing)
		}
		rec.Fail("Cache dependency lookup failed: %s", detail)
	}
	result := make([]any, len(lines))
	for index, line := range lines {
		result[index] = line
	}
	return result
}

func fontInventory(paths []string) string {
	roots := []string{}
	for _, path := range paths {
		if !fontFile.MatchString(path) {
			continue
		}
		if prefix, _, found := strings.Cut(path, "/fonts/"); found {
			roots = append(roots, filepath.Join(prefix, "fonts"))
		} else {
			roots = append(roots, filepath.Dir(path))
		}
	}
	roots = append(roots, systemFontRoots()...)
	sort.Strings(roots)
	inventory := rec.Map{}
	for _, root := range roots {
		inventory[root] = stringList(globEntries(root))
	}
	return digest(inventory)
}

// globEntries is Dir.glob(root/**/*) relative to root: files and directories, sorted.
func globEntries(root string) []string {
	result := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		result = append(result, fspath.Name(root, path))
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		rec.Fail("%v", err)
	}
	sort.Strings(result)
	return result
}

// copyFiles restores a stage's products from source into destination, read-write.
func (c *StageCache) copyFiles(source, destination string, expected rec.Map) {
	mkdirAll(destination)
	for _, relative := range sortedKeys(expected) {
		artifacts.Relative(relative)
		target := filepath.Join(destination, relative)
		if _, err := os.Lstat(target); err == nil {
			rec.Fail("Stage restore would overwrite a product")
		}
		mkdirAll(filepath.Dir(target))
		copyFile(filepath.Join(source, relative), target, 0o644)
	}
	if !rec.Equal(stageInventory(destination), expected) {
		rec.Fail("Restored stage differs from cache inventory")
	}
}

func (c *StageCache) publish(entry, directory string, metadata ojson.Object) {
	staging := command.MakeTemp(c.root, ".stage-publish-")
	defer removeAll(staging)
	c.copyFiles(directory, filepath.Join(staging, "files"), rec.AsMap(ojson.Plain(metadata.Get("files"))))
	writeOrdered(filepath.Join(staging, "manifest.json"), metadata)
	err := filepath.WalkDir(staging, func(path string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if item.Type().IsRegular() {
			return os.Chmod(path, 0o444)
		}
		return nil
	})
	if err != nil {
		rec.Fail("%v", err)
	}
	if _, statErr := os.Lstat(entry); statErr == nil {
		holder := command.MakeTemp(c.root, ".rejected-stage-")
		rename(entry, filepath.Join(holder, filepath.Base(entry)))
	}
	rename(staging, entry)
}

func sortedKeys(m rec.Map) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func stringList(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func mkdirAll(path string) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		rec.Fail("%v", err)
	}
}

func symlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&fs.ModeSymlink != 0
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func modified(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func touch(path string) {
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		rec.Fail("%v", err)
	}
}

func rename(from, to string) {
	if err := os.Rename(from, to); err != nil {
		rec.Fail("%v", err)
	}
}

// removeAll is FileUtils.remove_entry_secure for a tree the run owns: read-only files and
// directories made writable first.
func removeAll(path string) {
	if _, err := os.Lstat(path); err != nil {
		return
	}
	_ = filepath.WalkDir(path, func(item string, entry fs.DirEntry, walkErr error) error {
		if walkErr == nil && entry.IsDir() {
			_ = os.Chmod(item, 0o755)
		}
		return nil
	})
	if err := os.RemoveAll(path); err != nil {
		rec.Fail("%v", err)
	}
}

// copyFile copies a file's bytes into a new file with mode.
func copyFile(from, to string, mode os.FileMode) {
	data, err := os.ReadFile(from)
	if err != nil {
		rec.Fail("%v", err)
	}
	if err := os.WriteFile(to, data, mode); err != nil {
		rec.Fail("%v", err)
	}
	if err := os.Chmod(to, mode); err != nil {
		rec.Fail("%v", err)
	}
}

// failureClass is the class Ruby names for a failure in a receipt.
func failureClass(recovered any) string {
	if failure, ok := recovered.(rec.Failure); ok {
		if failure.Lookup {
			return "KeyError"
		}
		return "Relatex::Failure"
	}
	return fmt.Sprintf("%T", recovered)
}

// lookupMismatch names why a kpsewhich answer was rejected when it printed nothing on stderr.
func lookupMismatch(status *os.ProcessState, lines, names, missing int) string {
	if status == nil || !status.Exited() {
		return fmt.Sprintf("kpsewhich did not exit normally (%v)", status)
	}
	if lines != names {
		return fmt.Sprintf("kpsewhich printed %d lines for %d names", lines, names)
	}
	return fmt.Sprintf("kpsewhich exit status %d, expected %d missing names", status.ExitCode(), missing%256)
}

// padLastMiss restores the empty answer kpsewhich leaves out for a missing last name.
func padLastMiss(lines []string, names int) []string {
	if len(lines) == names-1 {
		return append(lines, "")
	}
	return lines
}
