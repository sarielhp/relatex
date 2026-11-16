package engine

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/legacyinput"
	"github.com/sarielhp/relatex/internal/rec"
)

// Decision 143: a source that declares an 8-bit input encoding (\usepackage[latin1]{inputenc}) is
// re-encoded to UTF-8 in each build's working copy, once the inputenc shim is applied (LegacyInput in
// Ruby). Only files that are not valid UTF-8 change, each decoded with the one encoding the source
// declares; a byte that encoding leaves undefined leaves the file as it is, so the build fails as before.

// unicodeInputs are options that need no re-encoding.
var unicodeInputs = []string{"utf8", "utf8x", "ascii"}

var (
	inputencDeclaration = regexp.MustCompile(`(?m)^[^%\n]*?\\(?:usepackage|RequirePackage)\s*\[([^\]]*)\]\s*\{inputenc\}`)
	inputencSwitch      = regexp.MustCompile(`(?m)^[^%\n]*?\\inputencoding\s*\{\s*([A-Za-z0-9]+)\s*\}`)
	// legacyTextFile are the files TeX and the bibliography processors read as text.
	legacyTextFile = regexp.MustCompile(`(?i)\.(?:tex|ltx|sty|cls|clo|def|cfg|fd|bbl|bib|bst|ind|idx|txt)$`)
)

// legacyTextFiles are root's text files, sorted, without links.
func legacyTextFiles(root string) []string {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() && legacyTextFile.MatchString(path) {
			files = append(files, path)
		}
		return nil
	})
	check(err)
	sort.Strings(files)
	return files
}

// declaredInputEncodings are the inputenc option names root's files declare outside comments, other than UTF-8's.
func declaredInputEncodings(root string) []string {
	found := []string{}
	for _, path := range legacyTextFiles(root) {
		data, err := os.ReadFile(path)
		check(err)
		text := strings.ToValidUTF8(string(data), "�")
		names := []string{}
		for _, match := range inputencDeclaration.FindAllStringSubmatch(text, -1) {
			names = append(names, strings.Split(match[1], ",")...)
		}
		for _, match := range inputencSwitch.FindAllStringSubmatch(text, -1) {
			names = append(names, match[1])
		}
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name != "" && !slices.Contains(unicodeInputs, name) && !slices.Contains(found, name) {
				found = append(found, name)
			}
		}
	}
	return found
}

// reencodeLegacyInput re-encodes root's text files that are not valid UTF-8 from the one encoding the
// source declares, and returns each file it changed: {path, encoding, sha256, reencoded_sha256}.
func reencodeLegacyInput(root string) []rec.Map {
	declared := declaredInputEncodings(root)
	if len(declared) != 1 || !legacyinput.Known(declared[0]) {
		return nil
	}
	changed := []rec.Map{}
	for _, path := range legacyTextFiles(root) {
		data, err := os.ReadFile(path)
		check(err)
		text, ok := legacyinput.Decode(data, declared[0])
		if !ok {
			continue
		}
		check(os.WriteFile(path, text, 0o644))
		relative, err := filepath.Rel(root, path)
		check(err)
		changed = append(changed, rec.Map{"path": filepath.ToSlash(relative), "encoding": declared[0],
			"sha256": hexDigest(data), "reencoded_sha256": hexDigest(text)})
	}
	return changed
}

// recordReencoded keeps the files a build's working copy re-encoded; every build re-encodes the same.
func (p *Probe) recordReencoded(changed []rec.Map) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.reencoded = []any{}
	for _, entry := range changed {
		p.reencoded = append(p.reencoded, entry)
	}
}

// reencodedRecord is probe.json's reencoded_inputs: the files re-encoded, empty without the inputenc shim.
func (p *Probe) reencodedRecord() []any {
	if p.reencoded == nil {
		return []any{}
	}
	return p.reencoded
}
