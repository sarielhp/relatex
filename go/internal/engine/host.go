package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/texsandbox"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Host is the author's own LaTeX build that a build reproduces: the author's TeX search path
// (decisions 101 and 103) and, for relatex synctex, the time zone, which the sandbox otherwise hides.
// A paper whose styles come from the author's TEXINPUTS (a shared styles directory) typesets
// other text without it, and \today and the PDF dates follow the time zone.
type Host struct {
	// SearchPath is the author's TEXINPUTS split at its colons, a relative directory made absolute
	// against the author's working directory; "." stays (TeX works in the copy of that directory)
	// and "" is kpathsea's default path. Empty when the author has no TEXINPUTS.
	SearchPath []string
	// Given is the TEXINPUTS as the caller wrote it, the home directory shown as "~": what a run
	// records to explain its result without naming the author's home.
	Given string
	// LocalTime shows the sandbox the host's /etc/localtime: without TZ, the author's build
	// read its dates in the host's zone. TZ itself passes to the build as it is.
	LocalTime bool
}

// NewHost is the host of a build run in directory with this TEXINPUTS ("" when unset), its time zone
// included (relatex synctex, decision 101).
func NewHost(texinputs, directory string) *Host {
	host := SearchHost(texinputs, directory)
	if host == nil {
		host = &Host{}
	}
	host.LocalTime = os.Getenv("TZ") == ""
	return host
}

// SearchHost is the TeX search path of a build run in directory with this TEXINPUTS, as lualatex
// would read it there (decision 103); nil when TEXINPUTS is empty.
func SearchHost(texinputs, directory string) *Host {
	if texinputs == "" {
		return nil
	}
	home, _ := os.UserHomeDir()
	host := &Host{Given: homeAbbreviated(texinputs, home)}
	for _, entry := range command.SplitSearchList(texinputs) {
		host.SearchPath = append(host.SearchPath, absoluteEntry(entry, directory, home))
	}
	return host
}

// homeAbbreviated is a TEXINPUTS with each entry under the home directory written from "~".
func homeAbbreviated(texinputs, home string) string {
	entries := command.SplitSearchList(texinputs)
	for index, entry := range entries {
		body := strings.TrimPrefix(entry, "!!")
		mark := entry[:len(entry)-len(body)]
		if home != "" && fspath.Within(home, body) {
			entries[index] = mark + "~" + body[len(home):]
		}
	}
	return command.SearchList(entries...)
}

// absoluteEntry is one TEXINPUTS entry made absolute, its kpathsea marks ("!!" before, "//" after) kept.
func absoluteEntry(entry, directory, home string) string {
	if entry == "" || entry == "." {
		return entry
	}
	body := strings.TrimPrefix(entry, "!!")
	mark := entry[:len(entry)-len(body)]
	if body == "~" || strings.HasPrefix(body, "~/") || strings.HasPrefix(body, "~"+string(filepath.Separator)) {
		body = home + body[1:]
	}
	// Made clean: the sandbox does not show the author's directory, so "paper/../styles" would not
	// resolve there; "//" (search below) stays.
	below := strings.HasSuffix(body, "//")
	if !filepath.IsAbs(body) {
		body = filepath.Join(directory, body)
	}
	body = filepath.Clean(body)
	if below {
		body = strings.TrimSuffix(body, string(filepath.Separator)) + "//"
	}
	return mark + body
}

// searchPath is the build's TEXINPUTS: the author's with the collector before kpathsea's default
// path (or last when the author's has none).
func (h *Host) searchPath(collector string) string {
	entries := []string{}
	placed := false
	for _, entry := range h.SearchPath {
		if entry == "" && !placed {
			entries = append(entries, collector)
			placed = true
		}
		entries = append(entries, entry)
	}
	if !placed {
		entries = append(entries, collector)
	}
	return command.SearchList(entries...)
}

// directories are the search path's existing absolute directories, each with whether kpathsea
// searches below it ("//").
func (h *Host) directories() (paths []string, recursive []bool) {
	paths = []string{}
	if h == nil {
		return paths, recursive
	}
	for _, entry := range h.SearchPath {
		path := strings.TrimPrefix(entry, "!!")
		if !filepath.IsAbs(path) {
			continue
		}
		below := strings.HasSuffix(path, "//")
		path = filepath.Clean(path)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			paths, recursive = append(paths, path), append(recursive, below)
		}
	}
	return paths, recursive
}

// mounts are the directories the sandbox must show for the build to read what the author's read.
func (h *Host) mounts() []string {
	paths, _ := h.directories()
	if h != nil && h.LocalTime && texsandbox.Sandboxed {
		paths = append(paths, "/etc/localtime")
	}
	return paths
}

// Fingerprint names what the search path's directories hold: every file TeX could read there, by
// path, size and modification time ("" for no host). A build's cache entry and rlx's up-to-date
// stamp change with it, as they change with the paper's own files.
func (h *Host) Fingerprint() string {
	if h == nil {
		return ""
	}
	paths, recursive := h.directories()
	hash := sha256.New()
	for index, path := range paths {
		fmt.Fprintf(hash, "%s\x00", path)
		_ = filepath.WalkDir(path, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if name != path && !recursive[index] {
					return filepath.SkipDir
				}
				return nil
			}
			if info, statErr := entry.Info(); statErr == nil && info.Mode().IsRegular() {
				fmt.Fprintf(hash, "%s\x00%d\x00%d\n", name, info.Size(), info.ModTime().UnixNano())
			}
			return nil
		})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
