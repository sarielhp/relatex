// Package bibstyle finds the Typst style of a document's bibliography (decision 172, the plan's
// decision B): the style names the build loaded (a biblatex style package and .bbx, or BibTeX's .bst)
// are looked up as NAME.typ along BSTINPUTS, as BibTeX finds a .bst (kpathsea's own search:
// `kpsewhich -path="$(kpsewhich -show-path=bst)" NAME.typ`), with relatex's library
// (profiles/typst-bibstyles) as the path's last entry. The first file found is the style.
package bibstyle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/command"
)

// Library is relatex's own Typst styles, relative to the resource root.
const Library = "profiles/typst-bibstyles"

// Origins of a style: found along the BSTINPUTS path, or in relatex's library.
const (
	OriginPath    = "bstinputs"
	OriginLibrary = "library"
)

// lookupTimeout bounds one kpsewhich call.
const lookupTimeout = 30 * time.Second

// Style is the Typst style file a style name resolved to.
type Style struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Origin string `json:"origin"`
}

// Search is where styles are looked up.
type Search struct {
	// Kpsewhich is the kpsewhich of the TeX installation; "" searches the library only.
	Kpsewhich string
	// BstInputs is the caller's BSTINPUTS ("" for none, or a hermetic run).
	BstInputs string
	// Directory is the document's directory: kpathsea's ".".
	Directory string
	// Root is the resource root that holds the library.
	Root string
}

// Path is the BSTINPUTS search path kpathsea expands for .bst files, "" without kpsewhich.
func (s Search) Path() string {
	if s.Kpsewhich == "" {
		return ""
	}
	return s.kpsewhich("-show-path=bst")
}

// Find is the style of the first name that has one: NAME.typ along the path, else in the library.
// It also returns the path searched.
func (s Search) Find(names []string) (Style, string, bool) {
	path := s.Path()
	for _, name := range names {
		if !validName(name) {
			continue
		}
		if found := s.alongPath(path, name+".typ"); found != "" {
			if style, ok := describe(name, found, OriginPath); ok {
				return style, path, true
			}
		}
		library := filepath.Join(s.Root, filepath.FromSlash(Library), name+".typ")
		if style, ok := describe(name, library, OriginLibrary); ok {
			return style, path, true
		}
	}
	return Style{}, path, false
}

// validName: a style name is a file name, with no directory part.
func validName(name string) bool {
	return name != "" && !strings.ContainsAny(name, `/\`) && name != "." && name != ".."
}

func (s Search) alongPath(path, file string) string {
	if path == "" {
		return ""
	}
	found := s.kpsewhich("-path="+path, file)
	if found == "" {
		return ""
	}
	if !filepath.IsAbs(found) {
		found = filepath.Join(s.Directory, found)
	}
	return found
}

// kpsewhich runs kpsewhich in the document's directory with the caller's BSTINPUTS and no other TeX
// search variable; its first output line, "" when it finds nothing.
func (s Search) kpsewhich(arguments ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.Kpsewhich, arguments...)
	cmd.Dir = s.Directory
	extra := map[string]string{}
	if s.BstInputs != "" {
		extra["BSTINPUTS"] = s.BstInputs
	}
	cmd.Env = command.Environment(extra)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	line, _, _ := strings.Cut(out.String(), "\n")
	return strings.TrimSpace(line)
}

func describe(name, path, origin string) (Style, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Style{}, false
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Style{}, false
	}
	sum := sha256.Sum256(data)
	return Style{Name: name, Path: absolute, SHA256: hex.EncodeToString(sum[:]), Origin: origin}, true
}
