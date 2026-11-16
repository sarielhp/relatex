package resourcestager

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// The stages' implementation identities (internal/pipeline/identity.go) name the Go source each can
// reach instead of the whole binary, so a change to the reconciliation, the layout or the writers
// leaves the engine's cached TeX runs valid. A stage's digest covers the packages it imports (with the
// module's dependencies by version), the pipeline package's own files (the glue that calls them) and go.mod.
var stageAnchors = []struct {
	stage   string
	anchors []string
}{
	{"input", []string{"./internal/input"}},
	{"engine", []string{"./internal/engine", "./internal/texsandbox"}},
	{"reconcile", []string{"./internal/papersource"}},
	{"layout", []string{"./internal/layout"}},
	{"synchronize", []string{"./internal/synchronize"}},
	{"export", []string{"./internal/exportwriter", "./internal/bundle", "./internal/browser", "./internal/pdfua", "./internal/pdftag"}},
	{"publish", []string{"./internal/split", "./internal/command", "./internal/fspath", "./internal/ojson", "./internal/rec"}},
}

const (
	sourceFormat = "{{if not .Standard}}{{.ImportPath}}|{{.Dir}}|{{if .Module}}{{.Module.Path}}@{{.Module.Version}}{{end}}{{end}}"
	modulePrefix = "github.com/sarielhp/relatex@"
	pipelinePath = "github.com/sarielhp/relatex/internal/pipeline"
)

type stageDigest struct {
	stage, digest string
}

func stageDigests(root string) ([]stageDigest, error) {
	module := filepath.Join(root, "go")
	glue, err := sourceFiles(filepath.Join(module, "internal", "pipeline"))
	if err != nil {
		return nil, err
	}
	gluePart, err := fileParts(glue, "pipeline/")
	if err != nil {
		return nil, err
	}
	extras, err := moduleParts(module)
	if err != nil {
		return nil, err
	}
	result := []stageDigest{}
	for _, entry := range stageAnchors {
		parts, err := packageParts(module, entry.anchors, glue)
		if err != nil {
			return nil, err
		}
		parts = append(append(parts, gluePart...), extras...)
		result = append(result, stageDigest{entry.stage, hashString(strings.Join(parts, "\n"))})
	}
	return result, nil
}

func moduleParts(module string) ([]string, error) {
	files := []string{}
	for _, name := range []string{"go.mod", "go.sum"} {
		path := filepath.Join(module, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			files = append(files, path)
		}
	}
	return fileParts(files, "")
}

// fileParts are "prefix+basename=sha256" lines, in the order of files.
func fileParts(files []string, prefix string) ([]string, error) {
	parts := []string{}
	for _, file := range files {
		digest, err := hashFile(file)
		if err != nil {
			return nil, err
		}
		parts = append(parts, prefix+filepath.Base(file)+"="+digest)
	}
	return parts, nil
}

func packageParts(module string, anchors, glue []string) ([]string, error) {
	lines, err := goList(module, append([]string{"-deps", "-f", sourceFormat}, anchors...))
	if err != nil {
		return nil, err
	}
	sort.Strings(lines)
	parts := []string{}
	for _, line := range lines {
		path, _, _ := strings.Cut(line, "|")
		digest, err := packageDigest(line, glue)
		if err != nil {
			return nil, err
		}
		parts = append(parts, path+"="+digest)
	}
	return parts, nil
}

func goList(module string, arguments []string) ([]string, error) {
	command := exec.Command("go", append([]string{"list"}, arguments...)...)
	command.Dir = module
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("go list failed: %w\n%s", err, stderr.String())
	}
	lines := []string{}
	for _, line := range strings.Split(string(output), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// packageDigest is "path@version" for a dependency and the hash of the sources for this module's own packages.
func packageDigest(line string, glue []string) (string, error) {
	fields := strings.SplitN(line, "|", 3)
	if len(fields) < 3 || !strings.HasPrefix(fields[2], modulePrefix) {
		version := ""
		if len(fields) == 3 {
			version = fields[2]
		}
		return fields[0] + "@" + version, nil
	}
	files := glue
	if fields[0] != pipelinePath {
		var err error
		if files, err = sourceFiles(fields[1]); err != nil {
			return "", err
		}
	}
	parts := []string{}
	for _, file := range files {
		digest, err := hashFile(file)
		if err != nil {
			return "", err
		}
		parts = append(parts, filepath.Base(file)+":"+digest)
	}
	return hashString(strings.Join(parts, "\n")), nil
}

// sourceFiles are the non-test .go, .c and .h files of a directory, sorted by name.
func sourceFiles(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	files := []string{}
	for _, entry := range entries {
		if isSource(entry.Name()) {
			files = append(files, filepath.Join(directory, entry.Name()))
		}
	}
	return files, nil
}

func isSource(name string) bool {
	extension := filepath.Ext(name)
	return (extension == ".go" || extension == ".c" || extension == ".h") && !strings.HasSuffix(name, "_test.go")
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func hashString(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// digestJSON renders the digests as JSON with two-space indentation and a final newline.
func digestJSON(digests []stageDigest) string {
	var out strings.Builder
	out.WriteString("{\n")
	for i, entry := range digests {
		comma := ","
		if i == len(digests)-1 {
			comma = ""
		}
		fmt.Fprintf(&out, "  %q: %q%s\n", entry.stage, entry.digest, comma)
	}
	out.WriteString("}\n")
	return out.String()
}
